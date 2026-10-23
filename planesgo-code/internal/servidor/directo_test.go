package servidor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func escribe(t *testing.T, dir, nombre, texto string) {
	t.Helper()
	f := filepath.Join(dir, nombre)
	os.MkdirAll(filepath.Dir(f), 0o755)
	if err := os.WriteFile(f, []byte(texto), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectCommand(t *testing.T) {
	casos := []struct {
		nombre string
		fich   map[string]string
		kind   string
		orden  string // «» si no se sabe
	}{
		{"vacía", nil, "build", ""},
		{"vacía test", nil, "test", ""},
		{"makefile build", map[string]string{"Makefile": ".PHONY: build test\nbuild:\n\tgo build\ntest: build\n\tgo test\nVAR := 1\n"}, "build", "make build"},
		{"makefile test", map[string]string{"Makefile": "build:\n\tx\ntest:\n\ty\n"}, "test", "make test"},
		{"makefile check", map[string]string{"Makefile": "check:\n\tx\n"}, "typecheck", "make check"},
		{"makefile sin lint", map[string]string{"Makefile": "build:\n\tx\n"}, "lint", ""},
		{"makefile variable no es objetivo", map[string]string{"Makefile": "test := 1\nbuild:\n\tx\n"}, "test", ""},
		{"makefile verifica", map[string]string{"Makefile": "verifica:\n\tx\n"}, "typecheck", "make verifica"},
		{"package build", map[string]string{"package.json": `{"scripts":{"build":"vite build","test":"node --test"}}`}, "build", "npm run build"},
		{"package test", map[string]string{"package.json": `{"scripts":{"test":"node --test"}}`}, "test", "npm test"},
		{"package test por defecto", map[string]string{"package.json": `{"scripts":{"test":"echo \"Error: no test specified\" && exit 1"}}`}, "test", ""},
		{"package typecheck", map[string]string{"package.json": `{"scripts":{"lint":"eslint .","typecheck":"tsc --noEmit"}}`}, "typecheck", "npm run typecheck"},
		{"package lint", map[string]string{"package.json": `{"scripts":{"lint":"eslint .","typecheck":"tsc --noEmit"}}`}, "lint", "npm run lint"},
		{"pnpm", map[string]string{"package.json": `{"scripts":{"build":"x"}}`, "pnpm-lock.yaml": ""}, "build", "pnpm run build"},
		{"yarn test", map[string]string{"package.json": `{"scripts":{"test":"jest"}}`, "yarn.lock": ""}, "test", "yarn test"},
		{"tsc sin script", map[string]string{"package.json": `{}`, "tsconfig.json": "{}", "node_modules/.bin/tsc": ""}, "typecheck", "npx tsc --noEmit -p ."},
		{"tsc sin instalar", map[string]string{"package.json": `{}`, "tsconfig.json": "{}"}, "typecheck", ""},
		{"makefile antes que package", map[string]string{"Makefile": "build:\n\tx\n", "package.json": `{"scripts":{"build":"y"}}`}, "build", "make build"},
		{"makefile sin objetivo y package", map[string]string{"Makefile": "otro:\n\tx\n", "package.json": `{"scripts":{"test":"y"}}`}, "test", "npm test"},
		{"go build", map[string]string{"go.mod": "module x\n"}, "build", "go build ./..."},
		{"go test", map[string]string{"go.mod": "module x\n"}, "test", "go test ./..."},
		{"go vet", map[string]string{"go.mod": "module x\n"}, "typecheck", "go vet ./..."},
		{"cargo", map[string]string{"Cargo.toml": "[package]\n"}, "test", "cargo test"},
		{"pytest", map[string]string{"pyproject.toml": "[tool.pytest.ini_options]\n"}, "test", "python3 -m pytest"},
		{"python sin pytest", map[string]string{"pyproject.toml": "[project]\n"}, "test", ""},
		{"python build", map[string]string{"pyproject.toml": "[tool.pytest.ini_options]\n"}, "build", ""},
		{"programa guardado", map[string]string{".psf/ejecutar.json": `[{"nombre":"Odoo","orden":"odoo-bin"},{"nombre":"Pruebas","orden":"./pruebas.sh"}]`, "go.mod": "module x\n"}, "test", "./pruebas.sh"},
		{"programa guardado de otra cosa", map[string]string{".psf/ejecutar.json": `[{"nombre":"Odoo","orden":"odoo-bin"}]`}, "build", ""},
		{"acción desconocida", map[string]string{"go.mod": "module x\n"}, "deploy", ""},
	}
	for _, c := range casos {
		d := t.TempDir()
		for f, x := range c.fich {
			escribe(t, d, f, x)
		}
		got, ok := projectCommand(d, c.kind)
		if (c.orden == "") == ok || got.Orden != c.orden {
			t.Errorf("%s: %q %v (quiere %q)", c.nombre, got.Orden, ok, c.orden)
		}
	}
}

func TestTrimOutput(t *testing.T) {
	if s, e := trimOutput(""); s != "" || e != "" {
		t.Fatalf("%q %q", s, e)
	}
	var l []string
	l = append(l, "compilando…", "main.go:12:3: undefined: foo", "\tdetalle del error")
	for i := 0; i < 500; i++ {
		l = append(l, "línea normal de progreso")
	}
	l = append(l, "--- FAIL: TestAlgo (0.00s)", "    algo_test.go:9: quería 2, tengo 3", "FAIL", "\x1b[31mFAIL\x1b[0m\tpaquete\t0.01s")
	s, e := trimOutput(strings.Join(l, "\n"))
	for _, x := range []string{"main.go:12:3: undefined: foo", "[… ", "--- FAIL: TestAlgo", "FAIL\tpaquete"} {
		if !strings.Contains(s, x) {
			t.Errorf("salida sin %q:\n%s", x, s)
		}
	}
	if strings.Count(s, "línea normal") > directTailLines || strings.Contains(s, "\x1b") {
		t.Errorf("salida sin recortar:\n%s", s)
	}
	for _, x := range []string{"undefined: foo", "detalle del error", "--- FAIL: TestAlgo", "algo_test.go:9"} {
		if !strings.Contains(e, x) {
			t.Errorf("errores sin %q:\n%s", x, e)
		}
	}
	if strings.Contains(e, "línea normal") || strings.Contains(e, "compilando") {
		t.Errorf("errores con ruido:\n%s", e)
	}
	// Muchísimas líneas de error: se limitan.
	var m []string
	for i := 0; i < 1000; i++ {
		m = append(m, "x.go:1:1: error número muy largo para llenar la salida y comprobar el límite de caracteres")
	}
	s, e = trimOutput(strings.Join(m, "\n"))
	if len(s) > directMaxOutput+10 || len(e) > directMaxErrors+10 {
		t.Fatalf("%d %d", len(s), len(e))
	}
}

func TestRunDirect(t *testing.T) {
	d := t.TempDir()
	out, code, _, err := runDirect(context.Background(), d, "echo hola; echo adios >&2", os.Environ(), time.Minute)
	if err != nil || code != 0 || !strings.Contains(out, "hola") || !strings.Contains(out, "adios") {
		t.Fatalf("%q %d %v", out, code, err)
	}
	out, code, _, err = runDirect(context.Background(), d, "echo mal; exit 3", os.Environ(), time.Minute)
	if !errors.Is(err, errExit) || code != 3 || !strings.Contains(out, "mal") {
		t.Fatalf("%q %d %v", out, code, err)
	}
	t0 := time.Now()
	_, _, _, err = runDirect(context.Background(), d, "sleep 30 & sleep 30", os.Environ(), 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "tiempo máximo") || time.Since(t0) > 10*time.Second {
		t.Fatalf("%v %s", err, time.Since(t0))
	}
	// Mucha salida: solo se guarda lo último.
	out, _, _, _ = runDirect(context.Background(), d, "i=0; while [ $i -lt 40000 ]; do echo línea-$i-xxxxxxxxxxxxxxxxxxxx; i=$((i+1)); done", os.Environ(), time.Minute)
	if len(out) > directKeepBytes+10 || !strings.Contains(out, "línea-39999") || !strings.HasPrefix(out, "[…]") {
		t.Fatalf("%d %q", len(out), out[:40])
	}
}

func TestCommitMessages(t *testing.T) {
	limpios := map[string]string{
		"Añade la exportación a CSV.":                "Añade la exportación a CSV",
		"«Corrige el cálculo del IVA»":               "Corrige el cálculo del IVA",
		"Mensaje: Actualiza el README":               "Actualiza el README",
		"```\nRefactoriza el lector de pedidos\n```": "Refactoriza el lector de pedidos",
		"**Commit message:** Arregla el login":       "Arregla el login",
		"ok":                                         "",
		"":                                           "",
		strings.Repeat("palabra ", 30):               "",
		"\n\n  Mueve las pruebas a su carpeta  \notra línea": "Mueve las pruebas a su carpeta",
	}
	for in, quiere := range limpios {
		if got := cleanCommitMessage(in); got != quiere {
			t.Errorf("%q → %q (quiere %q)", in, got, quiere)
		}
	}
	if got := ruleCommitMessage([]string{"a/b.go"}); got != "Cambios en b.go" {
		t.Error(got)
	}
	if got := ruleCommitMessage([]string{"a.go", "web/b.ts", "c.css"}); got != "Cambios en a.go, b.ts y c.css" {
		t.Error(got)
	}
	if got := ruleCommitMessage([]string{"web/a.ts", "web/b.ts", "web/c.ts", "x.go"}); got != "Cambios en 4 ficheros (web, raíz)" {
		t.Error(got)
	}
}

// pideDirecto: /api/directo en el banco del reparto (un repositorio del proyecto 1).
func pideDirecto(t *testing.T, b *bancoReparto, accion string, extra map[string]any) directResult {
	t.Helper()
	q := map[string]any{"proyecto": 1, "carpeta": b.madre, "sesion": "s1", "accion": accion}
	for k, v := range extra {
		q[k] = v
	}
	code, body := b.pide("/api/directo", q)
	if code != 200 {
		t.Fatalf("%s: %d %s", accion, code, body)
	}
	var r directResult
	json.Unmarshal(body, &r)
	return r
}

func TestDirectoEndpoint(t *testing.T) {
	b := nuevoBanco(t)
	viejo := commitLocal
	defer func() { commitLocal = viejo }()
	commitLocal = func(context.Context, string, string, int) (string, string, error) {
		return "", "", errors.New("sin IA local")
	}

	// Dónde estamos y sin cambios.
	if r := pideDirecto(t, b, "where", nil); !r.OK || r.Rama != "main" || r.Carpeta != b.madre || r.Copia {
		t.Fatalf("where: %+v", r)
	}
	if r := pideDirecto(t, b, "files", nil); !r.OK || len(r.Ficheros) != 0 {
		t.Fatalf("files: %+v", r)
	}
	if r := pideDirecto(t, b, "commit", nil); !r.OK || r.Hash != "" || !strings.Contains(r.Salida, "No hay cambios") {
		t.Fatalf("commit vacío: %+v", r)
	}
	// Cambios: estado, diff y ficheros.
	escribe(t, b.madre, "base.txt", "base cambiada\n")
	escribe(t, b.madre, "web/nuevo.ts", "export const x = 1;\n")
	if r := pideDirecto(t, b, "status", nil); !r.OK || !strings.Contains(r.Salida, "## main") || !strings.Contains(r.Salida, "base.txt") || len(r.Ficheros) != 2 {
		t.Fatalf("status: %+v", r)
	}
	if r := pideDirecto(t, b, "diff", nil); !r.OK || !strings.Contains(r.Salida, "+base cambiada") || !strings.Contains(r.Salida, "base.txt") {
		t.Fatalf("diff: %+v", r)
	}
	if r := pideDirecto(t, b, "files", nil); !r.OK || strings.Join(r.Ficheros, ",") != "base.txt,web/nuevo.ts" {
		t.Fatalf("files: %+v", r)
	}
	// Commit sin IA local: mensaje por reglas.
	r := pideDirecto(t, b, "commit", nil)
	if !r.OK || r.Hash == "" || r.Via != "reglas" || r.Mensaje != "Cambios en base.txt y nuevo.ts" {
		t.Fatalf("commit: %+v", r)
	}
	if log := b.git(b.madre, "log", "-1", "--format=%s"); log != r.Mensaje {
		t.Fatalf("log: %q", log)
	}
	if st := b.git(b.madre, "status", "--porcelain"); st != "" {
		t.Fatalf("sin commitear: %q", st)
	}
	// Con IA local: su mensaje (limpio).
	commitLocal = func(_ context.Context, sistema, texto string, _ int) (string, string, error) {
		if !strings.Contains(texto, "+otra") {
			t.Errorf("sin diff: %q", texto)
		}
		return "«Añade otra línea a base.txt»", "gemma3:4b", nil
	}
	escribe(t, b.madre, "base.txt", "base cambiada\notra\n")
	if r := pideDirecto(t, b, "commit", nil); !r.OK || r.Via != "local/gemma3:4b" || r.Mensaje != "Añade otra línea a base.txt" {
		t.Fatalf("commit IA: %+v", r)
	}
	// Con un agente trabajando en la carpeta, no.
	escribe(t, b.madre, "base.txt", "x\n")
	sal := b.s.trabajando.entra(b.madre)
	if r := pideDirecto(t, b, "commit", nil); r.OK || !r.SinOrden {
		t.Fatalf("ocupada: %+v", r)
	}
	sal()
	// A mitad de una fusión, no.
	gd := b.git(b.madre, "rev-parse", "--absolute-git-dir")
	escribe(t, gd, "MERGE_HEAD", "x")
	if r := pideDirecto(t, b, "commit", nil); r.OK || !r.SinOrden || !strings.Contains(r.Motivo, "fusión") {
		t.Fatalf("fusión: %+v", r)
	}
	os.Remove(filepath.Join(gd, "MERGE_HEAD"))
	b.git(b.madre, "checkout", "--", "base.txt")

	// Compilar y pruebas con el Makefile del proyecto (una que falla).
	if r := pideDirecto(t, b, "build", nil); r.OK || !r.SinOrden {
		t.Fatalf("sin orden: %+v", r)
	}
	escribe(t, b.madre, "Makefile", "build:\n\t@echo compilado\ntest:\n\t@echo 'ok  paquete/uno'\n\t@echo '--- FAIL: TestDos (0.00s)'\n\t@echo '    dos_test.go:7: quería 2, tengo 3'\n\t@exit 1\n")
	if r := pideDirecto(t, b, "build", nil); !r.OK || r.Orden != "make build" || r.Origen != "Makefile" || !strings.Contains(r.Salida, "compilado") {
		t.Fatalf("build: %+v", r)
	}
	r = pideDirecto(t, b, "test", nil)
	if r.OK || r.Codigo == 0 || r.Orden != "make test" || !strings.Contains(r.Errores, "--- FAIL: TestDos") || !strings.Contains(r.Errores, "dos_test.go:7") || strings.Contains(r.Errores, "ok  paquete") {
		t.Fatalf("test: %+v", r)
	}
	if r := pideDirecto(t, b, "lint", nil); r.OK || !r.SinOrden {
		t.Fatalf("lint: %+v", r)
	}
	// Lo que responde la interfaz solo se anota.
	if r := pideDirecto(t, b, "cost", nil); !r.OK {
		t.Fatalf("cost: %+v", r)
	}
	// Una carpeta que no es del proyecto o una acción desconocida: no.
	for _, q := range []map[string]any{{"proyecto": 1, "carpeta": t.TempDir(), "accion": "status"}, {"proyecto": 1, "carpeta": b.madre, "accion": "push"}} {
		if code, body := b.pide("/api/directo", q); code != 400 {
			t.Fatalf("%v: %d %s", q, code, body)
		}
	}
	// La sesión en otra rama: en su copia de trabajo.
	if r := pideDirecto(t, b, "where", map[string]any{"rama": "otra"}); !r.OK || r.Rama != "otra" || !r.Copia {
		t.Fatalf("copia: %+v", r)
	}
}

// PGO_PRUEBA_COMMIT=1 go test -run TestCommitLocalReal -v ./internal/servidor: mensajes de commit de la IA local de
// verdad (gemma3) para unos diffs de ejemplo.
func TestCommitLocalReal(t *testing.T) {
	if os.Getenv("PGO_PRUEBA_COMMIT") == "" {
		t.Skip()
	}
	casos := []struct{ stat, diff string }{
		{" web/src/costes.ts | 4 +++-", "--- a/web/src/costes.ts\n+++ b/web/src/costes.ts\n@@ -10,3 +10,5 @@\n-export const dinero = (x: number) => x.toFixed(2) + ' $';\n+// dinero: con coma decimal (español).\n+export const dinero = (x: number) => x.toFixed(2).replace('.', ',') + ' $';\n"},
		{" README.md | 6 ++++++", "--- a/README.md\n+++ b/README.md\n@@ -1,2 +1,8 @@\n # App\n+\n+## Instalación\n+\n+```\n+npm install\n+```\n"},
		{" internal/login.go | 3 ++-", "--- a/internal/login.go\n+++ b/internal/login.go\n@@ -20,3 +20,4 @@\n-\tif user == \"\" {\n+\tif strings.TrimSpace(user) == \"\" {\n+\t\t// usuario vacío o solo espacios\n \t\treturn errVacio\n"},
	}
	for _, c := range casos {
		t0 := time.Now()
		m, via := commitMessage(context.Background(), []string{"x"}, c.stat, c.diff)
		t.Logf("%s (%d ms): %s", via, time.Since(t0).Milliseconds(), m)
	}
}

// Salidas reales de unittest, pytest, go test, jest y tsc: «Pásaselo al agente» tiene que llevar el error.
func TestTrimOutputReal(t *testing.T) {
	casos := map[string][]string{
		"unittest.txt": {"ERROR: test_error", "Traceback (most recent call last):", `line 8, in test_error`,
			`raise ValueError("valor malo: %r" % x)`, "ValueError: valor malo: 5", "FAIL: test_suma",
			"self.assertEqual(1 + 1, 3)", "AssertionError: 2 != 3", "FAILED (failures=1, errors=1)"},
		"pytest.txt": {"assert c.aplica(100) == 90", "E       assert 100 == 90", "tests/test_cupon.py:7: AssertionError",
			"FAILED tests/test_cupon.py::test_descuento - assert 100 == 90"},
		"gotest.txt": {"--- FAIL: TestSuma", "a_test.go:3: suma: quería 3, tengo 2", "--- FAIL: TestPanic",
			"panic: assignment to entry in nil map", "FAIL\tgt"},
		"jest.txt": {"FAIL  src/cupon.test.js", "✕ no aplica caducado", "Expected: 100", "Received: 90",
			"at Object.toBe (src/cupon.test.js:14:27)"},
		"tsc.txt": {"error TS2322: Type 'string' is not assignable to type 'number'.", "error TS2339: Property 'foo'"},
	}
	for f, quiere := range casos {
		b, err := os.ReadFile(filepath.Join("testdata", "salidas", f))
		if err != nil {
			t.Fatal(err)
		}
		_, e := trimOutput(string(b))
		for _, x := range quiere {
			if !strings.Contains(e, x) {
				t.Errorf("%s: errores sin %q:\n%s", f, x, e)
			}
		}
		if strings.Contains(e, "Ran 3 tests") || strings.Contains(e, "Snapshots:") || strings.Contains(e, "test session starts") {
			t.Errorf("%s: errores con ruido:\n%s", f, e)
		}
	}
	// Tras el Traceback, la primera línea sin sangría (la excepción) entra, pero no lo que la sigue.
	_, e := trimOutput("Traceback (most recent call last):\n  File \"a.py\", line 1, in <module>\n    f()\nKeyError: 'x'\nresumen final")
	if !strings.Contains(e, "KeyError: 'x'") || strings.Contains(e, "resumen final") {
		t.Errorf("traceback: %q", e)
	}
}
