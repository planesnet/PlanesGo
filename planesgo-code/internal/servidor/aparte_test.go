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
		"**suelta**": "suelta", "tema": "conversacion", "no sé": "conversacion", "": "conversacion", "sueltas": "suelta"} {
		if g := tipoClasifica(resp); g != quiero {
			t.Errorf("%q: %q, quiero %q", resp, g, quiero)
		}
	}
}

// El clasificador recibe el mensaje y lo último del agente (recortados); si falla, a la conversación.
func TestClasificaMensaje(t *testing.T) {
	previo, previoLocal := clasificaGemini, clasificaLocal
	defer func() { clasificaGemini, clasificaLocal = previo, previoLocal }()
	// Sin IA local: el modelo de pago.
	clasificaLocal = func(context.Context, string, string, int) (string, string, error) {
		return "", "", errors.New("sin modelo")
	}
	var visto string
	clasificaGemini = func(_ context.Context, modelo, _, texto string, _, _ int) (string, llm.Uso, error) {
		visto = texto
		if modelo != llm.ModeloClasifica {
			t.Errorf("modelo %s", modelo)
		}
		return "suelta", llm.Uso{Entrada: 150, Salida: 1}, nil
	}
	tipo, _, _, err := clasificaMensaje(context.Background(), "¿qué puerto usa postgres?", strings.Repeat("x", 5000))
	if err != nil || tipo != "suelta" || !strings.Contains(visto, "postgres") || len(visto) > 3000 {
		t.Fatalf("%s %v (%d)", tipo, err, len(visto))
	}
	clasificaGemini = func(context.Context, string, string, string, int, int) (string, llm.Uso, error) {
		return "", llm.Uso{}, errors.New("sin red")
	}
	if tipo, _, _, err := clasificaMensaje(context.Background(), "hola", ""); err == nil || tipo != "conversacion" {
		t.Errorf("con fallo: %s %v", tipo, err)
	}
	// Con IA local que responde una de las palabras: decide ella y no se paga nada.
	clasificaLocal = func(context.Context, string, string, int) (string, string, error) {
		return "Contexto.", "gemma3:4b", nil
	}
	clasificaGemini = func(context.Context, string, string, string, int, int) (string, llm.Uso, error) {
		t.Error("no debía llamar al modelo de pago")
		return "", llm.Uso{}, nil
	}
	if tipo, modelo, u, err := clasificaMensaje(context.Background(), "¿por qué has cambiado eso?", ""); err != nil || tipo != "contexto" || modelo != "local/gemma3:4b" || u.Coste != 0 {
		t.Errorf("local: %s %s %+v %v", tipo, modelo, u, err)
	}
	// Si la local dice otra cosa, decide el de pago.
	clasificaLocal = func(context.Context, string, string, int) (string, string, error) { return "No sé", "gemma3:4b", nil }
	clasificaGemini = func(context.Context, string, string, string, int, int) (string, llm.Uso, error) {
		return "suelta", llm.Uso{}, nil
	}
	if tipo, modelo, _, _ := clasificaMensaje(context.Background(), "x", ""); tipo != "suelta" || modelo != llm.ModeloClasifica {
		t.Errorf("respaldo: %s %s", tipo, modelo)
	}
}
