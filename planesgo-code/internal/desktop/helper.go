// Package desktop is the client's desktop helper («pgocode-backend cliente»): what pgocode does on the person's
// desktop (the RDP and AnyDesk viewers and how long the keyboard has been idle), next to the client app and away
// from the server, which only keeps the accounting (internal/servidor/remotesessions.go).
//
// The desktop app (Electron) starts the helper as its child, with the server's address and the client's token in
// the environment (never as arguments), and talks to it in JSON lines: orders on its standard input, events on its
// standard output (logs go to standard error). The helper reports each remote session to the server, beats every
// 20 s while it is open and reports its end. When its input closes (the app closed), it closes its RDP windows and
// reports the end of everything.
package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"planesgo-code/internal/anydesk"
	"planesgo-code/internal/freerdp"
)

const (
	defaultBeat = 20 * time.Second // the server's remoteBeat
	idleLimit   = 10 * time.Minute // without keyboard or mouse this long: no more time
	idleStale   = 30 * time.Second // an idle time older than this is unknown (the app stopped sending it)
	maxOrder    = 64 << 10
)

// Environment the desktop app gives the helper.
const (
	EnvServer = "PGO_SERVIDOR"      // http://127.0.0.1:7801
	EnvToken  = "PGO_TOKEN_CLIENTE" // the client's token (secret: removed from the environment once read)
)

// Order: what the desktop app asks (one JSON line).
type Order struct {
	Op       string  `json:"op"`            // rdp, anydesk, disconnect, idle
	Req      string  `json:"req,omitempty"` // answered in the «reply» event with the same req
	Project  int     `json:"project,omitempty"`
	Name     string  `json:"name,omitempty"`
	Host     string  `json:"host,omitempty"`
	User     string  `json:"user,omitempty"`
	Password string  `json:"password,omitempty"` // rdp: only to FreeRDP's input
	Peer     string  `json:"peer,omitempty"`
	ID       string  `json:"id,omitempty"`           // disconnect
	Idle     float64 `json:"idle_seconds,omitempty"` // idle: seconds without keyboard or mouse
}

// Available: which viewers this computer has.
type Available struct {
	RDP     bool `json:"rdp"`
	AnyDesk bool `json:"anydesk"`
}

// Event: what the helper tells the desktop app (one JSON line). ready (once, with Available), reply (to an order
// with its req: ID or Error), state (a session ended: ID, Kind, Error) and error (something to show the person).
type Event struct {
	Event     string     `json:"event"`
	Req       string     `json:"req,omitempty"`
	ID        string     `json:"id,omitempty"`
	Kind      string     `json:"kind,omitempty"`
	State     string     `json:"state,omitempty"`
	Error     string     `json:"error,omitempty"`
	Available *Available `json:"available,omitempty"`
}

// session: a remote session this helper reported.
type session struct {
	id     string      // the first id the server gave it: the one the app knows
	server string      // its current id in the server (another one if the server had to be told again)
	info   SessionInfo // to tell the server again if it forgot it (it restarted, or the computer slept)
	rdp    *RDPWindow
	trace  string // AnyDesk: the session's key in the traces
}

// Helper: the desktop helper of one client.
type Helper struct {
	API    *API
	Beat   time.Duration                     // 0: 20 s
	Traces []string                          // AnyDesk's traces (anydesk.TraceFiles)
	Now    func() time.Time                  // tests
	OpenAD func(peer string) (string, error) // opens AnyDesk (tests: startAnyDesk replaced)

	outMu sync.Mutex
	out   *json.Encoder

	mu       sync.Mutex
	sessions map[string]*session // by id
	idle     time.Duration
	idleAt   time.Time
	launches []anydeskLaunch
	wg       sync.WaitGroup
}

func (h *Helper) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Helper) beat() time.Duration {
	if h.Beat > 0 {
		return h.Beat
	}
	return defaultBeat
}

func (h *Helper) emit(e Event) {
	h.outMu.Lock()
	defer h.outMu.Unlock()
	if err := h.out.Encode(e); err != nil {
		log.Printf("ayudante: no se puede escribir a la app: %v", err)
	}
}

