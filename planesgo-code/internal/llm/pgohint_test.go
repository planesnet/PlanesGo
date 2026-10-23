package llm

import "testing"

func TestPgoHint(t *testing.T) {
	for _, c := range []struct {
		text string
		want bool
	}{
		{"arranca el servidor de pruebas con pgo ejecutar web npm run dev", true},
		{"ejecuta con pgo el make lento y dime qué sale", true},
		{"mira la salida de pgo del programa web", true},
		{"pgo parar web", true},
		{"/pgo imputar on", false},
		{"arranca el servidor con npm run dev", false},
		{"vincula el proyecto pgo con Odoo", false},
		{"el binario pgocode ejecuta los programas", false},
	} {
		if got := pgoHintFor(c.text) != ""; got != c.want {
			t.Errorf("%q: %v", c.text, got)
		}
	}
	if !hasPgo([]string{"PGO_API=x", "PGO_TOKEN=y"}) || hasPgo([]string{"PGO_API=x"}) {
		t.Error("hasPgo")
	}
}
