package repo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPreparaRama(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip()
	}
	base := t.TempDir()
	r := filepath.Join(base, "repo")
	os.Mkdir(r, 0o755)
	for _, a := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.email=a@b", "-c", "user.name=a", "commit", "-q", "--allow-empty", "-m", "x"}, {"branch", "dev"}} {
		if out, err := exec.Command("git", append([]string{"-C", r}, a...)...).CombinedOutput(); err != nil {
			t.Fatal(string(out))
		}
	}
	ctx := context.Background()
	dev := filepath.Join(base, "dev")
	if err := PreparaRama(ctx, r, "dev", dev); err != nil {
		t.Fatal(err)
	}
	if Copias(ctx, r)["dev"] == "" || PreparaRama(ctx, r, "dev", dev) != nil {
		t.Fatal("la segunda vez debería valer la misma carpeta")
	}
	if err := PreparaRama(ctx, r, "dev", filepath.Join(base, "otra")); err == nil {
		t.Error("la rama ya está abierta en otra carpeta")
	}
	if err := PreparaRama(ctx, r, "main", dev); err == nil {
		t.Error("no se cambia de rama una carpeta que ya tiene otra")
	}
	if err := PreparaRama(ctx, r, "nueva", filepath.Join(base, "nueva")); err != nil {
		t.Errorf("rama nueva: %v", err)
	}
}
