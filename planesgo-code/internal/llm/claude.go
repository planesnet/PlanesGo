package llm

import (
	"context"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Claude usa la API de Anthropic con el SDK oficial. Necesita ANTHROPIC_API_KEY.
type Claude struct{ cache cacheModelos }

const claudePorDefecto = "claude-opus-5-5"

func (c *Claude) cliente() anthropic.Client {
	return anthropic.NewClient(option.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")))
}

func (c *Claude) ID() string { return "claude" }

func (c *Claude) Info(ctx context.Context) Info {
	i := Info{ID: "claude", Nombre: "Claude (API)", Tipo: "api", PorDefecto: claudePorDefecto,
		Modelos: []string{claudePorDefecto, "claude-sonnet-5-5", "claude-haiku-5-5", "claude-fable-5-1"}}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		i.Motivo = "falta ANTHROPIC_API_KEY"
		return i
	}
	i.Disponible = true
	m := c.cache.get(func(ctx context.Context) ([]string, error) {
		cl := c.cliente()
		var l []string
		it := cl.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})
		for it.Next() {
			l = append(l, it.Current().ID)
		}
		return l, it.Err()
	})
	if len(m) > 0 {
		// El modelo por defecto primero, el resto en orden.
		l := []string{claudePorDefecto}
		for _, x := range m {
			if x != claudePorDefecto {
				l = append(l, x)
			}
		}
		i.Modelos = l
	}
	return i
}

func (c *Claude) Chat(ctx context.Context, p Peticion, emite func(Evento)) error {
	modelo := p.Modelo
	if modelo == "" {
		modelo = claudePorDefecto
	}
	msgs := make([]anthropic.MessageParam, 0, len(p.Mensajes))
	for _, m := range p.Mensajes {
		if strings.TrimSpace(m.Texto) == "" {
			continue
		}
		if m.Rol == "assistant" {
			msgs = append(msgs, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Texto)))
		} else {
			msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Texto)))
		}
	}
	params := anthropic.MessageNewParams{Model: anthropic.Model(modelo), MaxTokens: 64000, Messages: msgs}
	if p.Sistema != "" {
		params.System = []anthropic.TextBlockParam{{Text: p.Sistema}}
	}
	cl := c.cliente()
	stream := cl.Messages.NewStreaming(ctx, params)
	for stream.Next() {
		if ev, ok := stream.Current().AsAny().(anthropic.ContentBlockDeltaEvent); ok {
			if d, ok := ev.Delta.AsAny().(anthropic.TextDelta); ok {
				emite(Evento{Texto: d.Text})
			}
		}
	}
	return stream.Err()
}
