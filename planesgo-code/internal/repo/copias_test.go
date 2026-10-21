package repo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// repoPrueba crea un repositorio con un commit en main y devuelve su carpeta y una función para lanzar git.
func repoPrueba(t *testing.T) (string, func(dir string, args ...string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip()
	}
	r := filepath.Join(t.TempDir(), "repo")
	os.Mkdir(r, 0o755)
	run := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run(r, "init", "-q", "-b", "main")
	run(r, "commit", "-q", "--allow-empty", "-m", "inicio")
	return r, run
}

func existe(p string) bool { _, err := os.Stat(p); return err == nil }

func TestCopiaConCambiosNoSeQuita(t *testing.T) {
	r, _ := repoPrueba(t)
	ctx := context.Background()
	dir := filepath.Join(filepath.Dir(r), "dev")
	if err := PreparaRama(ctx, r, "dev", dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "nuevo.txt"), []byte("x"), 0o644) // sin seguimiento
	e, err := EstadoCopia(ctx, dir)
	if err != nil || !e.Worktree || e.Cambios != 1 || e.SePuedeQuitar() {
		t.Fatalf("estado: %+v %v", e, err)
	}
	if _, err := QuitaCopia(ctx, dir, true); err == nil || !existe(filepath.Join(dir, "nuevo.txt")) {
		t.Fatal("se ha quitado una copia con cambios sin commit")
	}
}

func TestStashDeLaRamaNoSeQuita(t *testing.T) {
	r, run := repoPrueba(t)
	ctx := context.Background()
	dir := filepath.Join(filepath.Dir(r), "dev")
	if err := PreparaRama(ctx, r, "dev", dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644)
	run(dir, "stash", "push", "-q", "-u")
	if e, _ := EstadoCopia(ctx, dir); e.Stash != 1 || e.SePuedeQuitar() {
		t.Fatalf("stash no detectado: %+v", e)
	}
	if _, err := QuitaCopia(ctx, dir, false); err == nil || !existe(dir) {
		t.Fatal("se ha quitado una copia con stash")
	}
}

func TestRamaSinFusionarSeConserva(t *testing.T) {
	r, run := repoPrueba(t)
	ctx := context.Background()
	dir := filepath.Join(filepath.Dir(r), "dev")
	if err := PreparaRama(ctx, r, "dev", dir); err != nil {
		t.Fatal(err)
	}
	run(dir, "commit", "-q", "--allow-empty", "-m", "trabajo")
	e, _ := EstadoCopia(ctx, dir)
	if e.Fusionada || e.SinSubir != 1 || e.SePuedeQuitar() {
		t.Fatalf("estado: %+v", e)
	}
	if _, err := QuitaCopia(ctx, dir, true); err == nil || !existe(dir) {
		t.Fatal("se ha quitado una copia con commits sin subir")
	}
	if _, err := git(ctx, r, "show-ref", "--verify", "--quiet", "refs/heads/dev"); err != nil {
		t.Fatal("la rama sin fusionar se ha borrado")
	}
}

func TestCopiaLimpiaYFusionadaSeQuitaConSuRama(t *testing.T) {
	r, run := repoPrueba(t)
	ctx := context.Background()
	dir := filepath.Join(filepath.Dir(r), "dev")
	if err := PreparaRama(ctx, r, "dev", dir); err != nil {
		t.Fatal(err)
	}
	run(dir, "commit", "-q", "--allow-empty", "-m", "trabajo")
	run(r, "merge", "-q", "--ff-only", "dev")
	e, _ := EstadoCopia(ctx, dir)
	if !e.Fusionada || !e.SePuedeQuitar() || e.Defecto != "main" {
		t.Fatalf("estado: %+v", e)
	}
	borrada, err := QuitaCopia(ctx, dir, true)
	if err != nil || !borrada || existe(dir) {
		t.Fatalf("no se ha quitado: %v %v", borrada, err)
	}
	if _, err := git(ctx, r, "show-ref", "--verify", "--quiet", "refs/heads/dev"); err == nil {
		t.Error("la rama fusionada sigue")
	}
	if len(ListaCopias(ctx, r)) != 0 {
		t.Error("git sigue teniendo la copia")
	}
}

func TestNuncaQuitaLaPrincipal(t *testing.T) {
	r, _ := repoPrueba(t)
	ctx := context.Background()
	e, err := EstadoCopia(ctx, r)
	if err != nil || e.Worktree {
		t.Fatalf("la principal no es una copia: %+v %v", e, err)
	}
	if _, err := QuitaCopia(ctx, r, true); err == nil || !existe(filepath.Join(r, ".git")) {
		t.Fatal("se ha quitado la carpeta principal")
	}
	sub := filepath.Join(r, "sub")
	os.Mkdir(sub, 0o755)
	if _, err := QuitaCopia(ctx, sub, true); err == nil || !existe(sub) {
		t.Fatal("se ha quitado una subcarpeta del repositorio")
	}
	if _, err := QuitaCopia(ctx, t.TempDir(), true); err == nil {
		t.Fatal("una carpeta sin git no es una copia")
	}
}

func TestCopiaBorradaAManoSeRecrea(t *testing.T) {
	r, _ := repoPrueba(t)
	ctx := context.Background()
	base := t.TempDir()
	dir, err := CarpetaDeRama(ctx, r, "dev", base) // rama nueva desde la actual
	if err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(dir) // git la sigue teniendo registrada
	dir2, err := CarpetaDeRama(ctx, r, "dev", base)
	if err != nil || dir2 != dir || !existe(filepath.Join(dir, ".git")) {
		t.Fatalf("no se ha vuelto a crear: %s %v", dir2, err)
	}
	if e, _ := EstadoCopia(ctx, dir); e.Rama != "dev" || !e.Worktree {
		t.Fatalf("estado: %+v", e)
	}
}

func TestRegistro(t *testing.T) {
	f := filepath.Join(t.TempDir(), "copias.json")
	r := &Registro{Fichero: f}
	r.Apunta(CopiaCreada{Carpeta: "/a/b/", Rama: "dev", Sesion: "s1"})
	r.Apunta(CopiaCreada{Carpeta: "/a/c", Rama: "x"})
	if c, ok := r.Busca("/a/b"); !ok || c.Sesion != "s1" || c.Fecha.IsZero() {
		t.Fatalf("busca: %+v %v", c, ok)
	}
	if st, err := os.Stat(f); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("permisos: %v", err)
	}
	r.Quita("/a/b")
	if l := r.Lista(); len(l) != 1 || l[0].Carpeta != "/a/c" {
		t.Fatalf("lista: %+v", l)
	}
	var nada *Registro
	if nada.Apunta(CopiaCreada{Carpeta: "/x"}) != nil || nada.Lista() != nil {
		t.Error("registro nil")
	}
}
