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
	h := (&Servidor{Puerto: "7801", Sesiones: sesiones.NuevoAlmacen()}).Handler()
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
		{"sin X-PSF", "POST", "/api/login", "127.0.0.1:7801", "http://127.0.0.1:7801", false, 403},
		{"origen propio", "POST", "/api/login", "127.0.0.1:7801", "http://127.0.0.1:7801", true, 400},
	}
	for _, c := range casos {
		r := httptest.NewRequest(c.metodo, c.ruta, strings.NewReader("{}"))
		r.Host = c.host
		if c.origen != "" {
			r.Header.Set("Origin", c.origen)
		}
		if c.psf {
			r.Header.Set("X-PSF", "1")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.quiere {
			t.Errorf("%s: estado %d, quiero %d (%s)", c.nombre, w.Code, c.quiere, w.Body.String())
		}
	}
}

var _ http.Handler = (&Servidor{}).Handler()
