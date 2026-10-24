//go:build integracion

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

// A session's connection against a real SQL Server (PGO_SQLTEST_SERVER, PGO_SQLTEST_PASSWORD of «sa»): its agent
// uses it, another session's agent does not, read only unless allowed, and the audit has the batches but never the
// password.
func TestSQLSessionReal(t *testing.T) {
	server, pw := os.Getenv("PGO_SQLTEST_SERVER"), os.Getenv("PGO_SQLTEST_PASSWORD")
	if server == "" || pw == "" {
		t.Skip("sin PGO_SQLTEST_SERVER ni PGO_SQLTEST_PASSWORD")
	}
	s, _, _ := servidorAgente(t)
	s.SQLAudit = filepath.Join(t.TempDir(), "sql.jsonl")
	t.Cleanup(s.CloseConnections)
	ses, _ := s.Sesiones.Crea(2, "Prueba", "prueba", "sesion-odoo")
	ses.MarcaProyecto(7, time.Now().Add(time.Hour))
	h := s.Handler()
	ui := func(path string, body map[string]any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		return sqlCall(t, h, "POST", path, string(b), s.cookieSesion(), ses.Token, "")
	}
	conn := map[string]any{"project": 7, "session": "s1", "server": server, "user": "sa", "password": pw, "trust_certificate": true}
	if w := ui("/api/sql/connect", conn); w.Code != 200 || strings.Contains(w.Body.String(), pw) {
		t.Fatalf("connect: %d %s", w.Code, w.Body)
	}
	mine, _ := s.issueAgentToken(7, "s1")
	other, _ := s.issueAgentToken(7, "s2")
	agent := func(tok, body string) *httptest.ResponseRecorder {
		return sqlCall(t, h, "POST", "/api/pgo/sql", body, "", "", tok)
	}
	if w := agent(mine, `{"proyecto":7,"sql":"SELECT 40+2 AS r"}`); w.Code != 200 || !strings.Contains(w.Body.String(), "42") {
		t.Fatalf("agent: %d %s", w.Code, w.Body)
	}
	if w := agent(other, `{"proyecto":7,"sql":"SELECT 1"}`); w.Code != http.StatusConflict {
		t.Errorf("another session's agent: %d", w.Code)
	}
	if w := agent(mine, `{"proyecto":7,"sql":"CREATE TABLE dbo.pgo_x(a int)","escribir":true}`); w.Code != http.StatusForbidden {
		t.Errorf("write on a read-only connection: %d %s", w.Code, w.Body)
	}
	if s.SQLFor(7, "s1") == nil || s.SQLFor(7, "s2") != nil {
		t.Error("SQLFor")
	}
	if w := ui("/api/sql/disconnect", map[string]any{"project": 7, "session": "s1"}); w.Code != 200 || s.SQLFor(7, "s1") != nil {
		t.Errorf("disconnect: %d", w.Code)
	}
	audit, _ := os.ReadFile(s.SQLAudit)
	if !strings.Contains(string(audit), "SELECT 40+2") || !strings.Contains(string(audit), `"origin":"agente"`) || strings.Contains(string(audit), pw) {
		t.Errorf("audit:\n%s", audit)
	}
}
