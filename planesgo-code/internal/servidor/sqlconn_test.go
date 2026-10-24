package servidor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// sqlCall: a request of the interface (cookie) or of an agent (token) to the SQL routes.
func sqlCall(t *testing.T, h http.Handler, method, path, body, cookieName, cookieValue, agent string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://127.0.0.1:7801"+path, strings.NewReader(body))
	r.Header.Set("X-PGO", "1")
	if agent != "" {
		r.Header.Set("X-PGO-Agente", agent)
	} else {
		r.AddCookie(&http.Cookie{Name: cookieName, Value: cookieValue})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// Without a connection nothing runs; the session id is checked; a bad server is refused before connecting.
func TestSQLWithoutConnection(t *testing.T) {
	s, _, _ := servidorAgente(t)
	ses, _ := s.Sesiones.Crea(2, "Prueba", "prueba", "sesion-odoo")
	ses.MarcaProyecto(7, time.Now().Add(time.Hour))
	h := s.Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		return sqlCall(t, h, method, path, body, s.cookieSesion(), ses.Token, "")
	}
	if w := call("GET", "/api/sql?project=7&session=s1", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"connected":false`) {
		t.Errorf("state: %d %s", w.Code, w.Body)
	}
	if w := call("POST", "/api/sql/run", `{"project":7,"session":"s1","sql":"SELECT 1"}`); w.Code != http.StatusConflict {
		t.Errorf("run without connection: %d %s", w.Code, w.Body)
	}
	if w := call("GET", "/api/sql?project=7&session=a%20b", ""); w.Code != http.StatusBadRequest {
		t.Errorf("bad session: %d", w.Code)
	}
	if w := call("POST", "/api/sql/connect", `{"project":7,"session":"s1","server":"h;x=1","user":"u","password":"p"}`); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "servidor no válido") {
		t.Errorf("bad server: %d %s", w.Code, w.Body)
	}
	tok, _ := s.issueAgentToken(7, "s1")
	if w := sqlCall(t, h, "POST", "/api/pgo/sql", `{"proyecto":7,"sql":"SELECT 1"}`, "", "", tok); w.Code != http.StatusConflict {
		t.Errorf("agent without connection: %d %s", w.Code, w.Body)
	}
	if w := sqlCall(t, h, "GET", "/api/pgo/sql?proyecto=7", "", "", "", tok); w.Code != http.StatusForbidden {
		t.Errorf("agent with GET: %d", w.Code)
	}
}
