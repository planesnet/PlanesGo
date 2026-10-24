package bugreport

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// fakeGitHub keeps the issues in memory.
type fakeGitHub struct {
	open     map[int]string // number → body
	comments map[int][]string
	next     int
	canUse   bool
}

func newFake() *fakeGitHub {
	return &fakeGitHub{open: map[int]string{}, comments: map[int][]string{}, next: 100, canUse: true}
}

func (f *fakeGitHub) FindOpenIssue(_ context.Context, _, word string) (int, error) {
	for n, b := range f.open {
		if strings.Contains(b, word) {
			return n, nil
		}
	}
	return 0, nil
}

func (f *fakeGitHub) OpenReportIssue(_ context.Context, repo, title, body string) (int, string, error) {
	f.next++
	f.open[f.next] = body
	return f.next, "https://github.com/" + repo + "/issues/" + title, nil
}

func (f *fakeGitHub) CommentIssue(_ context.Context, _ string, n int, body string) error {
	f.comments[n] = append(f.comments[n], body)
	return nil
}

func (f *fakeGitHub) CanUse(context.Context, string) bool { return f.canUse }

func newReporter(t *testing.T, gh *fakeGitHub, version string) (*Reporter, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 24, 10, 0, 0, 0, time.Local)
	r := New(t.TempDir(), version, gh, nil)
	r.Now = func() time.Time { return now }
	return r, &now
}

func TestFingerprintIgnoresValuesAndLines(t *testing.T) {
	a := Report{Source: FromInterface, Message: `Cannot read properties of undefined (reading 'id') at 12`,
		Stack: "TypeError\n    at planesgo-code/web/src/componentes/Chat.tsx:120:4 (runPsf)"}
	b := Report{Source: FromInterface, Message: `Cannot read properties of undefined (reading 'nombre') at 99`,
		Stack: "TypeError\n    at planesgo-code/web/src/componentes/Chat.tsx:131:9 (runPsf)"}
	if Fingerprint(a) != Fingerprint(b) {
		t.Error("the same failure in another line or with other values must keep its fingerprint")
	}
	min1 := Report{Source: FromInterface, Message: "Uncaught TypeError: x.map is not a function", Stack: a.Stack}
	min2 := Report{Source: FromInterface, Message: "TypeError: Ve.map is not a function", Stack: b.Stack}
	if Fingerprint(min1) != Fingerprint(min2) {
		t.Error("interface: the minified names change with each build; the place and the class of the error stay")
	}
	if Fingerprint(min1) == Fingerprint(Report{Source: FromInterface, Message: "ReferenceError: q is not defined", Stack: a.Stack}) {
		t.Error("another class of error in the same place is another failure")
	}
	c := b
	c.Stack = "TypeError\n    at planesgo-code/web/src/componentes/Flujo.tsx:131:9 (pinta)"
	if Fingerprint(c) == Fingerprint(b) {
		t.Error("another place is another failure")
	}
	g1 := Report{Source: FromServer, Message: "runtime error: index out of range [3] with length 3",
		Stack: "goroutine 7 [running]:\nplanesgo-code/internal/servidor.(*Servidor).psfSync(0xc000123, {0x1, 0x2})\n\t/x/psf.go:120 +0x1c"}
	g2 := g1
	g2.Message = "runtime error: index out of range [5] with length 5"
	g2.Stack = strings.ReplaceAll(g1.Stack, "0xc000123", "0xc000999")
	if Fingerprint(g1) != Fingerprint(g2) {
		t.Error("Go: other indexes or addresses, same failure")
	}
}

func TestExternalFailuresAreIgnored(t *testing.T) {
	for _, m := range []string{"Failed to fetch", "NetworkError when attempting to fetch resource.", "ResizeObserver loop completed with undelivered notifications.", "Script error.", ""} {
		if !External(Report{Message: m}) {
			t.Errorf("%q is not a failure of pgocode", m)
		}
	}
	if External(Report{Message: "x is not a function"}) {
		t.Error("an exception of pgocode is reported")
	}
}

func TestCleanRemovesSecretsAndHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	r, _ := newReporter(t, newFake(), "0.5.9")
	c := r.clean(Report{Message: "fallo con token ghp_0123456789abcdefghijklmnopqrstuvwxyz en " + home + "/proyecto", Stack: strings.Repeat("x", 9000)})
	if strings.Contains(c.Message, "ghp_0123") || (home != "" && strings.Contains(c.Message, home)) || !strings.Contains(c.Message, "~/proyecto") {
		t.Errorf("message not clean: %q", c.Message)
	}
	if len(c.Stack) > maxStack+4 {
		t.Errorf("stack not bounded: %d", len(c.Stack))
	}
}

