// Package memory: la memoria persistente de cada proyecto. Un fichero corto (<raíz del repositorio>/.pgocode/memoria.md,
// fuera de git) con lo que sigue valiendo de una sesión a otra: decisiones, convenciones, datos clave y órdenes del
// proyecto («el IVA se redondea con ROUND_HALF_UP», «el puerto es 3005», «pruebas: `make test`»). pgocode la mantiene
// solo con reglas sobre la conversación (sin tokens de pago ni marcas del usuario) y se la da al agente solo cuando
// empieza de cero (sesión nueva, relevo, «Vaciar contexto», compactación), nunca en cada turno. El usuario puede
// verla, corregirla o borrarla en las propiedades de la sesión.
package memory

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// MaxTokens: el tope estricto de la memoria (lo que se le da al agente). MaxChars es ese tope en caracteres, con
// 3,5 caracteres por token (el español con rutas y órdenes da menos de 4): así nunca pasa de MaxTokens.
const (
	MaxTokens = 1500
	MaxChars  = MaxTokens * 7 / 2
)

// Secciones que mantiene pgocode (el usuario puede añadir otras: se conservan).
const (
	SectionFacts    = "Decisiones y datos"
	SectionCommands = "Órdenes"
)

const header = "# Memoria del proyecto\n<!-- pgocode la mantiene sola con lo que dices en la conversación (decisiones, convenciones, datos) y las órdenes que funcionan; puedes corregirla o borrarla. Se le da al agente solo cuando empieza de cero. -->\n"

// ErrTooLong: el texto que guarda el usuario pasa del tope.
var ErrTooLong = errors.New("la memoria pasa del tope de 1500 tokens: acórtala")

var mu sync.Mutex // lecturas y escrituras del fichero (un solo proceso: el backend)

// Root: la carpeta del proyecto donde vive la memoria: la principal del repositorio (las copias de trabajo de otras
// ramas comparten la de su repositorio); fuera de git, la propia carpeta.
func Root(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return filepath.Clean(dir)
	}
	common := strings.TrimSpace(string(out))
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common)
	}
	return filepath.Clean(dir)
}

// Path: el fichero de la memoria de la carpeta (ya resuelta con Root).
func Path(root string) string { return filepath.Join(root, ".pgocode", "memoria.md") }

// Load: el texto de la memoria («» si no hay).
func Load(root string) string {
	mu.Lock()
	defer mu.Unlock()
	return load(root)
}

func load(root string) string {
	b, err := os.ReadFile(Path(root))
	if err != nil {
		return ""
	}
	return string(b)
}

// Save: guarda el texto que ha escrito el usuario (vacío: la borra). Con más de MaxTokens, ErrTooLong.
func Save(root, text string) error {
	mu.Lock()
	defer mu.Unlock()
	if strings.TrimSpace(text) == "" {
		return remove(root)
	}
	if utf8.RuneCountInString(Body(text)) > MaxChars {
		return ErrTooLong
	}
	return write(root, text)
}

// Delete: borra la memoria.
func Delete(root string) error {
	mu.Lock()
	defer mu.Unlock()
	return remove(root)
}

func remove(root string) error {
	if err := os.Remove(Path(root)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func write(root, text string) error {
	f := Path(root)
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return err
	}
	excludeFromGit(root)
	tmp := f + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.TrimRight(text, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, f)
}

// excludeFromGit: «.pgocode/» en .git/info/exclude (como los adjuntos), para que la memoria no acabe en un commit.
func excludeFromGit(root string) {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return
	}
	f := filepath.Join(strings.TrimSpace(string(out)), "info", "exclude")
	if b, _ := os.ReadFile(f); strings.Contains(string(b), "\n.pgocode/") || strings.HasPrefix(string(b), ".pgocode/") {
		return
	}
	os.MkdirAll(filepath.Dir(f), 0o755)
	if fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fh.WriteString(".pgocode/\n")
		fh.Close()
	}
}

var comments = regexp.MustCompile(`(?s)<!--.*?-->`)

