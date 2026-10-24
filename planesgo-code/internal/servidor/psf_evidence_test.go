package servidor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveEvidence(t *testing.T) {
	dir := t.TempDir()
	f, err := saveEvidence(dir, Captura{Sesion: "s1", Titulo: "#4 T", Herramienta: "PSF · Builder"}, "psf-Builder", ".svg", []byte("<svg/>"))
	if err != nil || filepath.Ext(f) != ".svg" {
		t.Fatalf("%q %v", f, err)
	}
	g, _ := saveEvidence(dir, Captura{Sesion: "s1"}, "psf-Builder", ".svg", []byte("<svg/>"))
	if g == f {
		t.Error("two pictures in a row must not share a file")
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".pgocode", "evidencias", "s1", "indice.jsonl"))
	if !strings.Contains(string(b), `"herramienta":"PSF · Builder"`) || mimeDe(f) != "image/svg+xml" {
		t.Errorf("index: %s", b)
	}
}
