//go:build integracion

package qa

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Lanzar una prueba de la campaña necesita python3 (por eso va con -tags integracion).
func TestLanzarPrueba(t *testing.T) {
	d := filepath.Join(t.TempDir(), ".psf", "qa", "madera")
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "prueba.py"), []byte("print('hola desde la prueba')\n"), 0o644)

	var e Ejecutor
	if _, err := e.Lanza("0:madera", d, "../evil.py"); err == nil {
		t.Error("script fuera de la campaña aceptado")
	}
	x, err := e.Lanza("0:madera", d, "prueba.py")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50 && e.EnCurso() > 0; i++ {
		time.Sleep(100 * time.Millisecond) // espera con condición y máximo de 5 s
	}
	r := e.Lista("0:madera")[0]
	if r.ID != x.ID || r.Estado != "ok" || len(r.Salida) == 0 || r.Salida[0] != "hola desde la prueba" {
		t.Fatalf("ejecución: %+v", r)
	}
}
