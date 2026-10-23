package servidor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"planesgo-code/internal/llm"
	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/sesiones"
)

// «Barrer ahora»: POST /api/chat/vigencia?barrer=1 barre la sesión del agente (solo carpetas del proyecto y
// agentes que dejan podar su historial).
func TestBarrer(t *testing.T) {
	raiz := t.TempDir()
	dir := filepath.Join(raiz, "p1")
	os.Mkdir(dir, 0o755)
	os.WriteFile(filepath.Join(dir, ".planesgo.json"), []byte(`{"odoo_project_id": 1}`), 0o644)
	s := &Servidor{Puerto: "7801", PermitirNavegador: true, Sesiones: sesiones.NuevoAlmacen(), Indice: proyectos.NuevoIndice([]string{raiz})}
	h := s.Handler()
	ses, _ := s.Sesiones.Crea(1, "t", "t", "x")
	ses.MarcaProyecto(1, time.Now().Add(time.Hour))
	var visto struct {
		agente, sesion, carpeta, msg string
		enCurso                      bool
	}
	viejo := barrido
	defer func() { barrido = viejo }()
	barrido = func(_ context.Context, agente, sesion, carpeta, msg string, enCurso bool) (*llm.PruneReport, error) {
		visto.agente, visto.sesion, visto.carpeta, visto.msg, visto.enCurso = agente, sesion, carpeta, msg, enCurso
		return &llm.PruneReport{Removed: 23000}, nil
	}
	pide := func(cuerpo map[string]any, xpgo bool) *httptest.ResponseRecorder {
		b, _ := json.Marshal(cuerpo)
		r := httptest.NewRequest("POST", "/api/chat/vigencia?barrer=1", strings.NewReader(string(b)))
		r.Host = "127.0.0.1:7801"
		if xpgo {
			r.Header.Set("X-PGO", "1")
		}
		r.AddCookie(&http.Cookie{Name: cookie, Value: ses.Token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	ok := map[string]any{"proyecto": 1, "agente": "opencode", "sesion_agente": "ses_abc", "carpeta": dir, "mensaje": "hola", "en_curso": true}
	w := pide(ok, true)
	var rep llm.PruneReport
	if json.Unmarshal(w.Body.Bytes(), &rep); w.Code != 200 || rep.Removed != 23000 || visto.carpeta != dir || !visto.enCurso || visto.msg != "hola" {
		t.Fatalf("barrer: %d %s %+v", w.Code, w.Body, visto)
	}
	if w := pide(ok, false); w.Code != http.StatusForbidden {
		t.Fatalf("sin X-PGO: %d", w.Code)
	}
	otra := map[string]any{"proyecto": 1, "agente": "opencode", "sesion_agente": "ses_abc", "carpeta": raiz}
	if w := pide(otra, true); w.Code != http.StatusForbidden {
		t.Fatalf("carpeta de fuera del proyecto: %d", w.Code)
	}
	agy := map[string]any{"proyecto": 1, "agente": "antigravity", "sesion_agente": "x", "carpeta": dir}
	if w := pide(agy, true); w.Code != http.StatusBadRequest {
		t.Fatalf("Antigravity: %d", w.Code)
	}
}
