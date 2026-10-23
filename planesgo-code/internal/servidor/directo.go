package servidor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/proceso"
	"planesgo-code/internal/repo"
	"planesgo-code/internal/selector"
	"planesgo-code/internal/sesiones"
)

// Órdenes directas: lo mecánico que pgocode hace sin el agente (compilar, pasar las pruebas, comprobar tipos o
// lint, estado de git, diff, commit) y las preguntas de estado que ya sabe responder (ficheros cambiados, rama y
// carpeta). Así no se gasta un turno entero del agente, que reenvía todo su contexto. La interfaz decide cuándo
// (web/src/direct.ts: solo si el mensaje entero es la orden); aquí se hace, en la carpeta de la sesión.
// Nunca push, reset ni nada destructivo: el commit solo añade un commit en la rama actual.

// directKinds: lo que se puede pedir. «cost» y «last_turn» los responde la interfaz; llegan solo para anotarlos.
var directKinds = map[string]bool{"build": true, "test": true, "typecheck": true, "lint": true, "status": true, "diff": true,
	"commit": true, "files": true, "where": true, "cost": true, "last_turn": true}

// Tiempo máximo de cada orden (las pruebas, más).
var directTimeouts = map[string]time.Duration{"build": 5 * time.Minute, "typecheck": 5 * time.Minute, "lint": 5 * time.Minute, "test": 10 * time.Minute}

// Lo que se guarda de la salida de una orden (lo último) y lo que se devuelve recortado.
const (
	directKeepBytes   = 512 << 10
	directTailLines   = 40
	directMaxOutput   = 6000
	directMaxErrors   = 4000
	directMaxErrLines = 60
)

// directResult: lo que se devuelve a la interfaz.
type directResult struct {
	Accion   string   `json:"accion"`
	Orden    string   `json:"orden,omitempty"`     // la orden que se ha ejecutado (o «git …»)
	Origen   string   `json:"origen,omitempty"`    // de dónde se ha sacado (Makefile, package.json, go.mod…)
	OK       bool     `json:"ok"`                  // ha ido bien
	Codigo   int      `json:"codigo"`              // código de salida
	Segundos float64  `json:"segundos"`            // duración
	Salida   string   `json:"salida,omitempty"`    // recortada: errores y final
	Errores  string   `json:"errores,omitempty"`   // solo las líneas relevantes (para pasárselas al agente)
	Mensaje  string   `json:"mensaje,omitempty"`   // commit: el mensaje
	Hash     string   `json:"hash,omitempty"`      // commit: el hash corto
	Via      string   `json:"via,omitempty"`       // commit: quién redactó el mensaje (local/<modelo> o reglas)
	Ficheros []string `json:"ficheros,omitempty"`  // ficheros con cambios (git)
	Rama     string   `json:"rama,omitempty"`      // rama actual
	Carpeta  string   `json:"carpeta,omitempty"`   // carpeta de trabajo
	Copia    bool     `json:"copia,omitempty"`     // la carpeta es una copia de trabajo (git worktree)
	SinOrden bool     `json:"sin_orden,omitempty"` // no se sabe con seguridad qué orden es: que lo haga el agente
	Motivo   string   `json:"motivo,omitempty"`    // por qué no se ha hecho
}

// directCommand: la orden del proyecto para una acción, deducida sin IA.
type directCommand struct {
	Orden  string // para el sh
	Origen string
}

// Un candado por carpeta: dos órdenes directas no se pisan en la misma carpeta.
var directLocks sync.Map

func directLock(dir string) *sync.Mutex {
	m, _ := directLocks.LoadOrStore(filepath.Clean(dir), &sync.Mutex{})
	return m.(*sync.Mutex)
}

