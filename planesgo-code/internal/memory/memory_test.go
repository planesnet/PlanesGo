package memory

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// Frases reales (las de la BD de OpenCode de Luis) y otras de las que pide el encargo: las que deben ir a la
// memoria y las que no.
var (
	durables = []string{
		"el IVA se redondea con ROUND_HALF_UP",
		"el puerto es 3005",
		"las pruebas: make test",
		"ejecuta odoo con odt -d intradema",
		"Crea una rama de test y otra de desarrollo dev-lp deja por defecto dev-lp",
		"Nunca hagas push a master",
		"usa siempre tabs para indentar",
		"Importante: los importes van en céntimos (enteros), nunca en float",
		"la base de datos de pruebas se llama intradema_test",
		"A partir de ahora los textos de la interfaz van en español",
		"La API escucha en localhost:8069",
		"La rama principal es dev-lp",
	}
	transients = []string{
		"hola",
		"que ramas hay en este repositorio",
		"cancela el odoo",
		"borra los directorios que se han creado",
		"ejecuta directamente el servicio",
		"No funciona el login",
		"Quitar la palabra proyectos inicial que hay a la izquierda e los menús.",
		"eres capaz de ejecutar odoo ?",
		"No, mejor espera a que acabe",
		"ojo que ahora falla el test de facturas",
		"Mejor hazlo en otra función",
		"No me gusta ese color, cámbialo",
		"lanza el servidor con más memoria",
		"siempre falla el test de facturas",
		"la ruta es incorrecta",
		"¿Cuál es el puerto de Odoo?",
		"En la lista de proyectos tiene que haber una acción para un proyecto determinado que sea Verificar proyecto y que debe llamar a una función estándar",
	}
)

func TestFacts(t *testing.T) {
	for _, s := range durables {
		if f := Facts(s); len(f) != 1 {
			t.Errorf("debería ir a la memoria: %q (%v)", s, f)
		}
	}
	for _, s := range transients {
		if f := Facts(s); len(f) != 0 {
			t.Errorf("no debería ir a la memoria: %q (%v)", s, f)
		}
	}
}

func TestFactsMessage(t *testing.T) {
	msg := "Arregla el cálculo de la factura. Ojo: el IVA se redondea con ROUND_HALF_UP. ¿Lo ves?\n```\nnunca = 1\n```\nGracias."
	f := Facts(msg)
	if len(f) != 1 || f[0] != "El IVA se redondea con ROUND_HALF_UP" {
		t.Fatalf("frases: %q", f)
	}
	if f := Facts("Contexto del proyecto: las pruebas se ejecutan siempre con `make prueba-rapida`."); len(f) != 1 || f[0] != "Las pruebas se ejecutan siempre con `make prueba-rapida`" {
		t.Fatalf("sin la etiqueta: %q", f)
	}
	var largo []string
	for range 10 {
		largo = append(largo, "Nunca toques el fichero x.")
	}
	if n := len(Facts(strings.Join(largo, " "))); n != maxFactsPerTurn {
		t.Fatalf("como mucho %d por mensaje: %d", maxFactsPerTurn, n)
	}
}

func TestCommand(t *testing.T) {
	casos := []struct {
		tool, cmd string
		want      string // «» si no vale
	}{
		{"bash", "cd /home/x/proyecto && go test ./... 2>&1 | tail -20", "pruebas: `go test ./...`"},
		{"Bash", "npm test", "pruebas: `npm test`"},
		{"shell", "make build", "compilar: `make build`"},
		{"bash", "timeout 120 go vet ./...", "comprobar: `go vet ./...`"},
		{"bash", "npx tsc --noEmit -p .", "comprobar: `npx tsc --noEmit -p .`"},
		{"bash", "go test ./internal/llm -run TestPrune", ""},
		{"bash", "pytest tests/test_factura.py::test_iva", ""},
		{"bash", "make prueba-rapida 2>&1 | tail -30 && make lint 2>&1 | tail -5", "pruebas: `make prueba-rapida`"},
		{"bash", "ls -la", ""},
		{"read", "go test ./...", ""},
		{"bash", "cat <<EOF > x\nmake\nEOF", ""},
	}
	for _, c := range casos {
		e, ok := Command(c.tool, c.cmd)
		got := ""
		if ok {
			got = e.String()
		}
		if got != c.want {
			t.Errorf("%s %q: %q, quería %q", c.tool, c.cmd, got, c.want)
		}
	}
}

