// Package bugreport turns the failures of pgocode itself (exceptions of the interface, panics of the server and
// crashes) into GitHub issues of the PlanesGo repository, where the automatic PSF sessions can pick them up.
//
// Only pgocode's own failures: what fails outside (the network, Odoo, GitHub, the agents) is not reported, as an
// agent could not fix it. Each failure has a fingerprint (its normalised message and the code it comes from): if an
// open issue already has it, the repetition becomes a comment, at most once a day; at most MaxNewPerDay new issues
// a day. Secrets and the home folder are removed before anything leaves the computer. Reporting never blocks nor
// fails pgocode: it runs in its own goroutine and its errors only go to the log.
package bugreport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/depuracion"
)

// Repo: where the failures go (the PLANESGO project); a development instance can use another (Reporter.Repo).
const Repo = "planesnet/PlanesGo"

const (
	MaxNewPerDay = 5
	repeatEvery  = 24 * time.Hour
	queueSize    = 20
	maxMessage   = 500
	maxStack     = 6000
)

// Sources of a failure.
const (
	FromInterface = "interfaz" // an exception of the interface (or a component that cannot be drawn)
	FromServer    = "servidor" // a panic while serving a request
	FromCrash     = "cierre"   // the backend died of a panic (reported on the next start)
)

// Report: one failure of pgocode.
type Report struct {
	Source  string
	Message string
	Stack   string
	Where   string // the screen or the request
}

// Issuer is what the reports need from GitHub (github.Lector).
type Issuer interface {
	FindOpenIssue(ctx context.Context, repo, word string) (int, error)
	OpenReportIssue(ctx context.Context, repo, title, body string) (int, string, error)
	CommentIssue(ctx context.Context, repo string, n int, body string) error
	CanUse(ctx context.Context, repo string) bool
}

// Known: a failure already seen on this computer.
type Known struct {
	Title    string    `json:"title"`
	Issue    int       `json:"issue,omitempty"`
	URL      string    `json:"url,omitempty"`
	Count    int       `json:"count"`
	First    time.Time `json:"first"`
	Last     time.Time `json:"last"`
	LastSent time.Time `json:"last_sent,omitempty"`
}

type state struct {
	Enabled  *bool            `json:"enabled,omitempty"` // nil: the default (see Reporter.Enabled)
	Day      string           `json:"day,omitempty"`
	NewToday int              `json:"new_today,omitempty"`
	Known    map[string]Known `json:"known"`
}

// Reporter sends the reports of one pgocode installation.
type Reporter struct {
	Repo    string
	Issuer  Issuer
	Version string
	Maps    *Maps            // resolves the frames of the interface (nil: left as they are)
	Now     func() time.Time // tests

	file  string
	queue chan Report
	mu    sync.Mutex
	st    state
	// whether the gh account can use Repo (checked at most once an hour; it decides the default)
	access      bool
	accessFresh time.Time
}

// New loads the state from dir and starts the goroutine that sends the reports.
func New(dir, version string, is Issuer, maps *Maps) *Reporter {
	r := &Reporter{Repo: Repo, Issuer: is, Version: version, Maps: maps, Now: time.Now, file: filepath.Join(dir, "errores.json"),
		queue: make(chan Report, queueSize), st: state{Known: map[string]Known{}}}
	if b, err := os.ReadFile(r.file); err == nil {
		json.Unmarshal(b, &r.st)
	}
	if r.st.Known == nil {
		r.st.Known = map[string]Known{}
	}
	go func() {
		for rep := range r.queue {
			r.send(rep)
		}
	}()
	return r
}

// Report queues a failure; it never blocks (with the queue full, the failure is only logged).
func (r *Reporter) Report(rep Report) {
	if r == nil || External(rep) {
		return
	}
	select {
	case r.queue <- rep:
	default:
		log.Printf("bugreport: cola llena; no se informa de %q", rep.Message)
	}
}

