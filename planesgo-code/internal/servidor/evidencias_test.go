package servidor

import (
	"os"
	"path/filepath"
	"testing"

	"planesgo-code/internal/llm"
	"planesgo-code/internal/proyectos"
)

func TestGuardaCapturas(t *testing.T) {
	casa, _ := os.UserHomeDir()
	base, err := os.MkdirTemp(casa, ".pgocode-prueba-")
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	carpeta := filepath.Join(base, "repo")
	os.MkdirAll(carpeta, 0o755)
	h := &llm.Herramienta{Nombre: "mcp__navegador__computer", Resumen: "screenshot",
		Imagenes: []llm.Imagen{{Tipo: "image/png", Datos: []byte("png")}, {Tipo: "image/svg+xml", Datos: []byte("<svg/>")}}}
	guardaCapturas(carpeta, Captura{Sesion: "s/1", Titulo: "Prueba"}, h)
	if len(h.Capturas) != 1 || h.Imagenes != nil {
		t.Fatalf("capturas: %v (svg no se guarda)", h.Capturas)
	}
	if b, _ := os.ReadFile(h.Capturas[0]); string(b) != "png" {
		t.Error("contenido")
	}
	s := &Servidor{Indice: &proyectos.Indice{Registro: filepath.Join(base, "registro.json")}}
	if err := s.Indice.Registra(7, carpeta); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.rutaEvidencia(7, h.Capturas[0]); !ok {
		t.Error("la captura no vale")
	}
	otra := filepath.Join(base, "otro", ".pgocode", "evidencias", "s", "a.png")
	for _, r := range []string{otra, filepath.Join(carpeta, "a.png"), filepath.Join(carpeta, ".pgocode", "evidencias", "s", "..", "..", "..", "x.png"), "/etc/passwd"} {
		if _, ok := s.rutaEvidencia(7, r); ok {
			t.Errorf("aceptada: %s", r)
		}
	}
	if _, ok := s.rutaEvidencia(8, h.Capturas[0]); ok {
		t.Error("vale para otro proyecto")
	}
}
