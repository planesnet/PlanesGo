package servidor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"planesgo-code/internal/llm"
)

func TestTipoClasifica(t *testing.T) {
	for resp, quiero := range map[string]string{"suelta": "suelta", "Contexto.": "contexto", " conversación\n": "conversacion",
		"**suelta**": "suelta", "no sé": "conversacion", "": "conversacion", "sueltas": "suelta"} {
		if g := tipoClasifica(resp); g != quiero {
			t.Errorf("%q: %q, quiero %q", resp, g, quiero)
		}
	}
}

// El clasificador recibe el mensaje y lo último del agente (recortados); si falla, a la conversación.
func TestClasificaMensaje(t *testing.T) {
	previo := clasificaGemini
	defer func() { clasificaGemini = previo }()
	var visto string
	clasificaGemini = func(_ context.Context, modelo, _, texto string, _, _ int) (string, llm.Uso, error) {
		visto = texto
		if modelo != llm.ModeloClasifica {
			t.Errorf("modelo %s", modelo)
		}
		return "suelta", llm.Uso{Entrada: 150, Salida: 1}, nil
	}
	tipo, _, err := clasificaMensaje(context.Background(), "¿qué puerto usa postgres?", strings.Repeat("x", 5000))
	if err != nil || tipo != "suelta" || !strings.Contains(visto, "postgres") || len(visto) > 3000 {
		t.Fatalf("%s %v (%d)", tipo, err, len(visto))
	}
	clasificaGemini = func(context.Context, string, string, string, int, int) (string, llm.Uso, error) {
		return "", llm.Uso{}, errors.New("sin red")
	}
	if tipo, _, err := clasificaMensaje(context.Background(), "hola", ""); err == nil || tipo != "conversacion" {
		t.Errorf("con fallo: %s %v", tipo, err)
	}
}
