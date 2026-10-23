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
