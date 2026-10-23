package servidor

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"planesgo-code/internal/odoo"
	"planesgo-code/internal/procesos"
	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/sesiones"
)

// tokenProyecto: un token de agente de un turno del proyecto (para las pruebas de las órdenes «pgo»).
func (s *Servidor) tokenProyecto(proyecto int) string {
	t, _ := s.issueAgentToken(proyecto, fmt.Sprintf("prueba-%d", proyecto))
	return t
}

// odooContador: un Odoo que cuenta cada petición que le llega (no debe llegarle ninguna de un agente).
func odooContador(t *testing.T) (*odoo.Cliente, *atomic.Int64) {
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":[]}`)
	}))
	t.Cleanup(srv.Close)
	return odoo.Nuevo(srv.URL, "prueba"), &n
}

func servidorAgente(t *testing.T) (*Servidor, *atomic.Int64, string) {
	raiz := t.TempDir()
	dir := filepath.Join(raiz, "proyecto7")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, ".planesgo.json"), []byte(`{"odoo_project_id": 7}`), 0o644)
	cli, n := odooContador(t)
	s := &Servidor{Puerto: "7801", PermitirNavegador: true, TokenAgente: "agente", Sesiones: sesiones.NuevoAlmacen(),
		Procesos: procesos.Nuevo(), Indice: proyectos.NuevoIndice([]string{raiz}), Odoo: cli}
	s.AvisaProcesos()
	t.Cleanup(s.Procesos.ParaTodos)
	return s, n, dir
}

