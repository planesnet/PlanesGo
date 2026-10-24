package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"planesgo-code/internal/anydesk"
)

// fakeServer: the pgocode server as the helper sees it, recording what it is told.
type fakeServer struct {
	mu      sync.Mutex
	created []SessionInfo
	beats   map[string][]bool
	ended   map[string]string
	forget  bool   // the next beat answers 404 (the server restarted)
	refuse  string // creating a session with a project answers this error
	n       int
	token   string
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(ClientCookie)
	if err != nil || c.Value != f.token || r.Header.Get("X-PGO") != "1" || r.Method != http.MethodPost {
		http.Error(w, `{"error":"solo para la app de escritorio"}`, http.StatusForbidden)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.URL.Path == "/api/remote/sessions":
		var s SessionInfo
		json.NewDecoder(r.Body).Decode(&s)
		if f.refuse != "" && s.Project > 0 {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": f.refuse})
			return
		}
		f.n++
		f.created = append(f.created, s)
		json.NewEncoder(w).Encode(map[string]string{"id": fmt.Sprint("s", f.n)})
	case strings.HasSuffix(r.URL.Path, "/beat"):
		id := strings.Split(r.URL.Path, "/")[4]
		if f.forget {
			f.forget = false
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "no hay esa sesión remota"})
			return
		}
		var b struct{ Active bool }
		json.NewDecoder(r.Body).Decode(&b)
		f.beats[id] = append(f.beats[id], b.Active)
		w.Write([]byte(`{"ok":true}`))
	case strings.HasSuffix(r.URL.Path, "/end"):
		var b struct{ Error string }
		json.NewDecoder(r.Body).Decode(&b)
		f.ended[strings.Split(r.URL.Path, "/")[4]] = b.Error
		w.Write([]byte(`{"ok":true}`))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeServer) snapshot() (created []SessionInfo, beats map[string][]bool, ended map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	beats, ended = map[string][]bool{}, map[string]string{}
	for k, v := range f.beats {
		beats[k] = append([]bool(nil), v...)
	}
	for k, v := range f.ended {
		ended[k] = v
	}
	return append([]SessionInfo(nil), f.created...), beats, ended
}

// helperEnv: a helper running against a fake server, its input and its events.
type helperEnv struct {
	srv    *fakeServer
	in     *io.PipeWriter
	events chan Event
	done   chan error
}

func startHelper(t *testing.T, traces []string) *helperEnv {
	t.Helper()
	f := &fakeServer{beats: map[string][]bool{}, ended: map[string]string{}, token: "tok"}
	hs := httptest.NewServer(f)
	t.Cleanup(hs.Close)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	e := &helperEnv{srv: f, in: inW, events: make(chan Event, 100), done: make(chan error, 1)}
	h := &Helper{API: &API{Base: hs.URL, Token: "tok"}, Beat: 50 * time.Millisecond, Traces: traces}
	go func() { e.done <- h.Run(context.Background(), inR, outW); outW.Close() }()
	go func() {
		sc := bufio.NewScanner(outR)
		for sc.Scan() {
			var ev Event
			if json.Unmarshal(sc.Bytes(), &ev) == nil {
				e.events <- ev
			}
		}
		close(e.events)
	}()
	return e
}

func (e *helperEnv) send(t *testing.T, o Order) {
	t.Helper()
	b, _ := json.Marshal(o)
	if _, err := e.in.Write(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
}

// next: the next event of a type (the others are skipped).
func (e *helperEnv) next(t *testing.T, kind string) Event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-e.events:
			if !ok {
				t.Fatalf("sin evento %s: el ayudante ha terminado", kind)
			}
			if ev.Event == kind {
				return ev
			}
		case <-timeout:
			t.Fatalf("sin evento %s", kind)
		}
	}
}

