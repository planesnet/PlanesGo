package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Agente lanza un programa de agente (Claude Code, OpenCode, Gemini CLI…) en la carpeta del proyecto y
// retransmite su respuesta. Solo se le pasa el último mensaje: el agente lleva su propio historial.
type Agente struct {
	ID, Nombre, Programa string
	// Args construye los argumentos a partir del mensaje y la sesión anterior del agente.
	Args func(msg, sesion string) []string
	// Lee interpreta la salida; si es nil, la salida se pasa tal cual.
	Lee func(r io.Reader, emite func(Evento)) error
}

func (a *Agente) Info(ctx context.Context) Info {
	i := Info{ID: a.ID, Nombre: a.Nombre, Tipo: "agente", Modelos: []string{"por defecto"}, PorDefecto: "por defecto", NecesitaDir: true}
	if _, err := exec.LookPath(a.Programa); err != nil {
		i.Motivo = "no está instalado («" + a.Programa + "» no está en el PATH)"
		return i
	}
	i.Disponible = true
	return i
}

func (a *Agente) Chat(ctx context.Context, p Peticion, emite func(Evento)) error {
	if p.Carpeta == "" {
		return errors.New("este proyecto no tiene carpeta local: falta un .planesgo.json con su odoo_project_id")
	}
	if len(p.Mensajes) == 0 {
		return errors.New("no hay mensaje")
	}
	msg := p.Mensajes[len(p.Mensajes)-1].Texto
	cmd := exec.CommandContext(ctx, a.Programa, a.Args(msg, p.SesionAgente)...)
	cmd.Dir = p.Carpeta
	var errSalida strings.Builder
	cmd.Stderr = &limitado{b: &errSalida, max: 8 << 10}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	lee := a.Lee
	if lee == nil {
		lee = leeTexto
	}
	errLee := lee(out, emite)
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s terminó con error: %v %s", a.Programa, err, strings.TrimSpace(errSalida.String()))
	}
	return errLee
}

type limitado struct {
	b   *strings.Builder
	max int
}

func (l *limitado) Write(p []byte) (int, error) {
	if l.b.Len() < l.max {
		l.b.Write(p[:min(len(p), l.max-l.b.Len())])
	}
	return len(p), nil
}

func leeTexto(r io.Reader, emite func(Evento)) error {
	b := make([]byte, 4096)
	for {
		n, err := r.Read(b)
		if n > 0 {
			emite(Evento{Texto: string(b[:n])})
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// leeClaudeCode interpreta la salida stream-json de `claude -p`: texto de los mensajes del asistente, nombres de
// las herramientas que usa y el id de sesión para continuar con --resume.
func leeClaudeCode(r io.Reader, emite func(Evento)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 32<<20)
	for sc.Scan() {
		var ev struct {
			Type      string `json:"type"`
			SessionID string `json:"session_id"`
			Message   struct {
				Content []struct {
					Type  string          `json:"type"`
					Text  string          `json:"text"`
					Name  string          `json:"name"`
					Input json.RawMessage `json:"input"`
				} `json:"content"`
			} `json:"message"`
			IsError bool   `json:"is_error"`
			Result  string `json:"result"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.SessionID != "" && ev.Type == "system" {
			emite(Evento{SesionAgente: ev.SessionID})
		}
		switch ev.Type {
		case "assistant":
			for _, c := range ev.Message.Content {
				switch c.Type {
				case "text":
					emite(Evento{Texto: c.Text + "\n"})
				case "tool_use":
					emite(Evento{Texto: "\n> 🔧 " + c.Name + "\n\n"})
				}
			}
		case "result":
			if ev.IsError {
				return errors.New(ev.Result)
			}
		}
	}
	return sc.Err()
}

// Agentes devuelve los agentes conocidos.
func Agentes() []Proveedor {
	return []Proveedor{
		&Agente{ID: "claude-code", Nombre: "Claude Code (agente)", Programa: "claude", Lee: leeClaudeCode,
			Args: func(msg, sesion string) []string {
				a := []string{"-p", msg, "--output-format", "stream-json", "--verbose"}
				if sesion != "" {
					a = append(a, "--resume", sesion)
				}
				return a
			}},
		&Agente{ID: "opencode", Nombre: "OpenCode (agente)", Programa: "opencode",
			Args: func(msg, sesion string) []string { return []string{"run", msg} }},
		&Agente{ID: "gemini-cli", Nombre: "Gemini CLI (agente)", Programa: "gemini",
			Args: func(msg, sesion string) []string { return []string{"-p", msg} }},
	}
}
