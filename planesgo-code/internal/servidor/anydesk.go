package servidor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"planesgo-code/internal/anydesk"
	"planesgo-code/internal/imputacion"
	"planesgo-code/internal/proceso"
	"planesgo-code/internal/sesiones"
)

// AnyDesk: pgocode opens the installed AnyDesk client on a project (AnyDesk asks for the password itself; pgocode
// never sees it) and follows AnyDesk's traces (internal/anydesk) to know when its sessions, outgoing or incoming,
// start and end. Like the RDP remote desktop (rdp.go), the time of an open session goes to its project with the
// imputation service while the keyboard or mouse are in use. A session without a project (an incoming one, or one
// opened from AnyDesk itself to an unknown computer) is not logged until the person gives it one.

var (
	anydeskPoll  = 2 * time.Second // how often the traces are read (shorter in the tests)
	anydeskAlive = anydesk.Alive   // whether an AnyDesk process is still running (replaced in the tests)
)

const (
	anydeskBacklog    = 8 << 20          // on start, the end of each trace is read again to know what is open
	anydeskLaunchLife = 10 * time.Minute // a launch from pgocode waits this long for its session to start
	anydeskMaxPeers   = 300              // projects remembered per remote computer
)

// anydeskConn: an open AnyDesk session.
type anydeskConn struct {
	ID          string    `json:"id"`
	Direction   string    `json:"direction"` // outgoing, incoming
	Peer        string    `json:"peer"`      // the other side's ID or alias ("" if the trace did not say)
	PeerName    string    `json:"peer_name,omitempty"`
	Project     int       `json:"project"` // 0: without a project, not logged
	ProjectName string    `json:"project_name,omitempty"`
	Started     time.Time `json:"started"`
	uid         int
}

// anydeskLaunch: a client opened from pgocode, waiting for its session to start.
type anydeskLaunch struct {
	peer        string
	project     int
	projectName string
	uid         int
	at          time.Time
}

type anydeskProject struct {
	Project int    `json:"project"`
	Name    string `json:"name"`
}

// anydeskSettings (AnyDeskFile): who logs the AnyDesk sessions, their default project for incoming sessions and the
// last project of each remote computer. Nothing secret.
type anydeskSettings struct {
	UID         int                       `json:"uid"`
	Default     int                       `json:"default_project"`
	DefaultName string                    `json:"default_project_name"`
	Peers       map[string]anydeskProject `json:"peers"`
}

type anydeskSessions struct {
	mu       sync.Mutex
	conns    map[string]*anydeskConn // by the trace session's key
	launches []anydeskLaunch
	settings *anydeskSettings
}

// open: sessions being logged (with a project).
func (a *anydeskSessions) open() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, c := range a.conns {
		if c.Project > 0 {
			n++
		}
	}
	return n
}

// anydeskSettingsLocked: the settings, read from AnyDeskFile the first time (s.anydesk.mu held).
func (s *Servidor) anydeskSettingsLocked() *anydeskSettings {
	a := &s.anydesk
	if a.settings == nil {
		a.settings = &anydeskSettings{}
		if s.AnyDeskFile != "" {
			if b, err := os.ReadFile(s.AnyDeskFile); err == nil {
				if err := json.Unmarshal(b, a.settings); err != nil {
					log.Printf("anydesk: %s: %v", s.AnyDeskFile, err)
				}
			}
		}
		if a.settings.Peers == nil {
			a.settings.Peers = map[string]anydeskProject{}
		}
	}
	return a.settings
}

// anydeskSaveLocked writes the settings (s.anydesk.mu held).
func (s *Servidor) anydeskSaveLocked() {
	if s.AnyDeskFile == "" {
		return
	}
	st := s.anydeskSettingsLocked()
	for p := range st.Peers { // a bound on what is remembered (map order: any of them)
		if len(st.Peers) <= anydeskMaxPeers {
			break
		}
		delete(st.Peers, p)
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	os.MkdirAll(filepath.Dir(s.AnyDeskFile), 0o700)
	tmp := s.AnyDeskFile + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		err = os.Rename(tmp, s.AnyDeskFile)
		if err != nil {
			log.Printf("anydesk: %v", err)
		}
	}
}

