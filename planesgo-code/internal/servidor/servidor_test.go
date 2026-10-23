package servidor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"planesgo-code/internal/odoo"
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

// La cookie de sesión lleva el puerto: dos pgocode en el mismo host no se pisan la sesión. La antigua (sin puerto)
// se acepta una vez y pasa a la nueva.
func TestCookieConPuerto(t *testing.T) {
	a := &Servidor{Puerto: "8004", PermitirNavegador: true, Sesiones: sesiones.NuevoAlmacen(), Odoo: &odoo.Cliente{URL: "http://odoo"}}
	b := &Servidor{Puerto: "8005", PermitirNavegador: true, Sesiones: sesiones.NuevoAlmacen()}
	if a.cookieSesion() == b.cookieSesion() {
		t.Fatalf("mismo nombre de cookie en dos puertos: %s", a.cookieSesion())
	}
	ses, err := a.Sesiones.Crea(2, "Prueba", "prueba", "sesion-odoo")
	if err != nil {
		t.Fatal(err)
	}
	h := a.Handler()
	pide := func(c *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://localhost:8004/api/yo", nil)
		r.AddCookie(c)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	// La nueva vale; la de otro puerto (otro nombre) no cuenta.
	if w := pide(&http.Cookie{Name: "pgo_sesion_8005", Value: ses.Token}); w.Code != 401 {
		t.Errorf("cookie de otro puerto: %d", w.Code)
	}
	w := pide(&http.Cookie{Name: cookie, Value: ses.Token})
	if w.Code == 401 {
		t.Fatalf("cookie antigua rechazada: %s", w.Body.String())
	}
	var nueva, borrada bool
	for _, c := range w.Result().Cookies() {
		nueva = nueva || (c.Name == "pgo_sesion_8004" && c.Value == ses.Token)
		borrada = borrada || (c.Name == cookie && c.MaxAge < 0)
	}
	if !nueva || !borrada {
		t.Errorf("la antigua no pasa a la nueva: %v", w.Result().Cookies())
	}
}

// Una carpeta de sesión que ya no existe (copia de trabajo quitada) no se explica como «no es del proyecto».
func TestCarpetaAjena(t *testing.T) {
	if m := carpetaAjena(t.TempDir()+"/quitada", "P"); !strings.Contains(m, "copia de trabajo se quitó") {
		t.Errorf("carpeta quitada: %q", m)
	}
	if m := carpetaAjena(t.TempDir(), "P"); !strings.Contains(m, "no es del proyecto «P»") {
		t.Errorf("carpeta ajena: %q", m)
	}
}
