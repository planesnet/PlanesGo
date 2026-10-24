package actualizacion

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApply(t *testing.T) {
	root := t.TempDir()
	app, prep := filepath.Join(root, "app"), filepath.Join(root, "actualizacion")
	a := &Actualizador{DirApp: app, DirPreparado: prep}
	backend := filepath.Base(a.Backend())
	os.MkdirAll(app, 0o755)
	if err := a.Apply(); err == nil {
		t.Error("a development install (no paquete.sha256) must not be touched")
	}
	os.WriteFile(filepath.Join(app, "paquete.sha256"), []byte("viejo"), 0o644)
	os.WriteFile(filepath.Join(app, backend), []byte("v1"), 0o755)
	if err := a.Apply(); err == nil {
		t.Error("without a prepared version, nothing to apply")
	}
	nuevo := filepath.Join(prep, "nuevo")
	os.MkdirAll(nuevo, 0o755)
	os.WriteFile(filepath.Join(nuevo, backend), []byte("v2"), 0o755)
	os.WriteFile(filepath.Join(nuevo, "paquete.sha256"), []byte("nuevo"), 0o644)
	if err := a.Apply(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(a.Backend()); string(b) != "v2" {
		t.Errorf("installed: %s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(app+".anterior", backend)); string(b) != "v1" {
		t.Errorf("previous kept: %s", b)
	}
	if _, err := os.Stat(nuevo); !os.IsNotExist(err) {
		t.Error("the prepared folder becomes the app")
	}
}

// A swap that fails is remembered: the same package is not prepared nor applied again (no loop of restarts).
func TestApplyFailedIsRemembered(t *testing.T) {
	root := t.TempDir()
	app, prep := filepath.Join(root, "app"), filepath.Join(root, "actualizacion")
	a := &Actualizador{DirApp: app, DirPreparado: prep}
	backend := filepath.Base(a.Backend())
	os.MkdirAll(app, 0o755)
	os.WriteFile(filepath.Join(app, "paquete.sha256"), []byte("viejo"), 0o644)
	nuevo := filepath.Join(prep, "nuevo")
	os.MkdirAll(nuevo, 0o755)
	os.WriteFile(filepath.Join(nuevo, backend), []byte("v2"), 0o755)
	os.WriteFile(filepath.Join(nuevo, "paquete.sha256"), []byte("abc\n"), 0o644)
	os.WriteFile(app+".anterior", []byte("a file where the previous version would go"), 0o644)
	os.Chmod(root, 0o555) // nothing can be renamed in root
	defer os.Chmod(root, 0o755)
	if err := a.Apply(); err == nil {
		t.Skip("the swap did not fail (running as root?)")
	}
	if why := a.failed("abc"); why == "" {
		t.Error("the failed package is not remembered")
	}
	if a.failed("otro") != "" {
		t.Error("another package is not the failed one")
	}
	if _, err := os.Stat(nuevo); !os.IsNotExist(err) {
		t.Error("the failed version stays prepared: it would be applied again")
	}
}