func anydeskWork(c *anydeskConn) imputacion.Trabajo {
	who := c.Peer
	if c.PeerName != "" {
		who = c.PeerName + " (" + c.Peer + ")"
	}
	if who == "" {
		who = "equipo desconocido"
	}
	what := "AnyDesk en " + who
	if c.Direction == string(anydesk.Incoming) {
		what = "AnyDesk entrante desde " + who
	}
	return imputacion.Trabajo{Sesion: "anydesk-" + c.ID, Titulo: what, Proyecto: c.Project, NombreProy: c.ProjectName,
		UID: c.uid, Modelo: "AnyDesk", Desc: imputacion.Descripcion(what, "trabajo en el escritorio remoto")}
}

// anydeskApply: a session of AnyDesk's traces starts or ends.
func (s *Servidor) anydeskApply(ch anydesk.Change) {
	a := &s.anydesk
	a.mu.Lock()
	if !ch.Open {
		c := a.conns[ch.Session.Key]
		delete(a.conns, ch.Session.Key)
		a.mu.Unlock()
		if c == nil {
			return
		}
		if s.Imputacion != nil && c.Project > 0 {
			s.Imputacion.Cierra("anydesk-"+c.ID, "sesión de AnyDesk terminada")
		}
		log.Printf("anydesk: sesión %s %s terminada", c.Direction, c.Peer)
		s.Eventos.Emite(map[string]string{"tipo": "anydesk"})
		return
	}
	b := make([]byte, 6)
	rand.Read(b)
	ses := ch.Session
	c := &anydeskConn{ID: hex.EncodeToString(b), Direction: string(ses.Direction), Peer: ses.Peer, PeerName: ses.PeerName, Started: ses.Start}
	st := s.anydeskSettingsLocked()
	c.uid = st.UID
	// The project: the launch from pgocode, the last one of that computer or, incoming, the default one.
	now := time.Now()
	kept := a.launches[:0]
	found := false
	for _, l := range a.launches {
		switch {
		case now.Sub(l.at) > anydeskLaunchLife:
		case !found && ses.Direction == anydesk.Outgoing && (l.peer == ses.Peer || ses.Peer == ""):
			found = true
			c.Project, c.ProjectName, c.uid = l.project, l.projectName, l.uid
		default:
			kept = append(kept, l)
		}
	}
	a.launches = kept
	if p, ok := st.Peers[ses.Peer]; !found && ok && ses.Peer != "" {
		c.Project, c.ProjectName = p.Project, p.Name
	} else if !found && ses.Direction == anydesk.Incoming && st.Default > 0 {
		c.Project, c.ProjectName = st.Default, st.DefaultName
	}
	if c.uid == 0 {
		c.Project, c.ProjectName = 0, "" // nobody has used AnyDesk from pgocode: whose time would it be?
	}
	if a.conns == nil {
		a.conns = map[string]*anydeskConn{}
	}
	a.conns[ses.Key] = c
	a.mu.Unlock()
	if c.Project == 0 {
		log.Printf("anydesk: sesión %s %s sin proyecto: no se imputa hasta que se le asigne uno", c.Direction, c.Peer)
	} else {
		log.Printf("anydesk: sesión %s %s en el proyecto %d", c.Direction, c.Peer, c.Project)
	}
	s.Eventos.Emite(map[string]string{"tipo": "anydesk"})
}

