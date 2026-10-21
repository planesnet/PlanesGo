package servidor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/repo"
	"planesgo-code/internal/sesiones"
)

// quitar-copia solo quita copias de los repositorios del proyecto y sin trabajo sin guardar.
func TestQuitarCopia(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip()
	}
	raiz := t.TempDir()
	run := func(dir string, args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	// Dos proyectos, cada uno con su repositorio y una copia de trabajo.
	copia := map[int]string{}
	for _, id := range []int{1, 2} {
		r := filepath.Join(raiz, "p"+string(rune('0'+id)))
		os.Mkdir(r, 0o755)
		os.WriteFile(filepath.Join(r, ".planesgo.json"), []byte(`{"odoo_project_id": `+string(rune('0'+id))+`}`), 0o644)
		run(r, "init", "-q", "-b", "main")
		run(r, "add", ".")
		run(r, "commit", "-qm", "inicio")
		copia[id] = filepath.Join(raiz, "copias", "p"+string(rune('0'+id))+"-dev")
		run(r, "worktree", "add", "-q", "-b", "dev", copia[id])
	}
	s := &Servidor{Puerto: "7801", PermitirNavegador: true, Sesiones: sesiones.NuevoAlmacen(), Indice: proyectos.NuevoIndice([]string{raiz}),
		RegistroCopias: &repo.Registro{Fichero: filepath.Join(raiz, "copias.json")}}
	h := s.Handler()
	ses, _ := s.Sesiones.Crea(1, "t", "t", "x")
	ses.MarcaProyecto(1, time.Now().Add(time.Hour))
	ses.MarcaProyecto(2, time.Now().Add(time.Hour))
	pide := func(metodo, ruta, cuerpo string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
		r.Host = "127.0.0.1:7801"
		r.Header.Set("X-PGO", "1")
		r.AddCookie(&http.Cookie{Name: cookie, Value: ses.Token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	quita := func(proyecto int, carpeta string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]any{"proyecto": proyecto, "carpeta": carpeta, "borrar_rama": true})
		return pide("POST", "/api/proyecto/quitar-copia", string(b))
	}

	if w := quita(1, copia[2]); w.Code != http.StatusForbidden || !existe(copia[2]) {
		t.Fatalf("copia de otro proyecto: %d %s", w.Code, w.Body)
	}
	if w := quita(1, filepath.Join(raiz, "p1")); w.Code == http.StatusOK || !existe(filepath.Join(raiz, "p1", ".git")) {
		t.Fatalf("carpeta principal: %d %s", w.Code, w.Body)
	}
	os.WriteFile(filepath.Join(copia[1], "a.txt"), []byte("x"), 0o644)
	if w := quita(1, copia[1]); w.Code != http.StatusConflict || !existe(copia[1]) {
		t.Fatalf("copia con cambios: %d %s", w.Code, w.Body)
	}
	w := pide("GET", "/api/proyecto/copias?proyecto=1", "")
	var l struct{ Copias []copiaVista }
	if json.Unmarshal(w.Body.Bytes(), &l); len(l.Copias) != 1 || l.Copias[0].Quitable || l.Copias[0].Origen != "usuario" {
		t.Fatalf("lista: %s", w.Body)
	}
	os.Remove(filepath.Join(copia[1], "a.txt"))
	if w := quita(1, copia[1]); w.Code != http.StatusOK || existe(copia[1]) {
		t.Fatalf("copia limpia: %d %s", w.Code, w.Body)
	}
}

func existe(p string) bool { _, err := os.Stat(p); return err == nil }
