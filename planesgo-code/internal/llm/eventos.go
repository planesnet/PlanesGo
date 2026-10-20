package llm

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxEntrada = 4 << 10
	maxSalida  = 8 << 10
)

// recorta deja s en n bytes como mucho, sin partir caracteres.
func recorta(s string, n int) string {
	if len(s) <= n {
		return s
	}
	c := n
	for c > 0 && !utf8.RuneStart(s[c]) {
		c--
	}
	return s[:c] + fmt.Sprintf("\n… (%d bytes más)", len(s)-c)
}

// resumen saca lo más útil de la entrada de una herramienta, como hace Claude Code en su lista: el fichero, la
// orden, el patrón, la URL… o, si no, el primer texto que encuentre.
func resumen(entrada json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(entrada, &m) != nil {
		return ""
	}
	for _, k := range []string{"command", "CommandLine", "file_path", "FilePath", "Path", "path", "pattern", "Pattern",
		"url", "Url", "query", "Query", "description", "prompt", "skill", "TargetFile", "AbsolutePath"} {
		if v, ok := m[k].(string); ok && v != "" {
			return primeraLinea(v)
		}
	}
	claves := make([]string, 0, len(m))
	for k := range m {
		claves = append(claves, k)
	}
	sort.Strings(claves)
	for _, k := range claves {
		if v, ok := m[k].(string); ok && v != "" {
			return primeraLinea(v)
		}
	}
	return ""
}

func primeraLinea(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if utf8.RuneCountInString(s) > 160 {
		s = string([]rune(s)[:157]) + "…"
	}
	return s
}

// textoDe convierte un contenido de tool_result (texto o lista de bloques) en texto.
func textoDe(c json.RawMessage) string {
	var s string
	if json.Unmarshal(c, &s) == nil {
		return s
	}
	var bloques []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(c, &bloques) == nil {
		var b strings.Builder
		for _, x := range bloques {
			if x.Type == "text" {
				b.WriteString(x.Text)
			} else if x.Type != "" {
				b.WriteString("[" + x.Type + "]")
			}
		}
		return b.String()
	}
	return string(c)
}

