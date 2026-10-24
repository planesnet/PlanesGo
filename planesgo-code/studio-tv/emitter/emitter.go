// Package emitter sends a test's picture and state to a Studio TV (POST /api/frame and /api/estado), for tests
// written in Go. It is optional and never slows the test down: without a studio it does nothing, and when the
// studio is slow, frames are dropped (only the latest one waits to be sent).
//
// The studio comes from PCO_URL and PCO_TOKEN (pgocode passes them to the QA campaigns it launches) or, if they are
// missing, from ~/.config/pgocode/studio_tv.env (pgocode writes it pointing to its local studio). PCO_CANAL picks
// the channel; without it (or «auto»), qa-<repository>-<branch> of the folder the test runs in, so several tests
// can be on air at once. PCO_INSEGURO=1 accepts a studio's own certificate without checking it.
package emitter

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ConfigFile: where pgocode leaves the local studio's address and token.
func ConfigFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "pgocode", "studio_tv.env")
}

// Config: the studio and the channel.
type Config struct {
	URL, Token, Channel string
	Insecure            bool
}

// LoadConfig: the environment first, the configuration file for what is missing.
func LoadConfig() Config {
	c := Config{URL: os.Getenv("PCO_URL"), Token: os.Getenv("PCO_TOKEN"), Channel: os.Getenv("PCO_CANAL"), Insecure: os.Getenv("PCO_INSEGURO") != ""}
	if c.URL != "" && c.Token != "" {
		return c
	}
	f, err := os.Open(ConfigFile())
	if err != nil {
		return c
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || v == "" {
			continue
		}
		switch k {
		case "PCO_URL":
			c.URL = firstNonEmpty(c.URL, v)
		case "PCO_TOKEN":
			c.Token = firstNonEmpty(c.Token, v)
		case "PCO_CANAL":
			c.Channel = firstNonEmpty(c.Channel, v)
		}
	}
	return c
}

func firstNonEmpty(have, other string) string {
	if have != "" {
		return have
	}
	return other
}

var unsafeChannel = regexp.MustCompile(`[^a-z0-9._-]+`)

// DefaultChannel: qa-<repository>-<branch> of the git repository of dir («principal» outside one). The repository
// is the name of origin (planesnet/madpla.git → madpla) or of the repository's folder.
func DefaultChannel(dir string) string {
	git := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	root := git("rev-parse", "--show-toplevel")
	if root == "" {
		return "principal"
	}
	repo := filepath.Base(root)
	if origin := git("remote", "get-url", "origin"); origin != "" {
		origin = strings.TrimRight(origin, "/")
		origin = origin[strings.LastIndexAny(origin, "/:")+1:]
		repo = strings.TrimSuffix(origin, ".git")
	}
	branch := git("branch", "--show-current")
	if branch == "" {
		branch = firstNonEmpty(git("rev-parse", "--short", "HEAD"), "sin-rama")
	}
	ch := strings.Trim(unsafeChannel.ReplaceAllString(strings.ToLower("qa-"+repo+"-"+branch), "-"), "-")
	if len(ch) > 64 {
		ch = ch[:64]
	}
	return firstNonEmpty(ch, "principal")
}

// Emitter sends to a studio. The zero value and one without a studio do nothing.
type Emitter struct {
	cfg      Config
	query    string
	client   *http.Client
	minGap   time.Duration
	mu       sync.Mutex
	lastSent time.Time
	frame    []byte
	state    []byte
	wake     chan struct{}
	done     chan struct{}
	closed   bool
}

// New: an emitter for cfg (LoadConfig() if empty) at fps frames per second at most; its channel is cfg.Channel or,
// without one or with «auto», DefaultChannel of the current folder.
func New(cfg Config, fps int) *Emitter {
	if cfg == (Config{}) {
		cfg = LoadConfig()
	}
	if cfg.Channel == "" || cfg.Channel == "auto" {
		wd, _ := os.Getwd()
		cfg.Channel = DefaultChannel(wd)
	}
	if fps <= 0 {
		fps = 8
	}
	e := &Emitter{cfg: cfg, minGap: time.Second / time.Duration(fps), wake: make(chan struct{}, 1), done: make(chan struct{})}
	if !e.Active() {
		close(e.done)
		return e
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.Insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // a studio with its own certificate (PCO_INSEGURO)
	}
	e.client = &http.Client{Transport: tr, Timeout: 10 * time.Second}
	e.query = "?canal=" + url.QueryEscape(cfg.Channel)
	go e.loop()
	return e
}

// Active: whether there is a studio to send to.
func (e *Emitter) Active() bool { return e != nil && e.cfg.URL != "" && e.cfg.Token != "" }

// Channel the emitter sends to.
func (e *Emitter) Channel() string { return e.cfg.Channel }

// Frame queues a JPEG picture (dropped if the previous one was sent less than 1/fps ago).
func (e *Emitter) Frame(jpeg []byte) {
	if !e.Active() {
		return
	}
	e.mu.Lock()
	if time.Since(e.lastSent) < e.minGap || e.closed {
		e.mu.Unlock()
		return
	}
	e.lastSent, e.frame = time.Now(), append([]byte(nil), jpeg...) // a copy: the caller may reuse its buffer
	e.mu.Unlock()
	e.signal()
}

// State queues the test's state: {caso, titulo, paso, estado, seg, marcadores[{t, txt}]}.
func (e *Emitter) State(v any) {
	if !e.Active() {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	e.mu.Lock()
	if !e.closed {
		e.state = b
	}
	e.mu.Unlock()
	e.signal()
}

// signal wakes the sender (nothing once closed: wake is closed then, and a send on it would panic).
func (e *Emitter) signal() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// Close sends what is pending and stops.
func (e *Emitter) Close() {
	if !e.Active() {
		return
	}
	e.mu.Lock()
	if !e.closed {
		e.closed = true
		close(e.wake) // under the lock: signal never sends on it afterwards
	}
	e.mu.Unlock()
	<-e.done
}

func (e *Emitter) loop() {
	defer close(e.done)
	for range e.wake {
		e.flush()
	}
	e.flush()
}

func (e *Emitter) flush() {
	e.mu.Lock()
	state, frame := e.state, e.frame
	e.state, e.frame = nil, nil
	e.mu.Unlock()
	if state != nil {
		e.post("/api/estado", "application/json", state)
	}
	if frame != nil {
		e.post("/api/frame", "image/jpeg", frame)
	}
}

// post sends one request, trying once more if it fails (a studio that went away is not the test's problem).
func (e *Emitter) post(path, typ string, body []byte) {
	for range 2 {
		req, err := http.NewRequest(http.MethodPost, strings.TrimRight(e.cfg.URL, "/")+path+e.query, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Authorization", "Bearer "+e.cfg.Token)
		req.Header.Set("Content-Type", typ)
		res, err := e.client.Do(req)
		if err == nil {
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			return
		}
	}
}
