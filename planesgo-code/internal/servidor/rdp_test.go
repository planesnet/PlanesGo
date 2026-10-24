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

	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/sesiones"
)

func TestRdpArgs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := strings.Join(rdpArgs("Sage", "192.168.125.229", `MAZANA\planes`), " ")
	for _, want := range []string{"/v:192.168.125.229", "/u:planes", "/d:MAZANA", "/sec:nla", "/tls:seclevel:2", "/gfx:AVC444",
		"/from-stdin:force", "+clipboard", "/drive:Intercambio,", "/sound:sys:pulse", "/microphone:sys:pulse", "/printer", "/size:85%"} {
		if !strings.Contains(a, want) {
			t.Errorf("falta %s en %s", want, a)
		}
	}
	for _, not := range []string{"+f", "/p:"} {
		if strings.Contains(a, not) {
			t.Errorf("sobra %s en %s", not, a)
		}
	}
	if a = strings.Join(rdpArgs("Sage", "h", "planes"), " "); strings.Contains(a, "/d:") || !strings.Contains(a, "/u:planes") {
		t.Errorf("sin dominio: %s", a)
	}
}

// The project is mandatory and visible to the user; the password only reaches FreeRDP's input, never its arguments.
func TestRdpConnect(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FreeRDP simulado con sh")
	}
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	out := filepath.Join(dir, "salida")
	fake := filepath.Join(dir, "sdl-freerdp")
	os.WriteFile(fake, []byte("#!/bin/sh\nread pw\nread otra || otra=EOF\necho \"$pw|$otra|$*\" > "+out+"\n"), 0o755)
	t.Setenv("PGO_FREERDP", fake)

	s := &Servidor{Puerto: "7801", PermitirNavegador: true, Sesiones: sesiones.NuevoAlmacen(), Indice: proyectos.NuevoIndice(nil)}
	h := s.Handler()
	ses, _ := s.Sesiones.Crea(1, "t", "t", "x")
	ses.MarcaProyecto(590, time.Now().Add(time.Hour))
	pide := func(ruta, cuerpo string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", ruta, strings.NewReader(cuerpo))
		r.Host = "127.0.0.1:7801"
		r.Header.Set("X-PGO", "1")
		r.AddCookie(&http.Cookie{Name: cookie, Value: ses.Token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := pide("/api/rdp/conectar", `{"equipo":"h","usuario":"u","contrasena":"x"}`); w.Code == 200 {
		t.Fatal("sin proyecto no se conecta")
	}
	w := pide("/api/rdp/conectar", `{"proyecto":590,"nombre":"Sage","equipo":"192.168.125.229","usuario":"MAZANA\\luis","contrasena":"S3cr3t!"}`)
	if w.Code != 200 {
		t.Fatalf("conectar: %d %s", w.Code, w.Body)
	}
	var c rdpConn
	json.Unmarshal(w.Body.Bytes(), &c)
	for i := 0; i < 50; i++ {
		if b, _ := os.ReadFile(out); len(b) > 0 {
			got := strings.TrimSpace(string(b))
			f := strings.SplitN(got, "|", 3)
			if len(f) < 3 {
				t.Fatalf("FreeRDP recibió %q", got)
			}
			pw, rest, args := f[0], f[1], f[2]
			// The input ends right after the password (a new credentials prompt reads nothing else)
			if rest != "EOF" {
				t.Fatalf("la entrada sigue tras la contraseña: %q", rest)
			}
			if pw != "S3cr3t!" || strings.Contains(args, "S3cr3t!") || !strings.Contains(args, "/v:192.168.125.229") ||
				!strings.Contains(args, "/u:luis ") || !strings.Contains(args, "/d:MAZANA") {
				t.Fatalf("FreeRDP recibió %q", got)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	for i := 0; i < 50 && s.escritorios.open() > 0; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	s.escritorios.mu.Lock()
	state := s.escritorios.conns[c.ID].State
	s.escritorios.mu.Unlock()
	if s.escritorios.open() != 0 || state != "terminado" {
		t.Fatalf("el escritorio no se da por terminado: %s", state)
	}
}

// When pgocode closes, the remote desktops close with it (nobody would count their time any more), without an error.
func TestRdpClosedOnShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FreeRDP simulado con sh")
	}
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	fake := filepath.Join(dir, "sdl-freerdp")
	os.WriteFile(fake, []byte("#!/bin/sh\nread pw\nexec sleep 60\n"), 0o755)
	t.Setenv("PGO_FREERDP", fake)
	s := &Servidor{Puerto: "7801", PermitirNavegador: true, Sesiones: sesiones.NuevoAlmacen(), Indice: proyectos.NuevoIndice(nil)}
	h := s.Handler()
	ses, _ := s.Sesiones.Crea(1, "t", "t", "x")
	ses.MarcaProyecto(590, time.Now().Add(time.Hour))
	r := httptest.NewRequest("POST", "/api/rdp/conectar", strings.NewReader(`{"proyecto":590,"equipo":"h","usuario":"u","contrasena":"x"}`))
	r.Host = "127.0.0.1:7801"
	r.Header.Set("X-PGO", "1")
	r.AddCookie(&http.Cookie{Name: cookie, Value: ses.Token})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || s.escritorios.open() != 1 {
		t.Fatalf("conectar: %d %s", w.Code, w.Body)
	}
	s.CloseConnections()
	if s.escritorios.open() != 0 {
		t.Fatal("pgocode closing must close the remote desktop")
	}
	s.escritorios.mu.Lock()
	defer s.escritorios.mu.Unlock()
	for _, c := range s.escritorios.conns {
		if c.Error != "" {
			t.Errorf("closed by pgocode is not an error: %q", c.Error)
		}
	}
}
