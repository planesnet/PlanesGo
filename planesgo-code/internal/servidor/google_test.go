package servidor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
