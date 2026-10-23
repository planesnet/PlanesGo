package selector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// servidorIA: un servidor de IA simulado (api: «ollama» u «openai») que exige el token si se le da.
func servidorIA(t *testing.T, api, token string) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "sin permiso", http.StatusUnauthorized)
			return
		}
		switch {
		case api == "ollama" && r.URL.Path == "/api/tags":
			w.Write([]byte(`{"models":[{"name":"embeddinggemma:latest"}]}`))
		case api == "ollama" && r.URL.Path == "/api/embed":
			w.Write([]byte(`{"embeddings":[[1,2,3]]}`))
		case api == "openai" && r.URL.Path == "/v1/models":
			w.Write([]byte(`{"data":[{"id":"embeddinggemma"},{"id":"qwen3"}]}`))
		case api == "openai" && r.URL.Path == "/v1/embeddings":
			var p map[string]any
			json.NewDecoder(r.Body).Decode(&p)
			if p["model"] != "embeddinggemma" {
				http.Error(w, "modelo", http.StatusBadRequest)
				return
			}
			w.Write([]byte(`{"data":[{"embedding":[4,5,6]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func usaIA(t *testing.T, c LocalAI) {
	viejo := LocalAIConfig()
	t.Cleanup(func() { SaveLocalAI(viejo) })
	if err := SaveLocalAI(c); err != nil {
		t.Fatal(err)
	}
}

// Un servidor compartido con API de Ollama o de OpenAI (vLLM), con token: se detecta solo y se usa.
func TestLocalAIServidores(t *testing.T) {
	for _, api := range []string{"ollama", "openai"} {
		s := servidorIA(t, api, "secreto")
		usaIA(t, LocalAI{URL: s.URL + "/", Token: "secreto"})
		e := LocalAIStatus(context.Background(), true)
		if !e.Ready || e.API != api || e.Fallback {
			t.Fatalf("%s: %+v", api, e)
		}
		v, err := Ollama(context.Background(), nil, "hola", "RETRIEVAL_QUERY")
		if err != nil || len(v) != 3 {
			t.Fatalf("%s: %v %v", api, v, err)
		}
	}
	// Sin el token, no responde: el error se ve (y nunca se pasa a la IA de pago).
	s := servidorIA(t, "openai", "secreto")
	usaIA(t, LocalAI{URL: s.URL})
	if e := LocalAIStatus(context.Background(), true); e.Ready || e.Error == "" {
		t.Fatalf("sin token: %+v", e)
	}
	// Los embeddings salen entonces del Ollama del equipo, si lo hay (reserva); si no, error.
	if _, err := Ollama(context.Background(), nil, "hola", ""); (err == nil) != LocalAIStatus(context.Background(), false).Fallback {
		t.Fatalf("reserva: %v", err)
	}
}

// La configuración se guarda solo para el usuario y se vuelve a leer al arrancar; la URL tiene que ser http(s).
func TestLocalAIGuarda(t *testing.T) {
	f := filepath.Join(t.TempDir(), "ia-local.json")
	viejoF, viejo := localAIFile, LocalAIConfig()
	t.Cleanup(func() { localAIFile = viejoF; SaveLocalAI(viejo) })
	SetLocalAIFile(f)
	if err := SaveLocalAI(LocalAI{URL: "file:///etc"}); err == nil {
		t.Error("URL no válida")
	}
	if err := SaveLocalAI(LocalAI{URL: "http://ia.planes.lan:8000/", Token: "t"}); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(f); st == nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("permisos: %v", st)
	}
	localAI = LocalAI{}
	SetLocalAIFile(f)
	if c := LocalAIConfig(); c.URL != "http://ia.planes.lan:8000" || c.Token != "t" || c.EmbedModel != modeloLocal {
		t.Fatalf("%+v", c)
	}
}
