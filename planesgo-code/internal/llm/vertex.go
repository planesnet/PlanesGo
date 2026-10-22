package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

// proveedorDeepSeek: el proveedor de DeepSeek en Vertex AI para OpenCode, con el token de Google del usuario.
// limite: el «contexto» que se le declara a OpenCode (ver openCodeContextLimit), sin pasar del real del modelo (0: el
// real).
func proveedorDeepSeek(limite int) (map[string]any, error) {
	if limite <= 0 {
		limite = 163840
	}
	t, err := google.Token()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"npm":  "@ai-sdk/openai-compatible",
		"name": "DeepSeek (Vertex AI)",
		"options": map[string]any{
			"baseURL": "https://aiplatform.googleapis.com/v1/projects/" + google.Proyecto() + "/locations/global/endpoints/openapi",
			"apiKey":  t,
		},
		"models": map[string]any{"deepseek-ai/deepseek-v3.2-maas": map[string]any{
			"name": "DeepSeek V3.2", "tool_call": true, "limit": map[string]int{"context": min(limite, 163840), "output": 32768},
		}},
	}, nil
}

// entornoOpenCode: permisos según el modo y, con DeepSeek en Vertex, su proveedor (OPENCODE_CONFIG_CONTENT).
func entornoOpenCode(p Peticion) []string {
	e := []string{"OPENCODE_PERMISSION=" + permisosOpenCode(p.Permisos, p.Autorizadas)}
	// Carpeta de datos propia (sesiones de OpenCode de pgocode): la de OpenCode Desktop tiene otro formato de base
	// de datos que la versión de terminal no abre.
	if d := datosOpenCode(); d != "" {
		e = append(e, "XDG_DATA_HOME="+d)
	}
	cfg := configOpenCode(p.Modelo, openCodeContextLimit(p))
	if strings.HasPrefix(p.Modelo, proveedorDeepSeekVertex+"/") {
		if prov, err := proveedorDeepSeek(openCodeContextLimit(p)); err != nil {
			depuracion.Log("opencode", "sin token de Google para DeepSeek en Vertex: %v", err)
		} else {
			cfg["provider"].(map[string]any)[proveedorDeepSeekVertex] = prov
		}
	}
	b, _ := json.Marshal(cfg)
	return append(e, "OPENCODE_CONFIG_CONTENT="+string(b))
}

// Ahorro de tokens en OpenCode (cada paso reenvía todo el historial):
//   - compactación automática con poda de salidas viejas de herramientas;
//   - como mucho PasosOpenCode pasos por petición (luego responde sin más herramientas);
//   - para el modelo de la sesión, un «contexto» a medida (limite, ver openCodeContextLimit): compacta al pasar
//     del umbral de la sesión (si no, crecería hasta el millón y cada paso costaría cientos de miles de tokens);
//     sin él (limite 0, la gestión del contexto desactivada), el del catálogo de OpenCode.
const PasosOpenCode = 40

// ContinuacionesOpenCode: cuántas peticiones nuevas encadena un turno de OpenCode al llegar al límite de pasos
// (Chat); textoContinuacion, lo que se le dice en cada una (corto: va en su historial).
const ContinuacionesOpenCode = 5

const textoContinuacion = "Sigue con la tarea donde lo dejaste."

func configOpenCode(modelo string, limite int) map[string]any {
	cfg := map[string]any{
		"compaction": map[string]any{"auto": true, "prune": true},
		"agent":      map[string]any{"build": map[string]any{"steps": PasosOpenCode}},
		"provider":   map[string]any{},
	}
	// Cualquier proveedor del catálogo de OpenCode (models.dev): el límite se mezcla con el suyo. El de DeepSeek en
	// Vertex AI lo pone pgocode entero (proveedorDeepSeek). La salida no cambia: OpenCode pide como mucho 32k.
	if prov, id, ok := strings.Cut(modelo, "/"); ok && prov != proveedorDeepSeekVertex && limite > 0 {
		cfg["provider"].(map[string]any)[prov] = map[string]any{"models": map[string]any{
			id: map[string]any{"limit": map[string]int{"context": limite, "output": openCodeOutput}}}}
	}
	return cfg
}

// modelosOpenCode: Gemini y DeepSeek en Vertex AI; y los de la API de DeepSeek si hay clave.
func modelosOpenCode() []string {
	l := []string{"google-vertex/gemini-4-argon", modeloDeepSeekVertex}
	if os.Getenv("DEEPSEEK_API_KEY") != "" {
		l = append(l, "deepseek/deepseek-v4-pro", "deepseek/deepseek-v4-flash")
	}
	return l
}

var (
	unaVezOpenCode sync.Once
	dirOpenCode    string
)

// datosOpenCode: ~/.local/share/pgocode (con opencode/auth.json copiado del OpenCode del usuario, para sus claves).
func datosOpenCode() string {
	unaVezOpenCode.Do(func() {
		casa, err := os.UserHomeDir()
		if err != nil {
			return
		}
		d := filepath.Join(casa, ".local", "share", "pgocode")
		if err := os.MkdirAll(filepath.Join(d, "opencode"), 0o700); err != nil {
			return
		}
		auth := filepath.Join(d, "opencode", "auth.json")
		if _, err := os.Stat(auth); os.IsNotExist(err) {
			if b, err := os.ReadFile(filepath.Join(casa, ".local", "share", "opencode", "auth.json")); err == nil {
				os.WriteFile(auth, b, 0o600)
			}
		}
		dirOpenCode = d
	})
	return dirOpenCode
}