// directo: POST /api/directo {proyecto, carpeta, rama, sesion, accion}.
func (s *Servidor) directo(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Carpeta  string `json:"carpeta"`
		Rama     string `json:"rama"`
		Sesion   string `json:"sesion"`
		Accion   string `json:"accion"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if !directKinds[p.Accion] {
		jsonError(w, http.StatusBadRequest, "acción no válida")
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	inicio := time.Now()
	res := directResult{Accion: p.Accion}
	anota := func() {
		if s.Imputacion == nil || p.Sesion == "" {
			return
		}
		c := map[string]any{"accion": p.Accion, "ok": res.OK, "ms": time.Since(inicio).Milliseconds()}
		if res.Orden != "" {
			c["orden"] = res.Orden
		}
		if res.SinOrden {
			c["sin_orden"] = true
		}
		if res.Via != "" {
			c["via"] = res.Via
		}
		s.Imputacion.AnotaAgente(p.Sesion, "directo", c)
	}
	// Lo que responde la interfaz con lo que ya sabe: solo se anota (para medir los turnos que se ahorran).
	if p.Accion == "cost" || p.Accion == "last_turn" {
		res.OK = true
		anota()
		jsonOK(w, res)
		return
	}
	dir, copia, err := s.directDir(r.Context(), p.Proyecto, p.Carpeta, p.Rama, p.Sesion)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	res.Carpeta, res.Copia = dir, copia
	switch p.Accion {
	case "build", "test", "typecheck", "lint":
		c, ok := projectCommand(dir, p.Accion)
		if !ok {
			res.SinOrden, res.Motivo = true, "no se sabe con seguridad qué orden usa este proyecto"
			break
		}
		res.Orden, res.Origen = c.Orden, c.Origen
		mu := directLock(dir)
		mu.Lock()
		out, code, dur, err := runDirect(r.Context(), dir, c.Orden, append(os.Environ(), append(s.entornoPrograma(p.Proyecto), "CI=true")...), directTimeouts[p.Accion])
		mu.Unlock()
		res.Segundos, res.Codigo = dur.Seconds(), code
		res.OK = err == nil && code == 0
		res.Salida, res.Errores = trimOutput(out)
		if err != nil && !errors.Is(err, errExit) {
			res.Salida = strings.TrimSpace(res.Salida + "\n\n" + err.Error())
			if res.Errores == "" {
				res.Errores = err.Error()
			}
		}
	case "status", "diff", "files", "where":
		s.directGit(r.Context(), dir, &res)
	case "commit":
		if s.carpetaOcupada(dir) {
			res.SinOrden, res.Motivo = true, "hay un agente trabajando en esta carpeta"
			break
		}
		mu := directLock(dir)
		mu.Lock()
		// El commit acaba aunque se cierre la interfaz: un git cortado a medias dejaría index.lock.
		cc, cancela := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Minute)
		directCommit(cc, dir, &res)
		cancela()
		mu.Unlock()
	}
	if res.Segundos == 0 {
		res.Segundos = time.Since(inicio).Seconds()
	}
	anota()
	jsonOK(w, res)
}

// directDir: la carpeta de trabajo de la sesión, comprobada como la de /api/chat (del proyecto, no la personal) y,
// si la sesión trabaja en otra rama, su copia de trabajo.
func (s *Servidor) directDir(ctx context.Context, proyecto int, carpeta, rama, sesion string) (string, bool, error) {
	if carpeta == "" || !filepath.IsAbs(carpeta) || !s.Indice.Pertenece(proyecto, carpeta) {
		return "", false, errors.New("la carpeta de la sesión no es del proyecto")
	}
	if casa, _ := os.UserHomeDir(); filepath.Clean(carpeta) == "/" || filepath.Clean(carpeta) == filepath.Clean(casa) || dentroDe(filepath.Clean(carpeta), casa) {
		return "", false, errors.New("la carpeta de la sesión es tu carpeta personal (o una superior)")
	}
	if rama == "" {
		return carpeta, false, nil
	}
	c, err := repo.CopiaDeRama(ctx, carpeta, rama, s.DirRamas)
	if err != nil {
		return "", false, err
	}
	c.Sesion, c.Proyecto = sesion, proyecto
	s.apuntaCopia(c)
	return c.Carpeta, filepath.Clean(c.Carpeta) != filepath.Clean(carpeta), nil
}

// ---- Qué orden: la del proyecto, sin IA ----

// Nombres que valen para cada acción (programas guardados, objetivos del Makefile y scripts de package.json).
var directNames = map[string][]string{
	"build":     {"build", "compila", "compilar", "construir"},
	"test":      {"test", "tests", "pruebas", "prueba"},
	"typecheck": {"typecheck", "type-check", "types", "tipos", "check", "tsc", "verifica", "vet"},
	"lint":      {"lint", "linter", "eslint"},
}

var makeTarget = regexp.MustCompile(`(?m)^([A-Za-z0-9_][A-Za-z0-9_.-]*)\s*:([^=]|$)`)

// projectCommand: la orden de la acción en la carpeta: un programa guardado con ese nombre (.psf/ejecutar.json),
// un objetivo del Makefile, un script de package.json, o la de siempre de Go, Rust o Python. Si no se sabe con
// seguridad, nada (lo hará el agente).
func projectCommand(dir, kind string) (directCommand, bool) {
	names := directNames[kind]
	if len(names) == 0 {
		return directCommand{}, false
	}
	// 1. Programas guardados del proyecto.
	for _, n := range names {
		for _, c := range leeConfiguraciones(dir) {
			if strings.EqualFold(strings.TrimSpace(c.Nombre), n) && strings.TrimSpace(c.Orden) != "" {
				return directCommand{Orden: c.Orden, Origen: "programa guardado «" + c.Nombre + "»"}, true
			}
		}
	}
	// 2. Makefile.
	for _, f := range []string{"GNUmakefile", "Makefile", "makefile"} {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		targets := map[string]bool{}
		for _, m := range makeTarget.FindAllStringSubmatch(string(b), -1) {
			targets[m[1]] = true
		}
		for _, n := range names {
			if targets[n] {
				return directCommand{Orden: "make " + n, Origen: f}, true
			}
		}
		break
	}
	// 3. package.json.
	if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		var pk struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(b, &pk) == nil {
			pm := packageManager(dir)
			for _, n := range names {
				sc, ok := pk.Scripts[n]
				if !ok || strings.TrimSpace(sc) == "" || (n == "test" && strings.Contains(sc, "no test specified")) {
					continue
				}
				if n == "test" {
					return directCommand{Orden: pm + " test", Origen: "package.json"}, true
				}
				return directCommand{Orden: pm + " run " + n, Origen: "package.json"}, true
			}
			// TypeScript sin script: tsc del proyecto, si está instalado.
			if kind == "typecheck" && fileExists(filepath.Join(dir, "tsconfig.json")) && fileExists(filepath.Join(dir, "node_modules", ".bin", "tsc")) {
				return directCommand{Orden: "npx tsc --noEmit -p .", Origen: "tsconfig.json"}, true
			}
		}
	}
	// 4. Go, Rust, Python: las órdenes de siempre.
	if fileExists(filepath.Join(dir, "go.mod")) {
		switch kind {
		case "build":
			return directCommand{Orden: "go build ./...", Origen: "go.mod"}, true
		case "test":
			return directCommand{Orden: "go test ./...", Origen: "go.mod"}, true
		case "typecheck":
			return directCommand{Orden: "go vet ./...", Origen: "go.mod"}, true
		case "lint":
			if hayGolangci(dir) {
				return directCommand{Orden: "golangci-lint run", Origen: "go.mod"}, true
			}
			return directCommand{Orden: "go vet ./...", Origen: "go.mod"}, true
		}
	}
	if fileExists(filepath.Join(dir, "Cargo.toml")) {
		switch kind {
		case "build":
			return directCommand{Orden: "cargo build", Origen: "Cargo.toml"}, true
		case "test":
			return directCommand{Orden: "cargo test", Origen: "Cargo.toml"}, true
		case "typecheck":
			return directCommand{Orden: "cargo check", Origen: "Cargo.toml"}, true
		case "lint":
			return directCommand{Orden: "cargo clippy", Origen: "Cargo.toml"}, true
		}
	}
	py, _ := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	switch kind {
	case "test":
		if fileExists(filepath.Join(dir, "pytest.ini")) || bytes.Contains(py, []byte("[tool.pytest")) || fileExists(filepath.Join(dir, "conftest.py")) {
			return directCommand{Orden: "python3 -m pytest", Origen: "pytest"}, true
		}
	case "lint":
		if bytes.Contains(py, []byte("[tool.ruff")) || fileExists(filepath.Join(dir, "ruff.toml")) {
			return directCommand{Orden: "ruff check .", Origen: "ruff"}, true
		}
	case "typecheck":
		if bytes.Contains(py, []byte("[tool.mypy")) || fileExists(filepath.Join(dir, "mypy.ini")) {
			return directCommand{Orden: "mypy .", Origen: "mypy"}, true
		}
	}
	return directCommand{}, false
}

func packageManager(dir string) string {
	switch {
	case fileExists(filepath.Join(dir, "pnpm-lock.yaml")):
		return "pnpm"
	case fileExists(filepath.Join(dir, "yarn.lock")):
		return "yarn"
	case fileExists(filepath.Join(dir, "bun.lockb")) || fileExists(filepath.Join(dir, "bun.lock")):
		return "bun"
	}
	return "npm"
}

func hayGolangci(dir string) bool {
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		return false
	}
	for _, f := range []string{".golangci.yml", ".golangci.yaml", ".golangci.toml", ".golangci.json"} {
		if fileExists(filepath.Join(dir, f)) {
			return true
		}
	}
	return false
}

func fileExists(f string) bool { _, err := os.Stat(f); return err == nil }

// ---- Ejecutar ----

var errExit = errors.New("la orden ha terminado con error")

// tailBuffer: guarda solo lo último que escribe la orden (las de miles de líneas no llenan la memoria).
type tailBuffer struct {
	mu  sync.Mutex
	b   []byte
	max int
	cut bool
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = append([]byte(nil), t.b[len(t.b)-t.max:]...)
		t.cut = true
	}
	return len(p), nil
}

// runDirect: la orden con sh en la carpeta, con tiempo máximo (al cancelar, se mata su grupo de procesos).
// Devuelve la salida (stdout y stderr juntos), el código de salida y la duración; err es errExit si el código no
// es 0, u otro error si no se ha podido ejecutar o se ha pasado de tiempo.
func runDirect(ctx context.Context, dir, orden string, env []string, max time.Duration) (string, int, time.Duration, error) {
	if max <= 0 {
		max = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, max)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", orden)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = nil
	proceso.GrupoPropio(cmd)
	cmd.WaitDelay = 5 * time.Second
	buf := &tailBuffer{max: directKeepBytes}
	cmd.Stdout, cmd.Stderr = buf, buf
	t0 := time.Now()
	err := cmd.Run()
	dur := time.Since(t0)
	out := string(buf.b)
	if buf.cut {
		if i := strings.IndexByte(out, '\n'); i >= 0 {
			out = "[…]\n" + out[i+1:]
		}
	}
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
		switch {
		case ctx.Err() == context.DeadlineExceeded:
			return out, code, dur, fmt.Errorf("se ha pasado del tiempo máximo (%s)", max)
		case ctx.Err() != nil:
			return out, code, dur, errors.New("cancelada")
		case code != 0:
			return out, code, dur, errExit
		}
		return out, -1, dur, err
	}
	return out, 0, dur, nil
}

// Líneas que suelen ser errores: de compiladores, pruebas (Go, node --test, jest, pytest) y linters.
var errorLine = regexp.MustCompile(`(?i)(\berror\b|\berr!|\bfail(ed|ure|s)?\b|\bpanic\b|✗|✖|×|\bexception\b|traceback|^\s*not ok\b|^\s*at .*:\d+:\d+|^[^\s].*:\d+(:\d+)?:\s|\bexpected\b|\bassert(ion)?\b|\bundefined\b|cannot|no such|\bwarning\b.*\berror)`)
var ansi = regexp.MustCompile("\x1b\\[[0-9;?]*[A-Za-z]")

// trimOutput: la salida para el chat (las líneas de error que no estén al final, y el final) y solo los errores
// (con la línea de detalle sangrada que las siga), para pasárselos al agente.
func trimOutput(out string) (string, string) {
	out = strings.ReplaceAll(ansi.ReplaceAllString(out, ""), "\r\n", "\n")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return "", ""
	}
	// Errores: las líneas que lo parecen y la siguiente (suele ser el detalle), sin repetir.
	var errs []string
	marked := make([]bool, len(lines))
	for i, l := range lines {
		if strings.TrimSpace(l) == "" || !errorLine.MatchString(l) {
			continue
		}
		for j := i; j <= i+1 && j < len(lines); j++ {
			// La siguiente, solo si es su detalle (sangrada).
			if j > i && !strings.HasPrefix(lines[j], " ") && !strings.HasPrefix(lines[j], "\t") {
				break
			}
			if !marked[j] && strings.TrimSpace(lines[j]) != "" {
				marked[j] = true
				errs = append(errs, lines[j])
			}
		}
	}
	if len(errs) > directMaxErrLines {
		errs = append(errs[:directMaxErrLines], fmt.Sprintf("[… %d líneas más]", len(errs)-directMaxErrLines))
	}
	errores := limitText(strings.Join(errs, "\n"), directMaxErrors)
	// Salida: el final; y, si hay errores antes, esos primero.
	tailFrom := len(lines) - directTailLines
	if tailFrom < 0 {
		tailFrom = 0
	}
	var parts []string
	if tailFrom > 0 {
		var before []string
		for i := 0; i < tailFrom; i++ {
			if marked[i] {
				before = append(before, lines[i])
			}
		}
		if len(before) > 25 {
			before = append(before[:25], fmt.Sprintf("[… %d líneas de error más]", len(before)-25))
		}
		if len(before) > 0 {
			parts = append(parts, strings.Join(before, "\n"))
		}
		parts = append(parts, fmt.Sprintf("[… %d líneas …]", tailFrom))
	}
	parts = append(parts, strings.Join(lines[tailFrom:], "\n"))
	salida := strings.Join(parts, "\n")
	if len(salida) > directMaxOutput {
		salida = "[…]\n" + salida[len(salida)-directMaxOutput:]
	}
	return salida, errores
}

func limitText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n[…]"
}

// ---- git ----

func gitDirect(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

// changedFilesGit: los ficheros con cambios sin commitear (también los nuevos), en el orden de git.
func changedFilesGit(ctx context.Context, dir string) ([]string, error) {
	out, err := gitDirect(ctx, dir, "status", "--porcelain", "-uall")
	if err != nil {
		return nil, err
	}
	var l []string
	for _, x := range strings.Split(out, "\n") {
		if len(x) < 4 {
			continue
		}
		f := x[3:]
		if i := strings.Index(f, " -> "); i >= 0 {
			f = f[i+4:]
		}
		l = append(l, strings.Trim(f, `"`))
	}
	return l, nil
}