// Enabled: the choice of the person or, without one, on when the gh account can use the repository and this is
// not a development build (there the failures are usually the code being written).
func (r *Reporter) Enabled(ctx context.Context) (on, chosen bool) {
	r.mu.Lock()
	e := r.st.Enabled
	r.mu.Unlock()
	if e != nil {
		return *e, true
	}
	return r.Version != "dev" && r.CanUse(ctx), false
}

// CanUse: whether the gh account of this computer can open issues in r.Repo (cached for an hour).
func (r *Reporter) CanUse(ctx context.Context) bool {
	r.mu.Lock()
	if r.Now().Sub(r.accessFresh) < time.Hour {
		defer r.mu.Unlock()
		return r.access
	}
	r.mu.Unlock()
	ok := r.Issuer.CanUse(ctx, r.Repo)
	r.mu.Lock()
	r.access, r.accessFresh = ok, r.Now()
	r.mu.Unlock()
	return ok
}

// SetEnabled stores the choice of the person.
func (r *Reporter) SetEnabled(on bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.st.Enabled = &on
	return r.save()
}

// Recent: the failures seen on this computer, the latest first.
func (r *Reporter) Recent(n int) []Known {
	r.mu.Lock()
	defer r.mu.Unlock()
	l := make([]Known, 0, len(r.st.Known))
	for _, k := range r.st.Known {
		l = append(l, k)
	}
	sort.Slice(l, func(i, j int) bool { return l[i].Last.After(l[j].Last) })
	if len(l) > n {
		l = l[:n]
	}
	return l
}

// send: the issue of the failure (new, or a comment on the open one), within the limits.
func (r *Reporter) send(rep Report) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if on, _ := r.Enabled(ctx); !on {
		return
	}
	rep = r.clean(rep)
	fp := Fingerprint(rep)
	now := r.Now()
	r.mu.Lock()
	k := r.st.Known[fp]
	if k.First.IsZero() {
		k.First, k.Title = now, Title(rep)
	}
	k.Count++
	k.Last = now
	due := now.Sub(k.LastSent) >= repeatEvery
	if day := now.Format("2006-01-02"); r.st.Day != day {
		r.st.Day, r.st.NewToday = day, 0
	}
	full := r.st.NewToday >= MaxNewPerDay
	r.st.Known[fp] = k
	r.save()
	r.mu.Unlock()
	if !due {
		return
	}
	// The open issue of this failure: maybe opened from another computer, or closed since (then a new one: it is
	// back).
	n, err := r.Issuer.FindOpenIssue(ctx, r.Repo, Word(fp))
	if err != nil {
		log.Printf("bugreport: %v", err)
		return
	}
	url := k.URL
	if n > 0 {
		if n != k.Issue {
			url = fmt.Sprintf("https://github.com/%s/issues/%d", r.Repo, n)
		}
		err = r.Issuer.CommentIssue(ctx, r.Repo, n, RepeatBody(rep, k, r.Version))
	} else if full {
		log.Printf("bugreport: ya hay %d issues nuevas hoy; no se abre la de %q", MaxNewPerDay, rep.Message)
		return
	} else {
		n, url, err = r.Issuer.OpenReportIssue(ctx, r.Repo, Title(rep), Body(rep, fp, r.Version, now))
		if err == nil {
			r.mu.Lock()
			r.st.NewToday++
			r.mu.Unlock()
		}
	}
	if err != nil {
		log.Printf("bugreport: %v", err)
		return
	}
	log.Printf("bugreport: fallo informado en %s", url)
	r.mu.Lock()
	k = r.st.Known[fp]
	k.Issue, k.URL, k.LastSent = n, url, now
	r.st.Known[fp] = k
	r.save()
	r.mu.Unlock()
}

// save writes the state (with r.mu held).
func (r *Reporter) save() error {
	b, _ := json.MarshalIndent(r.st, "", " ")
	if err := os.WriteFile(r.file+".tmp", b, 0o600); err != nil {
		return err
	}
	return os.Rename(r.file+".tmp", r.file)
}

