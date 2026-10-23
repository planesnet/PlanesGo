package llm

import (
	"strings"
	"testing"
)

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
		{"busca en Drive el informe de ventas", true},
		{"lee https://docs.google.com/document/d/1AbCdEfGhIjK/edit y resúmelo", true},
		{"mira el documento de Google con los requisitos", true},
		{"compara con la hoja de cálculo de Google de gastos", true},
		{"pásalo a Google Sheets", true},
		{"actualiza los docs del proyecto", false},
		{"cambia la hoja de estilos (stylesheet)", false},
		{"/drive buscar x", false},
	} {
		if got := pgoHintFor(c.text) != ""; got != c.want {
			t.Errorf("%q: %v", c.text, got)
		}
	}
	if h := pgoHintFor("arranca con pgo ejecutar web npm run dev y lee el documento de Google"); !strings.Contains(h, "pgo ejecutar") || !strings.Contains(h, "pgo drive leer") {
		t.Errorf("los dos avisos: %q", h)
	}
	if !hasPgo([]string{"PGO_API=x", "PGO_TOKEN=y"}) || hasPgo([]string{"PGO_API=x"}) {
		t.Error("hasPgo")
	}
}
