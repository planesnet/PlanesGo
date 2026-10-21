package servidor

import (
	"os"
	"path/filepath"
	"testing"

	"planesgo-code/internal/proyectos"
)

func TestPlanIdaYVuelta(t *testing.T) {
	p := Plan{Titulo: "Añadir: la cola\nde mensajes", Proyecto: 447, Sesion: "s1", Agente: "claude-code", Estado: "borrador", Creado: "2026-10-21T10:00:00+02:00",
		Texto: "# Plan\n\n- paso 1\n---\n- paso 2\n"}
	l := leePlan(componePlan(p))
	if l.Titulo != "Añadir: la cola de mensajes" || l.Proyecto != 447 || l.Sesion != "s1" || l.Agente != "claude-code" || l.Estado != "borrador" || l.Creado != p.Creado {
		t.Errorf("cabecera: %+v", l)
	}
	if l.Texto != p.Texto {
		t.Errorf("texto: %q", l.Texto)
	}
}

func TestPlanSinCabecera(t *testing.T) {
	l := leePlan("# Solo texto\n")
	if l.Estado != "borrador" || l.Texto != "# Solo texto\n" {
		t.Errorf("%+v", l)
	}
	if l := leePlan("---\nestado: raro\n---\nx"); l.Estado != "borrador" {
		t.Errorf("estado no válido aceptado: %q", l.Estado)
	}
}

func TestSlug(t *testing.T) {
	for e, s := range map[string]string{"Modo plan: edición y ejecución": "modo-plan-edicion-y-ejecucion", "¿?": "plan", "": "plan"} {
		if g := slug(e); g != s {
			t.Errorf("slug(%q) = %q, quiero %q", e, g, s)
		}
	}
}

func TestRutaPlan(t *testing.T) {
	dir := "/p/.pgocode/planes"
	if _, err := rutaPlan(dir, dir+"/a.md"); err != nil {
		t.Error(err)
	}
	for _, r := range []string{dir + "/../secreto.md", dir + "/a.txt", "/etc/passwd", dir + "/sub/a.md"} {
		if _, err := rutaPlan(dir, r); err == nil {
			t.Errorf("aceptada: %s", r)
		}
	}
}

// El directorio de planes vale si es una carpeta del proyecto o contiene alguna (el directorio principal).
func TestDirPlanes(t *testing.T) {
	// Registra solo acepta carpetas dentro de la carpeta personal.
	casa, _ := os.UserHomeDir()
	base, err := os.MkdirTemp(casa, ".pgocode-prueba-")
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	repo := filepath.Join(base, "proyecto", "master")
	os.MkdirAll(repo, 0o755)
	s := &Servidor{Indice: &proyectos.Indice{Registro: filepath.Join(base, "registro.json")}}
	if err := s.Indice.Registra(447, repo); err != nil {
		t.Fatal(err)
	}
	if d, ok := s.dirPlanes(nil, 447, filepath.Join(base, "proyecto")); !ok || d != filepath.Join(base, "proyecto", ".pgocode", "planes") {
		t.Errorf("principal: %q %v", d, ok)
	}
	if _, ok := s.dirPlanes(nil, 447, repo); !ok {
		t.Error("la propia carpeta del proyecto no vale")
	}
	for _, d := range []string{filepath.Join(base, "otro"), filepath.Join(base, "proyecto", "mas"), "relativa", ""} {
		if _, ok := s.dirPlanes(nil, 447, d); ok {
			t.Errorf("aceptado: %q", d)
		}
	}
	if _, ok := s.dirPlanes(nil, 1, filepath.Join(base, "proyecto")); ok {
		t.Error("vale para otro proyecto")
	}
}

// Los planes de antes de la numeración reciben su número por orden de creación; los nuevos, el siguiente.
func TestNumeraPlanes(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "b.md"), []byte(componePlan(Plan{Titulo: "B", Proyecto: 7, Estado: "borrador", Creado: "2026-10-21T10:00:00Z", Texto: "b"})), 0o644)
	os.WriteFile(filepath.Join(d, "a.md"), []byte(componePlan(Plan{Titulo: "A", Proyecto: 7, Estado: "borrador", Creado: "2026-10-21T09:00:00Z", Texto: "a"})), 0o644)
	os.WriteFile(filepath.Join(d, "otro.md"), []byte(componePlan(Plan{Titulo: "Otro", Proyecto: 8, Estado: "borrador", Creado: "2026-10-21T08:00:00Z", Texto: "x"})), 0o644)
	numera(d, 7)
	l := planesDe(d, 7)
	if len(l) != 2 || l[0].Titulo != "A" || l[0].Numero != 1 || l[1].Numero != 2 {
		t.Fatalf("%+v", l)
	}
	if n := siguienteNumero(d, 7); n != 3 {
		t.Errorf("siguiente: %d", n)
	}
	// Borrado el último, su número no se reutiliza.
	os.Remove(filepath.Join(d, "b.md"))
	if n := siguienteNumero(d, 7); n != 4 {
		t.Errorf("tras borrar: %d (no se reutiliza)", n)
	}
	if n := siguienteNumero(d, 8); n != 1 {
		t.Errorf("otro proyecto: %d", n)
	}
}
