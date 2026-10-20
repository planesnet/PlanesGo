package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"planesgo-code/internal/proceso"
)

// sesionValida: los ids de sesión de los agentes son UUID o similares. Se valida porque el valor llega del
// navegador y va como argumento del programa (un «--opcion» se tomaría como opción).
var sesionValida = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

// Agente lanza un programa de agente (Claude Code, OpenCode, Gemini CLI…) en la carpeta del proyecto y
// retransmite su respuesta. Solo se le pasa el último mensaje: el agente lleva su propio historial.
type Agente struct {
	Clave, Nombre, Programa string
	// Args construye los argumentos a partir del mensaje y la petición (sesión anterior del agente, permisos,
	// modelo y esfuerzo, ya validados).
	Args func(msg string, p Peticion) []string
	// Modelos que se pueden elegir además de «por defecto», niveles de esfuerzo y si admite modos de permisos.
	Modelos, Esfuerzos []string
	Modos              bool
	// Lee interpreta la salida; si es nil, la salida se pasa tal cual.
	Lee func(r io.Reader, emite func(Evento)) error
	// NoDisponible: el agente existe pero no se puede usar desde aquí (y por qué).
	NoDisponible string
}

func (a *Agente) ID() string { return a.Clave }

func (a *Agente) Info(ctx context.Context) Info {
	i := Info{ID: a.Clave, Nombre: a.Nombre, Tipo: "agente", Modelos: append([]string{"por defecto"}, a.Modelos...), PorDefecto: "por defecto",
		NecesitaDir: true, Esfuerzos: a.Esfuerzos, Modos: a.Modos}
	if a.NoDisponible != "" {
		i.Motivo = a.NoDisponible
		return i
	}
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
	if p.SesionAgente != "" && !sesionValida.MatchString(p.SesionAgente) {
		return errors.New("identificador de sesión del agente no válido")
	}
	msg := p.Mensajes[len(p.Mensajes)-1].Texto
	cmd := exec.CommandContext(ctx, a.Programa, a.Args(msg, p)...)
	cmd.Dir = p.Carpeta
	// El agente va en su propio grupo de procesos: al cancelar se matan también sus hijos (servidores MCP,
	// shells…), y WaitDelay evita quedarse esperando a un nieto que conserve la salida abierta.
	proceso.GrupoPropio(cmd)
	cmd.WaitDelay = 5 * time.Second
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

// leeTexto pasa la salida tal cual, sin partir caracteres UTF-8 entre dos lecturas.
func leeTexto(r io.Reader, emite func(Evento)) error {
	b := make([]byte, 4096)
	var resto []byte
	for {
		n, err := r.Read(b)
		if n > 0 {
			datos := append(resto, b[:n]...)
			corte := len(datos)
			// Si el final es un carácter incompleto, se guarda para la siguiente lectura.
			for i := len(datos) - 1; i >= 0 && i >= len(datos)-utf8.UTFMax; i-- {
				if utf8.RuneStart(datos[i]) {
					if !utf8.FullRune(datos[i:]) {
						corte = i
					}
					break
				}
			}
			if corte > 0 {
				emite(Evento{Texto: string(datos[:corte])})
			}
			resto = append([]byte(nil), datos[corte:]...)
		}
		if err == io.EOF && len(resto) > 0 {
			emite(Evento{Texto: string(resto)})
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

// leeAntigravity interpreta la salida stream-json de `agy`: texto de las respuestas (text_delta), herramientas
// que usa, el id de conversación para continuar con --conversation y el resultado final.
func leeAntigravity(r io.Reader, emite func(Evento)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 32<<20)
	for sc.Scan() {
		var ev struct {
			Event          string `json:"event"`
			ConversationID string `json:"conversation_id"`
			Paso           struct {
				Estado string `json:"state"`
				Tipo   string `json:"step_type"`
				Texto  string `json:"text_delta"`
				Has    string `json:"tool_name"`
			} `json:"step_update"`
			Resultado struct {
				Estado string `json:"status"`
				Error  string `json:"error"`
			} `json:"result"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Event {
		case "init":
			if ev.ConversationID != "" {
				emite(Evento{SesionAgente: ev.ConversationID})
			}
		case "step_update":
			switch {
			case ev.Paso.Tipo == "agent_response" && ev.Paso.Texto != "":
				emite(Evento{Texto: ev.Paso.Texto})
			case ev.Paso.Tipo == "tool" && ev.Paso.Estado == "ACTIVE" && ev.Paso.Has != "":
				emite(Evento{Texto: "\n> 🔧 " + ev.Paso.Has + "\n\n"})
			}
		case "result":
			if ev.Resultado.Estado != "" && ev.Resultado.Estado != "SUCCESS" {
				if ev.Resultado.Error != "" {
					return errors.New(ev.Resultado.Error)
				}
				return errors.New("Antigravity terminó con estado " + ev.Resultado.Estado)
			}
		}
	}
	return sc.Err()
}

// Agentes devuelve los agentes conocidos: Claude Code, Antigravity y OpenCode.
func Agentes() []Proveedor {
	return []Proveedor{
		&Agente{Clave: "claude-code", Nombre: "Claude Code", Programa: "claude", Lee: leeClaudeCode, Modos: true,
			Modelos: []string{"opus", "sonnet", "haiku"}, Esfuerzos: []string{"low", "medium", "high", "xhigh", "max"},
			Args: func(msg string, p Peticion) []string {
				a := []string{"-p", "--output-format", "stream-json", "--verbose"}
				// Sin permiso solo lee; «editar» cambia ficheros; «auto» ejecuta comandos con su control de seguridad.
				switch p.Permisos {
				case "editar":
					a = append(a, "--permission-mode", "acceptEdits")
				case "auto":
					a = append(a, "--permission-mode", "auto")
				}
				if p.Modelo != "" && p.Modelo != "por defecto" {
					a = append(a, "--model", p.Modelo)
				}
				if p.Esfuerzo != "" {
					a = append(a, "--effort", p.Esfuerzo)
				}
				if p.SesionAgente != "" {
					a = append(a, "--resume", p.SesionAgente)
				}
				// «--» antes del mensaje: un texto que empiece por «-» no se toma como opción.
				return append(a, "--", msg)
			}},
		// Antigravity: su CLI «agy» en modo print con eventos stream-json; --conversation continúa la sesión.
		&Agente{Clave: "antigravity", Nombre: "Antigravity", Programa: "agy", Lee: leeAntigravity, Modos: true,
			Modelos: []string{"gemini-4-argon-high", "gemini-4-argon-medium", "gemini-4-argon-low", "gemini-3.8-flash-high",
				"gemini-3.8-flash-medium", "gemini-3.8-flash-low", "gemini-3.1-pro-high", "gemini-3.1-pro-low"},
			Esfuerzos: []string{"low", "medium", "high", "xhigh", "max"},
			Args: func(msg string, p Peticion) []string {
				a := []string{"--output-format", "stream-json"}
				// Solo lectura: modo plan (no cambia ficheros); «editar»: accept-edits; «auto»: auto.
				switch p.Permisos {
				case "editar":
					a = append(a, "--mode", "accept-edits")
				case "auto":
					a = append(a, "--mode", "auto")
				default:
					a = append(a, "--mode", "plan")
				}
				if p.Modelo != "" && p.Modelo != "por defecto" {
					a = append(a, "--model", p.Modelo)
				}
				if p.Esfuerzo != "" {
					a = append(a, "--effort", p.Esfuerzo)
				}
				if p.SesionAgente != "" {
					a = append(a, "--conversation", p.SesionAgente)
				}
				// El mensaje va pegado a la opción: así nunca se toma como otra opción.
				return append(a, "--print="+msg)
			}},
		&Agente{Clave: "opencode", Nombre: "OpenCode", Programa: "opencode",
			Modelos: []string{"google-vertex/gemini-4-argon"},
			Args: func(msg string, p Peticion) []string {
				a := []string{"run"}
				if p.Modelo != "" && p.Modelo != "por defecto" {
					a = append(a, "--model", p.Modelo)
				}
				return append(a, "--", msg)
			}},
	}
}