// stop closes the helper's input, as the app does when it closes.
func (e *helperEnv) stop(t *testing.T) {
	t.Helper()
	e.in.Close()
	select {
	case <-e.done:
	case <-time.After(10 * time.Second):
		t.Fatal("el ayudante no termina al cerrarse su entrada")
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no llega: %s", what)
}

// fakeFreeRDP: a FreeRDP that writes what it got (input and arguments) and then runs body.
func fakeFreeRDP(t *testing.T, body string) (out string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	out = filepath.Join(dir, "salida")
	fake := filepath.Join(dir, "sdl-freerdp")
	script := "#!/bin/sh\nread pw\nread otra || otra=EOF\necho \"$pw|$otra|$PGO_TOKEN_CLIENTE|$*\" > " + out + "\n" + body + "\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGO_FREERDP", fake)
	return out
}

// RDP: the session is reported with its project, the password reaches only FreeRDP's input (never its arguments,
// nor the server), beats carry the idle time the app sends, and FreeRDP's error is explained to the server.
func TestHelperRDP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FreeRDP simulado con sh")
	}
	out := fakeFreeRDP(t, "sleep 0.4\necho '[ERROR] ERRCONNECT_LOGON_FAILURE [0x00020014]'\nexit 131")
	t.Setenv("PGO_TOKEN_CLIENTE", "tok")
	e := startHelper(t, nil)
	if ev := e.next(t, "ready"); ev.Available == nil || !ev.Available.RDP {
		t.Fatalf("listo: %+v", ev)
	}
	e.send(t, Order{Op: "rdp", Req: "1", Host: "-x", User: "u", Password: "p", Project: 590})
	if ev := e.next(t, "reply"); ev.Error == "" {
		t.Fatal("un equipo que empieza por «-» se acepta")
	}
	e.send(t, Order{Op: "idle", Idle: 1})
	e.send(t, Order{Op: "rdp", Req: "2", Project: 590, Name: "Sage", Host: "192.168.125.229", User: `MAZANA\luis`, Password: "S3cr3t!"})
	ev := e.next(t, "reply")
	if ev.Req != "2" || ev.ID != "s1" || ev.Error != "" {
		t.Fatalf("respuesta: %+v", ev)
	}
	var got string
	waitFor(t, "FreeRDP", func() bool { b, _ := os.ReadFile(out); got = strings.TrimSpace(string(b)); return got != "" })
	f := strings.SplitN(got, "|", 4)
	if len(f) < 4 || f[0] != "S3cr3t!" || f[1] != "EOF" || f[2] != "" || strings.Contains(f[3], "S3cr3t!") ||
		!strings.Contains(f[3], "/v:192.168.125.229") || !strings.Contains(f[3], "/u:luis") || !strings.Contains(f[3], "/d:MAZANA") {
		t.Fatalf("FreeRDP recibió %q", got)
	}
	ended := e.next(t, "state")
	if ended.ID != "s1" || ended.State != "ended" || ended.Error != "Usuario o contraseña incorrectos." {
		t.Fatalf("fin: %+v", ended)
	}
	created, beats, ends := e.srv.snapshot()
	if len(created) != 1 || created[0].Kind != "rdp" || created[0].Project != 590 || created[0].User != `MAZANA\luis` {
		t.Fatalf("alta: %+v", created)
	}
	if b, _ := json.Marshal(created); strings.Contains(string(b), "S3cr3t!") {
		t.Fatal("la contraseña ha llegado al servidor")
	}
	if len(beats["s1"]) == 0 || !beats["s1"][0] || ends["s1"] != "Usuario o contraseña incorrectos." {
		t.Fatalf("latidos %v, fin %q", beats, ends["s1"])
	}
	e.stop(t)
}