// directGit: estado, diff, ficheros cambiados y dónde estamos (solo lectura).
func (s *Servidor) directGit(ctx context.Context, dir string, res *directResult) {
	res.Rama, _ = gitDirect(ctx, dir, "branch", "--show-current")
	if _, err := gitDirect(ctx, dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		res.OK = res.Accion == "where"
		if !res.OK {
			res.Motivo = "la carpeta no es un repositorio git"
		}
		res.Rama = ""
		return
	}
	switch res.Accion {
	case "status":
		res.Orden = "git status --short --branch"
		out, err := gitDirect(ctx, dir, "status", "--short", "--branch")
		res.OK = err == nil
		res.Salida = limitText(out, directMaxOutput)
		res.Ficheros, _ = changedFilesGit(ctx, dir)
	case "diff":
		res.Orden = "git diff HEAD"
		stat, err := gitDirect(ctx, dir, "diff", "HEAD", "--stat")
		if err != nil { // sin commits todavía
			stat, err = gitDirect(ctx, dir, "diff", "--stat")
		}
		diff, _ := gitDirect(ctx, dir, "diff", "HEAD")
		res.OK = err == nil
		res.Salida = stat
		if strings.TrimSpace(diff) != "" {
			res.Salida += "\n\n" + limitText(diff, directMaxOutput-len(stat))
		}
		res.Ficheros, _ = changedFilesGit(ctx, dir)
	case "files":
		res.Orden = "git status --porcelain"
		l, err := changedFilesGit(ctx, dir)
		res.OK, res.Ficheros = err == nil, l
	case "where":
		res.OK = true
	}
}

