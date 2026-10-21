package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"planesgo-code/internal/google"
)

// Modelos baratos de Gemini en Vertex AI, llamados directamente (sin agente, sin herramientas ni historial): para
// las preguntas aparte sueltas y para decidir si un mensaje va a la conversación. Precios en $ por millón de tokens.
const (
	ModeloClasifica = "gemini-2.5-flash-lite"
	ModeloAparte    = "gemini-2.5-flash"
)

var preciosGemini = map[string][2]float64{
	ModeloClasifica: {0.10, 0.40},
	ModeloAparte:    {0.30, 2.50}, // la salida incluye el razonamiento
}

// urlGemini se puede cambiar en las pruebas.
var urlGemini = func(modelo string) string {
	return "https://aiplatform.googleapis.com/v1/projects/" + google.Proyecto() + "/locations/global/publishers/google/models/" + modelo + ":generateContent"
}

var tokenGemini = google.Token

// Gemini hace una pregunta a un modelo de Gemini en Vertex AI y devuelve su respuesta y el consumo.
// pensar: tokens de razonamiento (0: sin razonar).
func Gemini(ctx context.Context, modelo, sistema, texto string, maxSalida, pensar int) (string, Uso, error) {
	t, err := tokenGemini()
	if err != nil {
		return "", Uso{}, fmt.Errorf("sin acceso a Vertex AI (gcloud auth application-default login): %w", err)
	}
	cuerpo := map[string]any{
		"contents":         []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": texto}}}},
		"generationConfig": map[string]any{"maxOutputTokens": maxSalida + pensar, "thinkingConfig": map[string]int{"thinkingBudget": pensar}},
	}
	if sistema != "" {
		cuerpo["systemInstruction"] = map[string]any{"parts": []any{map[string]string{"text": sistema}}}
	}
	b, _ := json.Marshal(cuerpo)
	ctx, cancela := context.WithTimeout(ctx, 60*time.Second)
	defer cancela()
	inicio := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlGemini(modelo), bytes.NewReader(b))
	if err != nil {
		return "", Uso{}, err
	}
	req.Header.Set("Authorization", "Bearer "+t)
	req.Header.Set("Content-Type", "application/json")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", Uso{}, err
	}
	defer r.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	var res struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Usage struct {
			Prompt   int `json:"promptTokenCount"`
			Salida   int `json:"candidatesTokenCount"`
			Razonado int `json:"thoughtsTokenCount"`
		} `json:"usageMetadata"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", Uso{}, fmt.Errorf("Vertex AI (%d): %s", r.StatusCode, ultimas(string(raw), 300))
	}
	if res.Error != nil {
		return "", Uso{}, errors.New("Vertex AI: " + res.Error.Message)
	}
	var sb strings.Builder
	for _, c := range res.Candidates {
		for _, p := range c.Content.Parts {
			if !p.Thought {
				sb.WriteString(p.Text)
			}
		}
	}
	salida := res.Usage.Salida + res.Usage.Razonado
	u := Uso{Entrada: res.Usage.Prompt, Salida: salida, Segundos: time.Since(inicio).Seconds()}
	if pr, ok := preciosGemini[modelo]; ok {
		u.Coste = (float64(u.Entrada)*pr[0] + float64(salida)*pr[1]) / 1e6
	}
	return strings.TrimSpace(sb.String()), u, nil
}
