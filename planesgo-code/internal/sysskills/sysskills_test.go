package sysskills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every system skill reads: name and description, a body, and chapters that say when to read them.
func TestEverySkillReads(t *testing.T) {
	names := Names()
	if len(names) < 7 {
		t.Fatalf("skills: %v", names)
	}
	for _, n := range names {
		sk, err := Get(n)
		if err != nil {
			t.Fatal(err)
		}
		if sk.Body == "" || strings.HasPrefix(sk.Body, "---") {
			t.Errorf("%s: body %q", n, sk.Body)
		}
		for _, c := range sk.Chapters {
			if c.Description == "" || c.body == "" {
				t.Errorf("%s/%s: a chapter needs a description (when to read it) and a text", n, c.Name)
			}
		}
	}
	if sk, _ := Get("psf-revisor-seguridad"); len(sk.Chapters) < 5 {
		t.Errorf("security reviewer chapters: %+v", sk.Chapters)
	}
}

func TestGetRefusesOtherNames(t *testing.T) {
	for _, n := range []string{"", "../x", "Psf", "no-existe", "psf-builder/../psf-supervisor"} {
		if _, err := Get(n); err == nil {
			t.Errorf("%q accepted", n)
		}
	}
}

// Install leaves the skill in the agent's folder (replacing an old copy) and says where each chapter is.
func TestInstall(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, Dir, "psf-revisor-seguridad", "capitulos", "viejo.md")
	os.MkdirAll(filepath.Dir(old), 0o755)
	os.WriteFile(old, []byte("x"), 0o644)
	sk, err := Install("psf-revisor-seguridad", dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("an old chapter stays")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, Dir, "psf-revisor-seguridad", "SKILL.md")); !strings.Contains(string(b), "name: psf-revisor-seguridad") {
		t.Errorf("SKILL.md: %s", b)
	}
	for _, c := range sk.Chapters {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(c.Path))); err != nil {
			t.Errorf("chapter %s: %v", c.Name, err)
		}
	}
}
