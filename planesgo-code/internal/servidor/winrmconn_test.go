package servidor

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"planesgo-code/internal/winrm"
)

// Without a connection nothing runs; the session id and the host are checked before connecting.
func TestWinRMWithoutConnection(t *testing.T) {
	s, _, _ := servidorAgente(t)
	ses, _ := s.Sesiones.Crea(2, "Prueba", "prueba", "sesion-odoo")
	ses.MarcaProyecto(7, time.Now().Add(time.Hour))
	h := s.Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		return sqlCall(t, h, method, path, body, s.cookieSesion(), ses.Token, "")
	}
	if w := call("GET", "/api/winrm?project=7&session=s1", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"connected":false`) {
		t.Errorf("state: %d %s", w.Code, w.Body)
	}
	if w := call("POST", "/api/winrm/run", `{"project":7,"session":"s1","script":"Get-Date"}`); w.Code != http.StatusConflict {
		t.Errorf("run without connection: %d %s", w.Code, w.Body)
	}
	if w := call("POST", "/api/winrm/mode", `{"project":7,"session":"s1","allow_changes":true}`); w.Code != http.StatusConflict {
		t.Errorf("mode without connection: %d", w.Code)
	}
	if w := call("GET", "/api/winrm?project=7&session=a%20b", ""); w.Code != http.StatusBadRequest {
		t.Errorf("bad session: %d", w.Code)
	}
	if w := call("GET", "/api/winrm?project=8&session=s1", ""); w.Code == 200 {
		t.Errorf("a project of another user: %d", w.Code)
	}
	if w := call("POST", "/api/winrm/connect", `{"project":7,"session":"s1","host":"h;x","user":"u","password":"p"}`); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "equipo no válido") {
		t.Errorf("bad host: %d %s", w.Code, w.Body)
	}
	tok, _ := s.issueAgentToken(7, "s1")
	agent := func(method, orden, body string) *httptest.ResponseRecorder {
		return sqlCall(t, h, method, "/api/pgo/"+orden, body, "", "", tok)
	}
	for _, orden := range []string{"ps", "ps-download"} {
		if w := agent("POST", orden, `{"proyecto":7,"script":"Get-Date","path":"C:\\x"}`); w.Code != http.StatusConflict {
			t.Errorf("agent %s without connection: %d %s", orden, w.Code, w.Body)
		}
	}
	if w := agent("GET", "ps?proyecto=7", ""); w.Code != http.StatusForbidden {
		t.Errorf("agent with GET: %d", w.Code)
	}
	if w := agent("POST", "ps-upload", `{"proyecto":7,"path":"C:\\x","data":""}`); w.Code != http.StatusBadRequest {
		t.Errorf("upload without --escribir: %d", w.Code)
	}
}

// A session's connection: its agent reaches it, another session's agent does not; read only refuses what is not
// a query (before reaching the server) and writing needs changes allowed; the audit has the order, never output.
func TestWinRMSessionConnection(t *testing.T) {
	s, _, _ := servidorAgente(t)
	s.WinRMAudit = filepath.Join(t.TempDir(), "winrm.jsonl")
	t.Cleanup(s.CloseConnections)
	ses, _ := s.Sesiones.Crea(2, "Prueba", "prueba", "sesion-odoo")
	ses.MarcaProyecto(7, time.Now().Add(time.Hour))
	h := s.Handler()
	s.winrms.open(&winrm.Conn{}, 7, "s1", "prueba") // never reaches a server in this test

	if s.WinRMFor(7, "s1") == nil || s.WinRMFor(7, "s2") != nil {
		t.Fatal("WinRMFor")
	}
	ui := func(path, body string) *httptest.ResponseRecorder {
		return sqlCall(t, h, "POST", path, body, s.cookieSesion(), ses.Token, "")
	}
	if w := sqlCall(t, h, "GET", "/api/winrm?project=7&session=s1", "", s.cookieSesion(), ses.Token, ""); !strings.Contains(w.Body.String(), `"connected":true`) {
		t.Errorf("state: %s", w.Body)
	}
	mine, _ := s.issueAgentToken(7, "s1")
	other, _ := s.issueAgentToken(7, "s2")
	agent := func(tok, orden, body string) *httptest.ResponseRecorder {
		return sqlCall(t, h, "POST", "/api/pgo/"+orden, body, "", "", tok)
	}
	if w := agent(other, "ps", `{"proyecto":7,"script":"Get-Date"}`); w.Code != http.StatusConflict {
		t.Errorf("another session's agent: %d %s", w.Code, w.Body)
	}
	if w := agent(mine, "ps", `{"proyecto":7,"script":"Remove-Item C:\\x"}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "solo lectura") {
		t.Errorf("read only: %d %s", w.Code, w.Body)
	}
	if w := agent(mine, "ps", `{"proyecto":7,"script":"Remove-Item C:\\x","escribir":true}`); w.Code != http.StatusForbidden {
		t.Errorf("write on a read-only connection: %d %s", w.Code, w.Body)
	}
	if w := agent(mine, "ps-upload", `{"proyecto":7,"path":"C:\\x","data":"aG9sYQ==","escribir":true}`); w.Code != http.StatusForbidden {
		t.Errorf("upload on a read-only connection: %d %s", w.Code, w.Body)
	}
	if w := agent(mine, "ps", `{"proyecto":8,"script":"Get-Date"}`); w.Code != http.StatusForbidden {
		t.Errorf("token of another project: %d", w.Code)
	}
	if w := ui("/api/winrm/mode", `{"project":7,"session":"s1","allow_changes":true}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"allow_changes":true`) {
		t.Errorf("mode: %d %s", w.Code, w.Body)
	}
	if w := ui("/api/winrm/disconnect", `{"project":7,"session":"s1"}`); w.Code != 200 || s.WinRMFor(7, "s1") != nil {
		t.Errorf("disconnect: %d", w.Code)
	}
	audit, _ := os.ReadFile(s.WinRMAudit)
	if !strings.Contains(string(audit), `Remove-Item C:\\x`) || !strings.Contains(string(audit), `"origin":"agente"`) || !strings.Contains(string(audit), "(modo: actualizable)") {
		t.Errorf("audit:\n%s", audit)
	}
	if info, err := os.Stat(s.WinRMAudit); err != nil || info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Errorf("the audit log is private: %v %v", info.Mode(), err)
	}
}
