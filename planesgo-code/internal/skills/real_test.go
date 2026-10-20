package skills

import (
	"os"
	"testing"
)

// PGO_PRUEBA_SKILLS=<carpeta> go test -v -run Real ./internal/skills
func TestInventarioReal(t *testing.T) {
	c := os.Getenv("PGO_PRUEBA_SKILLS")
	if c == "" {
		t.Skip()
	}
	l := Inventario(c)
	fijo := map[string]int{}
	for _, s := range l {
		for _, a := range s.Agentes {
			fijo[a] += s.TokensFijo
		}
	}
	t.Logf("%d skills; coste fijo por agente: %v", len(l), fijo)
	for _, s := range l[:min(8, len(l))] {
		t.Logf("%-28s %-12s %v fijo=%d uso=%d avisos=%d", s.Nombre, s.Ambito, s.Agentes, s.TokensFijo, s.TokensUso, len(s.Avisos))
	}
}
