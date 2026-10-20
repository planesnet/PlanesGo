package llm

import (
	"context"
	"strings"
	"testing"
	"testing/iotest"
)

// A1: el id de sesión llega del navegador y va como argumento: no puede ser una opción del programa.
func TestSesionAgenteValidada(t *testing.T) {
	a := &Agente{Clave: "x", Programa: "true", Args: func(string, Peticion) []string { return nil }}
	for _, mala := range []string{"--dangerously-skip-permissions", "-p", "a b", "../x", strings.Repeat("a", 200)} {
		err := a.Chat(context.Background(), Peticion{Carpeta: t.TempDir(), SesionAgente: mala, Mensajes: []Mensaje{{Rol: "user", Texto: "hola"}}}, func(Evento) {})
		if err == nil || !strings.Contains(err.Error(), "sesión") {
			t.Errorf("sesión %q aceptada (err=%v)", mala, err)
		}
	}
	if !sesionValida.MatchString("0f8c6c2e-5d0b-4b5c-9a6e-1d2f3a4b5c6d") {
		t.Error("un UUID debe ser válido")
	}
}

// A1: el mensaje va después de «--», así que un texto que empiece por «-» no es una opción.
func TestMensajeNoEsOpcion(t *testing.T) {
	for _, p := range Agentes() {
		a := p.(*Agente)
		if a.NoDisponible != "" {
			continue
		}
		args := a.Args("- arregla el test", Peticion{})
		ultimo := args[len(args)-1]
		if ultimo == "- arregla el test" && (len(args) < 2 || args[len(args)-2] != "--") {
			t.Errorf("%s: el mensaje va sin «--» delante: %q", a.Clave, args)
		}
	}
}

// M3: leeTexto no parte caracteres UTF-8 aunque lleguen de byte en byte.
func TestLeeTextoUTF8(t *testing.T) {
	texto := "Añadir ñandú y tilde: canción 🚀 fin"
	var b strings.Builder
	if err := leeTexto(iotest.OneByteReader(strings.NewReader(texto)), func(e Evento) { b.WriteString(e.Texto) }); err != nil {
		t.Fatal(err)
	}
	if b.String() != texto {
		t.Fatalf("salida %q", b.String())
	}
}

// Solo hay agentes: Claude Code el primero, Antigravity y OpenCode.
func TestSoloAgentes(t *testing.T) {
	l := NuevoRegistro(Agentes()...).Lista(context.Background())
	for _, i := range l {
		if i.Tipo != "agente" {
			t.Errorf("%s no es un agente", i.ID)
		}
	}
	for _, id := range []string{"claude-code", "antigravity", "opencode"} {
		if _, ok := NuevoRegistro(Agentes()...).Busca(id); !ok {
			t.Error("falta " + id)
		}
	}
	if _, ok := NuevoRegistro(Agentes()...).Busca("gemini-cli"); ok {
		t.Error("Gemini CLI ya no es un agente")
	}
}

// Los permisos de la sesión llegan a Claude Code como --permission-mode.
func TestPermisosClaudeCode(t *testing.T) {
	a := Agentes()[0].(*Agente)
	for permisos, quiero := range map[string]string{"": "", "lectura": "", "editar": "acceptEdits", "auto": "auto"} {
		args := strings.Join(a.Args("hola", Peticion{Permisos: permisos}), " ")
		if quiero == "" && strings.Contains(args, "--permission-mode") || quiero != "" && !strings.Contains(args, "--permission-mode "+quiero) {
			t.Errorf("permisos %q: %s", permisos, args)
		}
	}
}

// El modelo y el esfuerzo llegan a Claude Code como --model y --effort («por defecto»: sin opción).
func TestModeloEsfuerzoClaudeCode(t *testing.T) {
	a := Agentes()[0].(*Agente)
	if args := strings.Join(a.Args("hola", Peticion{Modelo: "por defecto"}), " "); strings.Contains(args, "--model") || strings.Contains(args, "--effort") {
		t.Errorf("sin modelo ni esfuerzo: %s", args)
	}
	if args := strings.Join(a.Args("hola", Peticion{Modelo: "opus", Esfuerzo: "high"}), " "); !strings.Contains(args, "--model opus") || !strings.Contains(args, "--effort high") {
		t.Errorf("con modelo y esfuerzo: %s", args)
	}
}

// Antigravity: el mensaje va pegado a --print=, solo lectura es el modo plan y la conversación se continúa.
func TestArgsAntigravity(t *testing.T) {
	a := Agentes()[1].(*Agente)
	args := a.Args("--hola", Peticion{SesionAgente: "abc", Modelo: "gemini-4-argon-high"})
	j := strings.Join(args, " ")
	if args[len(args)-1] != "--print=--hola" || !strings.Contains(j, "--mode plan") || !strings.Contains(j, "--conversation abc") || !strings.Contains(j, "--model gemini-4-argon-high") {
		t.Errorf("args: %q", args)
	}
}

func TestLeeAntigravity(t *testing.T) {
	in := `{"event":"init","conversation_id":"c1"}
{"event":"step_update","step_update":{"state":"ACTIVE","step_type":"tool","tool_name":"read_file"}}
{"event":"step_update","step_update":{"state":"ACTIVE","step_type":"agent_response","text_delta":"ho"}}
{"event":"step_update","step_update":{"state":"DONE","step_type":"agent_response","text_delta":"la"}}
{"event":"result","result":{"status":"SUCCESS"}}`
	var b strings.Builder
	sesion := ""
	if err := leeAntigravity(strings.NewReader(in), func(e Evento) {
		b.WriteString(e.Texto)
		if e.SesionAgente != "" {
			sesion = e.SesionAgente
		}
	}); err != nil {
		t.Fatal(err)
	}
	if sesion != "c1" || !strings.Contains(b.String(), "read_file") || !strings.HasSuffix(b.String(), "hola") {
		t.Errorf("sesión %q, texto %q", sesion, b.String())
	}
}
