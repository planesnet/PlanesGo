package llm

import (
	"context"
	"strings"
	"testing"
	"testing/iotest"
)

// A1: el id de sesión llega del navegador y va como argumento: no puede ser una opción del programa.
func TestSesionAgenteValidada(t *testing.T) {
	a := &Agente{Clave: "x", Programa: "true", Args: func(m, s, p string) []string { return nil }}
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
		args := a.Args("- arregla el test", "", "")
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

// Solo hay agentes: Claude Code el primero; Antigravity aparece, pero no se puede usar todavía.
func TestSoloAgentes(t *testing.T) {
	l := NuevoRegistro(Agentes()...).Lista(context.Background())
	for _, i := range l {
		if i.Tipo != "agente" {
			t.Errorf("%s no es un agente", i.ID)
		}
		if i.ID == "antigravity" && i.Disponible {
			t.Error("Antigravity no debería estar disponible")
		}
	}
	if a, ok := NuevoRegistro(Agentes()...).Busca("claude-code"); !ok || a.ID() != "claude-code" {
		t.Error("falta Claude Code")
	}
}

// Los permisos de la sesión llegan a Claude Code como --permission-mode.
func TestPermisosClaudeCode(t *testing.T) {
	a := Agentes()[0].(*Agente)
	for permisos, quiero := range map[string]string{"": "", "lectura": "", "editar": "acceptEdits", "auto": "auto"} {
		args := strings.Join(a.Args("hola", "", permisos), " ")
		if quiero == "" && strings.Contains(args, "--permission-mode") || quiero != "" && !strings.Contains(args, "--permission-mode "+quiero) {
			t.Errorf("permisos %q: %s", permisos, args)
		}
	}
}
