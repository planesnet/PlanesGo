package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCabecera(t *testing.T) {
	m, cuerpo := Cabecera("---\nname: prueba\ndescription: \"Hace X. Úsala cuando Y\"\nrisk: unknown\n---\n\n# Prueba\ncuerpo")
	if m["name"] != "prueba" || m["description"] != "Hace X. Úsala cuando Y" || cuerpo != "\n# Prueba\ncuerpo" {
		t.Errorf("%v %q", m, cuerpo)
	}
	m, _ = Cabecera("---\nname: b\ndescription: >\n  Línea uno\n  línea dos\nversion: 1\n---\nx")
	if m["description"] != "Línea uno línea dos" || m["version"] != "1" {
		t.Errorf("bloque: %v", m)
	}
}

func TestInventarioProyecto(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0o755)
	s := filepath.Join(dir, ".claude", "skills", "mia")
	os.MkdirAll(s, 0o755)
	os.WriteFile(filepath.Join(s, "SKILL.md"), []byte("---\nname: mia\ndescription: Hace cosas\n---\ncuerpo largo"), 0o644)
	os.WriteFile(filepath.Join(s, "ref.md"), []byte("x"), 0o644)
	var la *Skill
	for _, x := range Inventario(dir) {
		if x.Ruta == filepath.Join(s, "SKILL.md") {
			la = &x
		}
	}
	if la == nil || la.Ambito != "proyecto" || len(la.Agentes) != 2 || la.Ficheros != 1 || len(la.Avisos) != 1 {
		t.Fatalf("skill: %+v", la)
	}
	if DeRuta("/a/b/mia/SKILL.md") != "mia" || DeRuta("/a/b.md") != "" {
		t.Error("DeRuta")
	}
}