// Body: lo que se le da al agente: sin el título ni los comentarios ni las secciones vacías.
func Body(text string) string {
	d := parse(comments.ReplaceAllString(text, ""))
	var b strings.Builder
	for _, s := range d {
		lines := trimBlank(s.lines)
		if s.title == "" && len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "# ") {
			lines = trimBlank(lines[1:])
		}
		if len(lines) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		if s.title != "" {
			b.WriteString("## " + s.title + "\n")
		}
		b.WriteString(strings.Join(lines, "\n") + "\n")
	}
	return strings.TrimSpace(b.String())
}

// Tokens: estimación de lo que ocupa la memoria en el contexto del agente.
func Tokens(text string) int {
	n := utf8.RuneCountInString(Body(text))
	return (n*2 + 6) / 7
}

// ForAgent: el bloque que se añade al final del mensaje del agente cuando empieza de cero («» si no hay memoria).
// Al final del mensaje, para no romper el prefijo en caché; acotado a MaxChars aunque el usuario la haya alargado.
func ForAgent(root string) string {
	body := Body(Load(root))
	if body == "" {
		return ""
	}
	if utf8.RuneCountInString(body) > MaxChars {
		body = string([]rune(body)[:MaxChars-1]) + "…"
	}
	return "\n\n[Memoria del proyecto (pgocode, .pgocode/memoria.md): lo acordado y comprobado en sesiones anteriores; úsalo directamente, sin volver a buscarlo en los ficheros salvo que algo falle]\n" + body + "\n[Fin de la memoria del proyecto]"
}

// ---- Documento: secciones «## título» con sus líneas ----

type section struct {
	title string // «» para lo que va antes de la primera sección
	lines []string
}

func parse(text string) []section {
	d := []section{{}}
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if t, ok := strings.CutPrefix(strings.TrimSpace(l), "## "); ok {
			d = append(d, section{title: strings.TrimSpace(t)})
			continue
		}
		d[len(d)-1].lines = append(d[len(d)-1].lines, l)
	}
	return d
}

func render(d []section) string {
	var b strings.Builder
	for i, s := range d {
		lines := trimBlank(s.lines)
		if s.title == "" {
			if len(lines) > 0 {
				b.WriteString(strings.Join(lines, "\n") + "\n")
			}
			continue
		}
		if len(lines) == 0 {
			continue
		}
		if i > 0 && b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## " + s.title + "\n" + strings.Join(lines, "\n") + "\n")
	}
	return b.String()
}

func trimBlank(ls []string) []string {
	for len(ls) > 0 && strings.TrimSpace(ls[0]) == "" {
		ls = ls[1:]
	}
	for len(ls) > 0 && strings.TrimSpace(ls[len(ls)-1]) == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}

func item(l string) (string, bool) {
	t := strings.TrimSpace(l)
	if s, ok := strings.CutPrefix(t, "- "); ok {
		return strings.TrimSpace(s), true
	}
	if s, ok := strings.CutPrefix(t, "* "); ok {
		return strings.TrimSpace(s), true
	}
	return "", false
}

func sectionIndex(d []section, title string) int {
	for i, s := range d {
		if strings.EqualFold(s.title, title) {
			return i
		}
	}
	return -1
}

// ---- Actualización ----

// Update: añade a la memoria lo nuevo de un turno (frases del usuario ya filtradas con Facts y órdenes con
// Command), sin duplicados, y la deja dentro del tope (fuera lo más antiguo de lo que mantiene pgocode). Devuelve
// si ha cambiado.
func Update(root string, facts []string, commands []CommandEntry) (bool, error) {
	if len(facts) == 0 && len(commands) == 0 {
		return false, nil
	}
	mu.Lock()
	defer mu.Unlock()
	old := load(root)
	text := old
	if strings.TrimSpace(text) == "" {
		text = header
	}
	text = merge(text, facts, commands)
	if text == old {
		return false, nil
	}
	return true, write(root, text)
}