// Con el token de agente (aun con la cookie de una sesión válida), ninguna ruta fuera de /api/pgo: ni la interfaz
// ni lo que habla con Odoo. Y a Odoo no le llega nada.
func TestAgentTokenOnlyPgo(t *testing.T) {
	s, n, _ := servidorAgente(t)
	ses, _ := s.Sesiones.Crea(2, "Prueba", "prueba", "sesion-odoo")
	h := s.Handler()
	tok := s.tokenProyecto(7)
	for _, x := range rutasAPI(t) {
		if esRutaAgente(x.patron) {
			continue
		}
		r := httptest.NewRequest(x.metodo, "http://127.0.0.1:7801"+concreta(x.patron), strings.NewReader("{}"))
		r.Header.Set("X-PGO", "1")
		r.Header.Set("X-PGO-Agente", tok)
		r.AddCookie(&http.Cookie{Name: s.cookieSesion(), Value: ses.Token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s con token de agente: %d (%s)", x.metodo, x.patron, w.Code, w.Body.String())
		}
	}
	if n.Load() != 0 {
		t.Errorf("Odoo ha recibido %d peticiones de un agente", n.Load())
	}
}

// Lista blanca: solo las órdenes de agentEndpoints con su método; cualquier otra (las de Odoo que pudiera haber
// mañana, una inventada) no existe para el agente (404) y con otro método, 403. Ninguna de las permitidas llega a Odoo.
func TestAgentEndpointsWhitelist(t *testing.T) {
	s, n, dir := servidorAgente(t)
	h := s.Handler()
	tok := s.tokenProyecto(7)
	pide := func(metodo, orden, cuerpo string) (int, string) {
		u := "http://127.0.0.1:7801/api/pgo/" + orden
		var b io.Reader
		if metodo == http.MethodGet {
			u += "?proyecto=7&nombre=web&q=x&ref=x"
		} else {
			b = strings.NewReader(cuerpo)
		}
		r := httptest.NewRequest(metodo, u, b)
		r.Header.Set("X-PGO", "1")
		r.Header.Set("X-PGO-Agente", tok)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	for _, orden := range []string{"tiempo", "imputar", "imputacion", "proyecto", "vincular", "tareas", "odoo", "login", "unificar", "x"} {
		for _, m := range []string{http.MethodGet, http.MethodPost} {
			if c, b := pide(m, orden, `{"proyecto":7}`); c != http.StatusNotFound || !strings.Contains(b, "desconocida") {
				t.Errorf("%s %s: %d %s", m, orden, c, b)
			}
		}
	}
	// Método cambiado: tampoco.
	if c, _ := pide(http.MethodGet, "ejecutar", ""); c != http.StatusForbidden {
		t.Errorf("GET ejecutar: %d", c)
	}
	// Las permitidas responden (lo que toque) sin llegar a Odoo.
	for orden, m := range agentEndpoints {
		c, b := pide(m, orden, `{"proyecto":7,"nombre":"web","carpeta":"`+dir+`","orden":"true","url":"http://localhost:1/"}`)
		if c == http.StatusForbidden {
			t.Errorf("%s %s (permitida): %d %s", m, orden, c, b)
		}
	}
	if n.Load() != 0 {
		t.Errorf("Odoo ha recibido %d peticiones de las órdenes del agente", n.Load())
	}
}

// El token es de un turno: el de otro proyecto no vale, revocado no vale, caducado no vale.
func TestAgentTokenLifecycle(t *testing.T) {
	s, _, _ := servidorAgente(t)
	ahora := time.Now()
	s.agentes.now = func() time.Time { return ahora }
	h := s.Handler()
	pide := func(tok string) int {
		r := httptest.NewRequest("GET", "http://127.0.0.1:7801/api/pgo/procesos?proyecto=7", nil)
		r.Header.Set("X-PGO-Agente", tok)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	tok, revoca := s.issueAgentToken(7, "sesion-a")
	otro, _ := s.issueAgentToken(8, "sesion-b")
	if c := pide(tok); c != http.StatusOK {
		t.Fatalf("token válido: %d", c)
	}
	if c := pide(otro); c != http.StatusForbidden {
		t.Errorf("token de otro proyecto: %d", c)
	}
	if c := pide("agente"); c != http.StatusForbidden {
		t.Errorf("el secreto del servidor no es un token: %d", c)
	}
	revoca()
	if c := pide(tok); c != http.StatusForbidden {
		t.Errorf("token revocado: %d", c)
	}
	tok2, _ := s.issueAgentToken(7, "sesion-a")
	ahora = ahora.Add(agentTokenTTL + time.Second)
	if c := pide(tok2); c != http.StatusForbidden {
		t.Errorf("token caducado: %d", c)
	}
	// Sin TokenAgente (los agentes sin «pgo»), no se emite ninguno.
	s.TokenAgente = ""
	if tok, _ := s.issueAgentToken(7, "x"); tok != "" {
		t.Error("sin TokenAgente no debería haber token")
	}
	if e := s.entornoAgente(7, ""); strings.Contains(strings.Join(e, " "), "PGO_TOKEN") {
		t.Errorf("entorno con token vacío: %v", e)
	}
}

// Un proceso de agente (marcado por este pgocode) no usa la interfaz aunque tenga la cookie: ni /api/yo ni nada
// fuera de /api/pgo.
func TestAgentProcessBlocked(t *testing.T) {
	s, _, _ := servidorAgente(t)
	ses, _ := s.Sesiones.Crea(2, "Prueba", "prueba", "sesion-odoo")
	s.esProcesoAgente = func(*http.Request) bool { return true }
	h := s.Handler()
	r := httptest.NewRequest("GET", "http://127.0.0.1:7801/api/yo", nil)
	r.AddCookie(&http.Cookie{Name: s.cookieSesion(), Value: ses.Token})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("/api/yo desde un agente: %d", w.Code)
	}
	r = httptest.NewRequest("GET", "http://127.0.0.1:7801/api/pgo/procesos?proyecto=7", nil)
	r.Header.Set("X-PGO-Agente", s.tokenProyecto(7))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("/api/pgo desde un agente: %d", w.Code)
	}
}

// De verdad (Linux): un proceso con la marca de agente de este pgocode, o un descendiente suyo aunque se quite el
// entorno (env -i), recibe 403 en /api/yo; uno sin marca, 401 (sin sesión).
func TestAgentProcessBlockedReal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("solo Linux")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("sin curl")
	}
	s, _, _ := servidorAgente(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, puerto, _ := net.SplitHostPort(ln.Addr().String())
	s.Puerto = puerto // antes de servir (lo lee cada petición)
	srv := httptest.NewUnstartedServer(nil)
	srv.Listener.Close()
	srv.Listener = ln
	srv.Config.Handler = s.Handler()
	srv.Config.ConnContext = s.ConnContext
	srv.Start()
	defer srv.Close()
	url := "http://127.0.0.1:" + puerto + "/api/yo"
	codigo := func(marca string, orden string) string {
		cmd := exec.Command("bash", "-c", orden)
		cmd.Env = append(os.Environ(), "URL="+url)
		if marca != "" {
			cmd.Env = append(cmd.Env, "PGO_EJECUCION="+marca)
		}
		out, _ := cmd.Output()
		return strings.TrimSpace(string(out))
	}
	curl := `curl -s -o /dev/null -w '%{http_code}' "$URL"`
	if c := codigo("", curl); c != "401" {
		t.Errorf("sin marca: %s", c)
	}
	if c := codigo(fmt.Sprintf("%d-prueba", os.Getpid()), curl); c != "403" {
		t.Errorf("agente: %s", c)
	}
	if c := codigo(fmt.Sprintf("%d-prueba", os.Getpid()), `env -i /usr/bin/curl -s -o /dev/null -w '%{http_code}' "`+url+`"; true`); c != "403" {
		t.Errorf("agente con env -i: %s", c)
	}
	if c := codigo(fmt.Sprintf("prog-%d-web-1", os.Getpid()), curl); c != "403" {
		t.Errorf("programa lanzado: %s", c)
	}
	// La marca de otro pgocode (p. ej. uno de desarrollo lanzado por un agente) no cuenta.
	if c := codigo("1-otro", curl); c != "401" {
		t.Errorf("marca de otro pgocode: %s", c)
	}
}
