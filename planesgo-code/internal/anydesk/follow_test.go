package anydesk

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(s)
	f.Close()
}

func poll(t *testing.T, f *Follower) []string {
	t.Helper()
	l, err := f.Poll()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func testFollower(t *testing.T, keepOpen bool) {
	dir := t.TempDir()
	p := filepath.Join(dir, "anydesk.trace")
	f := NewFollower(p, 8)
	f.keepOpen = keepOpen
	defer f.Close()
	if l := poll(t, f); len(l) != 0 {
		t.Fatalf("sin fichero: %q", l)
	}
	appendTo(t, p, "vieja 1\nvieja 2\n") // created after the first Poll: read from the start
	if l := poll(t, f); !reflect.DeepEqual(l, []string{"vieja 1", "vieja 2"}) {
		t.Fatalf("%q", l)
	}
	appendTo(t, p, "a\r\nmedia")
	if l := poll(t, f); !reflect.DeepEqual(l, []string{"a"}) {
		t.Fatalf("línea a medias: %q", l)
	}
	appendTo(t, p, " línea\n")
	if l := poll(t, f); !reflect.DeepEqual(l, []string{"media línea"}) {
		t.Fatalf("%q", l)
	}
	// rotation by renaming: what was left in the old one, then the new one
	appendTo(t, p, "antes de rotar\n")
	os.Rename(p, p+".old")
	appendTo(t, p, "nueva\n")
	want := []string{"nueva"}
	if keepOpen {
		want = []string{"antes de rotar", "nueva"}
	}
	if l := poll(t, f); !reflect.DeepEqual(l, want) || f.ResetAt != len(want)-1 {
		t.Fatalf("rotación: %q (desde %d)", l, f.ResetAt)
	}
	// truncated in place
	os.WriteFile(p, []byte("x\n"), 0o644)
	if l := poll(t, f); !reflect.DeepEqual(l, []string{"x"}) || f.ResetAt != 0 {
		t.Fatalf("truncado: %q (desde %d)", l, f.ResetAt)
	}
	appendTo(t, p, "y\n")
	if l := poll(t, f); !reflect.DeepEqual(l, []string{"y"}) || f.ResetAt != -1 {
		t.Fatalf("sigue: %q (desde %d)", l, f.ResetAt)
	}
	os.Remove(p)
	if l := poll(t, f); len(l) != 0 {
		t.Fatalf("borrado: %q", l)
	}
	appendTo(t, p, "otra vez\n")
	if l := poll(t, f); !reflect.DeepEqual(l, []string{"otra vez"}) {
		t.Fatalf("recreado: %q", l)
	}
}

func TestFollowerOpen(t *testing.T)   { testFollower(t, true) }
func TestFollowerClosed(t *testing.T) { testFollower(t, false) }

// On the first Poll it starts Backlog bytes before the end, without the cut line.
func TestFollowerBacklog(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t")
	os.WriteFile(p, []byte("primera línea larga\nsegunda\ntercera\n"), 0o644)
	f := NewFollower(p, 16)
	defer f.Close()
	if l := poll(t, f); !reflect.DeepEqual(l, []string{"tercera"}) {
		t.Fatalf("%q", l)
	}
}
