package llm

import (
	"encoding/json"
	"os"
	"strings"

	"planesgo-code/internal/depuracion"
	"planesgo-code/internal/google"
)

// DeepSeek en Vertex AI (Model Garden, «MaaS»): usa el punto de acceso compatible con OpenAI, que el proveedor
// google-vertex de OpenCode no conoce. pgocode le pasa a OpenCode un proveedor propio (OPENCODE_CONFIG_CONTENT)
// con el token de Google del usuario (credenciales por defecto de gcloud), que caduca en una hora.
const (
	proveedorDeepSeekVertex = "vertex-deepseek"
	modeloDeepSeekVertex    = proveedorDeepSeekVertex + "/deepseek-ai/deepseek-v3.2-maas"
)

// configDeepSeekVertex: la configuración de OpenCode con el proveedor de DeepSeek en Vertex AI.
func configDeepSeekVertex() (string, error) {
	t, err := google.Token()
	if err != nil {
		return "", err
	}
	proyecto := google.Proyecto()
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
