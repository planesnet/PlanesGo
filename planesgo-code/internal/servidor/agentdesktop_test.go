//go:build linux

package servidor

import (
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"planesgo-code/internal/agentdesktop"
	"planesgo-code/internal/agentdesktop/agentdesktoptest"
)

// The person starts the desktop of a session; only that session's agents use it (coordinates of the last
// screenshot, screenshots in the agent's folder); the password is never in arguments, answers or the audit.
func TestAgentDesktopSession(t *testing.T) {
	s, _, dir := servidorAgente(t)
	fakes := t.TempDir()
	tools := agentdesktoptest.Tools(t, fakes, "")
	run := filepath.Join(t.TempDir(), "run")
	s.agentDesktopOptions = func() (agentdesktop.Options, error) {
		return agentdesktop.Options{Tools: tools, RuntimeDir: run, Exchange: filepath.Join(fakes, "intercambio")}, nil
	}
	s.AgentDesktopAudit = filepath.Join(t.TempDir(), "escritorio.jsonl")
	t.Cleanup(s.CloseConnections)
	ses, _ := s.Sesiones.Crea(2, "Prueba", "prueba", "sesion-odoo")
	ses.MarcaProyecto(7, time.Now().Add(time.Hour))
	h := s.Handler()
	ui := func(method, path, body string) *httptest.ResponseRecorder {
		return sqlCall(t, h, method, path, body, s.cookieSesion(), ses.Token, "")
	}
	const pw = "Cl@ve-Secreta-9"

	if w := ui("GET", "/api/agent-desktop?project=7&session=s1", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"running":false`) {
		t.Fatalf("estado inicial: %d %s", w.Code, w.Body)
	}
	for _, bad := range []string{
		`{"project":7,"session":"s1","host":"-x","user":"u","password":"p"}`,
		`{"project":7,"session":"s1","host":"h","user":"u"}`,
		`{"project":7,"session":"a b","host":"h","user":"u","password":"p"}`,
		`{"project":7,"session":"s1","host":"h","user":"u","password":"p","keyboard":"otro"}`,
	} {
		if w := ui("POST", "/api/agent-desktop/start", bad); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", bad, w.Code, w.Body)
		}
	}
	if w := ui("POST", "/api/agent-desktop/start", `{"project":8,"session":"s1","host":"h","user":"u","password":"p"}`); w.Code == 200 {
		t.Error("proyecto ajeno")
	}
	w := ui("POST", "/api/agent-desktop/start", `{"project":7,"session":"s1","host":"127.0.0.1:3392","user":"PRUEBAS\\agente","password":"`+pw+`"}`)
	if w.Code != 200 || strings.Contains(w.Body.String(), pw) || !strings.Contains(w.Body.String(), `"running":true`) {
		t.Fatalf("iniciar: %d %s", w.Code, w.Body)
	}
	if b, _ := os.ReadFile(filepath.Join(fakes, "xfreerdp.args")); strings.Contains(string(b), pw) || !strings.Contains(string(b), "/u:agente") {
		t.Fatalf("argumentos de FreeRDP: %s", b)
	}

	mine, _ := s.issueAgentToken(7, "s1")
	other, _ := s.issueAgentToken(7, "s2")
	agent := func(tok string, a map[string]any, folder string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]any{"proyecto": 7, "carpeta": folder, "desktop": a})
		return sqlCall(t, h, "POST", "/api/pgo/escritorio", string(b), "", "", tok)
	}
	f, _ := os.Create(agentdesktoptest.Screenshot(fakes))
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, 800, 450)))
	f.Close()
	if w := agent(other, map[string]any{"action": "capture"}, dir); w.Code != http.StatusConflict {
		t.Errorf("otra sesión: %d %s", w.Code, w.Body)
	}
	if w := agent(mine, map[string]any{"action": "capture", "scale": 0.5}, t.TempDir()); w.Code != http.StatusForbidden {
		t.Errorf("carpeta ajena: %d %s", w.Code, w.Body)
	}
	w = agent(mine, map[string]any{"action": "capture", "scale": 0.5}, dir)
	var res agentdesktop.Result
	json.Unmarshal(w.Body.Bytes(), &res)
	if w.Code != 200 || filepath.Dir(res.Path) != filepath.Join(dir, ".pgocode", "salidas") {
		t.Fatalf("captura: %d %s", w.Code, w.Body)
	}
	if w := agent(mine, map[string]any{"action": "click", "x": 400, "y": 225}, dir); w.Code != 200 {
		t.Fatalf("clic: %d %s", w.Code, w.Body)
	}
	if x := agentdesktoptest.Args(fakes, "xdotool"); x[len(x)-1] != "mousemove --sync 800 450 click 1" {
		t.Errorf("clic escalado: %v", x)
	}
	if w := agent(mine, map[string]any{"action": "click", "x": 900, "y": 1}, dir); w.Code != http.StatusBadRequest {
		t.Errorf("fuera de la captura: %d", w.Code)
	}
	if w := agent(mine, map[string]any{"action": "key", "keys": []string{"--help"}}, dir); w.Code != http.StatusBadRequest {
		t.Errorf("tecla no válida: %d", w.Code)
	}
	if w := agent(mine, map[string]any{"action": "type", "text": pw}, dir); w.Code != 200 {
		t.Errorf("escribe: %d %s", w.Code, w.Body)
	}
	if w := agent(mine, nil, dir); w.Code != http.StatusBadRequest {
		t.Errorf("sin orden: %d", w.Code)
	}

	if w := ui("POST", "/api/agent-desktop/stop", `{"project":7,"session":"s1"}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"running":false`) {
		t.Errorf("parar: %d %s", w.Code, w.Body)
	}
	if w := agent(mine, map[string]any{"action": "status"}, dir); w.Code != http.StatusConflict {
		t.Errorf("parado: %d", w.Code)
	}
	audit, _ := os.ReadFile(s.AgentDesktopAudit)
	if strings.Contains(string(audit), pw) || !strings.Contains(string(audit), `"characters":15`) || !strings.Contains(string(audit), `"action":"stop"`) {
		t.Errorf("auditoría: %s", audit)
	}
}

// Closing pgocode stops the agents' desktops.
func TestAgentDesktopClosedOnShutdown(t *testing.T) {
	s, _, _ := servidorAgente(t)
	fakes := t.TempDir()
	tools := agentdesktoptest.Tools(t, fakes, "")
	s.agentDesktopOptions = func() (agentdesktop.Options, error) {
		return agentdesktop.Options{Tools: tools, RuntimeDir: filepath.Join(t.TempDir(), "run"), Exchange: fakes}, nil
	}
	ses, _ := s.Sesiones.Crea(2, "Prueba", "prueba", "sesion-odoo")
	ses.MarcaProyecto(7, time.Now().Add(time.Hour))
	w := sqlCall(t, s.Handler(), "POST", "/api/agent-desktop/start", `{"project":7,"session":"s1","host":"h","user":"u","password":"p"}`, s.cookieSesion(), ses.Token, "")
	if w.Code != 200 || s.agentDesktopsRunning() != 1 || s.TrabajoEnCurso() == "" {
		t.Fatalf("iniciar: %d %s", w.Code, w.Body)
	}
	s.CloseConnections()
	if s.agentDesktopsRunning() != 0 {
		t.Error("sigue abierto")
	}
}
