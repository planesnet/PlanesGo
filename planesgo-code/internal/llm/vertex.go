package llm

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/depuracion"
)

// DeepSeek en Vertex AI (Model Garden, «MaaS»): usa el punto de acceso compatible con OpenAI, que el proveedor
// google-vertex de OpenCode no conoce. pgocode le pasa a OpenCode un proveedor propio (OPENCODE_CONFIG_CONTENT)
// con el token de Google del usuario (credenciales por defecto de gcloud), que caduca en una hora.
const (
	proveedorDeepSeekVertex = "vertex-deepseek"
	modeloDeepSeekVertex    = proveedorDeepSeekVertex + "/deepseek-ai/deepseek-v3.2-maas"
)

var (
	muToken     sync.Mutex
	tokenGoogle string
	tokenHasta  time.Time
)

// tokenVertex devuelve un token de acceso de Google (se renueva a los 45 minutos).
func tokenVertex() (string, error) {
	muToken.Lock()
	defer muToken.Unlock()
	if tokenGoogle != "" && time.Now().Before(tokenHasta) {
		return tokenGoogle, nil
	}
	ctx, cancela := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancela()
	b, err := exec.CommandContext(ctx, "gcloud", "auth", "application-default", "print-access-token").Output()
	if err != nil {
		return "", err
	}
	tokenGoogle, tokenHasta = strings.TrimSpace(string(b)), time.Now().Add(45*time.Minute)
	return tokenGoogle, nil
}

// configDeepSeekVertex: la configuración de OpenCode con el proveedor de DeepSeek en Vertex AI.
func configDeepSeekVertex() (string, error) {
	t, err := tokenVertex()
	if err != nil {
		return "", err
	}
	proyecto := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if proyecto == "" {
		proyecto = "planes-dev"
	}
	c := map[string]any{"provider": map[string]any{proveedorDeepSeekVertex: map[string]any{
		"npm":  "@ai-sdk/openai-compatible",
		"name": "DeepSeek (Vertex AI)",
		"options": map[string]any{
			"baseURL": "https://aiplatform.googleapis.com/v1/projects/" + proyecto + "/locations/global/endpoints/openapi",
			"apiKey":  t,
		},
		"models": map[string]any{"deepseek-ai/deepseek-v3.2-maas": map[string]any{
			"name": "DeepSeek V3.2", "tool_call": true, "limit": map[string]int{"context": 163840, "output": 32768},
		}},
	}}}
	b, _ := json.Marshal(c)
	return string(b), nil
}

// entornoOpenCode: permisos según el modo y, con DeepSeek en Vertex, su proveedor.
func entornoOpenCode(p Peticion) []string {
	e := []string{"OPENCODE_PERMISSION=" + permisosOpenCode(p.Permisos)}
	if strings.HasPrefix(p.Modelo, proveedorDeepSeekVertex+"/") {
		c, err := configDeepSeekVertex()
		if err != nil {
			depuracion.Log("opencode", "sin token de Google para DeepSeek en Vertex: %v", err)
		} else {
			e = append(e, "OPENCODE_CONFIG_CONTENT="+c)
		}
	}
	return e
}

// modelosOpenCode: Gemini y DeepSeek en Vertex AI; y los de la API de DeepSeek si hay clave.
func modelosOpenCode() []string {
	l := []string{"google-vertex/gemini-4-argon", modeloDeepSeekVertex}
	if os.Getenv("DEEPSEEK_API_KEY") != "" {
		l = append(l, "deepseek/deepseek-v4-pro", "deepseek/deepseek-v4-flash")
	}
	return l
}
