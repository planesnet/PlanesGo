package servidor

import (
	"os"
	"path/filepath"
	"testing"
)

// Solo valen adjuntos de la carpeta de adjuntos de la sesión (nada de rutas de fuera ni con «..»).
func TestAdjuntosValidos(t *testing.T) {
	c := t.TempDir()
	d := dirAdjuntos(c, "s1")
	os.MkdirAll(d, 0o755)
	ok := filepath.Join(d, "a.png")
	os.WriteFile(ok, []byte("x"), 0o644)
	fuera := filepath.Join(c, "secreto.txt")
	os.WriteFile(fuera, []byte("x"), 0o644)
	v := adjuntosValidos(c, []string{ok, fuera, filepath.Join(d, "..", "..", "..", "secreto.txt"), "/etc/passwd", filepath.Join(d, "no-existe")})
	if len(v) != 1 || v[0] != ok {
		t.Errorf("válidos: %v", v)
	}
}

// Carpetas autorizadas: rutas completas de carpetas existentes; nunca la raíz ni la carpeta personal entera.
func TestCarpetasAutorizadas(t *testing.T) {
	d := t.TempDir()
	casa, _ := os.UserHomeDir()
	f := filepath.Join(d, "fichero")
	os.WriteFile(f, []byte("x"), 0o644)
	got := carpetasAutorizadas([]string{d, d + "/", "relativa", "/", casa, f, filepath.Join(d, "no-existe")})
	if len(got) != 1 || got[0] != d {
		t.Errorf("%v", got)
	}
}