func merge(text string, facts []string, commands []CommandEntry) string {
	d := parse(text)
	add := func(title string) int {
		if i := sectionIndex(d, title); i >= 0 {
			return i
		}
		d = append(d, section{title: title})
		return len(d) - 1
	}
	for _, f := range facts {
		if !replaceFact(d, f) {
			i := add(SectionFacts)
			d[i].lines = append(trimBlank(d[i].lines), "- "+f)
		}
	}
	for _, c := range commands {
		placeCommand(d, add(SectionCommands), c)
	}
	// Tope: fuera lo más antiguo (lo primero) de las secciones de pgocode; primero las órdenes viejas no, los datos:
	// las órdenes son pocas y cortas. Si con eso no basta (el usuario la ha alargado), se queda como está.
	for utf8.RuneCountInString(Body(render(d))) > MaxChars {
		if !dropOldest(d, SectionFacts) && !dropOldest(d, SectionCommands) {
			break
		}
	}
	return render(d)
}

func dropOldest(d []section, title string) bool {
	i := sectionIndex(d, title)
	if i < 0 {
		return false
	}
	for k, l := range d[i].lines {
		if _, ok := item(l); ok {
			d[i].lines = append(d[i].lines[:k:k], d[i].lines[k+1:]...)
			return true
		}
	}
	return false
}

// replaceFact: una frase nueva sustituye a la que dice lo mismo (o lo mismo con otro valor: «el puerto es 3006» a
// «el puerto es 3005»), esté en la sección que esté (si no hay ninguna, false: va al final de «Decisiones y datos»).
func replaceFact(d []section, f string) bool {
	for si := range d {
		for li, l := range d[si].lines {
			if x, ok := item(l); ok && sameFact(x, f) {
				if strings.Join(words(x), " ") != strings.Join(words(f), " ") { // lo mismo con otras mayúsculas o signos: se queda
					d[si].lines[li] = "- " + f
				}
				return true
			}
		}
	}
	return false
}

func placeCommand(d []section, si int, c CommandEntry) {
	prefix := c.Kind + ": `" + c.head() // misma clase y programa: se queda la más general (la más corta)
	for li, l := range d[si].lines {
		x, ok := item(l)
		if !ok || !strings.HasPrefix(x, prefix) {
			continue
		}
		if cmd := strings.TrimSuffix(strings.TrimPrefix(x, c.Kind+": `"), "`"); len(c.Cmd) < len(cmd) {
			d[si].lines[li] = "- " + c.String()
		}
		return
	}
	d[si].lines = append(trimBlank(d[si].lines), "- "+c.String())
}

// ---- Parecido entre frases ----

func words(s string) []string {
	s = strings.ToLower(stripAccents(s))
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' })
}

func stripAccents(s string) string {
	return strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U").Replace(s)
}

func hasDigit(w string) bool { return strings.ContainsFunc(w, unicode.IsDigit) }

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := 0
	for w := range a {
		if b[w] {
			n++
		}
	}
	return float64(n) / float64(len(a)+len(b)-n)
}

// stop: palabras que no distinguen una frase de otra.
var stop = map[string]bool{"el": true, "la": true, "los": true, "las": true, "de": true, "del": true, "en": true, "y": true, "a": true,
	"que": true, "se": true, "un": true, "una": true, "con": true, "por": true, "para": true, "es": true, "al": true, "lo": true,
	"the": true, "is": true, "of": true, "to": true, "ahora": true}

func wordSet(s string, digits bool) map[string]bool {
	m := map[string]bool{}
	for _, w := range words(s) {
		if !stop[w] && (digits || !hasDigit(w)) {
			m[w] = true
		}
	}
	return m
}

// sameFact: dos frases dicen lo mismo (casi las mismas palabras) o lo mismo con otro valor (las mismas palabras
// salvo números: el puerto, una versión…).
func sameFact(a, b string) bool {
	if jaccard(wordSet(a, true), wordSet(b, true)) >= 0.8 {
		return true
	}
	wa, wb := wordSet(a, false), wordSet(b, false)
	return len(wa) >= 1 && len(wa) <= 6 && jaccard(wa, wb) >= 0.75 && digitsOf(a) != digitsOf(b) && digitsOf(a) != "" && digitsOf(b) != ""
}

func digitsOf(s string) string {
	var ds []string
	for _, w := range words(s) {
		if hasDigit(w) {
			ds = append(ds, w)
		}
	}
	return strings.Join(ds, " ")
}
