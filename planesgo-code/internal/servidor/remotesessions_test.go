package servidor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"planesgo-code/internal/clients"
	"planesgo-code/internal/imputacion"
	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/sesiones"
)

// noOdoo: an Odoo that is not there (the imputation service keeps its periods in memory all the same).
type noOdoo struct{}

func (noOdoo) CallKW(context.Context, string, string, string, []any, map[string]any, any) error {
	return errors.New("sin Odoo en las pruebas")
}

// fakeClock: the remote sessions' clock, moved by hand.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) pass(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type remoteEnv struct {
	s       *Servidor
	clock   *fakeClock
	ses     *sesiones.Sesion
	a, b    string // the tokens of two paired clients
	handler http.Handler
}

func newRemoteEnv(t *testing.T) *remoteEnv {
	t.Helper()
	dir := t.TempDir()
	reg := &clients.Registry{File: filepath.Join(dir, "clientes.json"), SecretFile: filepath.Join(dir, "emparejar")}
	if err := reg.Init(); err != nil {
		t.Fatal(err)
	}
	secret, _ := clients.ReadSecret(reg.SecretFile)
	_, a, _ := reg.Pair(secret, "a")
	_, b, _ := reg.Pair(secret, "b")
	clock := &fakeClock{t: time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)}
	s := &Servidor{Puerto: "7801", Sesiones: sesiones.NuevoAlmacen(), Indice: proyectos.NuevoIndice(nil), Clients: reg,
		AnyDeskFile: filepath.Join(dir, "anydesk.json"), Imputacion: &imputacion.Gestor{Odoo: noOdoo{}}}
	s.remote.now = clock.now
	h := s.Handler()
	ses, _ := s.Sesiones.Crea(7, "Luis", "luis", "odoo-x")
	for _, p := range []int{590, 591} {
		ses.MarcaProyecto(p, time.Now().Add(time.Hour))
	}
	s.nombres[590], s.nombres[591] = "Proyecto A", "Proyecto B"
	t.Cleanup(s.Imputacion.Espera)
	return &remoteEnv{s: s, clock: clock, ses: ses, a: a, b: b, handler: h}
}

// call: a request from a client (its token) and, if user, with the user's session cookie too.
func (e *remoteEnv) call(method, path, body, client string, user bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:7801"+path, strings.NewReader(body))
	r.Header.Set("X-PGO", "1")
	if client != "" {
		r.AddCookie(&http.Cookie{Name: cookieEscritorio, Value: client})
	}
	if user {
		r.AddCookie(&http.Cookie{Name: e.s.cookieSesion(), Value: e.ses.Token})
	}
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, r)
	return w
}

func (e *remoteEnv) create(t *testing.T, client, body string) remoteSession {
	t.Helper()
	w := e.call("POST", "/api/remote/sessions", body, client, false)
	if w.Code != http.StatusOK {
		t.Fatalf("alta %s: %d %s", body, w.Code, w.Body)
	}
	var c remoteSession
	json.Unmarshal(w.Body.Bytes(), &c)
	return c
}

func (e *remoteEnv) open() []string {
	_, l := e.s.Imputacion.Estado()
	var ids []string
	for _, a := range l {
		ids = append(ids, a.Sesion)
	}
	return ids
}

const rdpBody = `{"kind":"rdp","project":590,"name":"Sage","host":"192.168.125.229","user":"MAZANA\\luis"}`

