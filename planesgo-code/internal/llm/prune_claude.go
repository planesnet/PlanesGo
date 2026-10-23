package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"planesgo-code/internal/depuracion"
)

// Poda en Claude Code: el mismo recolector y la misma vigencia que OpenCode (prune.go, vigencia.go); cambia cómo se
// lee su historial y cómo se aplica lo retirado.
//
// Claude Code guarda la sesión en ~/.claude/projects/<carpeta>/<sesión>.jsonl (una línea JSON por entrada, con la
// cadena uuid/parentUuid) y, con --resume, reenvía la cadena desde la hoja hasta el último compact_boundary. Medido
// con 29 ejecuciones reales (Claude Code 2.1.308): basta cambiar el contenido de los tool_result por el texto nativo
// «[Old tool result content cleared]» y quitar su toolUseResult (que no se manda); no se rompen las firmas ni la
// cadena, y con ese texto (no con otro) el modelo no se desdice de lo que dedujo. Así que:
//   - se lee el fichero y se convierte en los mensajes y partes de la poda común (ocMessage/ocPart, con la forma de
//     OpenCode), para decidir con planPrune exactamente igual que en OpenCode;
//   - de lo decidido solo se aplica vaciar salidas («Clear»): nada de recortes, ni del texto de las ediciones, ni del
//     razonamiento; y nunca los resultados de Task/Agent, de Edit/Write ni los errores;
//   - se edita en el sitio, entre turnos y sin ningún «claude» de esa sesión en marcha: copia de seguridad en la
//     carpeta de pgocode (fuera de projects/), temporal en el mismo directorio, fsync y rename. Si el --resume
//     falla, se restaura la copia y se repite sin podar (agentes.go);
//   - la caché importa (escribir a 1 h cuesta 2×; leer, 0,1×): con la caché fría podar sale gratis; con la caliente,
//     solo lotes que se amorticen en pocas llamadas (claudeCacheWorth).
//
// Los originales de lo vaciado se guardan aparte (<sesión>.orig.json): si un intercambio vuelve a ser relevante, se
// repone como en OpenCode («fallo de página»).

const (
	// claudeColdMin: con la caché fría, lo mínimo que tiene que salir para tocar el fichero.
	claudeColdMin = 2000
	// claudeMaxCalls: con la caché caliente, en cuántas llamadas como mucho se tiene que amortizar la reescritura.
	claudeMaxCalls = 10
	// Precios relativos a la entrada: escribir en caché a 1 h y leer de ella.
	claudeCacheWrite = 2.0
	claudeCacheRead  = 0.1
)

// claudeCacheTTL: lo que dura la caché que escribe la primera llamada de cada turno (1 h). Se puede cambiar con
// PGOCODE_CLAUDE_CACHE_TTL (una duración de Go: «1s» para probar la poda con la caché «fría» sin esperar).
var claudeCacheTTL = time.Hour

func init() {
	if d, err := time.ParseDuration(os.Getenv("PGOCODE_CLAUDE_CACHE_TTL")); err == nil && d > 0 {
		claudeCacheTTL = d
	}
}

var (
	// claudeHomeFunc: la carpeta de configuración de Claude Code (CLAUDE_CONFIG_DIR o ~/.claude).
	claudeHomeFunc = func() string {
		if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
			return d
		}
		casa, _ := os.UserHomeDir()
		return filepath.Join(casa, ".claude")
	}
	// claudeBusyFunc: si hay algún proceso con la sesión abierta (en las pruebas, se sustituye).
	claudeBusyFunc = claudeSessionBusy
	// claudeAfterWrite: en las pruebas, para simular un fichero que Claude Code no acepta.
	claudeAfterWrite func(path string)
)

// claudeSessionFile: el fichero de la sesión (los identificadores son únicos: se busca en todas las carpetas).
func claudeSessionFile(id string) (string, error) {
	if !sesionValida.MatchString(id) {
		return "", errors.New("sesión no válida")
	}
	l, _ := filepath.Glob(filepath.Join(claudeHomeFunc(), "projects", "*", id+".jsonl"))
	if len(l) != 1 {
		return "", fmt.Errorf("no se encuentra la sesión %s de Claude Code (%d ficheros)", id, len(l))
	}
	return l[0], nil
}