// leeClaudeCode interpreta la salida stream-json de `claude -p`: texto, razonamiento, herramientas con su
// entrada y su resultado, consumo del turno y el id de sesión para continuar con --resume.
func leeClaudeCode(r io.Reader, emite func(Evento)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 32<<20)
	nombres := map[string]string{}
	for sc.Scan() {
		var ev struct {
			Type      string `json:"type"`
			Subtype   string `json:"subtype"`
			SessionID string `json:"session_id"`
			Padre     string `json:"parent_tool_use_id"`
			Message   struct {
				Content []struct {
					Type      string          `json:"type"`
					Text      string          `json:"text"`
					Thinking  string          `json:"thinking"`
					ID        string          `json:"id"`
					Name      string          `json:"name"`
					Input     json.RawMessage `json:"input"`
					ToolUseID string          `json:"tool_use_id"`
					Content   json.RawMessage `json:"content"`
					IsError   bool            `json:"is_error"`
				} `json:"content"`
			} `json:"message"`
			IsError  bool    `json:"is_error"`
			Result   string  `json:"result"`
			Coste    float64 `json:"total_cost_usd"`
			Duracion float64 `json:"duration_ms"`
			Usage    struct {
				Input         int `json:"input_tokens"`
				CacheCreation int `json:"cache_creation_input_tokens"`
				CacheRead     int `json:"cache_read_input_tokens"`
				Output        int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.SessionID != "" && ev.Type == "system" && ev.Subtype == "init" {
			emite(Evento{SesionAgente: ev.SessionID})
		}
		// Lo que hacen los subagentes (Task) no se mezcla con la conversación principal.
		if ev.Padre != "" {
			continue
		}
		switch ev.Type {
		case "assistant":
			for _, c := range ev.Message.Content {
				switch c.Type {
				case "text":
					emite(Evento{Texto: c.Text})
				case "thinking":
					if c.Thinking != "" {
						emite(Evento{Pensamiento: c.Thinking})
					}
				case "tool_use":
					nombres[c.ID] = c.Name
					emite(Evento{Herramienta: &Herramienta{ID: c.ID, Nombre: c.Name, Resumen: resumen(c.Input),
						Entrada: recorta(string(c.Input), maxEntrada), Estado: "curso"}})
				}
			}
		case "user":
			for _, c := range ev.Message.Content {
				if c.Type != "tool_result" {
					continue
				}
				estado := "ok"
				if c.IsError {
					estado = "error"
				}
				emite(Evento{Herramienta: &Herramienta{ID: c.ToolUseID, Nombre: nombres[c.ToolUseID],
					Salida: recorta(textoDe(c.Content), maxSalida), Estado: estado}})
			}
		case "result":
			emite(Evento{Uso: &Uso{Entrada: ev.Usage.Input + ev.Usage.CacheCreation + ev.Usage.CacheRead, Salida: ev.Usage.Output,
				Coste: ev.Coste, Segundos: ev.Duracion / 1000}})
			if ev.IsError {
				return errors.New(ev.Result)
			}
		}
	}
	return sc.Err()
}

// leeAntigravity interpreta la salida stream-json de `agy`: texto (text_delta), herramientas (pasos «tool», con
// su entrada al empezar y su salida al acabar), consumo y el id de conversación para continuar con --conversation.
func leeAntigravity(r io.Reader, emite func(Evento)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 32<<20)
	empezadas := map[int]bool{}
	for sc.Scan() {
		var ev struct {
			Event          string `json:"event"`
			ConversationID string `json:"conversation_id"`
			Paso           struct {
				Indice int    `json:"step_index"`
				Estado string `json:"state"`
				Tipo   string `json:"step_type"`
				Texto  string `json:"text_delta"`
				Has    string `json:"tool_name"`
				Info   struct {
					Parametros json.RawMessage `json:"parameters"`
					Salida     string          `json:"output"`
					Error      string          `json:"error"`
				} `json:"tool_info"`
			} `json:"step_update"`
			Resultado struct {
				Estado   string  `json:"status"`
				Error    string  `json:"error"`
				Segundos float64 `json:"duration_seconds"`
				Usage    struct {
					Input     int `json:"input_tokens"`
					CacheRead int `json:"cache_read_tokens"`
					Output    int `json:"output_tokens"`
				} `json:"usage"`
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
			p := ev.Paso
			switch {
			case p.Tipo == "agent_response" && p.Texto != "":
				emite(Evento{Texto: p.Texto})
			case p.Tipo == "tool" && p.Has != "":
				id := fmt.Sprintf("paso-%d", p.Indice)
				if !empezadas[p.Indice] {
					empezadas[p.Indice] = true
					emite(Evento{Herramienta: &Herramienta{ID: id, Nombre: p.Has, Resumen: resumen(p.Info.Parametros),
						Entrada: recorta(string(p.Info.Parametros), maxEntrada), Estado: "curso"}})
				}
				if p.Estado == "DONE" || p.Estado == "ERROR" || p.Estado == "FAILED" {
					estado, salida := "ok", p.Info.Salida
					if p.Estado != "DONE" || p.Info.Error != "" {
						estado = "error"
						if p.Info.Error != "" {
							salida = p.Info.Error
						}
					}
					emite(Evento{Herramienta: &Herramienta{ID: id, Nombre: p.Has, Salida: recorta(salida, maxSalida), Estado: estado}})
				}
			}
		case "result":
			res := ev.Resultado
			emite(Evento{Uso: &Uso{Entrada: res.Usage.Input + res.Usage.CacheRead, Salida: res.Usage.Output, Segundos: res.Segundos}})
			if res.Estado != "" && res.Estado != "SUCCESS" {
				if res.Error != "" {
					return errors.New(res.Error)
				}
				return errors.New("Antigravity terminó con estado " + res.Estado)
			}
		}
	}
	return sc.Err()
}
