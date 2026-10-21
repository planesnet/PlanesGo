package servidor

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"planesgo-code/internal/procesos"
	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/sesiones"
)

// El agente usa /api/pgo/* con el token de su proyecto (sin la cookie de la app); sin token, con el de otro
// proyecto o fuera de las carpetas del proyecto, nada. La URL del programa llega a la interfaz como aviso «abrir».
func TestPgoAgente(t *testing.T) {
	raiz := t.TempDir()
	dir := filepath.Join(raiz, "proyecto7")
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, ".planesgo.json"), []byte(`{"odoo_project_id": 7}`), 0o644)
	s := &Servidor{Puerto: "7801", TokenEscritorio: "electron", TokenAgente: "agente", Sesiones: sesiones.NuevoAlmacen(), Procesos: procesos.Nuevo(),
		Indice: proyectos.NuevoIndice([]string{raiz})}
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
	cuerpo := `{"proyecto":7,"nombre":"web","carpeta":"` + filepath.Join(dir, "sub") + `","orden":"echo en http://localhost:8123/; sleep 30"}`
	if c := pide("", cuerpo); c != http.StatusForbidden {
		t.Fatalf("sin token: %d", c)
	}
	if c := pide("otro", cuerpo); c != http.StatusForbidden {
		t.Fatalf("token malo: %d", c)
	}
	if c := pide("agente", cuerpo); c != http.StatusForbidden {
		t.Fatalf("con el secreto en vez del token del proyecto: %d", c)
	}
	if c := pide(s.tokenProyecto(8), cuerpo); c != http.StatusForbidden {
		t.Fatalf("con el token de otro proyecto: %d", c)
	}
	fuera := `{"proyecto":7,"nombre":"web","carpeta":"` + t.TempDir() + `","orden":"true"}`
	if c := pide(s.tokenProyecto(7), fuera); c != http.StatusForbidden {
		t.Fatalf("carpeta fuera del proyecto: %d", c)
	}
	if c := pide(s.tokenProyecto(7), cuerpo); c != http.StatusOK {
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

// Con el usuario usando la app (actividad reciente) no se actualiza sola; pasado el plazo, sí.
func TestOcupadoPorUso(t *testing.T) {
	s := &Servidor{}
	if s.Ocupado() {
		t.Fatal("sin actividad no está ocupado")
	}
	s.actividad.Store(time.Now().UnixNano())
	if !s.Ocupado() {
		t.Fatal("con actividad reciente está ocupado")
	}
	s.actividad.Store(time.Now().Add(-sinUsoAntes - time.Second).UnixNano())
	if s.Ocupado() {
		t.Fatal("tras el plazo ya no")
	}
}