func TestOneIssueThenDailyComments(t *testing.T) {
	gh := newFake()
	r, now := newReporter(t, gh, "0.5.9")
	r.st.Enabled = ptr(true)
	rep := Report{Source: FromInterface, Message: "x is not a function", Where: "/?proyecto=7"}
	r.send(rep)
	r.send(rep) // same day: only counted
	if len(gh.open) != 1 || len(gh.comments) != 0 {
		t.Fatalf("issues %d, comments %d", len(gh.open), len(gh.comments))
	}
	for n, body := range gh.open {
		if !strings.Contains(body, "pgoerr") || !strings.Contains(body, "0.5.9") || !strings.Contains(body, "x is not a function") {
			t.Errorf("body #%d: %s", n, body)
		}
	}
	*now = now.Add(25 * time.Hour)
	r.send(rep)
	if len(gh.open) != 1 || len(gh.comments[101]) != 1 || !strings.Contains(gh.comments[101][0], "3 veces") {
		t.Fatalf("the repetition must be a comment: %v", gh.comments)
	}
	// Closed since (fixed?) and back: a new issue.
	delete(gh.open, 101)
	*now = now.Add(25 * time.Hour)
	r.send(rep)
	if len(gh.open) != 1 || gh.open[102] == "" {
		t.Fatalf("a failure that is back opens a new issue: %v", gh.open)
	}
	if k := r.Recent(5); len(k) != 1 || k[0].Issue != 102 || k[0].Count != 4 {
		t.Errorf("recent: %+v", k)
	}
}

func TestDailyLimitAndDisabled(t *testing.T) {
	gh := newFake()
	r, _ := newReporter(t, gh, "0.5.9")
	for i := 0; i < MaxNewPerDay+3; i++ {
		r.send(Report{Source: FromServer, Message: "fallo", Stack: "planesgo-code/internal/x.F" + string(rune('a'+i)) + "(0x1)"})
	}
	if len(gh.open) != MaxNewPerDay {
		t.Errorf("%d new issues in a day", len(gh.open))
	}
	gh2 := newFake()
	r2, _ := newReporter(t, gh2, "dev") // development build and no choice: off
	r2.send(Report{Source: FromServer, Message: "fallo"})
	gh3 := newFake()
	gh3.canUse = false
	r3, _ := newReporter(t, gh3, "0.5.9") // no access to the repository: off
	r3.send(Report{Source: FromServer, Message: "fallo"})
	if len(gh2.open)+len(gh3.open) != 0 {
		t.Error("off by default in development builds and without access")
	}
	r3.SetEnabled(true)
	if on, chosen := r3.Enabled(context.Background()); !on || !chosen {
		t.Error("the choice of the person wins")
	}
}

func TestParseCrash(t *testing.T) {
	out := "panic: assignment to entry in nil map\n\ngoroutine 42 [running]:\nplanesgo-code/internal/servidor.(*Servidor).psfProgramPictures(0xc0001, {0x2})\n\t/b/psf_evidence.go:40 +0x5\ncreated by planesgo-code/internal/servidor.(*Servidor).psfSync in goroutine 9\n\ngoroutine 1 [select]:\nmain.main()\n"
	rep, ok := ParseCrash(out)
	if !ok || rep.Message != "assignment to entry in nil map" || rep.Source != FromCrash || strings.Contains(rep.Stack, "goroutine 1 ") {
		t.Fatalf("%v %+v", ok, rep)
	}
	if f := ownFrames(rep.Stack, 3); len(f) == 0 || f[0] != "planesgo-code/internal/servidor.(*Servidor).psfProgramPictures" {
		t.Errorf("frames: %v", f)
	}
	if _, ok := ParseCrash("cerrando Planes Go Code…\n"); ok {
		t.Error("a normal log is not a crash")
	}
}

func TestSourceMapResolvesFrames(t *testing.T) {
	// Generated line 1: column 0 → src/a.ts:1:1; column 10 → src/b.tsx:5:3 with the name «render».
	seg := func(v ...int) string {
		s := ""
		for _, n := range v {
			s += encodeVLQ(n)
		}
		return s
	}
	mappings := seg(0, 0, 0, 0) + "," + seg(10, 1, 4, 2, 0)
	m := `{"version":3,"sources":["../../src/a.ts","../../src/b.tsx"],"names":["render"],"mappings":"` + mappings + `"}`
	fs := fstest.MapFS{"assets/index-X.js.map": {Data: []byte(m)}}
	maps := NewMaps(fs, "planesgo-code/web/dist/assets")
	got := maps.Resolve("TypeError: x\n    at f (http://127.0.0.1:7801/assets/index-X.js:1:15)\n    at g (http://127.0.0.1:7801/assets/otro.js:1:1)")
	if !strings.Contains(got, "planesgo-code/web/src/b.tsx:5:3 (render)") || !strings.Contains(got, "/assets/otro.js:1:1") {
		t.Errorf("resolved:\n%s", got)
	}
	if NewMaps(nil, "").Resolve("a") != "a" {
		t.Error("without the interface, the stack stays")
	}
}

func encodeVLQ(n int) string {
	v := n << 1
	if n < 0 {
		v = (-n << 1) | 1
	}
	s := ""
	for {
		d := v & 31
		v >>= 5
		if v > 0 {
			d |= 32
		}
		s += string(base64VLQ[d])
		if v == 0 {
			return s
		}
	}
}

func ptr[T any](v T) *T { return &v }
