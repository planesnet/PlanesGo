package servidor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	// Al probar: una llamada de verdad a cada modelo (embeddings y texto), con lo que tarda.
	Pruebas []pruebaIALocal `json:"pruebas,omitempty"`
}

// pruebaIALocal: el resultado de probar un modelo de la IA local.
type pruebaIALocal struct {
	Tipo    string `json:"tipo"` // «embeddings» o «texto»
	Modelo  string `json:"modelo,omitempty"`
	OK      bool   `json:"ok"`
	Ms      int64  `json:"ms"`
	Detalle string `json:"detalle,omitempty"` // dimensiones del vector o la respuesta
	Error   string `json:"error,omitempty"`
}

// pruebaModelos: una llamada a cada modelo (la primera vez carga el modelo: puede tardar).
func pruebaModelos(ctx context.Context) []pruebaIALocal {
	ctx, cancela := context.WithTimeout(ctx, 60*time.Second)
	defer cancela()
	var out []pruebaIALocal
	t0 := time.Now()
	v, err := selector.Ollama(ctx, nil, "prueba de pgocode", "RETRIEVAL_QUERY")
	e := pruebaIALocal{Tipo: "embeddings", Modelo: selector.LocalAIConfig().EmbedModel, Ms: time.Since(t0).Milliseconds()}
	if err != nil {
		e.Error = err.Error()
	} else {
		e.OK, e.Detalle = true, fmt.Sprintf("vector de %d dimensiones", len(v))
	}
	out = append(out, e)
	t0 = time.Now()
	resp, modelo, err := selector.LocalChat(ctx, "Responde solo con la palabra: listo", "¿Funcionas?", 5)
	x := pruebaIALocal{Tipo: "texto", Modelo: modelo, Ms: time.Since(t0).Milliseconds()}
	if err != nil {
		x.Error = err.Error()
	} else {
		x.OK, x.Detalle = strings.TrimSpace(resp) != "", strings.TrimSpace(resp)
		if !x.OK {
			x.Error = "respuesta vacía"
		}
	}
	return append(out, x)
}

func vistaIALocal(r *http.Request, fresco bool) iaLocalVista {
	c := selector.LocalAIConfig()
	return iaLocalVista{URL: c.URL, ConToken: c.Token != "", EmbedModel: c.EmbedModel, ChatModel: c.ChatModel,
		Estado: selector.LocalAIStatus(r.Context(), fresco)}
}

// GET /api/ia-local[?probar=1]: la configuración y el estado (probar: sin esperar a los 30 s del último sondeo).
func (s *Servidor) iaLocal(w http.ResponseWriter, r *http.Request, _ *sesiones.Sesion) {
	probar := r.URL.Query().Get("probar") == "1"
	v := vistaIALocal(r, probar)
	if probar {
		v.Pruebas = pruebaModelos(r.Context())
	}
	jsonOK(w, v)
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