// Disconnecting from pgocode and closing the app close the window without an error; idle for long, no time; a
// server that forgot the session (it restarted) is told again.
func TestHelperRDPClose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FreeRDP simulado con sh")
	}
	fakeFreeRDP(t, "exec sleep 60")
	e := startHelper(t, nil)
	e.next(t, "ready")
	e.send(t, Order{Op: "idle", Idle: 3600})
	for _, r := range []string{"a", "b"} {
		e.send(t, Order{Op: "rdp", Req: r, Project: 590, Host: "h" + r, User: "u", Password: "x"})
		if ev := e.next(t, "reply"); ev.Error != "" {
			t.Fatal(ev.Error)
		}
	}
	waitFor(t, "latidos", func() bool { _, b, _ := e.srv.snapshot(); return len(b["s1"]) > 0 && len(b["s2"]) > 0 })
	if _, b, _ := e.srv.snapshot(); b["s1"][0] {
		t.Fatal("una hora sin teclado cuenta como uso")
	}
	e.srv.mu.Lock()
	e.srv.forget = true
	e.srv.mu.Unlock()
	waitFor(t, "otra vez", func() bool { c, _, _ := e.srv.snapshot(); return len(c) == 3 })
	e.send(t, Order{Op: "disconnect", Req: "d", ID: "s1"})
	if ev := e.next(t, "reply"); ev.Error != "" {
		t.Fatal(ev.Error)
	}
	if ev := e.next(t, "state"); ev.ID != "s1" || ev.Error != "" {
		t.Fatalf("desconectado: %+v", ev)
	}
	e.stop(t) // the app closes: the other window closes with it
	_, _, ends := e.srv.snapshot()
	if len(ends) != 2 {
		t.Fatalf("fines: %v", ends)
	}
	for id, msg := range ends {
		if msg != "" {
			t.Errorf("%s cerrado desde pgocode con error %q", id, msg)
		}
	}
}

// AnyDesk: the ID is validated and is the client's only argument (nothing secret reaches it); the session seen in
// the traces is reported with the project of its launch; an incoming one without a project; their ends too.
func TestHelperAnyDesk(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("AnyDesk simulado con sh")
	}
	oldPoll, oldAlive := anydeskPoll, anydeskAlive
	anydeskPoll = 20 * time.Millisecond
	pid := os.Getpid()
	anydeskAlive = func(p int) bool { return p == pid }
	defer func() { anydeskPoll, anydeskAlive = oldPoll, oldAlive }()
	dir := t.TempDir()
	args := filepath.Join(dir, "args")
	fake := filepath.Join(dir, "anydesk")
	os.WriteFile(fake, []byte("#!/bin/sh\necho \"$#|$*|$PGO_TOKEN_CLIENTE\" > "+args+"\n"), 0o755)
	t.Setenv("PGO_ANYDESK", fake)
	t.Setenv("PGO_TOKEN_CLIENTE", "tok")
	trace := filepath.Join(dir, "anydesk.trace")
	line := func(role, comp, msg string) string {
		return fmt.Sprintf("   info %s %10s   main %d %d %30s - %s\n", time.Now().Format("2006-01-02 15:04:05.000"), role, pid, pid+1, comp, msg)
	}
	// open when the helper starts: an incoming one
	os.WriteFile(trace, []byte(line("backend", "app.backend_session", "Incoming session request: Ana (444555666)")+
		line("backend", "app.backend_session", "Session started (ok).")), 0o644)
	e := startHelper(t, []string{trace})
	e.next(t, "ready")
	waitFor(t, "entrante", func() bool { c, _, _ := e.srv.snapshot(); return len(c) == 1 })
	if c, _, _ := e.srv.snapshot(); c[0].Direction != "incoming" || c[0].Peer != "444555666" || c[0].PeerName != "Ana" || c[0].Project != 0 {
		t.Fatalf("entrante: %+v", c[0])
	}
	for _, bad := range []Order{{Op: "anydesk", Req: "x", Project: 590, Peer: "-h"}, {Op: "anydesk", Req: "y", Project: 590, Peer: "1;id"},
		{Op: "anydesk", Req: "z", Peer: "123456789"}} {
		e.send(t, bad)
		if ev := e.next(t, "reply"); ev.Error == "" {
			t.Errorf("aceptado: %+v", bad)
		}
	}
	e.send(t, Order{Op: "anydesk", Req: "ok", Project: 590, Peer: " 123 456 789 "})
	if ev := e.next(t, "reply"); ev.Error != "" {
		t.Fatal(ev.Error)
	}
	waitFor(t, "AnyDesk", func() bool { b, _ := os.ReadFile(args); return len(b) > 0 })
	if b, _ := os.ReadFile(args); strings.TrimSpace(string(b)) != "1|123456789|" {
		t.Fatalf("AnyDesk recibió %q", b)
	}
	f, _ := os.OpenFile(trace, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(line("backend", "app.backend_session", "Session stopped."))
	f.WriteString(line("front", "unix_app.frontend", "Sending a connection request for address 123456789."))
	f.WriteString(line("front", "app.frontend_session", "Session started."))
	f.Close()
	waitFor(t, "saliente", func() bool { c, _, e := e.srv.snapshot(); return len(c) == 2 && len(e) == 1 })
	if c, _, ends := e.srv.snapshot(); c[1].Direction != "outgoing" || c[1].Project != 590 || c[1].Peer != "123456789" || ends["s1"] != "" {
		t.Fatalf("saliente: %+v %v", c[1], ends)
	}
	e.send(t, Order{Op: "disconnect", Req: "d", ID: "s2"})
	if ev := e.next(t, "reply"); ev.Error == "" {
		t.Fatal("una sesión de AnyDesk no se cierra desde pgocode")
	}
	e.stop(t)
	if _, _, ends := e.srv.snapshot(); len(ends) != 2 {
		t.Fatalf("al cerrar la app no se avisa del fin: %v", ends)
	}
}