func TestMergeAndDedup(t *testing.T) {
	text := merge(header, []string{"El puerto es 3005", "Nunca hagas push a master"}, []CommandEntry{{"pruebas", "go test ./internal/llm/"}})
	// Lo mismo con otro valor sustituye; lo mismo con otras palabras casi iguales, también; la orden más general gana.
	text = merge(text, []string{"El puerto es 3006", "nunca hagas push a master."}, []CommandEntry{{"pruebas", "go test ./..."}, {"compilar", "make build"}})
	body := Body(text)
	want := "## Decisiones y datos\n- El puerto es 3006\n- Nunca hagas push a master\n\n## Órdenes\n- pruebas: `go test ./...`\n- compilar: `make build`"
	if body != want {
		t.Fatalf("memoria:\n%s\n--- quería:\n%s", body, want)
	}
	// Una orden más concreta no sustituye a la general.
	if b := Body(merge(text, nil, []CommandEntry{{"pruebas", "go test ./internal/x/"}})); b != want {
		t.Fatalf("la orden concreta ha sustituido a la general:\n%s", b)
	}
	if !strings.HasPrefix(text, "# Memoria del proyecto") {
		t.Fatalf("sin cabecera: %q", text)
	}
}

func TestUserSectionsKept(t *testing.T) {
	text := "# Memoria\n\n## Mías\n- El cliente es Intradema\n\n## Decisiones y datos\n- Nunca hagas push a master\n"
	got := Body(merge(text, []string{"El puerto es 3005"}, nil))
	want := "## Mías\n- El cliente es Intradema\n\n## Decisiones y datos\n- Nunca hagas push a master\n- El puerto es 3005"
	if got != want {
		t.Fatalf("memoria:\n%s", got)
	}
}

func TestCap(t *testing.T) {
	text := header
	for i := range 200 {
		text = merge(text, []string{"Nunca uses la tabla número " + strings.Repeat("x", 40) + string(rune('a'+i%26)) + string(rune('a'+i/26))}, nil)
	}
	if n := utf8.RuneCountInString(Body(text)); n > MaxChars {
		t.Fatalf("pasa del tope: %d > %d", n, MaxChars)
	}
	if Tokens(text) > MaxTokens {
		t.Fatalf("pasa de %d tokens: %d", MaxTokens, Tokens(text))
	}
	// Se ha quitado lo más antiguo: queda lo último.
	if !strings.Contains(text, "xxxx"+string(rune('a'+199%26))+string(rune('a'+199/26))) {
		t.Fatal("se ha perdido lo más reciente")
	}
}

func TestFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	os.MkdirAll(repo, 0o755)
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Skipf("sin git: %v %s", err, out)
	}
	sub := filepath.Join(repo, "web")
	os.MkdirAll(sub, 0o755)
	root := Root(sub)
	if root != repo {
		t.Fatalf("raíz: %s, quería %s", root, repo)
	}
	if ForAgent(root) != "" {
		t.Fatal("sin memoria, sin bloque")
	}
	changed, err := Update(root, []string{"El puerto es 3005"}, nil)
	if err != nil || !changed {
		t.Fatalf("Update: %v %v", changed, err)
	}
	if changed, _ := Update(root, []string{"el puerto es 3005"}, nil); changed {
		t.Fatal("lo mismo otra vez no cambia nada")
	}
	b := ForAgent(root)
	if !strings.Contains(b, "- El puerto es 3005") || strings.Contains(b, "<!--") || strings.Contains(b, "# Memoria del proyecto\n") {
		t.Fatalf("bloque: %q", b)
	}
	ex, _ := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if !strings.Contains(string(ex), ".pgocode/") {
		t.Fatal("la memoria no está excluida de git")
	}
	if out, _ := exec.Command("git", "-C", repo, "status", "--porcelain").Output(); strings.Contains(string(out), ".pgocode") {
		t.Fatalf("git la ve: %s", out)
	}
	if err := Save(root, strings.Repeat("- Nunca hagas esto otra vez\n", 300)); err != ErrTooLong {
		t.Fatalf("debería pasar del tope: %v", err)
	}
	if err := Save(root, "## Mía\n- Lo que yo diga\n"); err != nil {
		t.Fatal(err)
	}
	if Body(Load(root)) != "## Mía\n- Lo que yo diga" {
		t.Fatalf("guardada: %q", Load(root))
	}
	if err := Save(root, "  "); err != nil || Load(root) != "" {
		t.Fatalf("vacía: la borra (%v)", err)
	}
}

func TestRootWorktree(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	run := func(args ...string) {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Skipf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q", repo)
	run("-C", repo, "-c", "user.email=a@b", "-c", "user.name=a", "commit", "-q", "--allow-empty", "-m", "x")
	wt := filepath.Join(dir, "rama")
	run("-C", repo, "worktree", "add", "-q", "-b", "rama", wt)
	if r := Root(wt); r != repo {
		t.Fatalf("la copia de trabajo comparte la memoria del repositorio: %s", r)
	}
	if r := Root(dir); r != dir {
		t.Fatalf("fuera de git, la carpeta: %s", r)
	}
}
