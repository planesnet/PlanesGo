package servidor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A program launched from a session gets a one-use code in its environment (never the token), exchanges it once for
// its token, uses only the programs' orders of its own session, and loses the access when it ends.
func TestProgramToken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("programa de prueba con sh")
	}
	s, _, dir := servidorAgente(t)
	ses, _ := s.Sesiones.Crea(2, "Prueba", "prueba", "sesion-odoo")
	ses.MarcaProyecto(7, time.Now().Add(time.Hour))
	h := s.Handler()
	envFile := filepath.Join(dir, "entorno.txt")
	body, _ := json.Marshal(map[string]any{"proyecto": 7, "carpeta": dir, "nombre": "pgosage", "sesion": "s1",
		"orden": "env > " + envFile + "; sleep 1"})
	if w := sqlCall(t, h, "POST", "/api/procesos/lanzar", string(body), s.cookieSesion(), ses.Token, ""); w.Code != 200 {
		t.Fatalf("lanzar: %d %s", w.Code, w.Body)
	}
	var code string
	for i := 0; i < 50 && code == ""; i++ {
		time.Sleep(20 * time.Millisecond)
		b, _ := os.ReadFile(envFile)
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(l, EnvProgramCode+"="); ok {
				code = v
			}
			if strings.HasPrefix(l, "PGO_TOKEN=") {
				t.Error("a program gets no agent token")
			}
		}
	}
	if code == "" {
		t.Fatal("no code in the program's environment")
	}
	call := func(method, orden, cred, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:7801/api/pgo/"+orden, strings.NewReader(body))
		r.Header.Set("X-PGO", "1")
		r.Header.Set("X-PGO-Programa", cred)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := call("POST", "programa-token", code, "")
	var tok struct{ Token string }
	json.Unmarshal(w.Body.Bytes(), &tok)
	if w.Code != 200 || tok.Token == "" || tok.Token == code {
		t.Fatalf("exchange: %d %s", w.Code, w.Body)
	}
	if w := call("POST", "programa-token", code, ""); w.Code != http.StatusForbidden {
		t.Error("a code works twice")
	}
	if w := call("GET", "sql-conexiones", tok.Token, ""); w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("connections: %d %s", w.Code, w.Body)
	}
	if w := call("POST", "sql", tok.Token, `{"sql":"SELECT 1"}`); w.Code != http.StatusConflict {
		t.Errorf("sql without a connection: %d %s", w.Code, w.Body)
	}
	for _, o := range []string{"ejecutar", "abrir", "drive-leer"} {
		if w := call("POST", o, tok.Token, `{}`); w.Code != http.StatusForbidden {
			t.Errorf("%s: %d", o, w.Code)
		}
	}
	// An agent's process cannot use it, even with the token.
	s.esProcesoAgente = func(*http.Request) bool { return true }
	if w := call("GET", "sql-conexiones", tok.Token, ""); w.Code != http.StatusForbidden {
		t.Errorf("from an agent's process: %d", w.Code)
	}
	s.esProcesoAgente = func(*http.Request) bool { return false }
	// When the program ends, the token is gone.
	deadline := time.Now().Add(5 * time.Second)
	for call("GET", "sql-conexiones", tok.Token, "").Code == 200 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if w := call("GET", "sql-conexiones", tok.Token, ""); w.Code != http.StatusForbidden {
		t.Errorf("after the program ended: %d", w.Code)
	}
}

// A program launched without a session gets nothing of one.
func TestProgramWithoutSession(t *testing.T) {
	s := &Servidor{Puerto: "7801"}
	env, watch := s.programEnv(programGrant{project: 7, program: "x"})
	watch(nil)
	for _, e := range env {
		if strings.HasPrefix(e, EnvProgramCode) {
			t.Errorf("code without a session: %v", env)
		}
	}
}