// anydeskBeat reports the activity of the open sessions with a project (inUse: someone at the keyboard); returns
// what it reported.
func (s *Servidor) anydeskBeat(inUse bool, now time.Time) []imputacion.Trabajo {
	a := &s.anydesk
	a.mu.Lock()
	var l []imputacion.Trabajo
	for _, c := range a.conns {
		if c.Project > 0 && c.uid > 0 && now.Sub(c.Started) >= remoteFirstBeat {
			l = append(l, anydeskWork(c))
		}
	}
	a.mu.Unlock()
	var done []imputacion.Trabajo
	for _, w := range l {
		if s.remoteActivity(w, inUse) {
			done = append(done, w)
		}
	}
	return done
}

// anydeskTrace: one followed trace.
type anydeskTrace struct {
	f       *anydesk.Follower
	t       *anydesk.Tracker
	first   bool
	warned  int
	lastErr string
}

// poll reads the trace's new lines and returns its changes. The first time (initial), it only learns what is open
// now (the past is not logged again).
func (x *anydeskTrace) poll() (ch []anydesk.Change, initial bool) {
	lines, err := x.f.Poll()
	if err != nil {
		if e := err.Error(); e != x.lastErr {
			x.lastErr = e
			log.Printf("anydesk: no se puede leer %s: %v", x.f.Path, err)
		}
	} else {
		x.lastErr = ""
	}
	for i, l := range lines {
		if i == x.f.ResetAt {
			x.t.Rewind()
		}
		ch = append(ch, x.t.Feed(l)...)
	}
	// A line cut when AnyDesk trims its trace is normal; more is a format that changed (another AnyDesk version).
	if x.t.Unknown >= 5 && x.t.Unknown >= 2*x.warned {
		log.Printf("anydesk: %s: %d de %d líneas con un formato que no se entiende (¿otra versión de AnyDesk?)", x.f.Path, x.t.Unknown, x.t.Lines)
		x.warned = x.t.Unknown
	}
	if !x.first {
		x.first = true
		ch = nil
		for _, o := range x.t.Open() {
			ch = append(ch, anydesk.Change{Session: o, Open: true, Time: o.Start})
		}
		return ch, true
	}
	return ch, false
}

