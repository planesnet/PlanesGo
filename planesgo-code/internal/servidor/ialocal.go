package servidor

import (
	"encoding/json"
	"net/http"

	"planesgo-code/internal/selector"
	"planesgo-code/internal/sesiones"
)

// iaLocalVista: la configuración de la IA local para la interfaz (el token nunca sale: solo si lo hay) y su estado.
type iaLocalVista struct {
	URL        string                `json:"url"`
	ConToken   bool                  `json:"con_token"`
	EmbedModel string                `json:"modelo_embeddings"`
	ChatModel  string                `json:"modelo_texto,omitempty"`
	Estado     selector.LocalAIState `json:"estado"`
}

func vistaIALocal(r *http.Request, fresco bool) iaLocalVista {
	c := selector.LocalAIConfig()
	return iaLocalVista{URL: c.URL, ConToken: c.Token != "", EmbedModel: c.EmbedModel, ChatModel: c.ChatModel,
		Estado: selector.LocalAIStatus(r.Context(), fresco)}
}

// GET /api/ia-local[?probar=1]: la configuración y el estado (probar: sin esperar a los 30 s del último sondeo).
func (s *Servidor) iaLocal(w http.ResponseWriter, r *http.Request, _ *sesiones.Sesion) {
	jsonOK(w, vistaIALocal(r, r.URL.Query().Get("probar") == "1"))
}

// POST /api/ia-local: guarda la configuración. El token: vacío lo deja como estaba; quitar_token lo borra.
func (s *Servidor) iaLocalGuarda(w http.ResponseWriter, r *http.Request, _ *sesiones.Sesion) {
	var p struct {
		URL         string `json:"url"`
		Token       string `json:"token"`
		QuitarToken bool   `json:"quitar_token"`
		EmbedModel  string `json:"modelo_embeddings"`
		ChatModel   string `json:"modelo_texto"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&p); err != nil {
		jsonError(w, http.StatusBadRequest, "petición no válida")
		return
	}
	c := selector.LocalAIConfig()
	c.URL, c.EmbedModel, c.ChatModel = p.URL, p.EmbedModel, p.ChatModel
	if p.QuitarToken {
		c.Token = ""
	} else if p.Token != "" {
		c.Token = p.Token
	}
	if err := selector.SaveLocalAI(c); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, vistaIALocal(r, true))
}
