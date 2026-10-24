package servidor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"planesgo-code/internal/anydesk"
	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/sesiones"
)

func anydeskServer(t *testing.T) (*Servidor, func(ruta, cuerpo string) *httptest.ResponseRecorder) {
	t.Helper()
	s := &Servidor{Puerto: "7801", PermitirNavegador: true, Sesiones: sesiones.NuevoAlmacen(), Indice: proyectos.NuevoIndice(nil),
		AnyDeskFile: filepath.Join(t.TempDir(), "anydesk.json")}
	h := s.Handler()
	ses, _ := s.Sesiones.Crea(7, "t", "t", "x")
	for _, p := range []int{590, 591} {
		ses.MarcaProyecto(p, time.Now().Add(time.Hour))
	}
	s.nombres[590], s.nombres[591] = "Proyecto A", "Proyecto B"
	return s, func(ruta, cuerpo string) *httptest.ResponseRecorder {
		method := "POST"
		if cuerpo == "" {
			method = "GET"
		}
		r := httptest.NewRequest(method, ruta, strings.NewReader(cuerpo))
		r.Host = "127.0.0.1:7801"
		r.Header.Set("X-PGO", "1")
		r.AddCookie(&http.Cookie{Name: cookie, Value: ses.Token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
}

// The ID is validated and is the client's only argument; the project is mandatory; nothing secret reaches it.
func TestAnyDeskConnect(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("AnyDesk simulado con sh")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "salida")
	fake := filepath.Join(dir, "anydesk")
	os.WriteFile(fake, []byte("#!/bin/sh\necho \"$#|$*|$PGO_TOKEN\" > "+out+"\n"), 0o755)
	t.Setenv("PGO_ANYDESK", fake)
	t.Setenv("PGO_TOKEN", "secreto")
	s, pide := anydeskServer(t)
	for _, body := range []string{`{"project":590,"peer":"-h"}`, `{"project":590,"peer":"1;id"}`, `{"project":590,"peer":"$(id)@ad"}`,
		`{"project":590,"peer":"123456789 --plain"}`, `{"peer":"123456789"}`} {
		if w := pide("/api/anydesk/connect", body); w.Code == 200 {
			t.Errorf("%s aceptado", body)
		}
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("se lanzó AnyDesk con datos no válidos")
	}
	w := pide("/api/anydesk/connect", `{"project":590,"peer":" 123 456 789 "}`)
	if w.Code != 200 {
		t.Fatalf("conectar: %d %s", w.Code, w.Body)
	}
	var got string
	for i := 0; i < 50 && got == ""; i++ {
		b, _ := os.ReadFile(out)
		got = strings.TrimSpace(string(b))
		time.Sleep(20 * time.Millisecond)
	}
	if got != "1|123456789|" {
		t.Fatalf("AnyDesk recibió %q", got)
	}
	// remembered for that computer (and the launch waits for its session)
	b, _ := os.ReadFile(s.AnyDeskFile)
	if !strings.Contains(string(b), `"123456789"`) || !strings.Contains(string(b), `"uid": 7`) || len(s.anydesk.launches) != 1 {
		t.Fatalf("ajustes: %s (%d lanzamientos)", b, len(s.anydesk.launches))
	}
}

func openChange(key string, d anydesk.Direction, peer string, start time.Time) anydesk.Change {
	return anydesk.Change{Open: true, Time: start, Session: anydesk.Session{Key: key, Direction: d, Peer: peer, PID: 1, Start: start}}
}

func (s *Servidor) anydeskConn(key string) *anydeskConn {
	s.anydesk.mu.Lock()
	defer s.anydesk.mu.Unlock()
	if c := s.anydesk.conns[key]; c != nil {
		x := *c
		return &x
	}
	return nil
}

// The project of each session: the launch from pgocode, the computer's last one, the default for incoming ones; none,
// no time until it is assigned.
func TestAnyDeskSessions(t *testing.T) {
	s, pide := anydeskServer(t)
	start := time.Now().Add(-time.Minute)
	// before anyone has used AnyDesk from pgocode: nothing is logged
	s.anydeskApply(openChange("k0", anydesk.Incoming, "444555666", start))
	if c := s.anydeskConn("k0"); c == nil || c.Project != 0 {
		t.Fatalf("sin usuario: %+v", c)
	}
	s.anydeskApply(anydesk.Change{Session: anydesk.Session{Key: "k0"}})

	s.anydesk.launches = []anydeskLaunch{{peer: "111222333", project: 590, projectName: "Proyecto A", uid: 7, at: time.Now()}}
	s.anydeskApply(openChange("k1", anydesk.Outgoing, "111222333", start))
	c := s.anydeskConn("k1")
	if c == nil || c.Project != 590 || c.uid != 7 || len(s.anydesk.launches) != 0 {
		t.Fatalf("saliente: %+v", c)
	}
	works := s.anydeskBeat(true, time.Now())
	if len(works) != 1 || works[0].Sesion != "anydesk-"+c.ID || works[0].Proyecto != 590 || works[0].UID != 7 || works[0].Modelo != "AnyDesk" {
		t.Fatalf("latido: %+v", works)
	}
	if len(s.anydeskBeat(false, time.Now())) != 0 {
		t.Fatal("sin nadie al teclado no hay tiempo")
	}
	if s.TrabajoEnCurso() == "" {
		t.Fatal("con una sesión de AnyDesk imputando no se actualiza")
	}

	// incoming, without a default project: not logged; assigned from the interface (and made the default)
	s.anydesk.mu.Lock()
	s.anydesk.settings.UID = 7
	s.anydesk.mu.Unlock()
	s.anydeskApply(openChange("k2", anydesk.Incoming, "444555666", start))
	c2 := s.anydeskConn("k2")
	if c2 == nil || c2.Project != 0 || len(s.anydeskBeat(true, time.Now())) != 1 {
		t.Fatalf("entrante sin proyecto: %+v", c2)
	}
	if w := pide("/api/anydesk/assign", fmt.Sprintf(`{"id":%q,"project":591,"default":true}`, c2.ID)); w.Code != 200 {
		t.Fatalf("asignar: %d %s", w.Code, w.Body)
	}
	if c2 = s.anydeskConn("k2"); c2.Project != 591 || c2.ProjectName != "Proyecto B" || len(s.anydeskBeat(true, time.Now())) != 2 {
		t.Fatalf("asignada: %+v", c2)
	}
	// another incoming one: the default; the same computer again (outgoing, from AnyDesk): its last project
	s.anydeskApply(openChange("k3", anydesk.Incoming, "777888999", start))
	s.anydeskApply(openChange("k4", anydesk.Outgoing, "444555666", start))
	s.anydeskApply(openChange("k5", anydesk.Outgoing, "777000111", start))
	if a, b, d := s.anydeskConn("k3"), s.anydeskConn("k4"), s.anydeskConn("k5"); a.Project != 591 || b.Project != 591 || d.Project != 0 {
		t.Fatalf("por defecto %d, recordado %d, desconocido %d", a.Project, b.Project, d.Project)
	}
	var st struct {
		Sessions []anydeskConn `json:"sessions"`
		Default  int           `json:"default_project"`
	}
	json.Unmarshal(pide("/api/anydesk", "").Body.Bytes(), &st)
	if len(st.Sessions) != 5 || st.Default != 591 {
		t.Fatalf("estado: %+v", st)
	}
	if w := pide("/api/anydesk/default", `{"project":0}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	for _, k := range []string{"k1", "k2", "k3", "k4", "k5"} {
		s.anydeskApply(anydesk.Change{Session: anydesk.Session{Key: k}})
	}
	if s.anydesk.open() != 0 || s.TrabajoEnCurso() != "" {
		t.Fatal("siguen abiertas")
	}
	// the settings survive a restart
	s2 := &Servidor{AnyDeskFile: s.AnyDeskFile}
	s2.anydesk.mu.Lock()
	st2 := s2.anydeskSettingsLocked()
	s2.anydesk.mu.Unlock()
	if st2.UID != 7 || st2.Default != 0 || st2.Peers["444555666"].Project != 591 {
		t.Fatalf("ajustes: %+v", st2)
	}
}

// From the trace file to the open sessions, and only what is open when pgocode starts (not the past).
func TestWatchAnyDesk(t *testing.T) {
	oldPoll, oldAlive := anydeskPoll, anydeskAlive
	anydeskPoll = 20 * time.Millisecond
	pid := os.Getpid()
	anydeskAlive = func(p int) bool { return p == pid }
	defer func() { anydeskPoll, anydeskAlive = oldPoll, oldAlive }()
	s, _ := anydeskServer(t)
	p := filepath.Join(t.TempDir(), "anydesk.trace")
	line := func(at time.Time, role, comp, msg string) string {
		return fmt.Sprintf("   info %s %10s   main %d %d %30s - %s\n", at.Format("2006-01-02 15:04:05.000"), role, pid, pid+1, comp, msg)
	}
	now := time.Now()
	past := line(now.Add(-time.Hour), "front", "unix_app.frontend", "Sending a connection request for address 111222333.") +
		line(now.Add(-time.Hour), "front", "app.frontend_session", "Session started.") +
		line(now.Add(-50*time.Minute), "front", "app.frontend_session", "Session stopped.") +
		line(now.Add(-time.Minute), "backend", "app.backend_session", "Incoming session request: Ana (444555666)") +
		line(now.Add(-time.Minute), "backend", "app.backend_session", "Session started (ok).")
	os.WriteFile(p, []byte(past), 0o644)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.WatchAnyDesk(ctx, []string{p}); close(done) }()
	defer func() { cancel(); <-done }()
	list := func() []anydeskConn {
		s.anydesk.mu.Lock()
		defer s.anydesk.mu.Unlock()
		var l []anydeskConn
		for _, c := range s.anydesk.conns {
			l = append(l, *c)
		}
		return l
	}
	wait := func(what string, ok func(c anydeskConn) bool) {
		for i := 0; i < 300; i++ {
			if l := list(); len(l) == 1 && ok(l[0]) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("%s: %+v", what, list())
	}
	wait("al arrancar", func(c anydeskConn) bool {
		return c.Direction == "incoming" && c.Peer == "444555666" && c.PeerName == "Ana"
	})
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(line(time.Now(), "backend", "app.backend_session", "Session stopped."))
	f.WriteString(line(time.Now(), "front", "unix_app.frontend", "Sending a connection request for address equipo@ad."))
	f.WriteString(line(time.Now(), "front", "app.frontend_session", "Session started."))
	f.Close()
	wait("nueva", func(c anydeskConn) bool { return c.Direction == "outgoing" && c.Peer == "equipo@ad" })
}
