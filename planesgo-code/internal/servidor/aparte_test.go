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
	tipo, _, err := clasificaMensaje(context.Background(), "¿qué puerto usa postgres?", strings.Repeat("x", 5000), "", false)
	if err != nil || tipo != "suelta" || !strings.Contains(visto, "postgres") || len(visto) > 3000 {
		t.Fatalf("%s %v (%d)", tipo, err, len(visto))
	}
	clasificaGemini = func(context.Context, string, string, string, int, int) (string, llm.Uso, error) {
		return "", llm.Uso{}, errors.New("sin red")
	}
	if tipo, _, err := clasificaMensaje(context.Background(), "hola", "", "", false); err == nil || tipo != "conversacion" {
		t.Errorf("con fallo: %s %v", tipo, err)
	}
}

// Otro tema: solo con el tema de la sesión; con el agente libre (soloTema), lo que no es otro tema, a la conversación.
func TestClasificaTema(t *testing.T) {
	previo := clasificaGemini
	defer func() { clasificaGemini = previo }()
	resp, sistema, visto := "tema", "", ""
	clasificaGemini = func(_ context.Context, _, s, texto string, _, _ int) (string, llm.Uso, error) {
		sistema, visto = s, texto
		return resp, llm.Uso{Entrada: 200, Salida: 1}, nil
	}
	if tipo, _, _ := clasificaMensaje(context.Background(), "Añadir las etiquetas a la lista de tareas", "", "Objetivo: diseño de la portada", true); tipo != "tema" ||
		!strings.Contains(visto, "diseño de la portada") || !strings.Contains(sistema, "tema:") || strings.Contains(sistema, "suelta") {
		t.Fatalf("solo tema: %s\n%s\n%s", tipo, sistema, visto)
	}
	if tipo, _, _ := clasificaMensaje(context.Background(), "x", "", "", false); tipo != "conversacion" || strings.Contains(sistema, "tema:") {
		t.Errorf("sin tema no puede salir «tema»: %s", tipo)
	}
	resp = "suelta"
	if tipo, _, _ := clasificaMensaje(context.Background(), "¿qué hora es?", "", "Objetivo: diseño", true); tipo != "conversacion" {
		t.Errorf("con el agente libre, suelta → conversación: %s", tipo)
	}
	if tipo, _, _ := clasificaMensaje(context.Background(), "¿qué hora es?", "", "Objetivo: diseño", false); tipo != "suelta" || !strings.Contains(sistema, "tema:") || !strings.Contains(sistema, "suelta") {
		t.Errorf("con el agente ocupado, las cuatro: %s", tipo)
	}
}
