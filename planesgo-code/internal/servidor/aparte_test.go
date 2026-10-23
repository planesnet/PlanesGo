package servidor

import (
	"context"
	"errors"
	"net/http/httptest"
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

// La pregunta suelta la responde la IA local (0 tokens); sin ella, si falla o si se pide el de pago, el modelo
// barato. Con contexto, nunca la IA local.
func TestAparteSueltaIALocal(t *testing.T) {
	previo, previoLocal := aparteGemini, aparteLocalChat
	defer func() { aparteGemini, aparteLocalChat = previo, previoLocal }()
	local, pago := 0, 0
	aparteLocalChat = func(context.Context, string, string, int) (string, string, error) {
		local++
		return " Canberra. ", "gemma3:4b", nil
	}
	aparteGemini = func(context.Context, string, string, string, int, int) (string, llm.Uso, error) {
		pago++
		return "Canberra (pago)", llm.Uso{Entrada: 10, Salida: 2}, nil
	}
	pregunta := func(tipo, motor string) (via, texto string) {
		s := &Servidor{}
		r := httptest.NewRequest("POST", "/api/chat", nil)
		pet := llm.Peticion{Mensajes: []llm.Mensaje{{Rol: "user", Texto: "¿Capital de Australia?"}}}
		s.aparte("t1", r, tipo, "resumen", motor, nil, llm.Info{}, pet, "s1", "claude-code", func(v any) {
			switch e := v.(type) {
			case map[string]string:
				if e["via"] != "" {
					via = e["via"]
				}
			case llm.Evento:
				texto += e.Texto
			}
		})
		return
	}
	if via, txt := pregunta("suelta", ""); via != "local/gemma3:4b" || txt != "Canberra." || local != 1 || pago != 0 {
		t.Errorf("local: via %q texto %q (local %d, pago %d)", via, txt, local, pago)
	}
	if via, txt := pregunta("suelta", "pago"); via != "modelo" || txt != "Canberra (pago)" || local != 1 || pago != 1 {
		t.Errorf("pago pedido: via %q texto %q", via, txt)
	}
	if via, _ := pregunta("contexto", ""); via != "modelo" || local != 1 {
		t.Errorf("con contexto (sin copia) no va a la IA local: via %q, local %d", via, local)
	}
	aparteLocalChat = func(context.Context, string, string, int) (string, string, error) {
		return "", "", errors.New("la IA local no tiene modelo de texto")
	}
	if via, txt := pregunta("suelta", ""); via != "modelo" || txt != "Canberra (pago)" {
		t.Errorf("sin IA local, respaldo: via %q texto %q", via, txt)
	}
}