// WatchAnyDesk follows AnyDesk's traces until ctx ends: sessions that start and end, and their time.
func (s *Servidor) WatchAnyDesk(ctx context.Context, files []string) {
	var traces []*anydeskTrace
	for _, p := range files {
		traces = append(traces, &anydeskTrace{f: anydesk.NewFollower(p, anydeskBacklog), t: anydesk.NewTracker(p)})
	}
	defer func() {
		for _, x := range traces {
			x.f.Close()
		}
	}()
	t := time.NewTicker(anydeskPoll)
	defer t.Stop()
	lastBeat := time.Now()
	for {
		for _, x := range traces {
			ch, initial := x.poll()
			for _, c := range ch {
				if !initial || anydeskAlive(c.Session.PID) { // a process gone without saying it: it is over
					s.anydeskApply(c)
				}
			}
		}
		if now := time.Now(); now.Sub(lastBeat) >= remoteBeat {
			lastBeat = now
			for _, x := range traces {
				for _, c := range x.t.CloseDead(anydeskAlive, now) {
					s.anydeskApply(c)
				}
			}
			if s.anydesk.open() > 0 {
				s.anydeskBeat(keyboardInUse(), now)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// anydeskState: GET /api/anydesk. Whether the client is installed, the open sessions and the default project.
func (s *Servidor) anydeskState(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	a := &s.anydesk
	a.mu.Lock()
	l := []anydeskConn{}
	for _, c := range a.conns {
		l = append(l, *c)
	}
	st := s.anydeskSettingsLocked()
	def, defName := st.Default, st.DefaultName
	a.mu.Unlock()
	jsonOK(w, map[string]any{"available": anydesk.Client() != "", "sessions": l, "default_project": def, "default_project_name": defName})
}

// anydeskConnect: POST /api/anydesk/connect {project, peer}. Opens AnyDesk on the computer (ID or alias); its
// session's time goes to the project (mandatory). AnyDesk asks for the password itself.
func (s *Servidor) anydeskConnect(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Project int    `json:"project"`
		Peer    string `json:"peer"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	peer, ok := anydesk.ValidID(p.Peer)
	if !ok {
		jsonError(w, http.StatusBadRequest, "el ID de AnyDesk son solo cifras (p. ej. 123 456 789) o un alias como equipo@ad")
		return
	}
	projectName, ok := s.proyecto(w, r, ses, p.Project)
	if !ok {
		return
	}
	bin := anydesk.Client()
	if bin == "" {
		jsonError(w, http.StatusServiceUnavailable, "AnyDesk no está instalado en este equipo")
		return
	}
	// Never through a shell; the ID, already validated, is the only argument. Nothing of pgocode's reaches it.
	cmd := exec.CommandContext(context.Background(), bin, peer)
	proceso.GrupoPropio(cmd)
	cmd.Env = proceso.CleanEnviron()
	if err := cmd.Start(); err != nil {
		jsonError(w, http.StatusInternalServerError, "no arranca AnyDesk: "+err.Error())
		return
	}
	go cmd.Wait() // it hands the connection to AnyDesk's service and ends (or stays as its window)
	a := &s.anydesk
	a.mu.Lock()
	a.launches = append(a.launches, anydeskLaunch{peer: peer, project: p.Project, projectName: projectName, uid: ses.UID, at: time.Now()})
	st := s.anydeskSettingsLocked()
	st.UID = ses.UID
	st.Peers[peer] = anydeskProject{Project: p.Project, Name: projectName}
	s.anydeskSaveLocked()
	a.mu.Unlock()
	log.Printf("anydesk: abierto hacia %s (proyecto %d)", peer, p.Project)
	jsonOK(w, map[string]any{"ok": true, "peer": peer})
}

// anydeskAssign: POST /api/anydesk/assign {id, project, default}. Gives an open session a project (and remembers it
// for that computer; default: also for every incoming session without one).
func (s *Servidor) anydeskAssign(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		ID      string `json:"id"`
		Project int    `json:"project"`
		Default bool   `json:"default"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	projectName, ok := s.proyecto(w, r, ses, p.Project)
	if !ok {
		return
	}
	a := &s.anydesk
	a.mu.Lock()
	var c *anydeskConn
	for _, x := range a.conns {
		if x.ID == p.ID {
			c = x
		}
	}
	if c == nil {
		a.mu.Unlock()
		jsonError(w, http.StatusNotFound, "esa sesión de AnyDesk ya no está abierta")
		return
	}
	old := c.Project
	c.Project, c.ProjectName, c.uid = p.Project, projectName, ses.UID
	st := s.anydeskSettingsLocked()
	st.UID = ses.UID
	if c.Peer != "" {
		st.Peers[c.Peer] = anydeskProject{Project: p.Project, Name: projectName}
	}
	if p.Default {
		st.Default, st.DefaultName = p.Project, projectName
	}
	s.anydeskSaveLocked()
	snap := *c
	a.mu.Unlock()
	if old > 0 && old != p.Project && s.Imputacion != nil {
		s.Imputacion.Cierra("anydesk-"+snap.ID, "proyecto cambiado") // what was logged stays in the old project
	}
	s.Eventos.Emite(map[string]string{"tipo": "anydesk"})
	jsonOK(w, snap)
}

// anydeskDefault: POST /api/anydesk/default {project}. The project of incoming sessions (0: none, ask each time).
func (s *Servidor) anydeskDefault(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Project int `json:"project"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	name := ""
	if p.Project != 0 {
		n, ok := s.proyecto(w, r, ses, p.Project)
		if !ok {
			return
		}
		name = n
	}
	a := &s.anydesk
	a.mu.Lock()
	st := s.anydeskSettingsLocked()
	st.UID = ses.UID
	st.Default, st.DefaultName = p.Project, name
	s.anydeskSaveLocked()
	a.mu.Unlock()
	s.Eventos.Emite(map[string]string{"tipo": "anydesk"})
	jsonOK(w, map[string]any{"default_project": p.Project, "default_project_name": name})
}
