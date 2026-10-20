package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// postSSE envía un JSON y llama a cada para cada línea "data:" de la respuesta en streaming.
func postSSE(ctx context.Context, url string, cab map[string]string, cuerpo any, cada func([]byte) error) error {
	b, err := json.Marshal(cuerpo)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range cab {
		req.Header.Set(k, v)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		e, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		return fmt.Errorf("%s: %s", r.Status, strings.TrimSpace(string(e)))
	}
	sc := bufio.NewScanner(r.Body)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		l := sc.Bytes()
		if !bytes.HasPrefix(l, []byte("data:")) {
			continue
		}
		d := bytes.TrimSpace(l[5:])
		if string(d) == "[DONE]" {
			return nil
		}
		if err := cada(d); err != nil {
			return err
		}
	}
	return sc.Err()
}

func getJSON(ctx context.Context, url string, cab map[string]string, res any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, v := range cab {
		req.Header.Set(k, v)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", r.Status)
	}
	return json.NewDecoder(r.Body).Decode(res)
}

// cacheModelos guarda la lista de modelos de una API durante una hora. Nunca bloquea: devuelve lo que tiene
// y, si está caducada, la vuelve a cargar en segundo plano (una sola carga a la vez).
type cacheModelos struct {
	mu       sync.Mutex
	modelos  []string
	hasta    time.Time
	cargando bool
}

func (c *cacheModelos) get(carga func(context.Context) ([]string, error)) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.cargando && time.Now().After(c.hasta) {
		c.cargando = true
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			l, err := carga(ctx)
			c.mu.Lock()
			defer c.mu.Unlock()
			c.cargando = false
			if err != nil || len(l) == 0 {
				c.hasta = time.Now().Add(time.Minute)
				return
			}
			sort.Strings(l)
			c.modelos, c.hasta = l, time.Now().Add(time.Hour)
		}()
	}
	return c.modelos
}

// esModeloChat descarta los modelos que no sirven para conversar (audio, imágenes, moderación, embeddings…).
func esModeloChat(id string) bool {
	for _, x := range []string{"audio", "realtime", "transcribe", "tts", "image", "moderation", "embedding", "search", "dall-e", "whisper", "aqa", "veo", "imagen"} {
		if strings.Contains(id, x) {
			return false
		}
	}
	return true
}

// elige el modelo por defecto: el preferido por entorno o el primero de la lista que contenga alguna de las
// pistas, en orden de preferencia; si no, el último de la lista (suele ser el más reciente).
func elige(modelos []string, entorno string, pistas ...string) string {
	if entorno != "" {
		return entorno
	}
	for _, p := range pistas {
		for i := len(modelos) - 1; i >= 0; i-- {
			if strings.Contains(modelos[i], p) {
				return modelos[i]
			}
		}
	}
	if len(modelos) > 0 {
		return modelos[len(modelos)-1]
	}
	return ""
}

// ChatGPT usa la API de OpenAI (Chat Completions en streaming). Necesita OPENAI_API_KEY.
// OPENAI_BASE_URL permite apuntar a cualquier servidor compatible (p. ej. modelos propios con vLLM u Ollama).
type ChatGPT struct{ cache cacheModelos }

func (c *ChatGPT) base() string {
	if u := os.Getenv("OPENAI_BASE_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "https://api.openai.com/v1"
}

func (c *ChatGPT) ID() string { return "openai" }

func (c *ChatGPT) Info(ctx context.Context) Info {
	i := Info{ID: "openai", Nombre: "ChatGPT (API)", Tipo: "api"}
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		i.Motivo = "falta OPENAI_API_KEY"
		return i
	}
	i.Disponible = true
	propio := os.Getenv("OPENAI_BASE_URL") != "" // servidor compatible (modelos propios): se listan todos
	i.Modelos = c.cache.get(func(ctx context.Context) ([]string, error) {
		var r struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := getJSON(ctx, c.base()+"/models", map[string]string{"Authorization": "Bearer " + key}, &r); err != nil {
			return nil, err
		}
		var l []string
		for _, m := range r.Data {
			chat := strings.HasPrefix(m.ID, "gpt-") || (len(m.ID) > 1 && m.ID[0] == 'o' && m.ID[1] >= '0' && m.ID[1] <= '9')
			if propio || (chat && esModeloChat(m.ID)) {
				l = append(l, m.ID)
			}
		}
		return l, nil
	})
	i.PorDefecto = elige(i.Modelos, os.Getenv("OPENAI_MODEL"), "gpt-5", "gpt-4.1", "gpt-4o")
	return i
}