// Creating, beating and ending a session; the user is the one of the client's window, and the time is counted
// only after the first seconds and with someone at the keyboard.
func TestRemoteSessionLifecycle(t *testing.T) {
	e := newRemoteEnv(t)
	// Without a client token: refused, even with a user session.
	if w := e.call("POST", "/api/remote/sessions", rdpBody, "", true); w.Code != http.StatusForbidden {
		t.Fatalf("sin cliente: %d", w.Code)
	}
	// The helper brings only its client's token: before its window has a user session, nobody to log the time to.
	if w := e.call("POST", "/api/remote/sessions", rdpBody, e.a, false); w.Code != http.StatusUnauthorized {
		t.Fatalf("sin usuario en el cliente: %d %s", w.Code, w.Body)
	}
	// The window of client a uses its session (both cookies): from then on, a's helper is that user.
	if w := e.call("GET", "/api/remote/sessions", "", e.a, true); w.Code != http.StatusOK {
		t.Fatalf("lista: %d", w.Code)
	}
	for _, bad := range []string{`{"kind":"rdp","host":"h","user":"u"}`, `{"kind":"rdp","project":590,"host":"h","user":""}`,
		`{"kind":"rdp","project":590,"host":"h x","user":"u"}`, `{"kind":"rdp","project":590,"host":"h\u0000x","user":"u"}`,
		`{"kind":"vnc","project":590,"host":"h","user":"u"}`, `{"kind":"anydesk","peer":"-h"}`} {
		if w := e.call("POST", "/api/remote/sessions", bad, e.a, false); w.Code == http.StatusOK {
			t.Errorf("aceptado: %s", bad)
		}
	}
	c := e.create(t, e.a, rdpBody)
	if c.ID == "" || c.State != remoteConnecting || c.ProjectName != "Proyecto A" || c.User != `MAZANA\luis` {
		t.Fatalf("alta: %+v", c)
	}
	if e.s.TrabajoEnCurso() == "" {
		t.Fatal("con un escritorio remoto abierto no se actualiza")
	}
	beat := func(client, body string) int {
		return e.call("POST", "/api/remote/sessions/"+c.ID+"/beat", body, client, false).Code
	}
	// Too soon (a failed login logs nothing).
	e.clock.pass(5 * time.Second)
	if beat(e.a, `{"active":true}`) != 200 || len(e.open()) != 0 {
		t.Fatalf("antes de tiempo: %v", e.open())
	}
	e.clock.pass(15 * time.Second)
	if beat(e.a, `{"active":false}`) != 200 || len(e.open()) != 0 {
		t.Fatalf("sin nadie al teclado: %v", e.open())
	}
	if beat(e.a, `{"active":true}`) != 200 || !slices.Equal(e.open(), []string{"rdp-" + c.ID}) {
		t.Fatalf("en uso: %v", e.open())
	}
	l := e.s.remote.listAll(false)
	if len(l) != 1 || l[0].State != remoteConnected || l[0].uid != 7 {
		t.Fatalf("conectada: %+v", l)
	}
	// Another client cannot beat or end it (nor learn that it exists).
	if code := beat(e.b, `{"active":true}`); code != http.StatusNotFound {
		t.Fatalf("latido ajeno: %d", code)
	}
	if w := e.call("POST", "/api/remote/sessions/"+c.ID+"/end", `{}`, e.b, true); w.Code != http.StatusNotFound {
		t.Fatalf("fin ajeno: %d", w.Code)
	}
	if e.s.remote.listAll(true) == nil || len(e.s.remote.listAll(true)) != 1 {
		t.Fatal("un cliente ajeno la ha cerrado")
	}
	// The end, with its explained error: the period closes and the interface sees why.
	if w := e.call("POST", "/api/remote/sessions/"+c.ID+"/end", `{"error":"Usuario o contraseña incorrectos."}`, e.a, false); w.Code != 200 {
		t.Fatalf("fin: %d %s", w.Code, w.Body)
	}
	if len(e.open()) != 0 || e.s.TrabajoEnCurso() != "" {
		t.Fatalf("sigue abierta: %v %q", e.open(), e.s.TrabajoEnCurso())
	}
	if code := beat(e.a, `{"active":true}`); code != http.StatusGone {
		t.Fatalf("latido de una terminada: %d", code)
	}
	var got []remoteSession
	json.Unmarshal(e.call("GET", "/api/remote/sessions", "", e.a, true).Body.Bytes(), &got)
	if len(got) != 1 || got[0].State != remoteEnded || got[0].Error != "Usuario o contraseña incorrectos." {
		t.Fatalf("lista: %+v", got)
	}
	if strings.Contains(e.call("GET", "/api/remote/sessions", "", e.a, true).Body.String(), e.a) {
		t.Fatal("la lista lleva el token del cliente")
	}
	// Ended sessions are forgotten after a while.
	e.clock.pass(remoteKeepEnded + time.Second)
	e.s.expireRemote()
	if len(e.s.remote.listAll(false)) != 0 {
		t.Fatal("no se olvida")
	}
}

// Without beats for 60 s (the client closed or fell), the server closes the period by itself.
func TestRemoteSessionExpires(t *testing.T) {
	e := newRemoteEnv(t)
	e.call("GET", "/api/remote/sessions", "", e.a, true)
	c := e.create(t, e.a, rdpBody)
	e.clock.pass(20 * time.Second)
	e.call("POST", "/api/remote/sessions/"+c.ID+"/beat", `{"active":true}`, e.a, false)
	if len(e.open()) != 1 {
		t.Fatal("no se imputa")
	}
	e.clock.pass(remoteSilence)
	e.s.expireRemote()
	if len(e.s.remote.listAll(true)) != 1 {
		t.Fatal("cerrada antes de los 60 s")
	}
	e.clock.pass(time.Second)
	e.s.expireRemote()
	if l := e.s.remote.listAll(false); len(l) != 1 || l[0].State != remoteEnded || l[0].Error == "" || len(e.open()) != 0 {
		t.Fatalf("sin latidos sigue abierta: %+v %v", l, e.open())
	}
	if e.s.TrabajoEnCurso() != "" {
		t.Fatal("cuenta una sesión caducada")
	}
}

