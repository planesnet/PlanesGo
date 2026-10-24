package servidor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"planesgo-code/internal/clients"
	"planesgo-code/internal/sesiones"
)

// A client pairs with the secret, gets in with its token, and stops getting in once revoked.
func TestClientPairing(t *testing.T) {
	dir := t.TempDir()
	reg := &clients.Registry{File: filepath.Join(dir, "clientes.json"), SecretFile: filepath.Join(dir, "emparejar")}
	if err := reg.Init(); err != nil {
		t.Fatal(err)
	}
	s := &Servidor{Puerto: "7801", Sesiones: sesiones.NuevoAlmacen(), Clients: reg}
	h := s.Handler()
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:7801"+path, strings.NewReader(body))
		r.Header.Set("X-PGO", "1")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: cookieEscritorio, Value: token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/", "", ""); w.Code != http.StatusForbidden {
		t.Errorf("not paired: %d", w.Code)
	}
	if w := call("POST", "/api/clientes/emparejar", `{"secret":"malo","name":"x"}`, ""); w.Code != http.StatusForbidden {
		t.Errorf("wrong secret: %d", w.Code)
	}
	if w := call("POST", "/api/clientes/emparejar", `{}`, "x"); w.Code == http.StatusOK {
		t.Error("pairing needs the secret")
	}
	secret, _ := clients.ReadSecret(reg.SecretFile)
	w := call("POST", "/api/clientes/emparejar", `{"secret":"`+secret+`","name":"pcluis"}`, "")
	var p struct{ ID, Token string }
	json.Unmarshal(w.Body.Bytes(), &p)
	if w.Code != http.StatusOK || p.Token == "" {
		t.Fatalf("pair: %d %s", w.Code, w.Body)
	}
	if w := call("GET", "/api/clientes/yo", "", p.Token); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), p.ID) {
		t.Errorf("me: %d %s", w.Code, w.Body)
	}
	if w := call("GET", "/api/version", "", p.Token); w.Code != http.StatusOK {
		t.Errorf("a paired client gets in: %d", w.Code)
	}
	if w := call("GET", "/api/clientes/yo", "", "otro"); w.Code != http.StatusForbidden && w.Code != http.StatusUnauthorized {
		t.Errorf("unknown token: %d", w.Code)
	}
	reg.Revoke(p.ID)
	if w := call("GET", "/api/version", "", p.Token); w.Code != http.StatusForbidden {
		t.Errorf("revoked: %d", w.Code)
	}
}

// Propiedades → Servidor: the clients are listed (without hashes), another one can be revoked but not the app
// that asks, and the server can be stopped.
func TestClientsRevokeAndStop(t *testing.T) {
	e := newRemoteEnv(t)
	w := e.call("GET", "/api/clientes", "", e.a, true)
	var l struct {
		Clients []clients.Client `json:"clients"`
		Me      string           `json:"me"`
	}
	json.Unmarshal(w.Body.Bytes(), &l)
	if w.Code != 200 || len(l.Clients) != 2 || l.Me == "" || strings.Contains(w.Body.String(), `"hash":"`+"a") {
		t.Fatalf("lista: %d %s", w.Code, w.Body)
	}
	for _, c := range l.Clients {
		if c.Hash != "" {
			t.Fatal("la lista lleva el hash del token")
		}
	}
	other := l.Clients[0].ID
	if other == l.Me {
		other = l.Clients[1].ID
	}
	if w := e.call("POST", "/api/clientes/revocar", `{"id":"`+l.Me+`"}`, e.a, true); w.Code != http.StatusBadRequest {
		t.Fatalf("revocarse a sí misma: %d", w.Code)
	}
	if w := e.call("POST", "/api/clientes/revocar", `{"id":"`+other+`"}`, e.a, true); w.Code != 200 {
		t.Fatalf("revocar: %d %s", w.Code, w.Body)
	}
	if w := e.call("GET", "/api/remote/sessions", "", e.b, true); w.Code != http.StatusForbidden {
		t.Fatalf("el revocado sigue entrando: %d", w.Code)
	}
	if w := e.call("POST", "/api/servidor/parar", `{}`, e.a, true); w.Code != http.StatusConflict {
		t.Fatalf("sin Stop: %d", w.Code)
	}
	stopped := make(chan struct{})
	e.s.Stop = func() { close(stopped) }
	if w := e.call("POST", "/api/servidor/parar", `{}`, e.a, true); w.Code != 200 {
		t.Fatalf("parar: %d", w.Code)
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("no se para")
	}
}

// A client app with another major version is refused (426) with the explanation; another minor version works.
func TestClientVersion(t *testing.T) {
	s := &Servidor{Puerto: "7801", Sesiones: sesiones.NuevoAlmacen(), Version: "0.7.0", PermitirNavegador: true}
	h := s.Handler()
	call := func(v string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://127.0.0.1:7801/api/version", nil)
		r.Header.Set("X-PGO", "1")
		if v != "" {
			r.Header.Set("X-PGO-Version", v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for v, code := range map[string]int{"": 200, "0.7.4": 200, "dev": 200, "0.8.0": http.StatusUpgradeRequired, "0.6.9": http.StatusUpgradeRequired} {
		w := call(v)
		if w.Code != code {
			t.Errorf("%q: %d %s", v, w.Code, w.Body)
		}
		if code == http.StatusUpgradeRequired && !strings.Contains(w.Body.String(), "misma versión mayor (0.7)") {
			t.Errorf("%q: %s", v, w.Body)
		}
	}
}

// A client that is not paired yet can ask for the server's proof (and nothing else); the proof is the registry's.
func TestClientsProofRoute(t *testing.T) {
	dir := t.TempDir()
	reg := &clients.Registry{File: filepath.Join(dir, "clientes.json"), SecretFile: filepath.Join(dir, "emparejar")}
	if err := reg.Init(); err != nil {
		t.Fatal(err)
	}
	s := &Servidor{Puerto: "7801", Sesiones: sesiones.NuevoAlmacen(), Clients: reg}
	h := s.Handler()
	nonce := strings.Repeat("0f", 16)
	r := httptest.NewRequest("GET", "http://127.0.0.1:7801/api/clientes/prueba?emparejar=1&nonce="+nonce, nil)
	r.Header.Set("X-PGO", "1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	want, _ := reg.SecretProof(nonce)
	if w.Code != 200 || !strings.Contains(w.Body.String(), want) {
		t.Errorf("proof: %d %s", w.Code, w.Body)
	}
	r = httptest.NewRequest("GET", "http://127.0.0.1:7801/api/clientes/prueba?id=nadie&nonce="+nonce, nil)
	r.Header.Set("X-PGO", "1")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown client: %d", w.Code)
	}
}