func (c *ChatGPT) Chat(ctx context.Context, p Peticion, emite func(Evento)) error {
	msgs := []map[string]string{}
	if p.Sistema != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": p.Sistema})
	}
	for _, m := range p.Mensajes {
		msgs = append(msgs, map[string]string{"role": m.Rol, "content": m.Texto})
	}
	return postSSE(ctx, c.base()+"/chat/completions", map[string]string{"Authorization": "Bearer " + os.Getenv("OPENAI_API_KEY")},
		map[string]any{"model": p.Modelo, "messages": msgs, "stream": true}, func(d []byte) error {
			var ev struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal(d, &ev) == nil && len(ev.Choices) > 0 && ev.Choices[0].Delta.Content != "" {
				emite(Evento{Texto: ev.Choices[0].Delta.Content})
			}
			return nil
		})
}

// Gemini usa la API de Google (generativelanguage). Necesita GEMINI_API_KEY o GOOGLE_API_KEY.
type Gemini struct{ cache cacheModelos }

func geminiKey() string {
	if k := os.Getenv("GEMINI_API_KEY"); k != "" {
		return k
	}
	return os.Getenv("GOOGLE_API_KEY")
}

const geminiBase = "https://generativelanguage.googleapis.com/v1beta"

func (g *Gemini) ID() string { return "gemini" }

func (g *Gemini) Info(ctx context.Context) Info {
	i := Info{ID: "gemini", Nombre: "Gemini (API)", Tipo: "api"}
	key := geminiKey()
	if key == "" {
		i.Motivo = "falta GEMINI_API_KEY"
		return i
	}
	i.Disponible = true
	i.Modelos = g.cache.get(func(ctx context.Context) ([]string, error) {
		var r struct {
			Models []struct {
				Name    string   `json:"name"`
				Metodos []string `json:"supportedGenerationMethods"`
			} `json:"models"`
		}
		if err := getJSON(ctx, geminiBase+"/models?pageSize=200", map[string]string{"x-goog-api-key": key}, &r); err != nil {
			return nil, err
		}
		var l []string
		for _, m := range r.Models {
			id := strings.TrimPrefix(m.Name, "models/")
			if !strings.HasPrefix(id, "gemini-") || !esModeloChat(id) {
				continue
			}
			for _, x := range m.Metodos {
				if x == "generateContent" {
					l = append(l, id)
					break
				}
			}
		}
		return l, nil
	})
	i.PorDefecto = elige(i.Modelos, os.Getenv("GEMINI_MODEL"), "pro", "flash")
	return i
}

func (g *Gemini) Chat(ctx context.Context, p Peticion, emite func(Evento)) error {
	type parte struct {
		Text string `json:"text"`
	}
	type contenido struct {
		Role  string  `json:"role,omitempty"`
		Parts []parte `json:"parts"`
	}
	cs := []contenido{}
	for _, m := range p.Mensajes {
		rol := "user"
		if m.Rol == "assistant" {
			rol = "model"
		}
		cs = append(cs, contenido{Role: rol, Parts: []parte{{m.Texto}}})
	}
	cuerpo := map[string]any{"contents": cs}
	if p.Sistema != "" {
		cuerpo["systemInstruction"] = contenido{Parts: []parte{{p.Sistema}}}
	}
	url := geminiBase + "/models/" + neturl.PathEscape(p.Modelo) + ":streamGenerateContent?alt=sse"
	return postSSE(ctx, url, map[string]string{"x-goog-api-key": geminiKey()}, cuerpo, func(d []byte) error {
		var ev struct {
			Candidates []struct {
				Content contenido `json:"content"`
			} `json:"candidates"`
		}
		if json.Unmarshal(d, &ev) == nil {
			for _, c := range ev.Candidates {
				for _, pt := range c.Content.Parts {
					if pt.Text != "" {
						emite(Evento{Texto: pt.Text})
					}
				}
			}
		}
		return nil
	})
}
