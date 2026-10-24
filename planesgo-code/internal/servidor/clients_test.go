package servidor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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
