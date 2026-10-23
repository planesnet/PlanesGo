package selector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// La «IA local»: la que usa pgocode sin gastar tokens de pago (embeddings del hilo de limpieza y del selector de
// skills; más adelante, un modelo de texto). Por defecto, el Ollama del propio equipo; puede ser un servidor
// compartido en la red (Ollama o compatible con OpenAI, p. ej. vLLM), con token. Si el configurado no responde, se
// usa el Ollama del equipo si lo hay; si tampoco, quien llama sigue sin IA (solo reglas): nunca la de pago.

// LocalAI: la configuración (en ia-local.json, en los datos de pgocode).
type LocalAI struct {
	URL        string `json:"url"`
	Token      string `json:"token,omitempty"`
	EmbedModel string `json:"modelo_embeddings,omitempty"`
	ChatModel  string `json:"modelo_texto,omitempty"` // opcional: aún no lo usa nadie
}

// LocalAIState: cómo está el servidor configurado (y, si no responde, el del equipo).
type LocalAIState struct {
	URL      string   `json:"url"`
	API      string   `json:"api,omitempty"` // «ollama» u «openai»
	Ready    bool     `json:"listo"`         // responde y tiene el modelo de embeddings
	Models   []string `json:"modelos,omitempty"`
	Error    string   `json:"error,omitempty"`
	Fallback bool     `json:"reserva,omitempty"` // se está usando el Ollama del equipo en su lugar
}

var (
	muLocalAI   sync.Mutex
	localAIFile string
	localAI     = LocalAI{URL: ollama, EmbedModel: modeloLocal}
	// sondeos: lo último que se supo de cada servidor (se pregunta como mucho cada 30 s).
	sondeos = map[string]sondeo{}
)

type sondeo struct {
	en     time.Time
	estado LocalAIState
}

// SetLocalAIFile: dónde se guarda la configuración (al arrancar); la lee si existe.
func SetLocalAIFile(f string) {
	muLocalAI.Lock()
	defer muLocalAI.Unlock()
	localAIFile = f
	if b, err := os.ReadFile(f); err == nil {
		var c LocalAI
		if json.Unmarshal(b, &c) == nil {
			localAI = c.withDefaults()
		}
	}
}

func (c LocalAI) withDefaults() LocalAI {
	c.URL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
	if c.URL == "" {
		c.URL = ollama
	}
	if c.EmbedModel == "" {
		c.EmbedModel = modeloLocal
	}
	return c
}

// LocalAIConfig: la configuración en uso.
func LocalAIConfig() LocalAI {
	muLocalAI.Lock()
	defer muLocalAI.Unlock()
	return localAI
}

// SaveLocalAI guarda la configuración (la URL tiene que ser http o https).
func SaveLocalAI(c LocalAI) error {
	c = c.withDefaults()
	u, err := url.Parse(c.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("la URL tiene que ser http:// o https:// con servidor")
	}
	muLocalAI.Lock()
	defer muLocalAI.Unlock()
	if localAIFile != "" {
		b, _ := json.MarshalIndent(c, "", "  ")
		os.MkdirAll(filepath.Dir(localAIFile), 0o700)
		tmp := localAIFile + ".tmp"
		if err := os.WriteFile(tmp, b, 0o600); err != nil {
			return err
		}
		if err := os.Rename(tmp, localAIFile); err != nil {
			return err
		}
	}
	localAI = c
	sondeos = map[string]sondeo{}
	return nil
}

// probe: si el servidor responde (Ollama: /api/tags; si no, OpenAI: /v1/models) y tiene el modelo.
func probe(ctx context.Context, c LocalAI, fresco bool) LocalAIState {
	muLocalAI.Lock()
	if s, ok := sondeos[c.URL]; ok && !fresco && time.Since(s.en) < 30*time.Second {
		muLocalAI.Unlock()
		return s.estado
	}
	muLocalAI.Unlock()
	ctx, cancela := context.WithTimeout(ctx, 3*time.Second)
	defer cancela()
	e := LocalAIState{URL: c.URL}
	var r struct {
		Models []struct{ Name string } `json:"models"`
		Data   []struct{ ID string }   `json:"data"`
	}
	err := getJSON(ctx, c, "/api/tags", &r)
	if err == nil && r.Models != nil {
		e.API = "ollama"
		for _, m := range r.Models {
			e.Models = append(e.Models, m.Name)
		}
	} else if err2 := getJSON(ctx, c, "/v1/models", &r); err2 == nil && r.Data != nil {
		e.API = "openai"
		for _, m := range r.Data {
			e.Models = append(e.Models, m.ID)
		}
	} else {
		if err == nil {
			err = err2
		}
		e.Error = err.Error()
	}
	if e.API != "" {
		for _, m := range e.Models {
			if m == c.EmbedModel || m == c.EmbedModel+":latest" {
				e.Ready = true
			}
		}
		if !e.Ready {
			e.Error = "no tiene el modelo " + c.EmbedModel
		}
	}
	muLocalAI.Lock()
	sondeos[c.URL] = sondeo{time.Now(), e}
	muLocalAI.Unlock()
	return e
}