// AnyDesk: an incoming session without a project is listed without one (no time) and the interface gives it one;
// the project is remembered for that computer and, if asked, for every incoming session.
func TestRemoteAnyDesk(t *testing.T) {
	e := newRemoteEnv(t)
	// Before anyone used AnyDesk from pgocode in this client: listed, without a project.
	c0 := e.create(t, e.a, `{"kind":"anydesk","direction":"incoming","peer":"444 555 666","peer_name":"Ana"}`)
	if c0.Project != 0 || c0.Peer != "444555666" {
		t.Fatalf("entrante: %+v", c0)
	}
	if e.s.TrabajoEnCurso() != "" {
		t.Fatal("una sesión sin proyecto no impide actualizar")
	}
	// Outgoing from pgocode (project chosen in the window): remembered for that computer.
	e.call("GET", "/api/remote/sessions", "", e.a, true)
	c1 := e.create(t, e.a, `{"kind":"anydesk","direction":"outgoing","peer":"111222333","project":590}`)
	if c1.Project != 590 {
		t.Fatalf("saliente: %+v", c1)
	}
	var st struct {
		Sessions []remoteSession `json:"sessions"`
		Default  int             `json:"default_project"`
	}
	json.Unmarshal(e.call("GET", "/api/anydesk", "", e.a, true).Body.Bytes(), &st)
	if len(st.Sessions) != 2 {
		t.Fatalf("estado: %+v", st)
	}
	if w := e.call("POST", "/api/anydesk/assign", `{"id":"`+c0.ID+`","project":591,"default":true}`, e.a, true); w.Code != 200 {
		t.Fatalf("asignar: %d %s", w.Code, w.Body)
	}
	e.clock.pass(20 * time.Second)
	for _, c := range []remoteSession{c0, c1} {
		e.call("POST", "/api/remote/sessions/"+c.ID+"/beat", `{"active":true}`, e.a, false)
	}
	if o := e.open(); len(o) != 2 {
		t.Fatalf("imputando: %v", o)
	}
	// Another incoming one: the default; the same computer again: its last project; an unknown one: none.
	c3 := e.create(t, e.a, `{"kind":"anydesk","direction":"incoming","peer":"777888999"}`)
	c4 := e.create(t, e.a, `{"kind":"anydesk","direction":"outgoing","peer":"111222333"}`)
	c5 := e.create(t, e.a, `{"kind":"anydesk","direction":"outgoing","peer":"777000111"}`)
	if c3.Project != 591 || c4.Project != 590 || c5.Project != 0 {
		t.Fatalf("por defecto %d, recordado %d, desconocido %d", c3.Project, c4.Project, c5.Project)
	}
	// From an app where nobody has a pgocode session: no project on a guess (whose time would it be?).
	if c6 := e.create(t, e.b, `{"kind":"anydesk","direction":"incoming","peer":"777888999"}`); c6.Project != 0 {
		t.Fatalf("sin nadie en esa app: %+v", c6)
	}
	// The settings survive a restart.
	s2 := &Servidor{AnyDeskFile: e.s.AnyDeskFile}
	s2.anydesk.mu.Lock()
	st2 := s2.anydeskSettingsLocked()
	s2.anydesk.mu.Unlock()
	if st2.Default != 591 || st2.Peers["444555666"].Project != 591 || st2.Peers["111222333"].Project != 590 {
		t.Fatalf("ajustes: %+v", st2)
	}
}

// Each user sees their own remote sessions and the ones nobody owns yet, never another user's.
func TestRemoteSessionsVisible(t *testing.T) {
	rs := remoteSessions{m: map[string]*remoteSession{"a": {ID: "a", uid: 7}, "b": {ID: "b", uid: 8}, "c": {ID: "c", client: "app1"}, "d": {ID: "d", client: "app2"}}}
	got := rs.list("", false, 7, "app1")
	if len(got) != 2 || slices.ContainsFunc(got, func(c remoteSession) bool { return c.ID == "b" || c.ID == "d" }) {
		t.Errorf("visible: %+v", got)
	}
}

// listAll: every session, whoever's it is (the tests look at the store as a whole).
func (rs *remoteSessions) listAll(onlyOpen bool) []remoteSession {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	l := []remoteSession{}
	for _, c := range rs.m {
		if !onlyOpen || c.open() {
			l = append(l, *c)
		}
	}
	return l
}
