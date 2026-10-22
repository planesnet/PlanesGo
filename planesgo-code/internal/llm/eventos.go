package llm

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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
// imagenDataURL: una imagen en una data: URL (base64), como las adjunta OpenCode.
func imagenDataURL(mime, url string) (Imagen, bool) {
	cab, datos, ok := strings.Cut(url, ",")
	if !ok || !strings.HasPrefix(cab, "data:image/") || !strings.HasSuffix(cab, ";base64") {
		return Imagen{}, false
	}
	d, err := base64.StdEncoding.DecodeString(datos)
	if err != nil || len(d) == 0 {
		return Imagen{}, false
	}
	if mime == "" || !strings.HasPrefix(mime, "image/") {
		mime = strings.TrimSuffix(strings.TrimPrefix(cab, "data:"), ";base64")
	}
	return Imagen{Tipo: mime, Datos: d}, true
}

// tiposImagen: las extensiones de las imágenes que se muestran en el chat cuando el agente las lee.
var tiposImagen = map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}

// imagenDeFichero: la imagen que ha leído el agente, por la ruta de la entrada de su herramienta (si es una imagen
// de menos de 10 MB).
func imagenDeFichero(entrada json.RawMessage) []Imagen {
	var in map[string]any
	if json.Unmarshal(entrada, &in) != nil {
		return nil
	}
	for _, k := range []string{"Path", "path", "AbsolutePath", "filePath", "file_path", "TargetFile"} {
		ruta, _ := in[k].(string)
		tipo := tiposImagen[strings.ToLower(filepath.Ext(ruta))]
		if ruta == "" || tipo == "" {
			continue
		}
		if st, err := os.Stat(ruta); err != nil || !st.Mode().IsRegular() || st.Size() > 10<<20 {
			return nil
		}
		if d, err := os.ReadFile(ruta); err == nil && len(d) > 0 {
			return []Imagen{{Tipo: tipo, Datos: d}}
		}
		return nil
	}
	return nil
}

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
	// Órdenes en segundo plano: la orden de cada Bash lanzado así y las tareas que siguen vivas.
	ordenes := map[string]string{}
	type viva struct {
		tipo string
		f    Fondo
	}
	vivas := map[string]viva{}
	lista := &listaClaude{}
	contexto := 0
	// Consumo real del turno, también en vivo. «usage» del result es el del turno pero solo del agente principal;
	// «modelUsage» y «total_cost_usd» llevan los subagentes pero son lo acumulado de TODA la sesión (también a través
	// de --resume). El turno es el acumulado menos el de la vez anterior (guardado por sesión: acumuladoClaude); sin
	// él, en una sesión retomada, se reconstruye con las llamadas vistas (consumoObservado). En vivo: lo de los
	// result anteriores de este proceso más las llamadas de después (cada mensaje una vez: Claude repite el mensaje
	// por cada bloque; su salida llega parcial y se completa con el result, cuyo vivo coincide así con la suma).
	var hecho Uso   // lo emitido en los result de este proceso
	var previo *Uso // el acumulado de la sesión en el result anterior (nil: aún no se sabe)
	sesion := ""
	costeSinDesglose := 0.0 // sin modelUsage: el coste del result también es acumulado (se resta el anterior)
	llamadas := map[string]llamadaClaude{}
	cerradas := map[string]bool{}
	vivo := func() *Uso {
		u := Uso{Entrada: hecho.Entrada, Salida: hecho.Salida}
		for _, l := range llamadas {
			u.Entrada += l.entrada()
			u.Salida += l.Salida
		}
		return &u
	}
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
				ID      string `json:"id"`
				Model   string `json:"model"`
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
				Usage struct {
					Input         int `json:"input_tokens"`
					CacheCreation int `json:"cache_creation_input_tokens"`
					CacheRead     int `json:"cache_read_input_tokens"`
					Output        int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
			ModelUsage    map[string]usoModeloClaude `json:"modelUsage"`
			ResultadoHerr json.RawMessage            `json:"tool_use_result"`
			IsError       bool                       `json:"is_error"`
			Result        string                     `json:"result"`
			Coste         float64                    `json:"total_cost_usd"`
			Duracion      float64                    `json:"duration_ms"`
			Usage         struct {
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
			lista.sesion = ev.SessionID
			sesion = ev.SessionID
		}
		// La lista de tareas (TodoWrite, TaskCreate, TaskUpdate), también si la toca un subagente.
		for _, c := range ev.Message.Content {
			var t *Tarea
			switch {
			case ev.Type == "assistant" && c.Type == "tool_use":
				t = lista.usa(c.ID, c.Name, c.Input)
			case ev.Type == "user" && c.Type == "tool_result":
				t = lista.resultado(c.ToolUseID, c.IsError, textoDe(c.Content), ev.ResultadoHerr)
			}
			if t != nil {
				emite(Evento{Tarea: t})
			}
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
		if u := ev.Message.Usage; ev.Type == "assistant" && ev.Message.ID != "" && !cerradas[ev.Message.ID] {
			antes := llamadas[ev.Message.ID]
			n := llamadaClaude{Modelo: ev.Message.Model, Sub: ev.Padre != "", Nueva: u.Input, Leida: u.CacheRead, Escrita: u.CacheCreation,
				Salida: max(u.Output, antes.Salida)}
			if n != antes {
				llamadas[ev.Message.ID] = n
				emite(Evento{Vivo: vivo()})
			}
		}
		v := vivo()
		tokens := v.Entrada + v.Salida // lo consumido en el turno hasta cada herramienta
		// Lo que hacen los subagentes va dentro de su tarea: sus herramientas, con Padre (su texto no se muestra).
		if ev.Padre != "" {
			for _, c := range ev.Message.Content {
				switch {
				case ev.Type == "assistant" && c.Type == "tool_use":
					nombres[c.ID] = c.Name
					emite(Evento{Herramienta: &Herramienta{ID: c.ID, Nombre: c.Name, Resumen: resumen(c.Input),
						Entrada: recorta(string(c.Input), maxEntrada), Estado: "curso", Padre: ev.Padre, Tokens: tokens}})
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
			if u := ev.Message.Usage; ev.Padre == "" && u.Input+u.CacheRead+u.CacheCreation > 0 {
				contexto = u.Input + u.CacheRead + u.CacheCreation // la última llamada del agente principal
			}
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
						Entrada: recorta(string(c.Input), maxEntrada), Estado: "curso", Skill: skillDe(c.Name, c.Input), Tokens: tokens}})
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
			total := acumuladoSesionClaude(ev.ModelUsage, ev.Coste)
			observado := consumoObservado(llamadas, ev.Usage.Output, ev.ModelUsage)
			if previo == nil && sesion != "" {
				if a, ok := acumuladoClaude(sesion); ok && a.Entrada <= total.Entrada {
					previo = &a
				}
			}
			var turno Uso
			switch {
			case len(ev.ModelUsage) == 0: // sin acumulado: lo del propio result (solo el agente principal)
				turno = Uso{Entrada: ev.Usage.Input + ev.Usage.CacheCreation + ev.Usage.CacheRead, Salida: ev.Usage.Output,
					CacheLeida: ev.Usage.CacheRead, CacheEscrita: ev.Usage.CacheCreation, Coste: max(ev.Coste-costeSinDesglose, 0)}
				costeSinDesglose = max(ev.Coste, costeSinDesglose)
			case previo != nil:
				turno = Uso{Entrada: max(total.Entrada-previo.Entrada, 0), Salida: max(total.Salida-previo.Salida, 0),
					CacheLeida: max(total.CacheLeida-previo.CacheLeida, 0), CacheEscrita: max(total.CacheEscrita-previo.CacheEscrita, 0),
					Coste: max(total.Coste-previo.Coste, 0)}
			case total.Entrada <= observado.Entrada+observado.Entrada/100+100:
				turno = total // sesión nueva: todo lo acumulado es de este turno
			default:
				turno = observado // sesión retomada sin acumulado guardado: lo visto en este proceso
			}
			if len(ev.ModelUsage) > 0 {
				previo = &total
				if sesion != "" {
					guardaAcumuladoClaude(sesion, total)
				}
			}
			coste := turno.Coste
			hecho.Entrada += turno.Entrada
			hecho.Salida += turno.Salida
			turno.Llamadas = len(llamadas)
			for id := range llamadas {
				cerradas[id] = true
			}
			clear(llamadas)
			emite(Evento{Uso: &Uso{Entrada: turno.Entrada, Salida: turno.Salida, CacheLeida: turno.CacheLeida, CacheEscrita: turno.CacheEscrita,
				Llamadas: turno.Llamadas, Coste: coste, Segundos: ev.Duracion / 1000, Contexto: contexto}, Vivo: vivo()})
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
	// El modelo a veces repite en su respuesta los avisos internos del agente (<SYSTEM_MESSAGE>…): fuera.
	sinSistema := &quitaBloque{abre: "<SYSTEM_MESSAGE>", cierra: "</SYSTEM_MESSAGE>"}
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
	llamadasAgy := map[int]bool{}           // pasos con consumo propio: cada uno, una llamada al modelo
	parametros := map[int]json.RawMessage{} // los de cada paso (solo llegan al empezar)
	for sc.Scan() {
		var ev struct {
			Event          string `json:"event"`
			ConversationID string `json:"conversation_id"`
			Paso           struct {
				Indice int    `json:"step_index"`
				Estado string `json:"state"`
				Uso    struct {
					Input int `json:"input_tokens"`
				} `json:"usage"`
				Tipo  string `json:"step_type"`
				Texto string `json:"text_delta"`
				Has   string `json:"tool_name"`
				Info  struct {
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
					Thinking  int `json:"thinking_tokens"`
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
			if p.Uso.Input > 0 {
				llamadasAgy[p.Indice] = true
			}
			switch {
			case p.Tipo == "agent_response" && p.Texto != "":
				if t := sinSistema.Pasa(p.Texto); t != "" {
					emite(Evento{Texto: t})
				}
			// Un subagente puede empezar como «tool» y acabar como «subagent» (o al revés): el mismo paso.
			case (p.Tipo == "tool" || p.Tipo == "subagent") && p.Has != "":
				id := fmt.Sprintf("paso-%d", p.Indice)
				if !vistas[p.Indice] {
					vistas[p.Indice] = true
					empezadas[p.Indice] = p.Has
					parametros[p.Indice] = p.Info.Parametros
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
					// Antigravity no devuelve la imagen que lee, solo su ruta: se toma del disco.
					var imgs []Imagen
					if estado == "ok" && strings.Contains(strings.ToLower(p.Has), "file") {
						imgs = imagenDeFichero(parametros[p.Indice])
					}
					emite(Evento{Herramienta: &Herramienta{ID: id, Nombre: p.Has, Salida: recorta(salida, maxSalida), Estado: estado, Imagenes: imgs}})
				}
			}
		case "result":
			res := ev.Resultado
			// El razonamiento se cobra como salida: va con ella. Llamadas: los pasos con consumo propio.
			emite(Evento{Uso: &Uso{Entrada: res.Usage.Input + res.Usage.CacheRead, Salida: res.Usage.Output + res.Usage.Thinking,
				CacheLeida: res.Usage.CacheRead, Llamadas: len(llamadasAgy), Segundos: res.Segundos}})
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

// errLimitePasos: OpenCode ha llegado a su límite de pasos de la petición y Gemini ha rechazado el mensaje con el
// que OpenCode le pide que acabe sin herramientas. El trabajo está hecho; falta la respuesta (Chat la pide).
type errLimitePasos struct{ error }

func (e *errLimitePasos) Unwrap() error { return e.error }

// leeOpenCode interpreta «opencode run --format json»: text (al acabar cada bloque), tool_use (herramienta ya
// terminada, con entrada y salida), step_finish (tokens y coste), error; sessionID para continuar con --session.
func leeOpenCode(r io.Reader, emite func(Evento)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 32<<20)
	sesion := ""
	var errores []string
	limite := false // OpenCode llegó a su límite de pasos (y Gemini rechazó su último mensaje)
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
					// Adjuntos del resultado: las imágenes que lee (data: URL en base64).
					Adjuntos []struct {
						Mime string `json:"mime"`
						URL  string `json:"url"`
					} `json:"attachments"`
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
			var imgs []Imagen
			for _, a := range ev.Part.State.Adjuntos {
				if i, ok := imagenDataURL(a.Mime, a.URL); ok {
					imgs = append(imgs, i)
				}
			}
			emite(Evento{Herramienta: &Herramienta{ID: id, Nombre: ev.Part.Tool, Resumen: res,
				Entrada: recorta(string(ev.Part.State.Input), maxEntrada), Salida: recorta(salida, maxSalida), Estado: estado,
				Skill: skillDe(ev.Part.Tool, ev.Part.State.Input), Imagenes: imgs}})
			if t := tareaOpenCode(id, ev.Part.Tool, ev.Part.State.Input, ev.Part.State.Output, ev.Part.State.Meta, estado); t != nil {
				emite(Evento{Tarea: t})
			}
		case "step_finish":
			// Cada paso, su consumo ya (el gasto se ve crecer durante el turno) y el tamaño del contexto enviado.
			t := ev.Part.Tokens
			emite(Evento{Uso: &Uso{Entrada: t.Input + t.Cache.Read + t.Cache.Write, Salida: t.Output + t.Reasoning,
				CacheLeida: t.Cache.Read, CacheEscrita: t.Cache.Write, Llamadas: 1,
				Coste: ev.Part.Cost, Contexto: t.Input + t.Cache.Read + t.Cache.Write}})
		case "error":
			m := ev.Error.Data.Message
			if m == "" {
				m = ev.Error.Name
			}
			// Al llegar al límite de pasos, OpenCode añade al final un mensaje del modelo («responde sin
			// herramientas») y Gemini rechaza las peticiones que acaban así (OpenCode 1.19): se explica.
			if strings.Contains(m, "ending with a model turn") {
				limite = true
				m = fmt.Sprintf("OpenCode ha llegado al límite de %d pasos por petición y, con Gemini, lo corta con un error en vez de "+
					"resumir. El trabajo hecho hasta aquí se conserva: escribe «sigue» para continuar", PasosOpenCode)
			}
			errores = append(errores, m)
		}
	}
	if len(errores) > 0 {
		err := errors.New("OpenCode: " + strings.Join(errores, "; "))
		if limite {
			err = &errLimitePasos{err}
		}
		return err
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
		var l [][2]string
		for _, x := range md.Todos {
			l = append(l, [2]string{x.Contenido, x.Estado})
		}
		return tareaLista(l)
	}
	return nil
}

// tareaLista: la lista de tareas del agente (pares texto, estado: pending | in_progress | completed | cancelled)
// como la Tarea «lista»; nil si está vacía.
func tareaLista(l [][2]string) *Tarea {
	if len(l) == 0 {
		return nil
	}
	t := &Tarea{ID: "lista", Tipo: "lista", Titulo: "Lista de tareas", Estado: "ok"}
	for _, x := range l {
		e := "pendiente"
		switch x[1] {
		case "in_progress":
			e, t.Estado = "curso", "curso"
		case "completed", "cancelled":
			e = "hecho"
		}
		t.Elementos = append(t.Elementos, Elemento{Texto: x[0], Estado: e})
	}
	return t
}

// listaClaude monta la lista de tareas de Claude Code. Con TodoWrite llega entera en cada llamada; con
// TaskCreate/TaskUpdate (Claude Code 2.1 con los modelos actuales) llega a trozos y se aplica cada cambio cuando
// sale bien. Al continuar la sesión (--resume) la lista sigue guardada en <config>/tasks/<sesión>: se parte de ella.
type listaClaude struct {
	sesion    string
	cargada   bool
	elementos map[string]*elementoClaude
	entradas  map[string]json.RawMessage // tool_use de TaskCreate/TaskUpdate → su entrada, hasta su resultado
}

type elementoClaude struct {
	ID      string `json:"id"`
	Asunto  string `json:"subject"`
	Estado  string `json:"status"`
	TareaID string `json:"taskId"` // TaskUpdate
}

// carpetaTareasClaude: donde guarda Claude Code la lista de tareas de una sesión (variable para las pruebas).
var carpetaTareasClaude = func(sesion string) string {
	base := os.Getenv("CLAUDE_CONFIG_DIR")
	if base == "" {
		casa, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(casa, ".claude")
	}
	return filepath.Join(base, "tasks", sesion)
}

var tareaCreadaClaude = regexp.MustCompile(`Task #(\d+) created`)

func (l *listaClaude) usa(id, herr string, entrada json.RawMessage) *Tarea {
	switch herr {
	case "TodoWrite":
		var in struct {
			Todos []struct {
				Contenido string `json:"content"`
				Estado    string `json:"status"`
			} `json:"todos"`
		}
		if json.Unmarshal(entrada, &in) != nil {
			return nil
		}
		var ls [][2]string
		for _, x := range in.Todos {
			ls = append(ls, [2]string{x.Contenido, x.Estado})
		}
		return tareaLista(ls)
	case "TaskCreate", "TaskUpdate":
		if l.entradas == nil {
			l.entradas = map[string]json.RawMessage{}
		}
		l.entradas[id] = entrada
	}
	return nil
}

func (l *listaClaude) resultado(id string, fallo bool, texto string, res json.RawMessage) *Tarea {
	entrada, ok := l.entradas[id]
	if !ok {
		return nil
	}
	delete(l.entradas, id)
	if fallo {
		return nil
	}
	l.carga()
	var in elementoClaude
	if json.Unmarshal(entrada, &in) != nil {
		return nil
	}
	if in.TareaID == "" { // TaskCreate: el id lo da el resultado
		var r struct {
			Task struct {
				ID string `json:"id"`
			} `json:"task"`
		}
		json.Unmarshal(res, &r)
		in.ID = r.Task.ID
		if m := tareaCreadaClaude.FindStringSubmatch(texto); in.ID == "" && m != nil {
			in.ID = m[1]
		}
		if in.ID == "" {
			return nil
		}
		if in.Estado == "" {
			in.Estado = "pending"
		}
		if e := l.elementos[in.ID]; e == nil || e.Asunto == "" {
			l.elementos[in.ID] = &in
		}
	} else {
		e := l.elementos[in.TareaID]
		if e == nil {
			e = &elementoClaude{ID: in.TareaID, Estado: "pending"}
			l.elementos[in.TareaID] = e
		}
		if in.Asunto != "" {
			e.Asunto = in.Asunto
		}
		if in.Estado == "deleted" {
			delete(l.elementos, in.TareaID)
		} else if in.Estado != "" {
			e.Estado = in.Estado
		}
	}
	return l.tarea()
}

// carga: la lista guardada de la sesión (la primera vez que hace falta).
func (l *listaClaude) carga() {
	if l.elementos == nil {
		l.elementos = map[string]*elementoClaude{}
	}
	if l.cargada || l.sesion == "" {
		return
	}
	l.cargada = true
	ds, _ := filepath.Glob(filepath.Join(carpetaTareasClaude(l.sesion), "*.json"))
	for _, f := range ds {
		var e elementoClaude
		if b, err := os.ReadFile(f); err == nil && json.Unmarshal(b, &e) == nil && e.ID != "" && e.Estado != "deleted" {
			l.elementos[e.ID] = &e
		}
	}
}

func (l *listaClaude) tarea() *Tarea {
	ids := make([]string, 0, len(l.elementos))
	for id := range l.elementos {
		ids = append(ids, id)
	}
	// Ids numéricos («1», «2»… «10»): por número.
	sort.Slice(ids, func(i, j int) bool {
		if len(ids[i]) != len(ids[j]) {
			return len(ids[i]) < len(ids[j])
		}
		return ids[i] < ids[j]
	})
	var ls [][2]string
	for _, id := range ids {
		e := l.elementos[id]
		texto := e.Asunto
		if texto == "" {
			texto = "Tarea #" + id
		}
		ls = append(ls, [2]string{texto, e.Estado})
	}
	return tareaLista(ls)
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

// quitaBloque quita de un texto que llega a trozos lo que va entre dos marcas (también si una marca llega partida
// entre dos trozos: lo que podría ser el principio de una marca se guarda hasta el trozo siguiente).
type quitaBloque struct {
	abre, cierra string
	dentro       bool
	pendiente    string
}

func (q *quitaBloque) Pasa(t string) string {
	t = q.pendiente + t
	q.pendiente = ""
	var b strings.Builder
	for t != "" {
		marca := q.abre
		if q.dentro {
			marca = q.cierra
		}
		if i := strings.Index(t, marca); i >= 0 {
			if !q.dentro {
				b.WriteString(t[:i])
			}
			t = t[i+len(marca):]
			q.dentro = !q.dentro
			continue
		}
		// ¿Acaba con el principio de la marca? Se guarda para el siguiente trozo.
		corte := len(t)
		for n := min(len(marca)-1, len(t)); n > 0; n-- {
			if strings.HasSuffix(t, marca[:n]) {
				corte = len(t) - n
				break
			}
		}
		if !q.dentro {
			b.WriteString(t[:corte])
		}
		q.pendiente = t[corte:]
		break
	}
	return b.String()
}

// Lector: cómo interpretar la salida de un agente (su clave), para usarlo fuera de pgocode (cmd/consumo lanza
// los agentes a pelo para calibrar). nil si no es un agente conocido. Antigravity da el consumo acumulado de su
// conversación: quien lo use ha de restar el del turno anterior.
func Lector(clave string) func(io.Reader, func(Evento)) error {
	switch clave {
	case "claude-code":
		return leeClaudeCode
	case "antigravity":
		return leeAntigravity
	case "opencode":
		return leeOpenCode
	}
	return nil
}

// llamadaClaude: una llamada al modelo vista en el flujo de Claude Code (su mensaje), con su consumo.
type llamadaClaude struct {
	Modelo                        string
	Sub                           bool // de un subagente
	Nueva, Leida, Escrita, Salida int  // entrada nueva, leída y escrita en caché; salida (parcial hasta el result)
}

func (l llamadaClaude) entrada() int { return l.Nueva + l.Leida + l.Escrita }

// usoModeloClaude: lo acumulado de la sesión de un modelo («modelUsage»).
type usoModeloClaude struct {
	Input         int     `json:"inputTokens"`
	Output        int     `json:"outputTokens"`
	CacheRead     int     `json:"cacheReadInputTokens"`
	CacheCreation int     `json:"cacheCreationInputTokens"`
	Coste         float64 `json:"costUSD"`
}

// peso: lo que pesa en el precio (entrada nueva 1, escritura en caché 1,25, lectura 0,1, salida 5).
func peso(nueva, escrita, leida, salida int) float64 {
	return float64(nueva) + 1.25*float64(escrita) + 0.1*float64(leida) + 5*float64(salida)
}

// acumuladoSesionClaude: el acumulado de toda la sesión (todos sus modelos).
func acumuladoSesionClaude(mu map[string]usoModeloClaude, coste float64) Uso {
	var u Uso
	for _, m := range mu {
		u.Entrada += m.Input + m.CacheCreation + m.CacheRead
		u.Salida += m.Output
		u.CacheLeida += m.CacheRead
		u.CacheEscrita += m.CacheCreation
	}
	u.Coste = coste
	return u
}

// consumoObservado: el del turno según las llamadas vistas en este proceso. La entrada es exacta; la salida, la
// del result para el agente principal (exacta) más lo visto de los subagentes; el coste, la parte del acumulado de
// cada modelo que le toca por el peso de sus tokens en el precio.
func consumoObservado(ls map[string]llamadaClaude, salidaPrincipal int, mu map[string]usoModeloClaude) Uso {
	var u Uso
	pesos := map[string]float64{}
	principal, vistaPrincipal := "", 0
	for _, l := range ls {
		u.Entrada += l.entrada()
		u.CacheLeida += l.Leida
		u.CacheEscrita += l.Escrita
		if l.Sub {
			u.Salida += l.Salida
		} else {
			principal = l.Modelo
			vistaPrincipal += l.Salida
		}
		pesos[l.Modelo] += peso(l.Nueva, l.Escrita, l.Leida, l.Salida)
	}
	u.Salida += salidaPrincipal
	if s := salidaPrincipal - vistaPrincipal; s > 0 {
		pesos[principal] += 5 * float64(s)
	}
	for modelo, w := range pesos {
		m, ok := mu[modelo]
		if !ok && len(mu) == 1 {
			for _, x := range mu {
				m, ok = x, true
			}
		}
		if wc := peso(m.Input, m.CacheCreation, m.CacheRead, m.Output); ok && wc > 0 {
			u.Coste += m.Coste * math.Min(1, w/wc)
		}
	}
	return u
}
