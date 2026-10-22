package llm

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
)

// «Continuar desde aquí» en Claude Code: una copia de la sesión hasta el mensaje elegido.
func TestCorteClaudeCode(t *testing.T) {
	var cc *Agente
	for _, p := range Agentes() {
		if a := p.(*Agente); a.Clave == "claude-code" {
			cc = a
		}
	}
	args := cc.Args("hola", Peticion{SesionAgente: "s1", Corte: "u-1"})
	i := slices.Index(args, "--resume-session-at")
	if i < 0 || args[i+1] != "u-1" || !slices.Contains(args, "--fork-session") || !slices.Contains(args, "--resume") {
		t.Errorf("args con corte: %q", args)
	}
	for _, p := range []Peticion{{Corte: "u-1"}, {SesionAgente: "s1"}, {SesionAgente: "s1", Corte: "u-1", Aparte: true}} {
		if a := cc.Args("hola", p); slices.Contains(a, "--resume-session-at") {
			t.Errorf("%+v: no debe cortar: %q", p, a)
		}
	}
}

// El punto de cada respuesta va con su consumo: el último mensaje del agente principal (no el de un subagente).
func TestPuntoClaudeCode(t *testing.T) {
	in := `{"type":"system","subtype":"init","session_id":"s1"}
{"type":"assistant","uuid":"a1","message":{"content":[{"type":"text","text":"uno"}]}}
{"type":"assistant","uuid":"sub","parent_tool_use_id":"t9","message":{"content":[{"type":"text","text":"del subagente"}]}}
{"type":"assistant","uuid":"a2","message":{"content":[{"type":"text","text":"dos"}]}}
{"type":"result","uuid":"r1","usage":{"input_tokens":3,"output_tokens":4}}`
	punto := ""
	if err := leeClaudeCode(strings.NewReader(in), func(e Evento) {
		if e.Punto != "" {
			if e.Uso == nil {
				t.Error("el punto va con el consumo")
			}
			punto = e.Punto
		}
	}); err != nil {
		t.Fatal(err)
	}
	if punto != "a2" {
		t.Errorf("punto %q", punto)
	}
}

func TestPuntoOpenCode(t *testing.T) {
	in := `{"type":"text","sessionID":"ses_1","part":{"type":"text","text":"hola","messageID":"msg_2"}}
{"type":"step_finish","sessionID":"ses_1","part":{"messageID":"msg_2","tokens":{"input":10,"output":2}}}`
	punto := ""
	if err := leeOpenCode(strings.NewReader(in), func(e Evento) {
		if e.Punto != "" {
			punto = e.Punto
		}
	}); err != nil {
		t.Fatal(err)
	}
	if punto != "msg_2" {
		t.Errorf("punto %q", punto)
	}
}

// Un agente que no puede cortar su sesión (Antigravity) sigue en una nueva, con el traspaso delante del mensaje.
func TestCorteConTraspaso(t *testing.T) {
	var vistos []Peticion
	var msgs []string
	a := &Agente{Clave: "antigravity", Nombre: "Prueba", Programa: "sh",
		Args: func(msg string, p Peticion) []string { vistos = append(vistos, p); msgs = append(msgs, msg); return []string{"-c", "echo ok"} }}
	var texto strings.Builder
	p := Peticion{Carpeta: t.TempDir(), SesionAgente: "conv-1", Corte: "x1", Traspaso: "[Traspaso] AZUL", Mensajes: []Mensaje{{Rol: "user", Texto: "¿qué palabras?"}}}
	if err := a.Chat(context.Background(), p, func(e Evento) { texto.WriteString(e.Texto) }); err != nil {
		t.Fatal(err)
	}
	if len(vistos) != 1 || vistos[0].SesionAgente != "" || vistos[0].Corte != "" || !strings.HasPrefix(msgs[0], "[Traspaso] AZUL\n\n") || !strings.HasSuffix(msgs[0], "¿qué palabras?") {
		t.Errorf("petición: %+v / %q", vistos, msgs)
	}
	if !strings.Contains(texto.String(), "conversación nueva") {
		t.Errorf("sin aviso: %q", texto.String())
	}
	// Sin traspaso no hay forma de seguir.
	p.Traspaso = ""
	if err := a.Chat(context.Background(), p, func(Evento) {}); err == nil {
		t.Error("sin traspaso debería fallar")
	}
}

// Claude: si el corte falla antes de responder (no encuentra el mensaje), se pasa al traspaso; si va bien, no.
func TestCorteClaudeFallaYTraspasa(t *testing.T) {
	var vistos []Peticion
	a := &Agente{Clave: "claude-code", Nombre: "Prueba", Programa: "sh", Lee: func(r io.Reader, emite func(Evento)) error {
		b, _ := io.ReadAll(r)
		if len(b) > 0 {
			emite(Evento{Texto: string(b)})
		}
		return nil
	}, Args: func(msg string, p Peticion) []string {
		vistos = append(vistos, p)
		if p.Corte != "" {
			return []string{"-c", "echo 'No message found' >&2; exit 1"}
		}
		return []string{"-c", "echo ok"}
	}}
	p := Peticion{Carpeta: t.TempDir(), SesionAgente: "s1", Corte: "u-9", Traspaso: "T", Mensajes: []Mensaje{{Rol: "user", Texto: "hola"}}}
	if err := a.Chat(context.Background(), p, func(Evento) {}); err != nil {
		t.Fatal(err)
	}
	if len(vistos) != 2 || vistos[0].Corte != "u-9" || vistos[1].SesionAgente != "" || vistos[1].Corte != "" {
		t.Errorf("peticiones: %+v", vistos)
	}
	// Punto con forma de opción: rechazado antes de lanzar nada.
	vistos = nil
	p.Corte = "--dangerously-skip-permissions"
	if err := a.Chat(context.Background(), p, func(Evento) {}); err == nil || len(vistos) > 0 {
		t.Errorf("punto no válido aceptado: %v", err)
	}
}

func TestConsultasCorteOpenCode(t *testing.T) {
	q := consultasCorteOpenCode("ses_A", "msg_5")
	if len(q) != 3 || !strings.HasPrefix(q[0], "SELECT") || !strings.Contains(q[1], "id>'msg_5'") || !strings.HasPrefix(q[2], "DELETE FROM message") {
		t.Errorf("consultas: %q", q)
	}
	if err := cortaOpenCode(context.Background(), &Agente{}, Peticion{}, "ses_A", "msg_5'; DROP TABLE x;--"); err == nil {
		t.Error("mensaje no válido aceptado")
	}
}