// ---- commit ----

const systemCommit = `Escribe el mensaje de un commit de git para los cambios que te paso. Responde SOLO con el mensaje: una línea en español, en imperativo o como descripción breve (como «Añade la exportación a CSV de facturas»), de 72 caracteres como mucho, sin comillas, sin prefijos y sin explicaciones.`

// commitLocal: el modelo de texto de la IA local (en las pruebas se sustituye).
var commitLocal = selector.LocalChat

const commitLocalMax = 25 * time.Second

// directCommit: git add -A y git commit con un mensaje redactado por la IA local a partir del diff (si no hay, uno
// por reglas). Nada más: ni push, ni reset, ni enmiendas. No se hace en mitad de una fusión o un rebase.
func directCommit(ctx context.Context, dir string, res *directResult) {
	res.Orden = "git add -A && git commit"
	gitDir, err := gitDirect(ctx, dir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		res.Motivo = "la carpeta no es un repositorio git"
		return
	}
	for _, f := range []string{"MERGE_HEAD", "rebase-merge", "rebase-apply", "CHERRY_PICK_HEAD", "REVERT_HEAD"} {
		if fileExists(filepath.Join(gitDir, f)) {
			res.SinOrden, res.Motivo = true, "hay una fusión, un rebase o un cherry-pick a medias"
			return
		}
	}
	res.Rama, _ = gitDirect(ctx, dir, "branch", "--show-current")
	if res.Rama == "" {
		res.SinOrden, res.Motivo = true, "no hay ninguna rama activa (HEAD suelto)"
		return
	}
	if out, err := gitDirect(ctx, dir, "add", "-A"); err != nil {
		res.Salida = out
		res.Errores = out
		return
	}
	names, _ := gitDirect(ctx, dir, "diff", "--cached", "--name-only")
	if strings.TrimSpace(names) == "" {
		res.OK = true
		res.Salida = "No hay cambios que commitear."
		return
	}
	res.Ficheros = strings.Split(names, "\n")
	stat, _ := gitDirect(ctx, dir, "diff", "--cached", "--stat")
	diff, _ := gitDirect(ctx, dir, "diff", "--cached", "--no-color", "-U2")
	msg, via := commitMessage(ctx, res.Ficheros, stat, diff)
	res.Mensaje, res.Via = msg, via
	args := append(gitIdentity(ctx, dir), "commit", "-q", "-m", msg)
	out, err := gitDirect(ctx, dir, args...)
	if err != nil {
		res.Salida, res.Errores = trimOutput(out)
		if res.Salida == "" {
			res.Salida = err.Error()
		}
		return
	}
	res.OK = true
	res.Hash, _ = gitDirect(ctx, dir, "rev-parse", "--short", "HEAD")
	res.Salida = stat
}