func getJSON(ctx context.Context, c LocalAI, ruta string, v any) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.URL+ruta, nil)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", ruta, res.Status)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(v)
}

// activa: el servidor que se usa ahora: el configurado si está listo; si no, el Ollama del equipo si lo está.
func activa(ctx context.Context) (LocalAI, LocalAIState) {
	c := LocalAIConfig()
	e := probe(ctx, c, false)
	if e.Ready || c.URL == ollama {
		return c, e
	}
	local := LocalAI{URL: ollama, EmbedModel: c.EmbedModel}
	if l := probe(ctx, local, false); l.Ready {
		l.Fallback = true
		return local, l
	}
	return c, e
}

// LocalAIStatus: el estado para la interfaz (fresco: sin esperar a los 30 s).
func LocalAIStatus(ctx context.Context, fresco bool) LocalAIState {
	c := LocalAIConfig()
	e := probe(ctx, c, fresco)
	if e.Ready || c.URL == ollama {
		return e
	}
	local := LocalAI{URL: ollama, EmbedModel: c.EmbedModel}
	if l := probe(ctx, local, fresco); l.Ready {
		e.Fallback = true
	}
	return e
}

// embedLocal: el embedding con el servidor activo (Ollama: /api/embed; OpenAI: /v1/embeddings).
func embedLocal(ctx context.Context, cli *http.Client, texto string) ([]float32, error) {
	c, e := activa(ctx)
	if !e.Ready {
		if e.Error == "" {
			e.Error = "no responde"
		}
		return nil, fmt.Errorf("IA local (%s): %s", c.URL, e.Error)
	}
	ruta, cuerpo := "/api/embed", map[string]any{"model": c.EmbedModel, "input": texto, "keep_alive": "30m"}
	if e.API == "openai" {
		ruta, cuerpo = "/v1/embeddings", map[string]any{"model": c.EmbedModel, "input": texto}
	}
	b, _ := json.Marshal(cuerpo)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.URL+ruta, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var r struct {
		Embeddings [][]float32 `json:"embeddings"`
		Data       []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error any `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("IA local: %s", res.Status)
	}
	switch {
	case len(r.Embeddings) > 0:
		return r.Embeddings[0], nil
	case len(r.Data) > 0 && len(r.Data[0].Embedding) > 0:
		return r.Data[0].Embedding, nil
	}
	return nil, fmt.Errorf("IA local: %s %v", res.Status, r.Error)
}

// localMotorName: el nombre del motor local (va en la huella de los vectores: el mismo modelo da los mismos en
// cualquier servidor; otro modelo, otros).
func localMotorName() string {
	return "ollama/" + LocalAIConfig().EmbedModel
}

// chatModelsKnown: modelos de texto pequeños que se usan solos si están en el servidor y no se ha elegido ninguno.
var chatModelsKnown = []string{"gemma3:4b", "gemma3:1b", "qwen3:4b", "gemma3:12b"}

// LocalChatModel: el modelo de texto de la IA local: el elegido o, si no hay, uno conocido que tenga el servidor
// («» si ninguno).
func LocalChatModel(ctx context.Context) (LocalAI, LocalAIState, string) {
	c, e := activa(ctx)
	if e.API == "" {
		return c, e, ""
	}
	tiene := func(m string) bool {
		for _, x := range e.Models {
			if x == m || x == m+":latest" {
				return true
			}
		}
		return false
	}
	if c.ChatModel != "" {
		if tiene(c.ChatModel) {
			return c, e, c.ChatModel
		}
		return c, e, ""
	}
	for _, m := range chatModelsKnown {
		if tiene(m) {
			return c, e, m
		}
	}
	return c, e, ""
}

// LocalChat: una respuesta corta del modelo de texto de la IA local (sin tokens de pago). Devuelve el modelo usado.
func LocalChat(ctx context.Context, sistema, texto string, maxSalida int) (string, string, error) {
	c, e, modelo := LocalChatModel(ctx)
	if modelo == "" {
		return "", "", errors.New("la IA local no tiene modelo de texto")
	}
	msgs := []map[string]string{{"role": "system", "content": sistema}, {"role": "user", "content": texto}}
	ruta, cuerpo := "/api/chat", map[string]any{"model": modelo, "messages": msgs, "stream": false, "keep_alive": "30m",
		"options": map[string]any{"temperature": 0, "num_predict": maxSalida}}
	if e.API == "openai" {
		ruta, cuerpo = "/v1/chat/completions", map[string]any{"model": modelo, "messages": msgs, "temperature": 0, "max_tokens": maxSalida}
	}
	b, _ := json.Marshal(cuerpo)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.URL+ruta, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", modelo, err
	}
	defer res.Body.Close()
	var r struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error any `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&r); err != nil || res.StatusCode != http.StatusOK {
		return "", modelo, fmt.Errorf("IA local: %s %v", res.Status, r.Error)
	}
	if len(r.Choices) > 0 {
		return r.Choices[0].Message.Content, modelo, nil
	}
	return r.Message.Content, modelo, nil
}
