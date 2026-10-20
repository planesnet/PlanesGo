package repo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNombreRepositorio(t *testing.T) {
	for in, quiero := range map[string]string{
		"git@github.com:planesnet/madpla.git":      "planesnet/madpla",
		"https://github.com/planesnet/madpla":      "planesnet/madpla",
		"https://github.com/planesnet/madpla.git/": "planesnet/madpla",
	} {
		if r := nombreRepositorio(in); r != quiero {
			t.Errorf("%s -> %s, quiero %s", in, r, quiero)
		}
	}
}

func TestLeeYCarpetaDeRama(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "dev-lp")
	os.MkdirAll(filepath.Join(dir, ".psf"), 0o755)
	os.WriteFile(filepath.Join(dir, ".psf", "proyecto.yml"), []byte("entornos:\n  url: https://x-test-1.dev.odoo.com/\n"), 0o644)
	run("add", ".")
	run("commit", "-qm", "inicio")
	run("branch", "test")
	run("remote", "add", "origin", "git@github.com:planesnet/madpla.git")

	i := Lee(context.Background(), dir)
	if i.Tipo != OdooSH || i.Repositorio != "planesnet/madpla" || i.Rama != "dev-lp" || len(i.Ramas) != 2 {
		t.Fatalf("info: %+v", i)
	}
	if c, err := CarpetaDeRama(context.Background(), dir, "dev-lp", t.TempDir()); err != nil || c != dir {
		t.Errorf("rama actual: %s %v", c, err)
	}
	base := t.TempDir()
	c, err := CarpetaDeRama(context.Background(), dir, "test", base)
	if err != nil || c == dir {
		t.Fatalf("otra rama: %s %v", c, err)
	}
	if r := Lee(context.Background(), c); r.Rama != "test" {
		t.Errorf("la copia de trabajo no está en test: %+v", r)
	}
	if i2 := Lee(context.Background(), dir); i2.Rama != "dev-lp" {
		t.Errorf("la carpeta del usuario ha cambiado de rama: %s", i2.Rama)
	}
	if _, err := CarpetaDeRama(context.Background(), dir, "--evil", base); err == nil {
		t.Error("rama con «--» aceptada")
	}
}
