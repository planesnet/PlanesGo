package servidor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The person gives an external agent access to a session: a 600 file in its folder with the session's token, which
// opens the agents' orders of that session only, and goes (token and file) when the access is taken back.
func TestExternalAccess(t *testing.T) {
	s, _, dir := servidorAgente(t)
	s.DirBin = "/opt/x/bin"
	s.ExternalFile = filepath.Join(t.TempDir(), "accesos-externos.json")
	ses, _ := s.Sesiones.Crea(2, "Prueba", "prueba", "sesion-odoo")
	ses.MarcaProyecto(7, time.Now().Add(time.Hour))
	h := s.Handler()
	ui := func(path string, body map[string]any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		return sqlCall(t, h, "POST", path, string(b), s.cookieSesion(), ses.Token, "")
	}
	w := ui("/api/acceso-externo", map[string]any{"project": 7, "session": "s1", "folder": dir, "label": "Claude de Sage", "hours": 8})
	var l []externalView
	json.Unmarshal(w.Body.Bytes(), &l)
	if w.Code != 200 || len(l) != 1 || l[0].Label != "Claude de Sage" {
		t.Fatalf("grant: %d %s", w.Code, w.Body)
	}
	file := filepath.Join(dir, ExternalAccessFile)
	st, err := os.Stat(file)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("file: %v %v", st, err)
	}
	b, _ := os.ReadFile(file)
	var token string
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "export PGO_TOKEN="); ok {
			token = v
		}
	}
	if token == "" || !strings.Contains(string(b), "export PGO_PROYECTO=7") || !strings.Contains(string(b), "export PGO_SESION=s1") || !strings.Contains(string(b), `PATH="/opt/x/bin"`) {
		t.Fatalf("file:\n%s", b)
	}
	// The session's properties show it, with the variables to give the agent.
	if l[0].Token != token || !strings.Contains(l[0].Env, "export PGO_TOKEN="+token) {
		t.Errorf("properties: %+v", l[0])
	}
	// It is kept (600) and comes back when pgocode starts again.
	if st, err := os.Stat(s.ExternalFile); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("kept: %v", err)
	}
	s2, _, _ := servidorAgente(t)
	s2.ExternalFile = s.ExternalFile
	s2.LoadExternal()
	if g, ok := s2.agentes.lookup(token); !ok || !g.external || g.session != "s1" || g.uid != 2 {
		t.Errorf("after a restart: %+v %v", g, ok)
	}
	// Its orders say who gives them in the audit logs.
	if g, _ := s.agentes.lookup(token); g.origin() != "agente externo: Claude de Sage" {
		t.Errorf("origin: %q", g.origin())
	}
	// The token works like the session's agents': only /api/pgo, only its project.
	if w := sqlCall(t, h, "GET", "/api/pgo/procesos?proyecto=7", "", "", "", token); w.Code != 200 {
		t.Errorf("pgo procesos: %d %s", w.Code, w.Body)
	}
	if w := sqlCall(t, h, "GET", "/api/pgo/procesos?proyecto=8", "", "", "", token); w.Code != http.StatusForbidden {
		t.Errorf("another project: %d", w.Code)
	}
	if w := sqlCall(t, h, "GET", "/api/proyectos", "", "", "", token); w.Code != http.StatusForbidden {
		t.Errorf("outside /api/pgo: %d", w.Code)
	}
	// Taken back: the token stops working and the file goes.
	if w := ui("/api/acceso-externo/retirar", map[string]any{"project": 7, "session": "s1", "id": l[0].ID}); w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("revoke: %d %s", w.Code, w.Body)
	}
	if w := sqlCall(t, h, "GET", "/api/pgo/procesos?proyecto=7", "", "", "", token); w.Code != http.StatusForbidden {
		t.Errorf("after revoking: %d", w.Code)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("the file stays")
	}
	if b, _ := os.ReadFile(s.ExternalFile); strings.Contains(string(b), token) {
		t.Error("a revoked access is still kept")
	}
	// A folder outside the project is refused.
	if w := ui("/api/acceso-externo", map[string]any{"project": 7, "session": "s1", "folder": t.TempDir()}); w.Code != http.StatusForbidden {
		t.Errorf("folder outside the project: %d", w.Code)
	}
}