// claudeSessionBusy: si algún proceso tiene la sesión en su línea de órdenes (un «claude --resume <id>» en marcha,
// de pgocode o del usuario en un terminal). Claude Code escribe añadiendo líneas: si se renombra el fichero
// mientras lo tiene abierto, lo que escriba después se pierde.
func claudeSessionBusy(id string) bool {
	l, _ := filepath.Glob("/proc/[0-9]*/cmdline")
	for _, f := range l {
		b, err := os.ReadFile(f)
		if err == nil && bytes.Contains(b, []byte(id)) && bytes.Contains(b, []byte("claude")) {
			return true
		}
	}
	return false
}

// cjLine: lo que la poda mira de una línea del historial.
type cjLine struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	UUID             string          `json:"uuid"`
	Parent           *string         `json:"parentUuid"`
	Version          string          `json:"version"`
	Timestamp        string          `json:"timestamp"`
	IsMeta           bool            `json:"isMeta"`
	IsSidechain      bool            `json:"isSidechain"`
	IsCompactSummary bool            `json:"isCompactSummary"`
	ToolUseResult    json.RawMessage `json:"toolUseResult"`
	Message          *struct {
		ID      string          `json:"id"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			Input         int `json:"input_tokens"`
			CacheCreation int `json:"cache_creation_input_tokens"`
			CacheRead     int `json:"cache_read_input_tokens"`
			Output        int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// cjBlock: un bloque del contenido de un mensaje.
type cjBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
	Source    struct {
		MediaType string `json:"media_type"`
	} `json:"source"`
}

// claudeOrig: el original de un bloque vaciado (el bloque entero y el toolUseResult de su línea).
type claudeOrig struct {
	Block         json.RawMessage `json:"bloque"`
	ToolUseResult json.RawMessage `json:"toolUseResult,omitempty"`
}

// claudeSession: el historial leído, con lo que hace falta para decidir y aplicar.
type claudeSession struct {
	ID, Path string
	Lines    [][]byte // el fichero, línea a línea (sin el salto)
	Win      []*ocMessage
	Before   []*ocMessage // lo anterior a la última compactación (fuera del contexto del agente)
	// Dónde está cada bloque que se puede tocar: línea e índice dentro de message.content (por su id de parte).
	Where map[string][2]int
	// Pos: lo que llevaba el contexto al llegar cada bloque (tokens; de la llamada que lo pidió). Cache: hasta dónde
	// escribió la caché a 1 h la primera llamada del último turno. LastCall: la última llamada.
	Pos      map[string]int
	Cache    int
	LastCall time.Time
	Orig     map[string]claudeOrig
}

// claudeToolName: el nombre de la herramienta con el de su equivalente en OpenCode (las reglas de prune.go).
func claudeToolName(n string) string {
	switch n {
	case "NotebookEdit":
		return "edit"
	case "Task", "Agent":
		return "task"
	}
	return strings.ToLower(n)
}

// claudeClearable: las herramientas cuyas salidas se pueden vaciar (no las de los subagentes, que son su informe,
// ni las ediciones, que son pequeñas y le dicen al modelo qué cambió).
func claudeClearable(tool string) bool {
	return tool != "task" && !isEditTool(tool)
}

func parseMs(ts string) int64 {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

// toolOutput: el texto y las imágenes del contenido de un tool_result.
func toolOutput(c json.RawMessage) (string, int) {
	var s string
	if json.Unmarshal(c, &s) == nil {
		return s, 0
	}
	var bs []cjBlock
	if json.Unmarshal(c, &bs) != nil {
		return string(c), 0
	}
	var b strings.Builder
	imgs := 0
	for _, x := range bs {
		switch x.Type {
		case "text":
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(x.Text)
		case "image":
			imgs++
		}
	}
	return b.String(), imgs
}

// claudePartData: la parte (con la forma de OpenCode) de una herramienta o una imagen pegada.
func claudePartData(tool, id string, input json.RawMessage, status, out string, imgs int) string {
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	att := make([]map[string]string, imgs)
	for i := range att {
		att[i] = map[string]string{"mime": "image/png"}
	}
	m := map[string]any{"type": "tool", "tool": tool, "callID": id, "state": map[string]any{
		"status": status, "input": input, "output": out, "attachments": att, "time": map[string]any{"end": 1}}}
	b, _ := marshalNoEscape(m)
	return string(b)
}

func marshalNoEscape(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// claudeVersionOK: las versiones de Claude Code con el formato probado.
func claudeVersionOK(v string) bool { return v == "" || strings.HasPrefix(v, "2.1.") }

// parseClaudeSession lee el historial: la cadena desde la hoja hasta el último compact_boundary, convertida en
// mensajes y partes de la poda común. Error si algo no encaja con el formato esperado (entonces no se poda).
func parseClaudeSession(id, path string, data []byte, orig map[string]claudeOrig) (*claudeSession, error) {
	s := &claudeSession{ID: id, Path: path, Where: map[string][2]int{}, Pos: map[string]int{}, Orig: orig}
	if s.Orig == nil {
		s.Orig = map[string]claudeOrig{}
	}
	data = bytes.TrimSuffix(data, []byte("\n"))
	if len(data) == 0 {
		return nil, errors.New("historial vacío")
	}
	s.Lines = bytes.Split(data, []byte("\n"))
	ls := make([]*cjLine, len(s.Lines))
	porUUID := map[string]int{}
	hoja := -1
	for i, b := range s.Lines {
		var l cjLine
		if err := json.Unmarshal(b, &l); err != nil {
			return nil, fmt.Errorf("línea %d: %v", i+1, err)
		}
		ls[i] = &l
		if l.UUID != "" {
			porUUID[l.UUID] = i
		}
		if (l.Type == "user" || l.Type == "assistant") && l.UUID != "" && !l.IsSidechain {
			hoja = i
		}
	}
	if hoja < 0 {
		return nil, errors.New("sin mensajes")
	}
	// La cadena, de la hoja hacia atrás.
	var cadena []int
	visto := map[int]bool{}
	frontera := -1
	for i := hoja; ; {
		if visto[i] {
			return nil, errors.New("cadena con un ciclo")
		}
		visto[i] = true
		l := ls[i]
		if l.Type == "system" && l.Subtype == "compact_boundary" {
			frontera = i
			break
		}
		cadena = append(cadena, i)
		if !claudeVersionOK(l.Version) {
			return nil, fmt.Errorf("versión de Claude Code no probada: %s", l.Version)
		}
		if l.Parent == nil || *l.Parent == "" {
			break
		}
		j, ok := porUUID[*l.Parent]
		if !ok {
			return nil, fmt.Errorf("falta el mensaje %s de la cadena", *l.Parent)
		}
		i = j
	}
	var err error
	if s.Win, err = s.convert(ls, cadena); err != nil {
		return nil, err
	}
	// Lo anterior a la última compactación (fuera de la cadena): solo para la vigencia (fuera del contexto).
	if frontera > 0 {
		var antes []int
		for i := frontera - 1; i >= 0; i-- {
			if l := ls[i]; !visto[i] && !l.IsSidechain && (l.Type == "user" || l.Type == "assistant") {
				antes = append(antes, i)
			}
		}
		aparte := &claudeSession{Where: map[string][2]int{}, Pos: map[string]int{}, Orig: s.Orig}
		s.Before, _ = aparte.convert(ls, antes)
	}
	return s, nil
}

// convert: las líneas idx (de la última a la primera) en mensajes y partes de la poda común. Apunta dónde está cada
// bloque (Where), lo que llevaba el contexto (Pos), la caché del último turno y la última llamada.
func (s *claudeSession) convert(ls []*cjLine, cadena []int) ([]*ocMessage, error) {
	var win []*ocMessage
	// Mensajes y partes, en orden.
	type pend struct {
		p     *ocPart
		tool  string
		input json.RawMessage
	}
	tools := map[string]*pend{}
	var orden []string
	var cur *ocMessage // el mensaje del agente en curso (sus bloques van en líneas seguidas con el mismo message.id)
	curID := ""
	ultimoCtx := 0
	primeraDelTurno := false
	for k := len(cadena) - 1; k >= 0; k-- {
		i := cadena[k]
		l := ls[i]
		if l.Message == nil || (l.Type != "user" && l.Type != "assistant") {
			continue
		}
		t := parseMs(l.Timestamp)
		switch l.Type {
		case "assistant":
			var bs []cjBlock
			if err := json.Unmarshal(l.Message.Content, &bs); err != nil {
				return nil, fmt.Errorf("línea %d: contenido del agente: %v", i+1, err)
			}
			if u := l.Message.Usage; u != nil && u.Input+u.CacheRead+u.CacheCreation > 0 {
				ultimoCtx = u.Input + u.CacheRead + u.CacheCreation + u.Output
				if primeraDelTurno {
					s.Cache = u.Input + u.CacheRead + u.CacheCreation
					primeraDelTurno = false
				}
			}
			if t > 0 {
				s.LastCall = time.UnixMilli(t)
			}
			if cur == nil || curID != l.Message.ID || l.Message.ID == "" {
				cur = &ocMessage{Role: "assistant"}
				curID = l.Message.ID
				win = append(win, cur)
			}
			cur.ID, cur.Time = l.UUID, t
			if u := l.Message.Usage; u != nil {
				cur.Ctx = max(cur.Ctx, u.Input+u.CacheRead+u.CacheCreation)
			}
			for _, b := range bs {
				switch b.Type {
				case "text":
					d, _ := marshalNoEscape(map[string]any{"type": "text", "text": b.Text})
					cur.Parts = append(cur.Parts, newPart(l.UUID+":t", cur.ID, string(d), ""))
				case "tool_use":
					p := &ocPart{ID: b.ID, MessageID: cur.ID}
					cur.Parts = append(cur.Parts, p)
					tools[b.ID] = &pend{p: p, tool: claudeToolName(b.Name), input: b.Input}
					orden = append(orden, b.ID)
					s.Pos[b.ID] = ultimoCtx
				}
			}
		case "user":
			cur = nil
			var str string
			if json.Unmarshal(l.Message.Content, &str) == nil {
				if l.IsMeta {
					continue
				}
				win = append(win, claudeUserMessage(l, t, str))
				primeraDelTurno = !l.IsCompactSummary
				continue
			}
			var bs []cjBlock
			if err := json.Unmarshal(l.Message.Content, &bs); err != nil {
				return nil, fmt.Errorf("línea %d: contenido del usuario: %v", i+1, err)
			}
			resultados := false
			for j, b := range bs {
				if b.Type != "tool_result" {
					continue
				}
				resultados = true
				pd := tools[b.ToolUseID]
				if pd == nil {
					continue
				}
				out, imgs := toolOutput(b.Content)
				status := "completed"
				if b.IsError {
					status = "error"
				}
				id := b.ToolUseID
				s.Where[id] = [2]int{i, j}
				pd.p.Data = claudePartData(pd.tool, id, pd.input, status, out, imgs)
				if out == ClearedText && imgs == 0 && status == "completed" {
					if o, ok := s.Orig[id]; ok {
						var ob cjBlock
						json.Unmarshal(o.Block, &ob)
						oo, oi := toolOutput(ob.Content)
						pd.p.Data = claudePartData(pd.tool, id, pd.input, status, oo, oi)
						pd.p.Current, _ = renderPart(pd.p.Data, pruneEdit{Clear: true})
					} else {
						// Vaciado sin original (por Claude Code o perdido): fuera, sin vuelta atrás.
						pd.p.Data, _ = renderPart(pd.p.Data, pruneEdit{Clear: true})
					}
				}
				pd.p.Data = strings.TrimSpace(pd.p.Data)
				json.Unmarshal([]byte(pd.p.Data), &pd.p.v)
				if pd.p.Current != "" && pd.p.Current == pd.p.Data {
					pd.p.Current = ""
				}
			}
			if resultados || l.IsMeta {
				continue
			}
			// Un mensaje del usuario con texto e imágenes pegadas (cada imagen, una pieza que se puede vaciar).
			var txt strings.Builder
			for _, b := range bs {
				if b.Type == "text" {
					txt.WriteString(b.Text)
				}
			}
			m := claudeUserMessage(l, t, txt.String())
			for j, b := range bs {
				if b.Type != "image" && !(b.Type == "text" && b.Text == ClearedText && s.Orig[imageID(l.UUID, j)].Block != nil) {
					continue
				}
				id := imageID(l.UUID, j)
				s.Where[id] = [2]int{i, j}
				s.Pos[id] = ultimoCtx
				data := claudePartData("image", id, json.RawMessage(`{"imagen":`+fmt.Sprint(j+1)+`}`), "completed", "", 1)
				cur := ""
				if b.Type == "text" { // ya vaciada por pgocode
					cur, _ = renderPart(data, pruneEdit{Clear: true})
				}
				m.Parts = append(m.Parts, newPart(id, m.ID, data, cur))
			}
			win = append(win, m)
			primeraDelTurno = true
		}
	}
	// Herramientas sin resultado (el turno se cortó): en curso, no se tocan.
	for _, id := range orden {
		if pd := tools[id]; pd.p.Data == "" {
			pd.p.Data = claudePartData(pd.tool, id, pd.input, "running", "", 0)
			json.Unmarshal([]byte(pd.p.Data), &pd.p.v)
		}
	}
	return win, nil
}

func imageID(uuid string, i int) string { return fmt.Sprintf("img:%s:%d", uuid, i) }

func claudeUserMessage(l *cjLine, t int64, text string) *ocMessage {
	m := &ocMessage{ID: l.UUID, Role: "user", Time: t}
	d, _ := marshalNoEscape(map[string]any{"type": "text", "text": text})
	m.Parts = append(m.Parts, newPart(l.UUID+":t", l.UUID, string(d), ""))
	return m
}

// claudePlan: de lo decidido, solo lo que se aplica en Claude Code (vaciar salidas que se pueden vaciar).
func claudePlan(plan prunePlan, win []*ocMessage) prunePlan {
	for id, e := range plan.Edits {
		p := partByID(win, id)
		if !e.Clear || !claudeClearable(p.v.Tool) {
			delete(plan.Edits, id)
		}
	}
	for _, pv := range plan.Pieces {
		e, ok := plan.Edits[pv.PartID]
		pv.Out = ok && e.Clear
		pv.Note = ""
	}
	return plan
}

// claudeCacheWorth: si compensa aplicar el cambio ahora, por la caché (receta medida con Claude Code):
//
//	sobrecoste de la 1.ª llamada  E = (C − P − X₁)·w − (C − P)·r − X₂·w + R·w
//	ahorro de cada llamada después     X·r
//
// P: hasta dónde se sigue leyendo de la caché: el prompt de sistema (sys), porque Claude Code solo tiene ahí un
// punto de caché fijo y lo demás va en el último mensaje: un cambio por la mitad hace reescribir todo lo de después
// del sistema, no solo desde el primer cambio (medido: un aviso de ~7k escribió 46 266). C: hasta dónde llega la
// caché aprovechable; X₁/X₂: lo que sale antes/después de C; R: lo que se repone. Con la caché fría (más de una
// hora desde la última llamada), gratis.
func claudeCacheWorth(s *claudeSession, d pruneDiff, cleared map[string]int, sys int, now time.Time) (bool, string) {
	if d.Saved <= 0 && len(d.Restore) == 0 {
		return false, "nada que retirar"
	}
	if s.LastCall.IsZero() || now.Sub(s.LastCall) > claudeCacheTTL {
		if d.Saved >= claudeColdMin {
			return true, "caché fría"
		}
		return false, fmt.Sprintf("caché fría, pero solo %d tokens", d.Saved)
	}
	if d.Saved < pruneMinTokens {
		return false, fmt.Sprintf("caché caliente y solo %d tokens", d.Saved)
	}
	P, X1, X2 := -1, 0, 0
	mark := func(id string) {
		if pos, ok := s.Pos[id]; ok && (P < 0 || pos < P) {
			P = pos
		}
	}
	for id, x := range cleared {
		if x < 0 {
			continue // lo que vuelve va en R
		}
		mark(id)
		if s.Pos[id] < s.Cache {
			X1 += x
		} else {
			X2 += x
		}
	}
	R := 0
	for _, id := range d.Restore {
		mark(id)
		R += max(-cleared[id], 0)
	}
	if P < 0 {
		P = 0
	}
	if sys > 0 {
		P = min(P, sys) // lo de después del sistema se reescribe entero
	}
	C := max(s.Cache, P)
	X := X1 + X2
	E := float64(C-P-X1)*claudeCacheWrite - float64(C-P)*claudeCacheRead - float64(X2)*claudeCacheWrite + float64(R)*claudeCacheWrite
	if E <= 0 {
		return true, "se amortiza desde la primera llamada"
	}
	if X <= 0 {
		return false, "no se amortiza"
	}
	n := E / (float64(X) * claudeCacheRead)
	if n <= claudeMaxCalls {
		return true, fmt.Sprintf("se amortiza en %.1f llamadas", n)
	}
	return false, fmt.Sprintf("caché caliente: se amortizaría en %.0f llamadas", n)
}

// applyClaude: el fichero nuevo, con lo que sale vaciado y lo que vuelve repuesto. Las demás líneas, byte a byte.
// Devuelve también los originales que hay que guardar antes de escribirlo.
func applyClaude(s *claudeSession, clear []string, restore []string) ([]byte, map[string]claudeOrig, error) {
	porLinea := map[int]map[int]string{} // línea → índice del bloque → «c» (vaciar) o «r» (reponer)
	for _, id := range clear {
		w, ok := s.Where[id]
		if !ok {
			return nil, nil, fmt.Errorf("no se encuentra el bloque %s", id)
		}
		if porLinea[w[0]] == nil {
			porLinea[w[0]] = map[int]string{}
		}
		porLinea[w[0]][w[1]] = "c:" + id
	}
	for _, id := range restore {
		w, ok := s.Where[id]
		if _, hay := s.Orig[id]; !ok || !hay {
			continue
		}
		if porLinea[w[0]] == nil {
			porLinea[w[0]] = map[int]string{}
		}
		porLinea[w[0]][w[1]] = "r:" + id
	}
	nuevos := map[string]claudeOrig{}
	out := make([][]byte, len(s.Lines))
	copy(out, s.Lines)
	for i, cambios := range porLinea {
		var top map[string]json.RawMessage
		if err := json.Unmarshal(s.Lines[i], &top); err != nil {
			return nil, nil, err
		}
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(top["message"], &msg); err != nil {
			return nil, nil, err
		}
		var bs []json.RawMessage
		if err := json.Unmarshal(msg["content"], &bs); err != nil {
			return nil, nil, err
		}
		tur := top["toolUseResult"]
		quitaTUR, poneTUR := false, json.RawMessage(nil)
		var err error
		for j, c := range cambios {
			if j >= len(bs) {
				return nil, nil, fmt.Errorf("bloque %d fuera de la línea %d", j, i+1)
			}
			accion, id, _ := strings.Cut(c, ":")
			if accion == "r" {
				o := s.Orig[id]
				bs[j] = o.Block
				if len(o.ToolUseResult) > 0 {
					poneTUR = o.ToolUseResult
				}
				continue
			}
			nuevos[id] = claudeOrig{Block: append(json.RawMessage(nil), bs[j]...), ToolUseResult: tur}
			if strings.HasPrefix(id, "img:") {
				bs[j] = json.RawMessage(`{"type":"text","text":"` + ClearedText + `"}`)
				continue
			}
			var b map[string]json.RawMessage
			if err := json.Unmarshal(bs[j], &b); err != nil {
				return nil, nil, err
			}
			b["content"], _ = json.Marshal(ClearedText)
			if bs[j], err = marshalNoEscape(b); err != nil {
				return nil, nil, err
			}
			quitaTUR = true
		}
		if msg["content"], err = marshalNoEscape(bs); err != nil {
			return nil, nil, err
		}
		if top["message"], err = marshalNoEscape(msg); err != nil {
			return nil, nil, err
		}
		switch {
		case poneTUR != nil:
			top["toolUseResult"] = poneTUR
		case quitaTUR:
			delete(top, "toolUseResult")
		}
		if out[i], err = marshalNoEscape(top); err != nil {
			return nil, nil, err
		}
	}
	return append(bytes.Join(out, []byte("\n")), '\n'), nuevos, nil
}

// claudeDir: la carpeta de pgocode con las copias y los originales de las sesiones de Claude Code.
func claudeDir() string { return filepath.Join(pruneDirFunc(), "claude") }

// claudeBackup: la copia del historial de antes de podar (para restaurarla si el --resume falla).
type claudeBackup struct{ path, copy string }

// restore: vuelve a poner el historial de antes de podar.
func (b *claudeBackup) restore() error {
	if b == nil {
		return nil
	}
	data, err := os.ReadFile(b.copy)
	if err != nil {
		return err
	}
	return writeAtomic(b.path, data)
}

// writeAtomic: temporal en el mismo directorio, fsync y rename.
func writeAtomic(path string, data []byte) error {
	tmp := path + ".pgocode.tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func loadClaudeOrig(id string) map[string]claudeOrig {
	m := map[string]claudeOrig{}
	if b, err := os.ReadFile(filepath.Join(claudeDir(), id+".orig.json")); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func saveClaudeOrig(id string, m map[string]claudeOrig) error {
	if err := os.MkdirAll(claudeDir(), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(claudeDir(), id+".orig.json"), b)
}

// loadClaude: el historial de la sesión, leído y convertido.
func loadClaude(id string) (*claudeSession, error) {
	path, err := claudeSessionFile(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseClaudeSession(id, path, data, loadClaudeOrig(id))
}

// sweepClaude: el barrido al empezar el turno de Claude Code (en el hilo del recolector). Si cambia el historial,
// devuelve la copia de antes.
// force: lo pide el usuario («Barrer ahora»): sin el mínimo ni la cuenta de la caché; si la sesión está abierta en
// otro proceso, queda pendiente para el próximo turno (ForcePending).
func (c *pruneCollector) sweepClaude(ctx context.Context, p Peticion, msg string, force bool) (*PruneReport, *claudeBackup, error) {
	c.setFolder(p.Carpeta)
	s, err := loadClaude(c.key)
	if err != nil {
		c.state.Prunable = false
		c.save()
		return &PruneReport{}, nil, err
	}
	c.state.Prunable = true
	win := s.Win
	var sims *pruneSims
	if p.Prune == PruneAuto {
		c.setPhase("marcando", 0)
		ce, cancela := context.WithTimeout(ctx, 15*time.Second)
		sims = c.sims(ce, win, msg)
		cancela()
		if sims == nil && len(c.state.Tiers) > 0 {
			sims = &pruneSims{PrevTier: c.state.Tiers}
		}
	}
	now := pruneNow()
	o := decideSweep(sweepInput{Win: win, Sims: sims, Young: pruneYoung, Now: now, K: c.params(), Msg: msg, Target: c.state.Target, Claude: true, Empty: c.emptyNext})
	plan, d := o.Plan, o.D
	vale, porque := claudeCacheWorth(s, d, claudeCleared(win, o), o.System, now)
	if force {
		vale, porque = d.changes(), "barrido pedido por el usuario"
		if !vale {
			c.state.ForcePending = false
		}
	}
	if vale && claudeBusyFunc(c.key) {
		vale, porque = false, "la sesión está abierta en otro proceso"
		if force {
			c.state.ForcePending = true // al empezar el próximo turno
		}
	}
	c.state.CacheUntil = 0
	if !s.LastCall.IsZero() {
		c.state.CacheUntil = s.LastCall.Add(claudeCacheTTL).UnixMilli()
	}
	if !vale {
		depuracion.Log("claude-code", "poda %s: %s (%s)", c.key, d.pendingString(), porque)
		c.state.Vig = vigSnapshot(plan, c.outNow, func(p *pieceVig) bool { return appliedOut(partByID(win, p.PartID)) })
		c.state.Vig.addCompacted(s.Before)
		c.noteClaudePending(s, o, now)
		c.save()
		return c.reportNow(now), nil, nil
	}
	c.setPhase("barriendo", len(d.Update)+len(d.Restore))
	var clear []string
	for id := range d.Update {
		clear = append(clear, id)
	}
	data, nuevos, err := applyClaude(s, clear, d.Restore)
	if err != nil {
		return &PruneReport{Podable: true}, nil, err
	}
	// Primero los originales y la copia (fuera de projects/); después, el historial.
	for id, o := range nuevos {
		if _, ya := s.Orig[id]; !ya {
			s.Orig[id] = o
		}
	}
	if err := saveClaudeOrig(c.key, s.Orig); err != nil {
		return &PruneReport{Podable: true}, nil, err
	}
	bak := &claudeBackup{path: s.Path, copy: filepath.Join(claudeDir(), c.key+".jsonl.bak")}
	orig, err := os.ReadFile(s.Path)
	if err != nil {
		return &PruneReport{Podable: true}, nil, err
	}
	if err := writeAtomic(bak.copy, orig); err != nil {
		return &PruneReport{Podable: true}, nil, err
	}
	if err := writeAtomic(s.Path, data); err != nil {
		return &PruneReport{Podable: true}, nil, err
	}
	if claudeAfterWrite != nil {
		claudeAfterWrite(s.Path)
	}
	tiers := map[string]pruneTier{}
	for _, x := range plan.Exchanges {
		if x.Tier == TierL3 {
			tiers[x.UserID] = TierL3
		}
	}
	c.state.Tiers = tiers
	c.state.Vig = vigSnapshot(plan, func(x *pruneExchange) bool { return x.Tier == TierL3 }, func(p *pieceVig) bool { return p.Out })
	c.state.Vig.addCompacted(s.Before)
	c.noteSweep(max(d.Saved, 0))
	c.state.Pending, c.state.CacheSuffix, c.state.ForcePending, c.state.Back, c.state.NoPays = 0, 0, false, 0, false
	c.state.Context = max(o.Ctx-d.Saved, 0)
	c.save()
	depuracion.Log("claude-code", "poda %s: %s (%d vaciados, %d repuestos; %s)", c.key, d, len(clear), len(d.Restore), porque)
	rep := &PruneReport{Removed: max(d.Saved, 0), Restored: len(d.Restore), Podable: true, Calibration: calibrationInfo(c.state.Folder),
		Threshold: c.state.Threshold, Target: c.state.Target, Context: c.state.Context}
	rep.Exchanges, rep.Pieces = c.state.Vig.report(now)
	return rep, bak, nil
}

// noteClaudePending: lo pendiente en Claude Code, con lo que costaría ahora en caché (lo de después del sistema, a
// precio de escritura) y si compensa (claudeCacheWorth).
func (c *pruneCollector) noteClaudePending(s *claudeSession, o sweepOutcome, now time.Time) {
	c.state.Pending, c.state.Back, c.state.CacheSuffix = max(o.D.Saved, 0), o.D.Back, o.Rewrite
	vale, _ := claudeCacheWorth(s, o.D, claudeCleared(s.Win, o), o.System, now)
	c.state.NoPays = !vale
	if o.Ctx > 0 {
		c.state.Context = o.Ctx
	}
}

// markClaudeNow: al acabar el turno, la vigencia con lo nuevo (y los vectores de lo que falte).
func (c *pruneCollector) markClaudeNow(ctx context.Context, embeddings bool) {
	s, err := loadClaude(c.key)
	if err != nil {
		depuracion.Log("claude-code", "poda: leer la sesión %s: %v", c.key, err)
		c.state.Prunable = false
		c.save()
		return
	}
	c.state.Prunable = true
	c.state.CacheUntil = 0
	if !s.LastCall.IsZero() {
		c.state.CacheUntil = s.LastCall.Add(claudeCacheTTL).UnixMilli()
	}
	o := c.markWin(ctx, s.Win, s.Before, embeddings, true)
	c.noteClaudePending(s, o, pruneNow())
	c.save()
	c.publish(c.reportNow(pruneNow()))
}

// claudeCleared: lo que sale (positivo) o vuelve (negativo) de cada parte que cambia (tokens reales).
func claudeCleared(win []*ocMessage, o sweepOutcome) map[string]int {
	cleared := map[string]int{}
	vuelve := map[string]bool{}
	for _, id := range o.D.Restore {
		vuelve[id] = true
	}
	for _, m := range win {
		for _, pt := range m.Parts {
			cur := curData(pt)
			if nuevo, ok := o.D.Update[pt.ID]; ok {
				cleared[pt.ID] = int(float64(tokensOf(cur)-tokensOf(nuevo)) * o.Ratio)
			}
			if vuelve[pt.ID] {
				cleared[pt.ID] = int(float64(tokensOf(cur)-tokensOf(pt.Data)) * o.Ratio)
			}
		}
	}
	return cleared
}

// pruneClaude: el barrido al empezar el turno de Claude Code. Devuelve lo que ve la interfaz y, si ha cambiado el
// historial, la copia para restaurarlo si el --resume falla.
func pruneClaude(ctx context.Context, p Peticion) (*PruneReport, *claudeBackup) {
	if p.Prune == PruneOff || p.Aparte || p.Corte != "" || p.SesionAgente == "" || !sesionValida.MatchString(p.SesionAgente) || len(p.Mensajes) == 0 {
		return nil, nil
	}
	c := collectorFor(p.SesionAgente)
	var rep *PruneReport
	var bak *claudeBackup
	ce, cancela := context.WithTimeout(ctx, 60*time.Second)
	defer cancela()
	err := c.do(ce, func() {
		defer c.setPhase("", 0)
		c.noteLimits(p)
		var err error
		// Un «Barrer ahora» que esperaba a que se cerrara la sesión: ahora.
		if rep, bak, err = c.sweepClaude(ce, p, p.Mensajes[len(p.Mensajes)-1].Texto, c.state.ForcePending); err != nil {
			depuracion.Log("claude-code", "poda %s: %v", p.SesionAgente, err)
		}
		c.publish(rep)
	})
	if err != nil {
		depuracion.Log("claude-code", "poda %s: %v", p.SesionAgente, err)
	}
	return rep, bak
}

// markClaude: al acabar el turno, el marcado de lo nuevo (en segundo plano).
func markClaude(p Peticion, ses string) {
	if p.Prune == PruneOff || p.Aparte || ses == "" || !sesionValida.MatchString(ses) {
		return
	}
	c := collectorFor(ses)
	c.setPhase("marcando", 0)
	c.later(func() {
		defer c.setPhase("", 0)
		ctx, cancela := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancela()
		c.setFolder(p.Carpeta)
		c.noteLimits(p)
		c.markClaudeNow(ctx, p.Prune == PruneAuto)
		noteTurn(c.state.Folder)
	})
}
