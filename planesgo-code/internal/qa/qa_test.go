package qa

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCampanaYLanzar(t *testing.T) {
	proyecto := t.TempDir()
	d := filepath.Join(proyecto, ".psf", "qa", "madera")
	os.MkdirAll(filepath.Join(d, "visor", "media"), 0o755)
	os.WriteFile(filepath.Join(d, "visor", "casos.json"), []byte(`{"fase":"Tanda 1","casos":[{"id":"A"},{"id":"B"}]}`), 0o644)
	os.WriteFile(filepath.Join(d, "visor", "media", "A.mp4"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(d, "prueba.py"), []byte("print('hola desde la prueba')\n"), 0o644)
	os.WriteFile(filepath.Join(d, "rpc.py"), []byte(""), 0o644)

	l := Busca([]string{proyecto})
	if len(l) != 1 || l[0].ID != "0:madera" || l[0].Casos != 2 || l[0].Fase != "Tanda 1" {
		t.Fatalf("campañas: %+v", l)
	}
	dir, err := Dir([]string{proyecto}, "0:madera")
	if err != nil || dir != d {
		t.Fatalf("dir: %s %v", dir, err)
	}
	for _, malo := range []string{"1:madera", "0:../x", "x", "0:"} {
		if _, err := Dir([]string{proyecto}, malo); err == nil {
			t.Errorf("id %q aceptado", malo)
		}
	}
	if p := Pruebas(d); len(p) != 1 || p[0] != "prueba.py" {
		t.Errorf("pruebas: %v", p)
	}
	if _, err := Media(d, "media/A.mp4"); err != nil {
		t.Error(err)
	}
	for _, malo := range []string{"../casos.json", "media/../../x.mp4", "casos.json"} {
		if _, err := Media(d, malo); err == nil {
			t.Errorf("media %q aceptado", malo)
		}
	}
	if err := Comenta(d, Comentario{Caso: "A", T: 3.5, Texto: "falla el aviso", Autor: "luis"}); err != nil {
		t.Fatal(err)
	}
	if c := Comentarios(d); len(c) != 1 || c[0].T != 3.5 {
		t.Errorf("comentarios: %+v", c)
	}
	// La comprobación de la ruta del script no ejecuta nada (lanzar de verdad está en qa_integracion_test.go).
	var e Ejecutor
	if _, err := e.Lanza("0:madera", d, "../evil.py"); err == nil {
		t.Error("script fuera de la campaña aceptado")
	}
}
