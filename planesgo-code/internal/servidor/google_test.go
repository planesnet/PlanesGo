package servidor

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"planesgo-code/internal/gworkspace"
	"planesgo-code/internal/sesiones"
)

// El agente busca y lee en Drive con el token de su proyecto («pgo drive …»); sin token, nada. Solo GET.
func TestPgoDrive(t *testing.T) {
	g := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/tokeninfo":
			json.NewEncoder(w).Encode(map[string]string{"scope": gworkspace.ScopeDriveReadonly, "email": "luis@planesnet.com", "expires_in": "3600"})
		case r.URL.Path == "/drive/v3/files" && r.Header.Get("Authorization") == "Bearer tok":
			w.Write([]byte(`{"files":[{"id":"DOC1234567890","name":"Informe","mimeType":"application/vnd.google-apps.document","modifiedTime":"2026-10-01T00:00:00Z"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer g.Close()
	s := &Servidor{Puerto: "7801", TokenEscritorio: "electron", TokenAgente: "agente", Sesiones: sesiones.NuevoAlmacen(),
		Google: &gworkspace.Manager{Endpoints: gworkspace.Endpoints{TokenInfo: g.URL + "/tokeninfo", Drive: g.URL + "/drive/v3", Sheets: g.URL},
			HTTP: g.Client(), Fetch: func(context.Context, string) (string, error) { return "tok", nil }}}
	h := s.Handler()
	pide := func(metodo, ruta, token string) (int, string) {
		r := httptest.NewRequest(metodo, "http://127.0.0.1:7801"+ruta, nil)
		r.Header.Set("X-PGO", "1")
		if token != "" {
			r.Header.Set("X-PGO-Agente", token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	if c, _ := pide("GET", "/api/pgo/drive-buscar?q=informe&proyecto=7", ""); c != http.StatusForbidden {
		t.Fatalf("sin token: %d", c)
	}
	if c, _ := pide("GET", "/api/pgo/drive-buscar?q=informe&proyecto=7", s.tokenProyecto(8)); c != http.StatusForbidden {
		t.Fatalf("token de otro proyecto: %d", c)
	}
	c, b := pide("GET", "/api/pgo/drive-buscar?q=informe&proyecto=7", s.tokenProyecto(7))
	if c != http.StatusOK || !strings.Contains(b, `[doc] Informe · 2026-10-01 · DOC1234567890`) {
		t.Fatalf("%d %s", c, b)
	}
	if c, b := pide("GET", "/api/pgo/drive-leer?ref=NOEXISTE12345&proyecto=7", s.tokenProyecto(7)); c != http.StatusNotFound || !strings.Contains(b, "no existe") {
		t.Fatalf("%d %s", c, b)
	}
	if c, _ := pide("GET", "/api/pgo/drive-borrar?proyecto=7", s.tokenProyecto(7)); c != http.StatusNotFound {
		t.Fatalf("orden desconocida: %d", c)
	}
	// La interfaz sin sesión: nada.
	if c, _ := pide("GET", "/api/google", ""); c == http.StatusOK {
		t.Fatal("estado sin sesión")
	}
}

// El agente busca y lee en Gmail con el token de su proyecto («pgo gmail …»); solo GET; el registro dice cuántos
// correos y cuánto ha tardado, nunca la consulta ni el contenido.
func TestPgoGmail(t *testing.T) {
	var mu sync.Mutex
	scope := gworkspace.ScopeDriveReadonly + " " + gworkspace.ScopeGmailReadonly
	g := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.URL.Path != "/tokeninfo" {
			t.Errorf("petición que no es de lectura: %s %s", r.Method, r.URL.Path)
		}
		switch {
		case r.URL.Path == "/tokeninfo":
			mu.Lock()
			sc := scope
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]string{"scope": sc, "email": "luis@planesnet.com", "expires_in": "3600"})
		case r.URL.Path == "/gmail/v1/users/me/messages" && r.URL.Query().Get("q") == "from:ana":
			w.Write([]byte(`{"messages":[{"id":"18f0000000000001","threadId":"18f0000000000001"}]}`))
		case r.URL.Path == "/gmail/v1/users/me/messages/18f0000000000001":
			w.Write([]byte(`{"id":"18f0000000000001","threadId":"18f0000000000001","snippet":"secreto del correo","internalDate":"1760961780000",` +
				`"payload":{"mimeType":"text/plain","headers":[{"name":"From","value":"Ana <ana@x.es>"},{"name":"Subject","value":"Asunto confidencial"}],"body":{"data":"Q3VlcnBvIGNvbmZpZGVuY2lhbA=="}}}`))
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"error":{"code":404,"message":"Not found"}}`))
		}
	}))
	defer g.Close()
	s := &Servidor{Puerto: "7801", TokenEscritorio: "electron", TokenAgente: "agente", Sesiones: sesiones.NuevoAlmacen(),
		Google: &gworkspace.Manager{Endpoints: gworkspace.Endpoints{TokenInfo: g.URL + "/tokeninfo", Gmail: g.URL + "/gmail/v1"},
			HTTP: g.Client(), Fetch: func(context.Context, string) (string, error) { return "tok", nil }}}
	h := s.Handler()
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	pide := func(ruta, token string) (int, string) {
		r := httptest.NewRequest("GET", "http://127.0.0.1:7801"+ruta, nil)
		r.Header.Set("X-PGO", "1")
		if token != "" {
			r.Header.Set("X-PGO-Agente", token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	if c, _ := pide("/api/pgo/gmail-buscar?q=from%3Aana&proyecto=7", ""); c != http.StatusForbidden {
		t.Fatalf("sin token: %d", c)
	}
	c, b := pide("/api/pgo/gmail-buscar?q=from%3Aana&proyecto=7", s.tokenProyecto(7))
	if c != http.StatusOK || !strings.Contains(b, " · Asunto confidencial · 18f0000000000001") {
		t.Fatalf("%d %s", c, b)
	}
	c, b = pide("/api/pgo/gmail-leer?ref=18f0000000000001&mensaje=1&proyecto=7", s.tokenProyecto(7))
	if c != http.StatusOK || !strings.Contains(b, "Cuerpo confidencial") {
		t.Fatalf("%d %s", c, b)
	}
	if c, b := pide("/api/pgo/gmail-leer?ref=18f0000000000999&proyecto=7", s.tokenProyecto(7)); c != http.StatusNotFound || !strings.Contains(b, "no existe ese correo") {
		t.Fatalf("%d %s", c, b)
	}
	for _, o := range []string{"enviar", "borrar", "marcar"} {
		if c, _ := pide("/api/pgo/gmail-"+o+"?proyecto=7", s.tokenProyecto(7)); c != http.StatusNotFound {
			t.Fatalf("%s: %d", o, c)
		}
	}
	l := logs.String()
	if !strings.Contains(l, "gmail: buscar · 1 correos") || !strings.Contains(l, "gmail: leer · 1 correos") {
		t.Errorf("registro: %s", l)
	}
	for _, secret := range []string{"ana", "confidencial", "secreto", "18f0000000000001", "from"} {
		if strings.Contains(strings.ToLower(l), secret) {
			t.Errorf("el registro lleva %q: %s", secret, l)
		}
	}
	// Sin el permiso de Gmail: 503 con qué hacer.
	mu.Lock()
	scope = gworkspace.ScopeDriveReadonly
	mu.Unlock()
	s.Google.Fetch = func(context.Context, string) (string, error) { return "tok2", nil }
	if st := s.Google.Status(context.Background(), "luis", true); !st.Connected || st.Gmail || !st.Drive {
		t.Fatalf("%+v", st)
	}
	if c, b := pide("/api/pgo/gmail-listar?proyecto=7", s.tokenProyecto(7)); c != http.StatusServiceUnavailable || !strings.Contains(b, "Gmail no está autorizado") {
		t.Fatalf("%d %s", c, b)
	}
}

// La sección de Propiedades: cargar el fichero del cliente (sin devolverlo), conectar (la URL de Google) y
// desconectar. Todo con sesión y por usuario.
func TestGoogleOAuthEndpoints(t *testing.T) {
	dir := t.TempDir()
	s := &Servidor{Puerto: "7801", PermitirNavegador: true, Sesiones: sesiones.NuevoAlmacen(),
		Google: &gworkspace.Manager{Endpoints: gworkspace.Endpoints{Auth: "https://accounts.example/auth"}, Dir: dir}}
	h := s.Handler()
	ses, _ := s.Sesiones.Crea(1, "Luis", "luis@planesnet.com", "x")
	pide := func(metodo, ruta, cuerpo string, conSesion bool) (int, string) {
		r := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
		r.Host = "127.0.0.1:7801"
		r.Header.Set("X-PGO", "1")
		if conSesion {
			r.AddCookie(&http.Cookie{Name: cookie, Value: ses.Token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	const fichero = `{"installed":{"client_id":"1-a.apps.googleusercontent.com","client_secret":"GOCSPX-xyz","project_id":"planes-dev"}}`
	cuerpo, _ := json.Marshal(map[string]string{"contenido": fichero})
	if c, _ := pide("POST", "/api/google/cliente", string(cuerpo), false); c != http.StatusUnauthorized {
		t.Fatalf("sin sesión: %d", c)
	}
	if c, b := pide("POST", "/api/google/conectar", "{}", true); c != http.StatusConflict || !strings.Contains(b, "falta registrar") {
		t.Fatalf("sin cliente: %d %s", c, b)
	}
	if c, b := pide("POST", "/api/google/cliente", `{"contenido":"{\"web\":{}}"}`, true); c != http.StatusBadRequest || !strings.Contains(b, "escritorio") {
		t.Fatalf("cliente web: %d %s", c, b)
	}
	if c, b := pide("POST", "/api/google/cliente", string(cuerpo), true); c != http.StatusOK || strings.Contains(b, "GOCSPX") || !strings.Contains(b, "planes-dev") {
		t.Fatalf("%d %s", c, b)
	}
	c, b := pide("GET", "/api/google", "", true)
	if c != http.StatusOK || !strings.Contains(b, `"cliente":true`) || strings.Contains(b, "GOCSPX") || strings.Contains(b, `"conectado":true`) {
		t.Fatalf("%d %s", c, b)
	}
	c, b = pide("POST", "/api/google/conectar", "{}", true)
	var r struct{ URL string }
	json.Unmarshal([]byte(b), &r)
	if c != http.StatusOK || !strings.HasPrefix(r.URL, "https://accounts.example/auth?") || !strings.Contains(r.URL, "login_hint=luis%40planesnet.com") || strings.Contains(r.URL, "GOCSPX") {
		t.Fatalf("%d %s", c, b)
	}
	if c, b := pide("GET", "/api/google", "", true); !strings.Contains(b, `"en_curso":true`) {
		t.Fatalf("%d %s", c, b)
	}
	if c, b := pide("POST", "/api/google/desconectar", "{}", true); c != http.StatusOK || strings.Contains(b, "aviso") {
		t.Fatalf("%d %s", c, b)
	}
	if c, b := pide("GET", "/api/google", "", true); strings.Contains(b, `"en_curso":true`) {
		t.Fatalf("tras desconectar: %d %s", c, b)
	}
}
