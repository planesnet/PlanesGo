//go:build integracion

// As in cmd/pgocode: SQL Server's own certificate has a negative serial number.
//
//go:debug x509negativeserial=1
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
	// A second connection with its name and its own mode: the agent picks it; the main one stays read only.
	second := map[string]any{"project": 7, "session": "s1", "name": "contenedor", "server": server, "user": "sa", "password": pw, "trust_certificate": true, "allow_changes": true}
	if w := ui("/api/sql/connect", second); w.Code != 200 || !strings.Contains(w.Body.String(), `"names":["contenedor","principal"]`) {
		t.Fatalf("second connection: %d %s", w.Code, w.Body)
	}
	if w := agent(mine, `{"proyecto":7,"conexion":"contenedor","sql":"SELECT 1 AS uno","escribir":true}`); w.Code != 200 {
		t.Errorf("named connection, updatable: %d %s", w.Code, w.Body)
	}
	if w := agent(mine, `{"proyecto":7,"conexion":"otra","sql":"SELECT 1"}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "contenedor, principal") {
		t.Errorf("unknown connection: %d %s", w.Code, w.Body)
	}
	if w := agent(mine, `{"proyecto":7,"conexion":"Mal Nombre","sql":"SELECT 1"}`); w.Code != http.StatusBadRequest {
		t.Errorf("invalid name: %d", w.Code)
	}
	if w := ui("/api/sql/disconnect", map[string]any{"project": 7, "session": "s1"}); w.Code != 200 || s.SQLFor(7, "s1") != nil {
		t.Errorf("disconnect: %d", w.Code)
	}
	audit, _ := os.ReadFile(s.SQLAudit)
	if !strings.Contains(string(audit), "SELECT 40+2") || !strings.Contains(string(audit), `"origin":"agente"`) || strings.Contains(string(audit), pw) {
		t.Errorf("audit:\n%s", audit)
	}
}

// A program of the session reads a large catalog (30 000 rows) from a named connection, streamed in JSON lines.
func TestSQLStreamReal(t *testing.T) {
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
	conn, _ := json.Marshal(map[string]any{"project": 7, "session": "s1", "name": "pruebas", "server": server, "user": "sa", "password": pw, "trust_certificate": true})
	if w := sqlCall(t, h, "POST", "/api/sql/connect", string(conn), s.cookieSesion(), ses.Token, ""); w.Code != 200 {
		t.Fatalf("connect: %d %s", w.Code, w.Body)
	}
	code, revoke := s.programas.issue(programGrant{project: 7, session: "s1", program: "pgosage", login: "prueba"})
	defer revoke()
	token, _ := s.programas.exchange(code)
	q := `{"conexion":"pruebas","sql":"SELECT TOP 30000 ROW_NUMBER() OVER (ORDER BY (SELECT NULL)) AS n, N'año' AS t FROM sys.all_objects a CROSS JOIN sys.all_objects b"}`
	r := httptest.NewRequest("POST", "http://127.0.0.1:7801/api/pgo/sql-flujo", strings.NewReader(q))
	r.Header.Set("X-PGO", "1")
	r.Header.Set("X-PGO-Programa", token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if w.Code != 200 || len(lines) != 30002 || !strings.HasPrefix(lines[0], `{"columns":["n","t"]`) || !strings.Contains(lines[len(lines)-1], `"rows":30000`) {
		t.Fatalf("stream: %d, %d lines, first %s, last %s", w.Code, len(lines), lines[0], lines[len(lines)-1])
	}
	if lines[30000] != `{"row":[30000,"año"]}` {
		t.Errorf("last row: %s", lines[30000])
	}
	audit, _ := os.ReadFile(s.SQLAudit)
	if !strings.Contains(string(audit), `"origin":"programa pgosage"`) || !strings.Contains(string(audit), `"name":"pruebas"`) || !strings.Contains(string(audit), `"rows":30000`) {
		t.Errorf("audit:\n%s", audit)
	}
}
