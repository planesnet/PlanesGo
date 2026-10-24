package anydesk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Lines taken from real traces (Linux, AnyDesk 7), with IDs, names and PIDs changed.

func feedFile(t *testing.T, tr *Tracker, name string) []Change {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var ch []Change
	for _, l := range strings.Split(string(b), "\n") {
		ch = append(ch, tr.Feed(l)...)
	}
	return ch
}

func TestParseLine(t *testing.T) {
	l, ok := ParseLine("   info 2026-10-23 13:20:15.521      front   main 4100 4100                     unix_app.frontend - Sending a connection request for address 111222333.\r")
	if !ok || l.Role != "front" || l.PID != 4100 || l.Component != "unix_app.frontend" || l.Level != "info" ||
		l.Message != "Sending a connection request for address 111222333." || l.Time.Format("15:04:05.000") != "13:20:15.521" {
		t.Fatalf("%+v %v", l, ok)
	}
	// with the service's fiber number
	l, ok = ParseLine("   info 2026-09-21 09:59:48.551    service   main   3416   3416   90                      app.session - 5: Entering processing loop.")
	if !ok || l.Component != "app.session" || l.PID != 3416 || l.Message != "5: Entering processing loop." {
		t.Fatalf("%+v %v", l, ok)
	}
	for _, bad := range []string{"t connection failed.", "hola", "info 2026-10-23 x front main 1 1 c - m"} {
		if _, ok := ParseLine(bad); ok {
			t.Errorf("%q no es una línea del registro", bad)
		}
	}
}

func TestOutgoing(t *testing.T) {
	tr := NewTracker("u")
	ch := feedFile(t, tr, "outgoing.trace")
	if len(ch) != 4 {
		t.Fatalf("cambios: %+v", ch)
	}
	if !ch[0].Open || ch[0].Session.Direction != Outgoing || ch[0].Session.Peer != "111222333" || ch[0].Session.PID != 4100 {
		t.Errorf("inicio: %+v", ch[0])
	}
	if ch[1].Open || ch[1].Session.Key != ch[0].Session.Key || ch[1].Time.Sub(ch[0].Time) != 9162*time.Millisecond {
		t.Errorf("fin: %+v", ch[1])
	}
	// the same process, a second session (to a terminal user): another key
	if !ch[2].Open || ch[2].Session.Peer != "111222333" || ch[2].Session.Key == ch[0].Session.Key || ch[3].Open {
		t.Errorf("segunda sesión: %+v", ch[2:])
	}
	if len(tr.Open()) != 0 || tr.Unknown != 0 || tr.Lines != 26 {
		t.Errorf("abiertas %d, desconocidas %d, líneas %d", len(tr.Open()), tr.Unknown, tr.Lines)
	}
}

func TestIncoming(t *testing.T) {
	tr := NewTracker("u")
	ch := feedFile(t, tr, "incoming.trace")
	if len(ch) != 4 {
		t.Fatalf("cambios: %+v", ch)
	}
	s := ch[0].Session
	if !ch[0].Open || s.Direction != Incoming || s.Peer != "444555666" || s.PeerName != "Ana Pruebas" {
		t.Errorf("entrante: %+v", ch[0])
	}
	if ch[1].Open || ch[1].Time.Format("15:04:05") != "10:11:38" {
		t.Errorf("fin: %+v", ch[1])
	}
	// an alias without a name; it ends when the backend shuts down
	if s := ch[2].Session; !ch[2].Open || s.Peer != "equipo-01@ad" || s.PeerName != "" || ch[3].Open {
		t.Errorf("alias: %+v", ch[2:])
	}
}

// The service's lines are not sessions (they are the same ones the user's trace has).
func TestServiceIgnored(t *testing.T) {
	tr := NewTracker("s")
	if ch := feedFile(t, tr, "service.trace"); len(ch) != 0 || tr.Unknown != 0 {
		t.Fatalf("%+v (desconocidas %d)", ch, tr.Unknown)
	}
}

func TestUnknownAndDead(t *testing.T) {
	tr := NewTracker("u")
	tr.Feed("formato nuevo que no se entiende")
	tr.Feed("")
	tr.Feed(" * * * * *")
	if tr.Unknown != 1 || tr.Lines != 1 {
		t.Fatalf("desconocidas %d de %d", tr.Unknown, tr.Lines)
	}
	tr.Feed("   info 2026-10-23 13:20:39.866      front   ctrl 4100 4198                  app.frontend_session - Session started.")
	if o := tr.Open(); len(o) != 1 || o[0].Peer != "" {
		t.Fatalf("sin petición: %+v", o)
	}
	if ch := tr.CloseDead(func(int) bool { return true }, time.Now()); len(ch) != 0 {
		t.Fatal("vivo")
	}
	if ch := tr.CloseDead(func(pid int) bool { return pid != 4100 }, time.Now()); len(ch) != 1 || ch[0].Open || len(tr.Open()) != 0 {
		t.Fatalf("muerto: %+v", ch)
	}
}

func TestValidID(t *testing.T) {
	for in, want := range map[string]string{
		"111222333": "111222333", " 111 222 333 ": "111222333", "1234567890": "1234567890",
		"Equipo-01@AD": "equipo-01@ad", "mi.pc@ad": "mi.pc@ad", "hon-nb-01@s": "hon-nb-01@s",
		// as AnyDesk shows a computer with its user: only the ID (the rest never reaches the command)
		"800 929 247/maderasplanes/administrador": "800929247", "111222333/--plain": "111222333",
	} {
		if got, ok := ValidID(in); !ok || got != want {
			t.Errorf("%q -> %q %v", in, got, ok)
		}
	}
	for _, bad := range []string{"", "-h", "--plain", "12345", "1234567890123", "12345/dom/usu", "pc@ad/x", "111222333.", "a b@ad",
		"x;rm -rf ~@ad", "$(id)@ad", "pc@", "@ad", "-pc@ad", "pc@ad --x", "111222333 --plain"} {
		if got, ok := ValidID(bad); ok {
			t.Errorf("%q aceptado como %q", bad, got)
		}
	}
}

func TestNormalizePeer(t *testing.T) {
	for in, want := range map[string]string{"111222333.": "111222333", "'111222333/dom/usu'": "111222333", "PC@AD.": "pc@ad"} {
		if got := NormalizePeer(in); got != want {
			t.Errorf("%q -> %q", in, got)
		}
	}
}

// AnyDesk cuts its trace keeping the end: read again, what was already read does not start sessions twice.
func TestRewind(t *testing.T) {
	tr := NewTracker("u")
	b, _ := os.ReadFile(filepath.Join("testdata", "outgoing.trace"))
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	cut := 22 // up to the second «Session started.»
	var ch []Change
	for _, l := range lines[:cut] {
		ch = append(ch, tr.Feed(l)...)
	}
	if len(ch) != 3 || !ch[2].Open {
		t.Fatalf("antes del corte: %+v", ch)
	}
	tr.Rewind()
	ch = nil
	for _, l := range lines[10:] { // the trace after the cut: its last part again, then new lines
		ch = append(ch, tr.Feed(l)...)
	}
	if len(ch) != 1 || ch[0].Open || ch[0].Time.Format("15:04") != "17:48" {
		t.Fatalf("tras el corte: %+v", ch)
	}
}
