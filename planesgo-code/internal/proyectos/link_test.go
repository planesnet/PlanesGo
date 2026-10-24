package proyectos

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func gitRepo(t *testing.T, dir string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	for _, a := range [][]string{{"init", "-q"}, {"config", "user.email", "a@b"}, {"config", "user.name", "a"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, a...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", a, out)
		}
	}
}

func TestLinkReadAndMigrate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip()
	}
	root := t.TempDir()
	// A: the PlanesGo file, not in git → migrated. B: in git → kept (and still read). C: both → .pgo.json wins.
	a, b, c := filepath.Join(root, "a"), filepath.Join(root, "b"), filepath.Join(root, "c")
	gitRepo(t, a)
	gitRepo(t, b)
	os.MkdirAll(c, 0o755)
	os.WriteFile(filepath.Join(a, legacyLinkFile), []byte(`{"odoo_project_id": 589, "odoo_project_name": "PRUEBAS", "planesgo_url": "x", "heartbeat_interval_seconds": 30}`), 0o644)
	os.WriteFile(filepath.Join(b, legacyLinkFile), []byte(`{"odoo_project_id": 277}`), 0o644)
	exec.Command("git", "-C", b, "add", legacyLinkFile).Run()
	exec.Command("git", "-C", b, "commit", "-q", "-m", "x").Run()
	os.WriteFile(filepath.Join(c, legacyLinkFile), []byte(`{"odoo_project_id": 1}`), 0o644)
	if err := WriteLink(c, Link{ProjectID: 2}); err != nil {
		t.Fatal(err)
	}
	if l, ok := ReadLink(c); !ok || l.ProjectID != 2 {
		t.Errorf(".pgo.json first: %+v", l)
	}
	if m := busca([]string{root}); len(m[2]) != 1 || len(m[1]) != 0 || len(m[589]) != 1 || len(m[277]) != 1 {
		t.Errorf("index: %v", m)
	}

	// Without removing: both files stay everywhere (older pgocode versions only read the old name).
	if m, k := MigrateLinks([]string{root}, false); len(m) != 0 || len(k) != 3 {
		t.Fatalf("without removing: %v %v", m, k)
	}
	if _, err := os.Stat(filepath.Join(a, legacyLinkFile)); err != nil {
		t.Fatal("without removing, the old file stays")
	}
	migrated, kept := MigrateLinks([]string{root}, true)
	if len(migrated) != 2 || len(kept) != 1 || kept[0] != b { // a and c migrated, b kept
		t.Fatalf("migrated %v kept %v", migrated, kept)
	}
	if _, err := os.Stat(filepath.Join(a, legacyLinkFile)); !os.IsNotExist(err) {
		t.Error("the old file of a folder not in git is removed")
	}
	if got, _ := os.ReadFile(filepath.Join(a, LinkFile)); strings.Contains(string(got), "planesgo_url") || !strings.Contains(string(got), `"odoo_project_name": "PRUEBAS"`) {
		t.Errorf("new link: %s", got)
	}
	if _, err := os.Stat(filepath.Join(b, legacyLinkFile)); err != nil {
		t.Error("the old file in git stays")
	}
	if out, _ := exec.Command("git", "-C", a, "status", "--porcelain").Output(); len(out) != 0 {
		t.Errorf(".pgo.json must not show in git: %s", out)
	}
	if l, ok := ReadLink(b); !ok || l.ProjectID != 277 {
		t.Errorf("b: %+v", l)
	}
	if m, k := MigrateLinks([]string{root}, true); len(m) != 0 || len(k) != 1 {
		t.Errorf("again: %v %v", m, k)
	}
}

// A cloned repository nobody checked brings links named like the files pgocode writes: they are never followed (the
// project name it also brings would end up wherever the link points, e.g. in ~/.bashrc).
func TestWriteLinkNeverFollowsLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "repo")
	os.MkdirAll(dir, 0o755)
	victim := filepath.Join(root, "bashrc")
	os.WriteFile(victim, []byte("original\n"), 0o644)
	os.WriteFile(filepath.Join(dir, legacyLinkFile), []byte(`{"odoo_project_id":1,"odoo_project_name":"$(touch pwned)"}`), 0o644)
	os.Symlink(victim, filepath.Join(dir, LinkFile+".tmp")) // the old temporary name
	MigrateLinks([]string{root}, false)
	if b, _ := os.ReadFile(victim); string(b) != "original\n" {
		t.Fatalf("the link was followed: %q", b)
	}
	if l, ok := ReadLink(dir); !ok || l.ProjectID != 1 {
		t.Errorf("link not written: %+v", l)
	}
	// A LinkFile that is a link is left alone.
	other := filepath.Join(root, "otro")
	os.MkdirAll(other, 0o755)
	os.Symlink(victim, filepath.Join(other, LinkFile))
	if err := WriteLink(other, Link{ProjectID: 2}); err == nil {
		t.Error("a LinkFile that is a link is replaced")
	}
	if b, _ := os.ReadFile(victim); string(b) != "original\n" {
		t.Fatalf("the link was followed: %q", b)
	}
}