// Main runs the helper as «pgocode-backend cliente»: the server and token from the environment, orders on stdin.
func Main() int {
	log.SetPrefix("[ayudante] ")
	base, token := os.Getenv(EnvServer), os.Getenv(EnvToken)
	os.Unsetenv(EnvToken) // nothing it starts inherits it (also proceso.CleanEnviron)
	if base == "" || token == "" {
		log.Printf("faltan %s y %s: lo arranca la app de escritorio", EnvServer, EnvToken)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	h := &Helper{API: &API{Base: base, Token: token}, Traces: anydesk.TraceFiles()}
	if err := h.Run(ctx, os.Stdin, os.Stdout); err != nil {
		log.Print(err)
		return 1
	}
	return 0
}

// Run serves the orders of in until it closes or ctx ends; then it closes and reports every session.
func (h *Helper) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	h.out = json.NewEncoder(out)
	h.mu.Lock()
	h.sessions = map[string]*session{}
	h.mu.Unlock()
	work, stop := context.WithCancel(ctx)
	defer stop()
	h.emit(Event{Event: "ready", Available: &Available{RDP: RDPClient() != "", AnyDesk: anydesk.Client() != ""}})
	var bg sync.WaitGroup
	bg.Add(2)
	go func() { defer bg.Done(); watchAnyDesk(work, h.Traces, h.beat(), h.anydeskChange) }()
	go func() { defer bg.Done(); h.beats(work) }()

	orders := make(chan Order)
	readErr := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(in)
		sc.Buffer(make([]byte, 0, 4096), maxOrder)
		for sc.Scan() {
			var o Order
			if err := json.Unmarshal(sc.Bytes(), &o); err != nil {
				log.Printf("orden no válida: %v", err) // the line is not logged: it may carry a password
				continue
			}
			select {
			case orders <- o:
			case <-work.Done():
				return
			}
		}
		readErr <- sc.Err()
	}()
	var err error
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case err = <-readErr:
			break loop
		case o := <-orders:
			h.handle(o)
		}
	}
	stop()
	bg.Wait()
	h.closeAll()
	return err
}

// handle answers an order (the slow ones in the background: the idle time keeps arriving meanwhile).
func (h *Helper) handle(o Order) {
	switch o.Op {
	case "idle":
		h.mu.Lock()
		h.idle, h.idleAt = time.Duration(o.Idle*float64(time.Second)), h.now()
		h.mu.Unlock()
		return
	case "rdp", "anydesk", "disconnect":
	default:
		h.emit(Event{Event: "reply", Req: o.Req, Error: "orden desconocida"})
		return
	}
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		var id string
		var err error
		switch o.Op {
		case "rdp":
			id, err = h.openRDP(o)
		case "anydesk":
			id, err = h.openAnyDesk(o)
		case "disconnect":
			err = h.disconnect(o.ID)
		}
		e := Event{Event: "reply", Req: o.Req, ID: id}
		if err != nil {
			e.Error = err.Error()
		}
		h.emit(e)
	}()
}

// active: someone used the keyboard or mouse lately (true if it is not known).
func (h *Helper) active() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.idleAt.IsZero() || h.now().Sub(h.idleAt) > idleStale {
		return true
	}
	return h.idle < idleLimit
}

func (h *Helper) add(s *session) {
	h.mu.Lock()
	h.sessions[s.id] = s
	h.mu.Unlock()
}

// openRDP reports the session (the server checks the project) and opens FreeRDP with the password on its input.
func (h *Helper) openRDP(o Order) (string, error) {
	bin := RDPClient()
	if bin == "" {
		return "", errors.New("falta FreeRDP: ejecuta connectors/instala.sh")
	}
	if err := freerdp.ValidateTarget(o.Host, o.User); err != nil {
		return "", err
	}
	if o.Name == "" {
		o.Name = o.Host
	}
	info := SessionInfo{Kind: "rdp", Direction: string(anydesk.Outgoing), Project: o.Project, Name: o.Name, Host: o.Host, User: o.User}
	id, err := h.API.Create(info)
	if err != nil {
		return "", err
	}
	w, err := StartRDP(bin, o.Name, o.Host, o.User, o.Password)
	if err != nil {
		h.API.End(id, err.Error())
		return "", err
	}
	s := &session{id: id, server: id, info: info, rdp: w}
	h.add(s)
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.finish(s, w.Wait())
	}()
	return id, nil
}

// openAnyDesk opens AnyDesk; its session is reported when it shows in the traces (anydeskChange).
func (h *Helper) openAnyDesk(o Order) (string, error) {
	if o.Project <= 0 {
		return "", errors.New("elige el proyecto al que se imputa el tiempo de la sesión")
	}
	open := h.OpenAD
	if open == nil {
		open = startAnyDesk
	}
	peer, err := open(o.Peer)
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	h.launches = append(h.launches, anydeskLaunch{peer: peer, project: o.Project, at: h.now()})
	h.mu.Unlock()
	log.Printf("anydesk: abierto hacia %s (proyecto %d)", peer, o.Project)
	return "", nil
}

