package servidor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"planesgo-code/internal/procesos"
	"planesgo-code/internal/sesiones"
)

// El agente usa /api/pgo/* con su token (sin la cookie de la app); sin token, nada. La URL del programa llega
// a la interfaz como aviso «abrir».
func TestPgoAgente(t *testing.T) {
	s := &Servidor{Puerto: "7801", TokenEscritorio: "electron", TokenAgente: "agente", Sesiones: sesiones.NuevoAlmacen(), Procesos: procesos.Nuevo()}
	s.AvisaProcesos()
	h := s.Handler()
	avisos := s.Eventos.alta()
	pide := func(token, cuerpo string) int {
		r := httptest.NewRequest("POST", "http://127.0.0.1:7801/api/pgo/ejecutar", strings.NewReader(cuerpo))
		r.Header.Set("X-PGO", "1")
		if token != "" {
			r.Header.Set("X-PGO-Agente", token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	dir := t.TempDir()
	cuerpo := `{"proyecto":7,"nombre":"web","carpeta":"` + dir + `","orden":"echo en http://localhost:8123/; sleep 30"}`
	if c := pide("", cuerpo); c != http.StatusForbidden {
		t.Fatalf("sin token: %d", c)
	}
	if c := pide("otro", cuerpo); c != http.StatusForbidden {
		t.Fatalf("token malo: %d", c)
	}
	if c := pide("agente", cuerpo); c != http.StatusOK {
		t.Fatalf("con token: %d", c)
	}
	defer s.Procesos.ParaTodos()
	fin := time.After(3 * time.Second)
	for {
		select {
		case b := <-avisos:
			if strings.Contains(string(b), `"abrir"`) {
				if !strings.Contains(string(b), "http://localhost:8123/") {
					t.Fatalf("aviso %s", b)
				}
				return
			}
		case <-fin:
			t.Fatal("no llegó el aviso de abrir")
		}
	}
}
