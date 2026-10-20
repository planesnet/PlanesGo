package llm

import (
	"context"
	"strings"
	"testing"
	"testing/iotest"
)

// A1: el id de sesión llega del navegador y va como argumento: no puede ser una opción del programa.
func TestSesionAgenteValidada(t *testing.T) {
	a := &Agente{Clave: "x", Programa: "true", Args: func(m, s string) []string { return nil }}
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
		args := a.Args("- arregla el test", "")
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

// A5: crear el registro no llama a Info (que puede ir a la red).
func TestRegistroSinRed(t *testing.T) {
	r := NuevoRegistro(&Claude{}, &ChatGPT{}, &Gemini{})
	for _, id := range []string{"claude", "openai", "gemini"} {
		if _, ok := r.Busca(id); !ok {
			t.Errorf("falta %s", id)
		}
	}
}
