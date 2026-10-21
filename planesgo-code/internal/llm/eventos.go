package llm

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
	for _, k := range []string{"command", "CommandLine", "file_path", "filePath", "FilePath", "Path", "path", "pattern", "Pattern",
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

// skillDe dice qué skill carga una herramienta: la herramienta Skill de Claude Code ({"skill": …}), la skill
// de OpenCode ({"name": …}) o cualquier lectura de un …/<skill>/SKILL.md (Antigravity y los demás).
func skillDe(herramienta string, entrada json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(entrada, &m) != nil {
		return ""
	}
	texto := func(k string) string { v, _ := m[k].(string); return v }
	switch strings.ToLower(herramienta) {
	case "skill":
		for _, k := range []string{"skill", "name", "command"} {
			if v := strings.TrimPrefix(texto(k), "/"); v != "" {
				return v
			}
		}
	}
	for _, k := range []string{"file_path", "filePath", "FilePath", "Path", "path", "AbsolutePath", "TargetFile"} {
		if p := texto(k); strings.HasSuffix(p, "/SKILL.md") {
			p = strings.TrimSuffix(p, "/SKILL.md")
			return p[strings.LastIndex(p, "/")+1:]
		}
	}
	return ""
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

// imagenesDe: las imágenes (base64) del resultado de una herramienta, también dentro de bloques anidados
// (los resultados de herramientas MCP llevan su contenido como lista de bloques).
func imagenesDe(c json.RawMessage) []Imagen {
	var bloques []struct {
		Type   string `json:"type"`
		Source struct {
			Type      string `json:"type"`
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
		} `json:"source"`
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(c, &bloques) != nil {
		return nil
	}
	var l []Imagen
	for _, b := range bloques {
		switch {
		case b.Type == "image" && b.Source.Type == "base64" && strings.HasPrefix(b.Source.MediaType, "image/"):
			if d, err := base64.StdEncoding.DecodeString(b.Source.Data); err == nil && len(d) > 0 {
				l = append(l, Imagen{Tipo: b.Source.MediaType, Datos: d})
			}
		case len(b.Content) > 0:
			l = append(l, imagenesDe(b.Content)...)
		}
	}
	return l
}

// leeClaudeCode interpreta la salida stream-json de `claude -p`: texto, razonamiento, herramientas con su
// entrada y su resultado, consumo del turno y el id de sesión para continuar con --resume.
func leeClaudeCode(r io.Reader, emite func(Evento)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 32<<20)
	nombres := map[string]string{}
	// total_cost_usd es lo acumulado por el proceso: con tareas en segundo plano hay varios «result» y cada uno
	// lleva el total hasta entonces; se emite solo la diferencia para no contar dos veces.
	costeAntes := 0.0
	// Órdenes en segundo plano: la orden de cada Bash lanzado así y las tareas que siguen vivas.
	ordenes := map[string]string{}
	type viva struct {
		tipo string
		f    Fondo
	}
	vivas := map[string]viva{}
	for sc.Scan() {
		var ev struct {
			Type      string `json:"type"`
			Subtype   string `json:"subtype"`
			SessionID string `json:"session_id"`
			Padre     string `json:"parent_tool_use_id"`
			// Tareas (system/task_*): subagentes y órdenes en segundo plano.
			TaskID       string `json:"task_id"`
			TaskToolUse  string `json:"tool_use_id"`
			Descripcion  string `json:"description"`
			SegundoPlano bool   `json:"is_backgrounded"`
			TipoTarea    string `json:"task_type"`
			Subagente    string `json:"subagent_type"`
			TareaPadre   string `json:"parent_task_id"`
			Estado       string `json:"status"`
			Fichero      string `json:"output_file"`
			Resumen      string `json:"summary"`
			UltimaHerr   string `json:"last_tool_name"`
			Patch        struct {
				Status string `json:"status"`
			} `json:"patch"`
			Message struct {
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
				Total         int `json:"total_tokens"` // task_progress
				Herramientas  int `json:"tool_uses"`
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
		if ev.Type == "system" && ev.TaskID != "" {
			switch {
			case ev.Subtype == "task_started":
				tipo := "subagente"
				if ev.TipoTarea == "local_bash" {
					tipo = "segundo_plano"
				}
				vivas[ev.TaskID] = viva{tipo, Fondo{ID: ev.TaskID, Orden: ordenes[ev.TaskToolUse], Titulo: ev.Descripcion}}
			case ev.Subtype == "task_notification" || (ev.Subtype == "task_updated" && ev.Patch.Status != "" && ev.Patch.Status != "running" && ev.Patch.Status != "pending"):
				delete(vivas, ev.TaskID)
			}
			if t := tareaClaude(ev.Subtype, ev.TaskID, ev.TaskToolUse, ev.Descripcion, ev.TipoTarea, ev.Subagente, ev.TareaPadre,
				ev.Estado, ev.Patch.Status, ev.Fichero, ev.Resumen, ev.UltimaHerr, ev.SegundoPlano, ev.Usage.Total, ev.Usage.Herramientas); t != nil {
				emite(Evento{Tarea: t})
			}
			continue
		}
		// Lo que hacen los subagentes va dentro de su tarea: sus herramientas, con Padre (su texto no se muestra).
		if ev.Padre != "" {
			for _, c := range ev.Message.Content {
				switch {
				case ev.Type == "assistant" && c.Type == "tool_use":
					nombres[c.ID] = c.Name
					emite(Evento{Herramienta: &Herramienta{ID: c.ID, Nombre: c.Name, Resumen: resumen(c.Input),
						Entrada: recorta(string(c.Input), maxEntrada), Estado: "curso", Padre: ev.Padre}})
				case ev.Type == "user" && c.Type == "tool_result":
					estado := "ok"
					if c.IsError {
						estado = "error"
					}
					emite(Evento{Herramienta: &Herramienta{ID: c.ToolUseID, Nombre: nombres[c.ToolUseID],
						Salida: recorta(textoDe(c.Content), maxSalida), Estado: estado, Padre: ev.Padre, Imagenes: imagenesDe(c.Content)}})
				}
			}
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
					var in struct {
						Orden string `json:"command"`
						Fondo bool   `json:"run_in_background"`
					}
					if c.Name == "Bash" && json.Unmarshal(c.Input, &in) == nil && in.Fondo {
						ordenes[c.ID] = in.Orden
					}
					emite(Evento{Herramienta: &Herramienta{ID: c.ID, Nombre: c.Name, Resumen: resumen(c.Input),
						Entrada: recorta(string(c.Input), maxEntrada), Estado: "curso", Skill: skillDe(c.Name, c.Input)}})
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
					Salida: recorta(textoDe(c.Content), maxSalida), Estado: estado, Imagenes: imagenesDe(c.Content)}})
			}
		case "result":
			coste := max(ev.Coste-costeAntes, 0)
			costeAntes = max(ev.Coste, costeAntes)
			emite(Evento{Uso: &Uso{Entrada: ev.Usage.Input + ev.Usage.CacheCreation + ev.Usage.CacheRead, Salida: ev.Usage.Output,
				Coste: coste, Segundos: ev.Duracion / 1000}})
			if ev.IsError {
				return errors.New(ev.Result)
			}
			// Respuesta acabada y solo quedan órdenes en segundo plano: no se esperan (ni su turno extra).
			if len(vivas) > 0 {
				fin := &FinConFondo{}
				for _, v := range vivas {
					if v.tipo != "segundo_plano" {
						fin = nil
						break
					}
					v.f.Fichero = salidaTareaClaude(v.f.ID)
					fin.Ordenes = append(fin.Ordenes, v.f)
				}
				if fin != nil {
					return fin
				}
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
	empezadas := map[int]string{} // paso → herramienta, mientras no acaba
	// Al acabar, lo que no llegó a cerrarse (p. ej. una orden bloqueada) no se queda «en curso» para siempre.
	defer func() {
		for i, n := range empezadas {
			emite(Evento{Herramienta: &Herramienta{ID: fmt.Sprintf("paso-%d", i), Nombre: n, Salida: "Sin terminar: el agente acabó antes", Estado: "error"}})
		}
	}()
	vistas := map[int]bool{}
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
				Sub struct {
					Subagentes []struct {
						Tipo string `json:"type_name"`
						Rol  string `json:"role"`
						Conv string `json:"conversation_id"`
						Log  string `json:"log_uri"`
					} `json:"subagents"`
				} `json:"subagent_info"`
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
			// Un subagente puede empezar como «tool» y acabar como «subagent» (o al revés): el mismo paso.
			case (p.Tipo == "tool" || p.Tipo == "subagent") && p.Has != "":
				id := fmt.Sprintf("paso-%d", p.Indice)
				if !vistas[p.Indice] {
					vistas[p.Indice] = true
					empezadas[p.Indice] = p.Has
					emite(Evento{Herramienta: &Herramienta{ID: id, Nombre: p.Has, Resumen: resumen(p.Info.Parametros),
						Entrada: recorta(string(p.Info.Parametros), maxEntrada), Estado: "curso", Skill: skillDe(p.Has, p.Info.Parametros)}})
				}
				if t := tareaAntigravity(id, p.Has, p.Tipo, p.Estado, p.Sub.Subagentes); t != nil {
					emite(Evento{Tarea: t})
				}
				if p.Estado == "DONE" || p.Estado == "ERROR" || p.Estado == "FAILED" {
					delete(empezadas, p.Indice)
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

// leeOpenCode interpreta «opencode run --format json»: text (al acabar cada bloque), tool_use (herramienta ya
// terminada, con entrada y salida), step_finish (tokens y coste), error; sessionID para continuar con --session.
func leeOpenCode(r io.Reader, emite func(Evento)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 32<<20)
	sesion := ""
	uso := &Uso{}
	hayUso := false
	var errores []string
	for sc.Scan() {
		var ev struct {
			Type      string `json:"type"`
			SessionID string `json:"sessionID"`
			Part      struct {
				ID     string `json:"id"`
				Text   string `json:"text"`
				Tool   string `json:"tool"`
				CallID string `json:"callID"`
				State  struct {
					Status string          `json:"status"`
					Input  json.RawMessage `json:"input"`
					Output string          `json:"output"`
					Title  string          `json:"title"`
					Error  string          `json:"error"`
					Meta   json.RawMessage `json:"metadata"`
				} `json:"state"`
				Cost   float64 `json:"cost"`
				Tokens struct {
					Input     int `json:"input"`
					Output    int `json:"output"`
					Reasoning int `json:"reasoning"`
					Cache     struct {
						Read  int `json:"read"`
						Write int `json:"write"`
					} `json:"cache"`
				} `json:"tokens"`
			} `json:"part"`
			Error struct {
				Name string `json:"name"`
				Data struct {
					Message string `json:"message"`
				} `json:"data"`
			} `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			// Una pregunta interactiva (permiso) no se puede contestar desde aquí: se para y se explica.
			if l := sc.Text(); strings.Contains(l, "Permission required:") {
				_, que, _ := strings.Cut(l, "Permission required:")
				return fmt.Errorf("OpenCode pide permiso para %s y no se le puede contestar desde aquí; cambia el modo de la sesión (Auto lo permite todo)", strings.TrimSpace(que))
			}
			continue
		}
		if ev.SessionID != "" && ev.SessionID != sesion {
			sesion = ev.SessionID
			emite(Evento{SesionAgente: sesion})
		}
		switch ev.Type {
		case "text":
			if ev.Part.Text != "" {
				emite(Evento{Texto: ev.Part.Text})
			}
		case "tool_use":
			id := ev.Part.CallID
			if id == "" {
				id = ev.Part.ID
			}
			estado, salida := "ok", ev.Part.State.Output
			if ev.Part.State.Status == "error" {
				estado, salida = "error", ev.Part.State.Error
			}
			res := resumen(ev.Part.State.Input)
			if res == "" {
				res = primeraLinea(ev.Part.State.Title)
			}
			emite(Evento{Herramienta: &Herramienta{ID: id, Nombre: ev.Part.Tool, Resumen: res,
				Entrada: recorta(string(ev.Part.State.Input), maxEntrada), Salida: recorta(salida, maxSalida), Estado: estado,
				Skill: skillDe(ev.Part.Tool, ev.Part.State.Input)}})
			if t := tareaOpenCode(id, ev.Part.Tool, ev.Part.State.Input, ev.Part.State.Output, ev.Part.State.Meta, estado); t != nil {
				emite(Evento{Tarea: t})
			}
		case "step_finish":
			t := ev.Part.Tokens
			uso.Entrada += t.Input + t.Cache.Read + t.Cache.Write
			uso.Salida += t.Output + t.Reasoning
			uso.Coste += ev.Part.Cost
			hayUso = true
		case "error":
			m := ev.Error.Data.Message
			if m == "" {
				m = ev.Error.Name
			}
			errores = append(errores, m)
		}
	}
	if hayUso {
		emite(Evento{Uso: uso})
	}
	if len(errores) > 0 {
		return errors.New("OpenCode: " + strings.Join(errores, "; "))
	}
	return sc.Err()
}

// tareaClaude: los eventos system/task_* de Claude Code como Tarea.
func tareaClaude(sub, id, herr, desc, tipo, subagente, padre, estado, patch, fichero, res, ultima string, fondo bool, tokens, usos int) *Tarea {
	t := &Tarea{ID: id, Herramienta: herr, Padre: padre}
	switch tipo {
	case "local_bash", "monitor":
		t.Tipo = "segundo_plano"
	case "":
	default: // local_agent, remote_agent, local_workflow
		t.Tipo = "subagente"
	}
	fin := func(s string) string {
		switch s {
		case "completed":
			return "ok"
		case "failed":
			return "error"
		case "killed", "stopped", "cancelled":
			return "parada"
		}
		return ""
	}
	switch sub {
	case "task_started":
		t.Titulo, t.Subtipo, t.SegundoPlano, t.Estado = desc, subagente, fondo, "curso"
	case "task_progress":
		p := strings.TrimPrefix(desc, "Running ")
		if ultima != "" {
			p += " · " + ultima
		}
		if usos > 0 {
			p += fmt.Sprintf(" · %d herramientas", usos)
		}
		if tokens > 0 {
			p += fmt.Sprintf(" · %d tokens", tokens)
		}
		t.Progreso, t.Estado = p, "curso"
	case "task_updated":
		t.Estado = fin(patch)
		if t.Estado == "" {
			return nil
		}
	case "task_notification":
		t.Estado, t.Salida, t.Detalle = fin(estado), recorta(res, maxSalida), fichero
	default:
		return nil
	}
	return t
}

// tareaAntigravity: un paso de Antigravity que es un subagente (invoke_subagent) como Tarea. Sus órdenes
// asíncronas no se distinguen en el stream (la marca no viaja en los parámetros): quedan como herramientas.
func tareaAntigravity(id, herr, tipo, estado string, subs []struct {
	Tipo string `json:"type_name"`
	Rol  string `json:"role"`
	Conv string `json:"conversation_id"`
	Log  string `json:"log_uri"`
}) *Tarea {
	t := &Tarea{ID: id, Herramienta: id, Estado: "curso"}
	switch estado {
	case "DONE":
		t.Estado = "ok"
	case "ERROR", "FAILED":
		t.Estado = "error"
	}
	switch {
	case herr == "invoke_subagent" || tipo == "subagent":
		t.Tipo = "subagente"
		if len(subs) > 0 {
			s := subs[0]
			t.Titulo, t.Subtipo, t.SesionHija, t.Detalle = s.Rol, s.Tipo, s.Conv, strings.TrimPrefix(s.Log, "file://")
		}
	default:
		return nil
	}
	return t
}

var estadoTaskOpenCode = regexp.MustCompile(`<task[^>]*state="([a-z]+)"`)
var resultadoOpenCode = regexp.MustCompile(`(?s)<task_result>(.*?)</task_result>`)

// tareaOpenCode: la herramienta «task» (subagente, con su subsesión) y «todowrite» (la lista de tareas) como Tarea.
func tareaOpenCode(id, herr string, entrada json.RawMessage, salida string, meta json.RawMessage, estado string) *Tarea {
	switch herr {
	case "task":
		var in struct {
			Desc string `json:"description"`
			Tipo string `json:"subagent_type"`
		}
		var md struct {
			Sesion string `json:"sessionId"`
		}
		json.Unmarshal(entrada, &in)
		json.Unmarshal(meta, &md)
		t := &Tarea{ID: id, Tipo: "subagente", Herramienta: id, Titulo: in.Desc, Subtipo: in.Tipo, SesionHija: md.Sesion, Estado: estado}
		if m := estadoTaskOpenCode.FindStringSubmatch(salida); m != nil && m[1] == "running" {
			t.Estado, t.SegundoPlano = "curso", true // subagente en segundo plano (experimental)
		}
		if m := resultadoOpenCode.FindStringSubmatch(salida); m != nil {
			t.Salida = recorta(strings.TrimSpace(m[1]), maxSalida)
		}
		return t
	case "todowrite":
		var md struct {
			Todos []struct {
				Contenido string `json:"content"`
				Estado    string `json:"status"`
			} `json:"todos"`
		}
		if json.Unmarshal(meta, &md) != nil || len(md.Todos) == 0 {
			return nil
		}
		t := &Tarea{ID: "lista", Tipo: "lista", Titulo: "Lista de tareas", Estado: "ok"}
		for _, x := range md.Todos {
			e := "pendiente"
			switch x.Estado {
			case "in_progress":
				e, t.Estado = "curso", "curso"
			case "completed", "cancelled":
				e = "hecho"
			}
			t.Elementos = append(t.Elementos, Elemento{Texto: x.Contenido, Estado: e})
		}
		return t
	}
	return nil
}

// salidaTareaClaude: el fichero donde Claude Code guarda la salida de una tarea en segundo plano
// (<tmp>/claude-<uid>/<carpeta>/<sesión>/tasks/<id>.output).
func salidaTareaClaude(id string) string {
	for _, base := range []string{os.TempDir(), "/tmp"} {
		if l, _ := filepath.Glob(filepath.Join(base, "claude-*", "*", "*", "tasks", id+".output")); len(l) > 0 {
			return l[0]
		}
	}
	return ""
}
