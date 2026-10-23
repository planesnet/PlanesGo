package servidor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"planesgo-code/internal/llm"
	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/repo"
	"planesgo-code/internal/sesiones"
)

// bancoReparto: un servidor con un repositorio del proyecto 1 y una función para llamar a su API.
type bancoReparto struct {
	t      *testing.T
	s      *Servidor
	madre  string
	pide   func(ruta string, cuerpo any) (int, []byte)
	git    func(dir string, args ...string) string
	hijas  map[int]map[string]string
	partes *llm.Request
}

func nuevoBanco(t *testing.T) *bancoReparto {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip()
	}
	raiz := t.TempDir()
	t.Setenv("HOME", raiz) // las carpetas del proyecto tienen que estar en la personal
	b := &bancoReparto{t: t, hijas: map[int]map[string]string{}}
	b.git = func(dir string, args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	b.madre = filepath.Join(raiz, "p1")
	os.Mkdir(b.madre, 0o755)
	os.WriteFile(filepath.Join(b.madre, ".planesgo.json"), []byte(`{"odoo_project_id": 1}`), 0o644)
	os.WriteFile(filepath.Join(b.madre, "base.txt"), []byte("base\n"), 0o644)
	b.git(b.madre, "init", "-q", "-b", "main")
	b.git(b.madre, "add", ".")
	b.git(b.madre, "commit", "-qm", "inicio")
	b.s = &Servidor{Puerto: "7801", PermitirNavegador: true, Sesiones: sesiones.NuevoAlmacen(), Indice: proyectos.NuevoIndice([]string{raiz}),
		RegistroCopias: &repo.Registro{Fichero: filepath.Join(raiz, "copias.json")}, DirRamas: filepath.Join(raiz, "ramas"),
		Partes: func(context.Context, string) *llm.Request { return b.partes }}
	h := b.s.Handler()
	ses, _ := b.s.Sesiones.Crea(1, "t", "t", "x")
	ses.MarcaProyecto(1, time.Now().Add(time.Hour))
	ses.MarcaProyecto(2, time.Now().Add(time.Hour))
	b.pide = func(ruta string, cuerpo any) (int, []byte) {
		j, _ := json.Marshal(cuerpo)
		r := httptest.NewRequest("POST", ruta, strings.NewReader(string(j)))
		r.Host = "127.0.0.1:7801"
		r.Header.Set("X-PGO", "1")
		r.AddCookie(&http.Cookie{Name: cookie, Value: ses.Token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code, w.Body.Bytes()
	}
	return b
}

func (b *bancoReparto) prepara(n int) map[string]string {
	b.t.Helper()
	code, body := b.pide("/api/reparto/prepara", map[string]any{"proyecto": 1, "sesion": "mad1", "carpeta": b.madre, "n": n, "hija": "hija" + strconv.Itoa(n)})
	if code != 200 {
		b.t.Fatalf("prepara %d: %d %s", n, code, body)
	}
	var r map[string]string
	json.Unmarshal(body, &r)
	b.hijas[n] = r
	return r
}

func (b *bancoReparto) une(n int) repo.Union {
	b.t.Helper()
	h := b.hijas[n]
	code, body := b.pide("/api/reparto/une", map[string]any{"proyecto": 1, "madre": b.madre, "rama_madre": "main", "hija": h["carpeta"], "rama": h["rama"], "n": n})
	if code != 200 {
		b.t.Fatalf("une %d: %d %s", n, code, body)
	}
	var u repo.Union
	json.Unmarshal(body, &u)
	return u
}

func (b *bancoReparto) trabaja(n int, fichero, texto string, commit bool) {
	b.t.Helper()
	d := b.hijas[n]["carpeta"]
	os.WriteFile(filepath.Join(d, fichero), []byte(texto), 0o644)
	if commit {
		b.git(d, "add", "-A")
		b.git(d, "commit", "-qm", "parte")
	}
}

func TestRepartoPropone(t *testing.T) {
	b := nuevoBanco(t)
	req := func(permisos string) propuestaReparto {
		code, body := b.pide("/api/reparto/propone", map[string]any{"proyecto": 1, "carpeta": b.madre, "texto": "x", "permisos": permisos})
		if code != 200 {
			t.Fatalf("%d %s", code, body)
		}
		var p propuestaReparto
		json.Unmarshal(body, &p)
		return p
	}
	// Sin partes: nada.
	if p := req("editar"); len(p.Partes) != 0 {
		t.Fatalf("%+v", p)
	}
	b.partes = &llm.Request{Intro: "En la app:", Items: []string{"a", "b", "c"}, Outro: "Pruebas al acabar."}
	if p := req("editar"); len(p.Partes) != 3 || p.RamaMadre != "main" || p.MadreSucia || p.Max != 4 || p.EsperaS != 8 || p.Intro != "En la app:" {
		t.Fatalf("%+v", p)
	}
	// Una sesión que no edita no reparte.
	if p := req("lectura"); len(p.Partes) != 0 || !strings.Contains(p.Motivo, "no edita") {
		t.Fatalf("lectura: %+v", p)
	}
	// Con la madre sucia, propone y avisa.
	os.WriteFile(filepath.Join(b.madre, "sucio.txt"), []byte("x"), 0o644)
	if p := req("auto"); len(p.Partes) != 3 || !p.MadreSucia {
		t.Fatalf("sucia: %+v", p)
	}
	// Una carpeta que no es del proyecto o no es un repositorio no reparte.
	if code, body := b.pide("/api/reparto/propone", map[string]any{"proyecto": 1, "carpeta": t.TempDir(), "texto": "x", "permisos": "editar"}); code != 200 || strings.Contains(string(body), `"partes"`) {
		t.Fatalf("%d %s", code, body)
	}
	// PGOCODE_PARALELO=0 lo desactiva.
	t.Setenv("PGOCODE_PARALELO", "0")
	if p := req("editar"); len(p.Partes) != 0 {
		t.Fatalf("desactivado: %+v", p)
	}
}

func TestRepartoPreparaYUne(t *testing.T) {
	b := nuevoBanco(t)
	for n := 1; n <= 3; n++ {
		h := b.prepara(n)
		if !strings.HasPrefix(h["carpeta"], b.s.DirRamas) || h["rama"] != "pgo/mad1-"+strconv.Itoa(n) || h["rama_madre"] != "main" {
			t.Fatalf("%v", h)
		}
		if !b.s.Indice.Pertenece(1, h["carpeta"]) {
			t.Fatal("la copia no es del proyecto")
		}
		if c, ok := b.s.RegistroCopias.Busca(h["carpeta"]); !ok || c.Sesion != "hija"+strconv.Itoa(n) || c.Proyecto != 1 {
			t.Fatalf("registro: %+v", c)
		}
	}
	// Repetir el prepara no duplica.
	if h := b.prepara(2); h["carpeta"] != b.hijas[2]["carpeta"] {
		t.Fatal("otra copia")
	}
	b.trabaja(1, "uno.txt", "1\n", true)
	b.trabaja(2, "dos.txt", "2\n", false) // sin commitear
	b.trabaja(3, "base.txt", "tres\n", true)
	for n := 1; n <= 3; n++ {
		if u := b.une(n); u.Estado != "unida" || !u.Quitada {
			t.Fatalf("%d: %+v", n, u)
		}
	}
	for _, f := range []string{"uno.txt", "dos.txt"} {
		if !existe(filepath.Join(b.madre, f)) {
			t.Errorf("falta %s", f)
		}
	}
	if len(b.s.RegistroCopias.Lista()) != 0 || b.s.Indice.Pertenece(1, b.hijas[1]["carpeta"]) {
		t.Errorf("registro: %v", b.s.RegistroCopias.Lista())
	}
	if l := b.git(b.madre, "branch", "--list", "pgo/*"); l != "" {
		t.Errorf("ramas: %s", l)
	}
	if l := b.git(b.madre, "worktree", "list"); strings.Count(l, "\n") != 0 {
		t.Errorf("copias: %s", l)
	}
}

func TestRepartoConflictoOcupadaYSucia(t *testing.T) {
	b := nuevoBanco(t)
	for n := 1; n <= 3; n++ {
		b.prepara(n)
	}
	b.trabaja(1, "base.txt", "uno\n", true)
	b.trabaja(2, "base.txt", "dos\n", true)
	b.trabaja(3, "tres.txt", "3\n", true)

	// La madre trabajando: pendiente, sin tocar nada.
	salir := b.s.trabajando.entra(b.madre)
	if u := b.une(1); u.Estado != "pendiente" || !strings.Contains(u.Motivo, "trabajando") || existe(filepath.Join(b.madre, "uno.txt")) {
		t.Fatalf("madre ocupada: %+v", u)
	}
	salir()
	// Una hija que sigue trabajando: pendiente.
	salir = b.s.trabajando.entra(b.hijas[1]["carpeta"])
	if u := b.une(1); u.Estado != "pendiente" || !strings.Contains(u.Motivo, "parte sigue") {
		t.Fatalf("hija ocupada: %+v", u)
	}
	salir()
	// La madre con cambios sin guardar: pendiente.
	os.WriteFile(filepath.Join(b.madre, "sucio.txt"), []byte("x"), 0o644)
	if u := b.une(1); u.Estado != "pendiente" {
		t.Fatalf("sucia: %+v", u)
	}
	os.Remove(filepath.Join(b.madre, "sucio.txt"))
	// Ahora sí: la 1 se une, la 2 choca (se conserva), la 3 se une igualmente.
	if u := b.une(1); u.Estado != "unida" {
		t.Fatalf("1: %+v", u)
	}
	u := b.une(2)
	if u.Estado != "conflicto" || len(u.Ficheros) != 1 || u.Ficheros[0] != "base.txt" || u.Quitada {
		t.Fatalf("2: %+v", u)
	}
	if u := b.une(3); u.Estado != "unida" {
		t.Fatalf("3: %+v", u)
	}
	if ok, _ := repo.Limpia(context.Background(), b.madre); !ok {
		t.Error("la madre no queda limpia")
	}
	if !existe(b.hijas[2]["carpeta"]) || b.git(b.madre, "branch", "--list", b.hijas[2]["rama"]) == "" {
		t.Error("la hija en conflicto no se conserva")
	}
	if _, ok := b.s.RegistroCopias.Busca(b.hijas[2]["carpeta"]); !ok {
		t.Error("la hija en conflicto ya no está en el registro")
	}
}

func TestRepartoNoUneLoQueNoEsSuyo(t *testing.T) {
	b := nuevoBanco(t)
	b.prepara(1)
	h := b.hijas[1]
	for _, c := range []map[string]any{
		{"proyecto": 1, "madre": b.madre, "rama_madre": "main", "hija": b.madre, "rama": h["rama"], "n": 1},          // la principal
		{"proyecto": 1, "madre": b.madre, "rama_madre": "main", "hija": h["carpeta"], "rama": "pgo/otra-1", "n": 1},  // rama que no es la suya
		{"proyecto": 1, "madre": t.TempDir(), "rama_madre": "main", "hija": h["carpeta"], "rama": h["rama"], "n": 1}, // madre de fuera
		{"proyecto": 1, "madre": b.madre, "rama_madre": "main", "hija": t.TempDir(), "rama": h["rama"], "n": 1},      // carpeta de fuera
		{"proyecto": 2, "madre": b.madre, "rama_madre": "main", "hija": h["carpeta"], "rama": h["rama"], "n": 1},     // otro proyecto
		{"proyecto": 1, "madre": "relativa", "rama_madre": "main", "hija": h["carpeta"], "rama": h["rama"], "n": 1},  // ruta no absoluta
		{"proyecto": 1, "madre": b.madre, "rama_madre": "main", "hija": h["carpeta"], "rama": "--abort", "n": 1},     // opción
	} {
		if code, body := b.pide("/api/reparto/une", c); code == 200 {
			t.Errorf("%v aceptado: %s", c, body)
		}
	}
	if !existe(h["carpeta"]) {
		t.Fatal("se ha quitado la copia")
	}
}

// Cuatro hijas a la vez llamando a la API: las copias se crean y las uniones se hacen de una en una.
func TestRepartoCuatroHijasALaVez(t *testing.T) {
	b := nuevoBanco(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ramas := map[int]map[string]string{}
	for n := 1; n <= 4; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, body := b.pide("/api/reparto/prepara", map[string]any{"proyecto": 1, "sesion": "mad1", "carpeta": b.madre, "n": n, "hija": "h" + strconv.Itoa(n)})
			if code != 200 {
				t.Errorf("prepara %d: %d %s", n, code, body)
				return
			}
			var r map[string]string
			json.Unmarshal(body, &r)
			mu.Lock()
			ramas[n] = r
			mu.Unlock()
		}()
	}
	wg.Wait()
	if t.Failed() {
		return
	}
	b.hijas = ramas
	for n := 1; n <= 4; n++ {
		b.trabaja(n, "f"+strconv.Itoa(n)+".txt", "x\n", n%2 == 0)
	}
	for n := 1; n <= 4; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, body := b.pide("/api/reparto/une", map[string]any{"proyecto": 1, "madre": b.madre, "rama_madre": "main", "hija": ramas[n]["carpeta"], "rama": ramas[n]["rama"], "n": n})
			var u repo.Union
			json.Unmarshal(body, &u)
			if code != 200 || u.Estado != "unida" || !u.Quitada {
				t.Errorf("une %d: %d %s", n, code, body)
			}
		}()
	}
	wg.Wait()
	for n := 1; n <= 4; n++ {
		if !existe(filepath.Join(b.madre, "f"+strconv.Itoa(n)+".txt")) {
			t.Errorf("falta f%d", n)
		}
	}
	if ok, _ := repo.Limpia(context.Background(), b.madre); !ok {
		t.Error("madre sucia")
	}
}

// Una hija parada: lo que dejó se guarda en su rama y se une igual si se pide; si no, se conserva.
func TestRepartoHijaParada(t *testing.T) {
	b := nuevoBanco(t)
	b.prepara(1)
	b.trabaja(1, "a medias.txt", "medio\n", false)
	// Sin pedir la unión nada cambia: la copia y su trabajo siguen ahí.
	if !existe(filepath.Join(b.hijas[1]["carpeta"], "a medias.txt")) || existe(filepath.Join(b.madre, "a medias.txt")) {
		t.Fatal("no debería haberse movido")
	}
	if u := b.une(1); u.Estado != "unida" || !u.Commit || !existe(filepath.Join(b.madre, "a medias.txt")) {
		t.Fatalf("%+v", u)
	}
}