// The project of a session: its launch from pgocode (outgoing, the same peer or an unknown one), never an old one.
func TestTakeLaunch(t *testing.T) {
	now := time.Now()
	l := []anydeskLaunch{{peer: "1", project: 10, at: now.Add(-time.Hour)}, {peer: "2", project: 20, at: now}, {peer: "3", project: 30, at: now}}
	p, rest := takeLaunch(l, anydesk.Session{Direction: anydesk.Outgoing, Peer: "3"}, now)
	if p != 30 || len(rest) != 1 || rest[0].peer != "2" {
		t.Fatalf("%d %+v", p, rest)
	}
	if p, rest = takeLaunch(rest, anydesk.Session{Direction: anydesk.Incoming, Peer: "2"}, now); p != 0 || len(rest) != 1 {
		t.Fatalf("entrante: %d", p)
	}
	if p, rest = takeLaunch(rest, anydesk.Session{Direction: anydesk.Outgoing}, now); p != 20 || len(rest) != 0 {
		t.Fatalf("sin ID: %d", p)
	}
}

// An AnyDesk session whose project the server refuses is still reported, without it (the person assigns one).
func TestHelperAnyDeskRefused(t *testing.T) {
	f := &fakeServer{beats: map[string][]bool{}, ended: map[string]string{}, token: "tok", refuse: "ese proyecto no existe en Odoo o no tienes acceso"}
	hs := httptest.NewServer(f)
	defer hs.Close()
	var out strings.Builder
	h := &Helper{API: &API{Base: hs.URL, Token: "tok"}, sessions: map[string]*session{}, out: json.NewEncoder(&out),
		launches: []anydeskLaunch{{peer: "123456789", project: 999, at: time.Now()}}}
	h.anydeskChange(anydesk.Change{Open: true, Session: anydesk.Session{Key: "k", Direction: anydesk.Outgoing, Peer: "123456789"}})
	created, _, _ := f.snapshot()
	if len(created) != 1 || created[0].Project != 0 || len(h.sessions) != 1 || !strings.Contains(out.String(), "no tienes acceso") {
		t.Fatalf("alta %+v, sesiones %d, eventos %s", created, len(h.sessions), out.String())
	}
}

func TestRDPArgs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := strings.Join(rdpArgs("Sage", "192.168.125.229", `MAZANA\planes`), " ")
	for _, want := range []string{"/v:192.168.125.229", "/u:planes", "/d:MAZANA", "/sec:nla", "/tls:seclevel:2", "/gfx:AVC444",
		"/from-stdin:force", "+clipboard", "/drive:Intercambio,", "/sound:sys:pulse", "/microphone:sys:pulse", "/printer", "/size:85%", "/title:Sage"} {
		if !strings.Contains(a, want) {
			t.Errorf("falta %s en %s", want, a)
		}
	}
	for _, not := range []string{"+f", "/p:"} {
		if strings.Contains(a, not) {
			t.Errorf("sobra %s en %s", not, a)
		}
	}
}