// gitIdentity: si git no tiene autor configurado, uno propio solo para esta orden (como repo.identidad).
func gitIdentity(ctx context.Context, dir string) []string {
	var a []string
	if n, _ := gitDirect(ctx, dir, "config", "user.name"); strings.TrimSpace(n) == "" {
		a = append(a, "-c", "user.name=pgocode")
	}
	if e, _ := gitDirect(ctx, dir, "config", "user.email"); strings.TrimSpace(e) == "" {
		a = append(a, "-c", "user.email=pgocode@localhost")
	}
	return a
}

// commitMessage: el de la IA local (si lo hay y responde a tiempo con algo razonable) o uno por reglas.
func commitMessage(ctx context.Context, files []string, stat, diff string) (string, string) {
	q := "Ficheros cambiados:\n" + limitText(stat, 2000) + "\n\nCambios:\n" + limitText(diff, 6000)
	cl, cancel := context.WithTimeout(ctx, commitLocalMax)
	resp, model, err := commitLocal(cl, systemCommit, q, 40)
	cancel()
	if err == nil {
		if m := cleanCommitMessage(resp); m != "" {
			return m, "local/" + model
		}
	}
	return ruleCommitMessage(files), "reglas"
}

// cleanCommitMessage: la primera línea útil, sin comillas, prefijos ni Markdown; «» si no sirve.
func cleanCommitMessage(s string) string {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(strings.Trim(strings.TrimSpace(l), "`*\"'«»“”"))
		low := strings.ToLower(l)
		for _, p := range []string{"mensaje del commit:", "mensaje de commit:", "mensaje:", "commit message:", "commit:"} {
			if strings.HasPrefix(low, p) {
				l = strings.TrimSpace(strings.Trim(strings.TrimSpace(l[len(p):]), "`*\"'«»“”"))
				low = strings.ToLower(l)
			}
		}
		if l == "" || strings.HasPrefix(l, "```") {
			continue
		}
		l = strings.TrimSpace(strings.TrimRight(l, "."))
		if r := []rune(l); len(r) < 6 || len(r) > 120 {
			return ""
		}
		return l
	}
	return ""
}

