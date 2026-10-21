package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Pregunta aparte: Claude la hace sobre una copia que no se guarda; OpenCode sobre una copia (--fork).
func TestArgsAparte(t *testing.T) {
	cl, oc := Agentes()[0].(*Agente), Agentes()[2].(*Agente)
	args := strings.Join(cl.Args("hola", Peticion{SesionAgente: "abc", Aparte: true}), " ")
	if !strings.Contains(args, "--resume abc --fork-session --no-session-persistence") {
		t.Errorf("claude aparte: %s", args)
	}
	if args := strings.Join(cl.Args("hola", Peticion{SesionAgente: "abc"}), " "); strings.Contains(args, "fork") || strings.Contains(args, "persistence") {
		t.Errorf("claude normal: %s", args)
	}
	if args := strings.Join(cl.Args("hola", Peticion{Aparte: true}), " "); strings.Contains(args, "--fork-session") || !strings.Contains(args, "--no-session-persistence") {
		t.Errorf("claude aparte sin sesión: %s", args)
	}
	if args := strings.Join(oc.Args("hola", Peticion{SesionAgente: "ses_1", Aparte: true}), " "); !strings.Contains(args, "--session ses_1 --fork") {
		t.Errorf("opencode aparte: %s", args)
	}
	if args := strings.Join(oc.Args("hola", Peticion{SesionAgente: "ses_1"}), " "); strings.Contains(args, "--fork") {
		t.Errorf("opencode normal: %s", args)
	}
}

// Gemini en Vertex: texto sin el razonamiento, consumo con el razonamiento y coste según el modelo.
func TestGemini(t *testing.T) {
	var pedido map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tk" {
			t.Errorf("sin token: %v", r.Header)
		}
		json.NewDecoder(r.Body).Decode(&pedido)
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"pienso…","thought":true},{"text":" París "}]}}],
			"usageMetadata":{"promptTokenCount":1000,"candidatesTokenCount":10,"thoughtsTokenCount":90}}`))
	}))
	defer srv.Close()
	u0, t0 := urlGemini, tokenGemini
	defer func() { urlGemini, tokenGemini = u0, t0 }()
	urlGemini = func(m string) string { return srv.URL + "/" + m }
	tokenGemini = func() (string, error) { return "tk", nil }
	r, u, err := Gemini(context.Background(), ModeloAparte, "sé breve", "¿capital de Francia?", 100, 50)
	if err != nil || r != "París" || u.Entrada != 1000 || u.Salida != 100 {
		t.Fatalf("%q %+v %v", r, u, err)
	}
	if want := (1000*0.30 + 100*2.50) / 1e6; u.Coste < want*0.999 || u.Coste > want*1.001 {
		t.Errorf("coste %v, quiero %v", u.Coste, want)
	}
	if pedido["systemInstruction"] == nil || pedido["generationConfig"].(map[string]any)["maxOutputTokens"].(float64) != 150 {
		t.Errorf("petición: %v", pedido)
	}
}
