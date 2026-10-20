package servidor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"planesgo-code/internal/sesiones"
)

// C1: solo se atiende a Host y Origin locales en el puerto del arnés (defensa contra DNS rebinding).
func TestHostYOrigen(t *testing.T) {
	h := (&Servidor{Puerto: "7801", PermitirNavegador: true, Sesiones: sesiones.NuevoAlmacen()}).Handler()
	casos := []struct {
		nombre, metodo, ruta, host, origen string
		psf                                bool
		quiere                             int
	}{
		{"local 127.0.0.1", "GET", "/healthz", "127.0.0.1:7801", "", false, 200},
		{"local localhost", "GET", "/healthz", "localhost:7801", "", false, 200},
		{"local ::1", "GET", "/healthz", "[::1]:7801", "", false, 200},
		{"rebinding", "GET", "/healthz", "evil.example:7801", "", false, 421},
		{"otro puerto", "GET", "/healthz", "127.0.0.1:9999", "", false, 421},
		{"sin puerto", "GET", "/healthz", "127.0.0.1", "", false, 421},
		{"origen ajeno", "POST", "/api/login", "127.0.0.1:7801", "http://evil.example", true, 403},
		{"origen engañoso", "POST", "/api/login", "127.0.0.1:7801", "http://127.0.0.1:7801.evil.example", true, 403},
		{"sin X-PGO", "POST", "/api/login", "127.0.0.1:7801", "http://127.0.0.1:7801", false, 403},
		{"origen propio", "POST", "/api/login", "127.0.0.1:7801", "http://127.0.0.1:7801", true, 400},
	}
	for _, c := range casos {
		r := httptest.NewRequest(c.metodo, c.ruta, strings.NewReader("{}"))
		r.Host = c.host
		if c.origen != "" {
			r.Header.Set("Origin", c.origen)
		}
		if c.psf {
			r.Header.Set("X-PGO", "1")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.quiere {
			t.Errorf("%s: estado %d, quiero %d (%s)", c.nombre, w.Code, c.quiere, w.Body.String())
		}
	}
}

// Sin -navegador, solo la app de escritorio (con su cookie secreta) puede usar Planes Go Code.
func TestSoloEscritorio(t *testing.T) {
	h := (&Servidor{Puerto: "7801", TokenEscritorio: "secreto-de-electron", Sesiones: sesiones.NuevoAlmacen()}).Handler()
	pide := func(ruta, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", ruta, nil)
		r.Host = "127.0.0.1:7801"
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: cookieEscritorio, Value: cookie})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := pide("/api/yo", ""); w.Code != 403 {
		t.Errorf("navegador sin cookie en la API: %d", w.Code)
	}
	if w := pide("/", ""); w.Code != 403 || !strings.Contains(w.Body.String(), "aplicación de escritorio") {
		t.Errorf("navegador sin cookie en la página: %d %q", w.Code, w.Body.String())
	}
	if w := pide("/api/yo", "otro"); w.Code != 403 {
		t.Errorf("cookie equivocada: %d", w.Code)
	}
	if w := pide("/api/yo", "secreto-de-electron"); w.Code != 401 { // pasa el filtro; falta iniciar sesión
		t.Errorf("app de escritorio: %d", w.Code)
	}
	if w := pide("/healthz", ""); w.Code != 200 {
		t.Errorf("healthz: %d", w.Code)
	}
	// Sin token ni -navegador no se atiende a nadie.
	h2 := (&Servidor{Puerto: "7801", Sesiones: sesiones.NuevoAlmacen()}).Handler()
	r := httptest.NewRequest("GET", "/api/yo", nil)
	r.Host = "127.0.0.1:7801"
	w := httptest.NewRecorder()
	h2.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Errorf("sin token ni -navegador: %d", w.Code)
	}
}