// clean: the frames of the interface resolved, no secrets, no home folder, and sizes bounded.
func (r *Reporter) clean(rep Report) Report {
	rep.Stack = r.Maps.Resolve(rep.Stack)
	home, _ := os.UserHomeDir()
	for _, p := range []*string{&rep.Message, &rep.Stack, &rep.Where} {
		t := depuracion.Tapa(*p)
		if home != "" {
			t = strings.ReplaceAll(t, home, "~")
		}
		*p = strings.TrimSpace(t)
	}
	rep.Message = cut(rep.Message, maxMessage)
	rep.Stack = cut(rep.Stack, maxStack)
	rep.Where = cut(rep.Where, 200)
	return rep
}

func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8Start(s[n]) {
		n--
	}
	return s[:n] + "…"
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// external: failures that come from outside pgocode (an agent could not fix them in its code).
var external = regexp.MustCompile(`(?i)failed to fetch|networkerror|load failed|network request failed|aborterror|the user aborted|` +
	`resizeobserver loop|^script error\.?$|chrome-extension://|moz-extension://|connection reset|broken pipe|context canceled`)

// External: whether the failure comes from outside pgocode.
func External(rep Report) bool {
	return strings.TrimSpace(rep.Message) == "" || external.MatchString(rep.Message) || external.MatchString(firstLine(rep.Stack))
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}

var (
	numbers    = regexp.MustCompile(`\b0x[0-9a-fA-F]+\b|\d+`)
	quoted     = regexp.MustCompile(`"[^"]*"|'[^']*'|«[^»]*»|` + "`[^`]*`")
	errorClass = regexp.MustCompile(`^[A-Za-z]*Error\b`)
	// interface frames once resolved, without lines or columns (they change from one version to the next)
	jsFrame = regexp.MustCompile(`(planesgo-code/web/src/[\w./-]+)(?::\d+)+(?: \(([\w$]+)\))?`)
)

// Fingerprint: the same failure gives the same fingerprint in every version and computer: the source, the message
// without numbers or quoted values, and the first three frames of pgocode's own code. In the interface, with frames
// of its own, only the class of the error: the message carries minified names, which change with every build
// («x.map is not a function» → «Ve.map is not a function»).
func Fingerprint(rep Report) string {
	frames := ownFrames(rep.Stack, 3)
	msg := quoted.ReplaceAllString(rep.Message, "…")
	msg = numbers.ReplaceAllString(msg, "0")
	if rep.Source == FromInterface && len(frames) > 0 {
		msg = errorClass.FindString(strings.TrimPrefix(rep.Message, "Uncaught "))
	}
	parts := []string{rep.Source, strings.ToLower(strings.TrimSpace(msg))}
	parts = append(parts, frames...)
	h := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(h[:])[:16]
}

// ownFrames: up to n frames of pgocode's code (server functions or interface files), the reporting ones excluded.
func ownFrames(stack string, n int) []string {
	var l []string
	for _, line := range strings.Split(stack, "\n") {
		// Go: «planesgo-code/internal/x.(*T).F(0xc000…)»: the function without its arguments
		i := strings.LastIndex(line, "(")
		if !strings.HasPrefix(line, "planesgo-code/") || i < 0 || strings.Contains(line, "bugreport") || strings.Contains(line, ".recovers") {
			continue
		}
		l = append(l, line[:i])
	}
	for _, m := range jsFrame.FindAllStringSubmatch(stack, -1) {
		l = append(l, strings.TrimSpace(m[1]+" "+m[2]))
	}
	if len(l) > n {
		l = l[:n]
	}
	return l
}

// Word: the fingerprint as the one word the issue body carries, to find it with a GitHub search.
func Word(fp string) string { return "pgoerr" + fp }

var sourceName = map[string]string{FromInterface: "Interfaz", FromServer: "Servidor (petición)", FromCrash: "Servidor (cierre por pánico)"}

