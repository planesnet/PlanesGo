package servidor

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"planesgo-code/internal/sesiones"
)

// ruta registrada en Handler: método, patrón y si va con conSesion.
type ruta struct {
	metodo, patron string
	conSesion      bool
}

// rutasAPI lee de servidor.go las rutas /api/* que registra Handler. Se leen del código (y no de una lista
// copiada aquí) para que una ruta nueva quede cubierta sin acordarse de añadirla a la prueba.
func rutasAPI(t *testing.T) []ruta {
	b, err := os.ReadFile("servidor.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`mux\.HandleFunc\("(GET|POST|PUT|DELETE|PATCH) (/api/[^"]*)",\s*(s\.conSesion\()?`)
	var l []ruta
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		l = append(l, ruta{m[1], m[2], m[3] != ""})
	}
	if len(l) < 40 {
		t.Fatalf("solo se encuentran %d rutas /api/ en servidor.go: ¿ha cambiado la forma de registrarlas?", len(l))
	}
	return l
}

// concreta cambia los comodines {x} del patrón por un valor.
func concreta(patron string) string {
	return regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(patron, "x")
}

// Toda ruta /api/* exige sesión (401 sin ella o con una cookie que no existe), salvo las públicas a propósito
// (login y versión) y las del agente, que exigen su token (403).
func TestRutasSinSesion(t *testing.T) {
	publicas := map[string]bool{"POST /api/login": true, "GET /api/version": true}
	// Las del canal de Google con Electron: sin sesión de usuario, pero solo con el secreto de la app de escritorio.
	soloElectron := map[string]bool{"GET /api/escritorio/google/trabajo": true, "POST /api/escritorio/google/resultado": true}
	s := &Servidor{Puerto: "7801", PermitirNavegador: true, TokenAgente: "agente", Sesiones: sesiones.NuevoAlmacen()}
	h := s.Handler()
	for _, x := range rutasAPI(t) {
		clave := x.metodo + " " + x.patron
		if publicas[clave] {
			if x.conSesion {
				t.Errorf("%s: se esperaba pública", clave)
			}
			continue
		}
		quiere := http.StatusUnauthorized
		if soloElectron[clave] {
			quiere = http.StatusForbidden
		} else if esRutaAgente(x.patron) {
			quiere = http.StatusForbidden
		} else if !x.conSesion {
			t.Errorf("%s: ruta /api/ sin conSesion (si es pública a propósito, añádela a «publicas»)", clave)
			continue
		}
		for _, c := range []string{"", "token-que-no-existe"} {
			r := httptest.NewRequest(x.metodo, "http://127.0.0.1:7801"+concreta(x.patron), strings.NewReader("{}"))
			r.Header.Set("X-PGO", "1")
			if c != "" {
				r.AddCookie(&http.Cookie{Name: cookie, Value: c})
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != quiere {
				t.Errorf("%s (cookie %q): estado %d, quiero %d (%s)", clave, c, w.Code, quiere, w.Body.String())
			}
		}
	}
}

// Toda escritura (POST…) sin la cabecera X-PGO se rechaza antes de llegar al manejador, aun con sesión válida.
func TestRutasPOSTSinCabecera(t *testing.T) {
	s := &Servidor{Puerto: "7801", PermitirNavegador: true, TokenAgente: "agente", Sesiones: sesiones.NuevoAlmacen()}
	ses, err := s.Sesiones.Crea(2, "Prueba", "prueba", "sesion-odoo")
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	n := 0
	for _, x := range rutasAPI(t) {
		if x.metodo == http.MethodGet {
			continue
		}
		n++
		r := httptest.NewRequest(x.metodo, "http://127.0.0.1:7801"+concreta(x.patron), strings.NewReader("{}"))
		r.AddCookie(&http.Cookie{Name: cookie, Value: ses.Token})
		if esRutaAgente(x.patron) { // fuera de /api/pgo, la cabecera del agente ya se rechaza por sí sola
			r.Header.Set("X-PGO-Agente", "agente")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "X-PGO") {
			t.Errorf("%s %s sin X-PGO: estado %d (%s)", x.metodo, x.patron, w.Code, w.Body.String())
		}
	}
	if n == 0 {
		t.Fatal("no hay rutas POST")
	}
}