// disconnect closes an RDP window (AnyDesk's sessions are closed in AnyDesk).
func (h *Helper) disconnect(id string) error {
	h.mu.Lock()
	s := h.sessions[id]
	h.mu.Unlock()
	switch {
	case s == nil:
		return errors.New("esa sesión remota ya no está abierta")
	case s.rdp == nil:
		return errors.New("las sesiones de AnyDesk se cierran desde AnyDesk")
	}
	s.rdp.Close()
	return nil
}

// finish reports the end of a session (msg: why, explained; "" if normally) to the server and the app.
func (h *Helper) finish(s *session, msg string) {
	h.mu.Lock()
	_, ok := h.sessions[s.id]
	delete(h.sessions, s.id)
	server := s.server
	h.mu.Unlock()
	if !ok {
		return
	}
	if err := h.API.End(server, msg); err != nil && !Gone(err) {
		log.Printf("no se puede avisar del fin de la sesión %s: %v", s.id, err)
	}
	kind := s.info.Kind
	h.emit(Event{Event: "state", ID: s.id, Kind: kind, State: "ended", Error: msg})
}

// anydeskChange: a session of AnyDesk's traces starts or ends.
func (h *Helper) anydeskChange(ch anydesk.Change) {
	if !ch.Open {
		h.mu.Lock()
		var s *session
		for _, x := range h.sessions {
			if x.trace == ch.Session.Key {
				s = x
			}
		}
		h.mu.Unlock()
		if s != nil {
			h.finish(s, "")
		}
		return
	}
	ses := ch.Session
	h.mu.Lock()
	project, kept := takeLaunch(h.launches, ses, h.now())
	h.launches = kept
	h.mu.Unlock()
	info := SessionInfo{Kind: "anydesk", Direction: string(ses.Direction), Project: project, Peer: ses.Peer, PeerName: ses.PeerName}
	id, err := h.API.Create(info)
	if err != nil && project > 0 {
		// Without the project (e.g. nobody logged in): the session is listed and the person gives it one later.
		h.emit(Event{Event: "error", Kind: "anydesk", Error: "La sesión de AnyDesk no se puede imputar en ese proyecto: " + err.Error()})
		info.Project = 0
		id, err = h.API.Create(info)
	}
	if err != nil {
		log.Printf("anydesk: no se puede avisar de la sesión %s %s: %v", ses.Direction, ses.Peer, err)
		h.emit(Event{Event: "error", Kind: "anydesk", Error: "No se puede avisar al servidor de la sesión de AnyDesk: " + err.Error()})
		return
	}
	h.add(&session{id: id, server: id, info: info, trace: ses.Key})
}

// beats reports every open session to the server each beat, until ctx ends.
func (h *Helper) beats(ctx context.Context) {
	t := time.NewTicker(h.beat())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		active := h.active()
		h.mu.Lock()
		l := make([]*session, 0, len(h.sessions))
		for _, s := range h.sessions {
			l = append(l, s)
		}
		h.mu.Unlock()
		for _, s := range l {
			h.beatOne(s, active)
		}
	}
}

func (h *Helper) beatOne(s *session, active bool) {
	h.mu.Lock()
	server := s.server
	h.mu.Unlock()
	err := h.API.Beat(server, active)
	if !Gone(err) {
		if err != nil {
			log.Printf("latido de la sesión %s: %v", s.id, err)
		}
		return
	}
	// The server forgot it (it restarted, or the computer slept and it was closed): it is still open here.
	id, err := h.API.Create(s.info)
	if err != nil {
		log.Printf("la sesión %s no se puede volver a avisar: %v", s.id, err)
		return
	}
	h.mu.Lock()
	s.server = id
	h.mu.Unlock()
}

// closeAll: the app is closing. The RDP windows close with it (nobody would count their time any more) and every
// session's end is reported.
func (h *Helper) closeAll() {
	h.mu.Lock()
	var rest []*session
	for _, s := range h.sessions {
		if s.rdp != nil {
			s.rdp.Close() // its goroutine reports the end
		} else {
			rest = append(rest, s)
		}
	}
	h.mu.Unlock()
	for _, s := range rest {
		h.finish(s, "")
	}
	done := make(chan struct{})
	go func() { h.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		log.Print("algunas sesiones no han terminado a tiempo")
	}
}

// String: an order as it would show in a log, never with its password.
func (o Order) String() string { return fmt.Sprintf("%s %s", o.Op, o.Req) }