// Title of the issue of a failure.
func Title(rep Report) string {
	return "Fallo de pgocode: " + cut(firstLine(rep.Message), 90)
}

// Body of the new issue: the data of the failure and what the automatic session has to do with it.
func Body(rep Report, fp, version string, when time.Time) string {
	var b strings.Builder
	b.WriteString("pgocode ha detectado un fallo propio y ha abierto esta issue de forma automática.\n\n")
	b.WriteString("| | |\n|---|---|\n")
	fmt.Fprintf(&b, "| Origen | %s |\n", sourceName[rep.Source])
	if rep.Where != "" {
		fmt.Fprintf(&b, "| Dónde | `%s` |\n", strings.ReplaceAll(rep.Where, "`", "'"))
	}
	fmt.Fprintf(&b, "| Versión | %s |\n| Sistema | %s/%s |\n| Primera vez | %s |\n\n", version, runtime.GOOS, runtime.GOARCH, when.Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "### Error\n\n```\n%s\n```\n\n", fence(rep.Message))
	if rep.Stack != "" {
		fmt.Fprintf(&b, "### Pila\n\n```\n%s\n```\n\n", fence(rep.Stack))
	}
	b.WriteString("### Qué hay que hacer\n\n" +
		"Busca la causa en el código de `planesgo-code` (la pila apunta a los ficheros), corrígela y añade un test que la cubra. " +
		"Si la versión es antigua, comprueba primero si ya está corregido en la rama principal. " +
		"Si no se puede reproducir o la causa está fuera de pgocode (la red, Odoo, GitHub o un agente), explícalo en un comentario y no cambies el código.\n\n")
	fmt.Fprintf(&b, "<sub>Huella %s · si el fallo se repite, pgocode lo comenta aquí (como mucho una vez al día).</sub>\n", Word(fp))
	return b.String()
}

// RepeatBody: the comment when the failure happens again.
func RepeatBody(rep Report, k Known, version string) string {
	where := ""
	if rep.Where != "" {
		where = fmt.Sprintf(" en `%s`", strings.ReplaceAll(rep.Where, "`", "'"))
	}
	return fmt.Sprintf("Se ha repetido%s (versión %s, %s/%s): %d %s en este equipo desde el %s.",
		where, version, runtime.GOOS, runtime.GOARCH, k.Count, map[bool]string{true: "vez", false: "veces"}[k.Count == 1], k.First.Format("02/01/2006"))
}

// fence keeps the text from closing its code block.
func fence(s string) string { return strings.ReplaceAll(s, "```", "'''") }

// WatchCrashes: a crash of the previous run (the runtime wrote it in file) becomes a report, and the runtime is told
// to write the next one there (a panic in any goroutine kills the backend before anything else can see it).
func (r *Reporter) WatchCrashes(file string) error {
	if b, err := os.ReadFile(file); err == nil {
		if rep, ok := ParseCrash(string(b)); ok {
			log.Printf("bugreport: la ejecución anterior acabó con un pánico: %s", rep.Message)
			r.Report(rep)
		}
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close() // the runtime keeps its own copy of the descriptor
	return debug.SetCrashOutput(f, debug.CrashOptions{})
}

// ParseCrash reads the output of a Go crash: the message of the panic and the stack of the goroutine that failed.
func ParseCrash(out string) (Report, bool) {
	var msg string
	for _, l := range strings.Split(out, "\n") {
		if m, ok := strings.CutPrefix(l, "panic: "); ok {
			msg = strings.TrimSuffix(m, " [recovered]")
			break
		}
		if m, ok := strings.CutPrefix(l, "fatal error: "); ok {
			msg = m
			break
		}
	}
	i := strings.Index(out, "\ngoroutine ")
	if msg == "" || i < 0 {
		return Report{}, false
	}
	stack := out[i+1:]
	if j := strings.Index(stack, "\n\n"); j > 0 {
		stack = stack[:j] // only the goroutine that failed
	}
	return Report{Source: FromCrash, Message: msg, Stack: stack}, true
}
