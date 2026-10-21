package repo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemotoSSH(t *testing.T) {
	for _, r := range []string{"git@github.com:planesnet/intradema.git", "git@gitlab.empresa.es:grupo/sub/proyecto.git", "ssh://git@github.com/planesnet/intradema.git", "ssh://git@servidor:2222/repo.git"} {
		if !RemotoSSH.MatchString(r) {
			t.Errorf("no acepta %s", r)
		}
	}
	for _, r := range []string{"https://github.com/planesnet/intradema.git", "git@github.com:a/b.git; rm -rf ~", "--upload-pack=x", "file:///etc", "git@github.com:a b"} {
		if RemotoSSH.MatchString(r) {
			t.Errorf("acepta %s", r)
		}
	}
}

// Nunca se clona encima de ficheros.
func TestClonaNoPisa(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "algo.txt"), []byte("x"), 0o644)
	if err := Clona(context.Background(), "git@github.com:planesnet/intradema.git", "", d); err == nil || !strings.Contains(err.Error(), "tiene ficheros") {
		t.Errorf("error: %v", err)
	}
	if err := Clona(context.Background(), "https://github.com/x/y.git", "", t.TempDir()); err == nil || !strings.Contains(err.Error(), "SSH") {
		t.Errorf("error: %v", err)
	}
}

// Clonación real (PGO_PRUEBA_CLONAR=git@…): en una carpeta nueva y con una rama del remoto.
func TestClonaReal(t *testing.T) {
	remoto := os.Getenv("PGO_PRUEBA_CLONAR")
	if remoto == "" {
		t.Skip("PGO_PRUEBA_CLONAR sin definir")
	}
	d := filepath.Join(t.TempDir(), "nueva", "copia")
	if err := Clona(context.Background(), remoto, os.Getenv("PGO_PRUEBA_RAMA"), d); err != nil {
		t.Fatal(err)
	}
	i := Lee(context.Background(), d)
	if !i.Git || i.Repositorio == "" || (os.Getenv("PGO_PRUEBA_RAMA") != "" && i.Rama != os.Getenv("PGO_PRUEBA_RAMA")) {
		t.Errorf("info: %+v", i)
	}
	t.Logf("clonado %s en la rama %s", i.Repositorio, i.Rama)
}