// ruleCommitMessage: «Cambios en a.go, b.ts y c.css» o, si son muchos, en qué carpetas.
func ruleCommitMessage(files []string) string {
	if len(files) == 0 {
		return "Cambios"
	}
	if len(files) <= 3 {
		var b []string
		for _, f := range files {
			b = append(b, filepath.Base(f))
		}
		return "Cambios en " + joinY(b)
	}
	dirs := map[string]int{}
	for _, f := range files {
		d := filepath.Dir(f)
		if d == "." {
			d = "raíz"
		}
		dirs[d]++
	}
	var ds []string
	for d := range dirs {
		ds = append(ds, d)
	}
	sort.Slice(ds, func(i, j int) bool { return dirs[ds[i]] > dirs[ds[j]] || (dirs[ds[i]] == dirs[ds[j]] && ds[i] < ds[j]) })
	if len(ds) > 3 {
		ds = ds[:3]
		return fmt.Sprintf("Cambios en %d ficheros (%s…)", len(files), strings.Join(ds, ", "))
	}
	return fmt.Sprintf("Cambios en %d ficheros (%s)", len(files), strings.Join(ds, ", "))
}

func joinY(l []string) string {
	if len(l) <= 1 {
		return strings.Join(l, "")
	}
	return strings.Join(l[:len(l)-1], ", ") + " y " + l[len(l)-1]
}
