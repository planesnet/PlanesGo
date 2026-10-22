package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// Poda automática del historial del agente, decidida por pgocode (de momento, OpenCode).
//
// Los agentes reenvían todo su historial en cada paso. pgocode, que está por encima, sabe qué ya no importa y lo
// retira antes de cada turno, sin que el usuario marque nada ni el agente reciba instrucciones de más. Funciona como
// un recolector de basura con generaciones, con tres niveles como la memoria:
//
//   - L1: el turno en curso y los dos últimos intercambios (la generación joven). No se tocan nunca.
//   - L2: lo relacionado con lo último. Sigue en el contexto, pero sin lo que ya está superado: lecturas de un
//     fichero que después se releyó o editó, imágenes ya vistas, salidas de órdenes repetidas, salidas largas (quedan
//     sus primeras y últimas líneas), el texto viejo y nuevo de las ediciones y el razonamiento. Y, por piezas, las
//     lecturas o salidas grandes sin relación con lo último (dos granos de embedding: intercambio y pieza).
//   - L3: lo que no tiene relación con lo último (similitud por debajo del umbral frente al mensaje nuevo). Sus
//     herramientas pasan a una línea indicativa; los textos del usuario y del agente, que son pequeños, se quedan.
//     El original lo guarda pgocode con su embedding: si un mensaje nuevo se le parece («fallo de página»), vuelve
//     a L2 (se repone con el siguiente barrido que compense; ver worthIt).
//
// Lo que se quita deja una línea que dice qué era y cómo recuperarlo: equivocarse cuesta una relectura, no un
// error. El marcado (embeddings y estado de cada intercambio) se hace en segundo plano según llega cada cosa
// (prune_opencode.go); el barrido (cambiar el historial del agente) solo al empezar el turno, para no romper la
// caché a mitad de turno, y solo si lo que sale compensa perderla (pruneMinTokens).

// Modos de poda de una petición (Peticion.Prune).
const (
	PruneOff   = ""      // sin poda
	PruneAuto  = "auto"  // reglas y relevancia por embeddings
	PruneRules = "rules" // solo reglas (o sin Ollama)
)

// pruneTier: el nivel de un intercambio.
type pruneTier string

const (
	TierL1 pruneTier = "L1"
	TierL2 pruneTier = "L2"
	TierL3 pruneTier = "L3"
)

const (
	// pruneThreshold: similitud mínima para seguir en el contexto, con vectores de 256 dimensiones (Matrioska de
	// embeddinggemma). Calibrado con la conversación de INTRADEMA: 0,30 con 768 dimensiones equivale a 0,38 con 256
	// (el 91 % de las decisiones coinciden; con 256 las similitudes salen de media 0,08 más altas).
	pruneThreshold = 0.38
	// pruneReviveMargin: lo que tiene que pasar del umbral un intercambio de L3 para volver (evita que oscile de un
	// turno a otro y se pierda la caché cada vez).
	pruneReviveMargin = 0.04
	// pruneMinTokens: solo se barre si lo que sale pasa de esto; si no, no compensa perder la caché.
	pruneMinTokens = 15000
	// pruneShortWords: un mensaje más corto («compila y actualiza») hereda el anterior.
	pruneShortWords = 8
	// pruneYoung: intercambios de L1 (además del turno en curso).
	pruneYoung = 2
	// prunePieceTokens: tamaño a partir del que una pieza (lectura, salida) lleva su propio embedding.
	prunePieceTokens = 1500
	// Salidas largas: a partir de cuántas líneas o caracteres se recortan y cuánto queda de cada extremo.
	pruneLongLines = 60
	pruneLongChars = 6000
	pruneKeepLines = 20
	pruneKeepChars = 1500
	// pruneImageTokens: lo que cuenta una imagen adjunta (Gemini cobra por imagen, no por sus bytes).
	pruneImageTokens = 1500
	// pruneDims: dimensiones de los vectores (recortados: rápidos y pequeños).
	pruneDims = 256
)

// ocMessage y ocPart: un mensaje de la sesión de OpenCode y sus partes, tal como están en su base de datos.
type ocMessage struct {
	ID, Role, Parent string
	Summary          bool
	Parts            []*ocPart
}

type ocPart struct {
	ID, MessageID string
	// Data: el original (si pgocode la cambió, el que guardó). Current: la actual si es distinta del original.
	Data, Current string
	v             partView
}

// partView: lo que la poda mira de una parte (el resto se conserva tal cual al cambiarla).
type partView struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Tool        string `json:"tool"`
	CallID      string `json:"callID"`
	Synthetic   bool   `json:"synthetic"`
	TailStartID string `json:"tail_start_id"`
	State       struct {
		Status      string          `json:"status"`
		Title       string          `json:"title"`
		Input       json.RawMessage `json:"input"`
		Output      json.RawMessage `json:"output"`
		Attachments []struct {
			Mime string `json:"mime"`
		} `json:"attachments"`
		Time struct {
			Compacted int64 `json:"compacted"`
		} `json:"time"`
	} `json:"state"`
}

func newPart(id, msg, data, current string) *ocPart {
	p := &ocPart{ID: id, MessageID: msg, Data: data, Current: current}
	json.Unmarshal([]byte(data), &p.v)
	return p
}

// output: la salida de la herramienta (texto).
func (p *ocPart) output() string {
	var s string
	if json.Unmarshal(p.v.State.Output, &s) == nil {
		return s
	}
	return string(p.v.State.Output)
}

func (p *ocPart) input() map[string]any {
	var m map[string]any
	json.Unmarshal(p.v.State.Input, &m)
	return m
}

// filePath: el fichero de una lectura, edición o escritura.
func (p *ocPart) filePath() string {
	m := p.input()
	for _, k := range []string{"filePath", "file_path", "path"} {
		if s, ok := m[k].(string); ok && s != "" {
			return filepath.Clean(s)
		}
	}
	return ""
}

// label: cómo se nombra la pieza en la línea que la sustituye (el fichero relativo, la orden…).
func (p *ocPart) label() string {
	if p.v.State.Title != "" && (p.v.Tool == "read" || p.v.Tool == "edit" || p.v.Tool == "write") {
		return p.v.State.Title
	}
	if f := p.filePath(); f != "" {
		return filepath.Base(f)
	}
	return recortaLinea(resumen(p.v.State.Input), 80)
}

func recortaLinea(s string, n int) string {
	s = primeraLinea(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// isTool: una herramienta acabada con salida (las que fallaron o ya vació OpenCode no se tocan).
func (p *ocPart) isTool() bool {
	return p.v.Type == "tool" && p.v.State.Status == "completed" && p.v.State.Time.Compacted == 0
}

// tokensOf: lo que ocupa lo que se manda al modelo de una parte. Las salidas de herramientas (código con números de
// línea, JSON) salen a unos 3 caracteres por token con Gemini (medido: una lectura de 61k caracteres eran 24k
// tokens, y aún así esto se queda corto); el texto, a unos 4.
func tokensOf(data string) int {
	var v partView
	if json.Unmarshal([]byte(data), &v) != nil {
		return len(data) / 4
	}
	switch v.Type {
	case "tool":
		if v.State.Time.Compacted != 0 {
			return (len(v.State.Input) + 40) / 3
		}
		return (len(v.State.Input)+len(v.State.Output))/3 + pruneImageTokens*len(v.State.Attachments)
	case "text", "reasoning":
		return len(v.Text) / 4
	}
	return 0
}

// pruneExchange: un intercambio (mensaje del usuario y la respuesta del agente, con sus pasos).
type pruneExchange struct {
	UserID, Point string // Point: el último mensaje del agente (el «punto» de pgocode)
	Messages      []*ocMessage
	Tier          pruneTier
	Sim           float64 // -1: sin similitud
	Reason        string
}

// userText y answerText: lo que escribió el usuario y la respuesta final del agente.
func (x *pruneExchange) userText() string {
	var b strings.Builder
	for _, p := range x.Messages[0].Parts {
		if p.v.Type == "text" && !p.v.Synthetic {
			b.WriteString(p.v.Text)
		}
	}
	return quitaIdioma(b.String())
}

func (x *pruneExchange) answerText() string {
	for i := len(x.Messages) - 1; i > 0; i-- {
		m := x.Messages[i]
		for j := len(m.Parts) - 1; j >= 0; j-- {
			if p := m.Parts[j]; p.v.Type == "text" && strings.TrimSpace(p.v.Text) != "" {
				return p.v.Text
			}
		}
	}
	return ""
}

// quitaIdioma: sin la instrucción de idioma que pgocode pone al principio de la conversación.
func quitaIdioma(s string) string {
	if strings.HasPrefix(s, "Idioma de las respuestas:") {
		if _, r, ok := strings.Cut(s, "\n\n"); ok {
			return r
		}
	}
	return s
}

// embedText: el documento del intercambio para su embedding (mensaje, respuesta y ficheros tocados).
func (x *pruneExchange) embedText() string {
	t := recorta(strings.TrimSpace(x.userText()), 1500) + "\nRespuesta: " + recorta(strings.TrimSpace(x.answerText()), 1500)
	var fs []string
	visto := map[string]bool{}
	for _, m := range x.Messages {
		for _, p := range m.Parts {
			if f := p.filePath(); p.v.Type == "tool" && f != "" && !visto[f] {
				visto[f] = true
				fs = append(fs, filepath.Base(f))
			}
		}
	}
	if len(fs) > 0 {
		t += "\nFicheros: " + strings.Join(fs, ", ")
	}
	return t
}

// pieceText: el documento de una pieza grande (herramienta, qué y el principio de su salida). Es el mismo que se
// arma con el evento de la herramienta al llegar (pieceEmbedText).
func (p *ocPart) pieceText() string {
	return pieceEmbedText(p.v.Tool, resumen(p.v.State.Input), p.output())
}

func pieceEmbedText(tool, res, out string) string {
	return tool + " " + res + "\n" + recorta(out, 1500)
}

// isPiece: si la pieza es lo bastante grande para llevar su propio embedding.
func (p *ocPart) isPiece() bool {
	return p.isTool() && len(p.v.State.Output)/4 >= prunePieceTokens && p.v.Tool != "edit" && p.v.Tool != "write"
}

// pruneWindow: los mensajes que OpenCode manda al modelo (tras su última compactación: la cola que conserva y lo
// posterior al resumen), sin el mensaje de compactación ni el resumen, que no se tocan.
func pruneWindow(msgs []*ocMessage) []*ocMessage {
	comp, tail, sum := "", "", ""
	for i, m := range msgs {
		if m.Role != "user" {
			continue
		}
		for _, p := range m.Parts {
			if p.v.Type != "compaction" {
				continue
			}
			for _, r := range msgs[i+1:] {
				if r.Role == "assistant" && r.Summary && r.Parent == m.ID {
					comp, tail, sum = m.ID, p.v.TailStartID, r.ID
					break
				}
			}
		}
	}
	if comp == "" {
		return msgs
	}
	var w []*ocMessage
	for _, m := range msgs {
		if (tail != "" && m.ID >= tail && m.ID < comp) || m.ID > sum {
			w = append(w, m)
		}
	}
	return w
}

// pruneExchanges: los intercambios de la ventana (cada uno empieza en un mensaje del usuario).
func pruneExchanges(win []*ocMessage) []*pruneExchange {
	var xs []*pruneExchange
	for _, m := range win {
		if m.Role == "user" {
			xs = append(xs, &pruneExchange{UserID: m.ID, Messages: []*ocMessage{m}, Tier: TierL2, Sim: -1})
			continue
		}
		if len(xs) > 0 {
			x := xs[len(xs)-1]
			x.Messages = append(x.Messages, m)
			x.Point = m.ID
		}
	}
	return xs
}

// pruneEdit: lo que se hace con una parte.
type pruneEdit struct {
	Delete bool // se quita (razonamiento)
	// Clear: la salida (y sus adjuntos) fuera, como hace OpenCode en su propia poda: con state.time.compacted, que
	// hace que mande «[Old tool result content cleared]» en su lugar. El texto nativo importa: con textos propios
	// («[pgocode: lectura retirada…]») el modelo concluía que sus respuestas anteriores eran inventadas y se
	// desdecía (8 de 8 en las pruebas con Claude Code); con el nativo, mantiene lo deducido (4 de 4). Lo que ve el
	// usuario (el motivo) va en la interfaz de pgocode, no en el historial del agente.
	Clear     bool
	Output    *string // nueva salida (salidas largas: principio y final)
	TrimInput bool    // ediciones y escrituras: sin el texto
	Why       string  // para el motivo que ve el usuario («» si no se muestra)
}

// ClearedText: el texto con que OpenCode sustituye lo que retira (y pgocode, en las entradas de las ediciones).
const ClearedText = "[Old tool result content cleared]"

// truncatedNote: la marca de OpenCode para una salida recortada («[Tool output truncated for compaction…]»).
func truncatedNote(n int) string {
	return fmt.Sprintf("\n[Tool output truncated for compaction: omitted %d chars]\n", n)
}

// pruneSims: similitudes frente al mensaje nuevo (por intercambio, con su UserID, y por pieza, con su id) y el
// nivel que tenía cada intercambio en el barrido anterior. Nil: solo reglas.
type pruneSims struct {
	Exchange map[string]float64
	Piece    map[string]float64
	PrevTier map[string]pruneTier
}

// prunePlan: lo que se quiere de cada parte y el nivel de cada intercambio.
type prunePlan struct {
	Edits     map[string]pruneEdit
	Exchanges []*pruneExchange
}

// planPrune decide qué se quita (sin tocar nada). young: intercambios de L1 (los últimos).
func planPrune(win []*ocMessage, sims *pruneSims, young int) prunePlan {
	xs := pruneExchanges(win)
	plan := prunePlan{Edits: map[string]pruneEdit{}, Exchanges: xs}
	// Lo que pasa después de cada parte: lecturas, ediciones y órdenes posteriores (en cualquier nivel).
	type pos struct{ x, m, p int }
	var orden []pos
	for i, x := range xs {
		for j, m := range x.Messages {
			for k := range m.Parts {
				orden = append(orden, pos{i, j, k})
			}
		}
	}
	part := func(q pos) *ocPart { return xs[q.x].Messages[q.m].Parts[q.p] }
	lastRead, lastWrite, lastCmd, lastTodo := map[string]int{}, map[string]int{}, map[string]int{}, -1
	for n, q := range orden {
		p := part(q)
		if p.v.Type != "tool" {
			continue
		}
		switch p.v.Tool {
		case "read":
			lastRead[p.filePath()] = n
		case "edit", "write", "patch", "apply_patch", "multiedit":
			lastWrite[p.filePath()] = n
		case "bash":
			lastCmd[strings.TrimSpace(resumen(p.v.State.Input))] = n
		case "todowrite":
			lastTodo = n
		}
	}
	first := 0
	for i, x := range xs {
		switch {
		case i >= len(xs)-young:
			x.Tier = TierL1
		case i == first || sims == nil:
			x.Tier = TierL2
		default:
			s, ok := sims.Exchange[x.UserID]
			if !ok {
				// Sin su similitud (Ollama no responde): lo que estaba fuera sigue fuera (reponerlo rompería la caché).
				if sims.PrevTier[x.UserID] == TierL3 {
					x.Tier, x.Reason = TierL3, "fuera del contexto: sin relación con lo último"
				}
				break
			}
			x.Sim = s
			limite := pruneThreshold
			if sims.PrevTier[x.UserID] == TierL3 {
				limite += pruneReviveMargin
			}
			if s < limite {
				x.Tier = TierL3
				x.Reason = "fuera del contexto: sin relación con lo último · " + strings.Replace(fmt.Sprintf("%.2f", s), ".", ",", 1)
			}
		}
	}
	whys := make([]map[string]bool, len(xs))
	for n, q := range orden {
		x := xs[q.x]
		if x.Tier == TierL1 {
			continue
		}
		m := x.Messages[q.m]
		p := part(q)
		e, ok := pruneRule(p, m, x.Tier, n, lastRead, lastWrite, lastCmd, lastTodo)
		if !ok && x.Tier == TierL2 && sims != nil && p.isPiece() {
			if s, hay := sims.Piece[p.ID]; hay && s < pruneThreshold {
				e, ok = pruneEdit{Clear: true, Why: "piezas sin relación"}, true
			}
		}
		if !ok {
			continue
		}
		plan.Edits[p.ID] = e
		if e.Why != "" {
			if whys[q.x] == nil {
				whys[q.x] = map[string]bool{}
			}
			whys[q.x][e.Why] = true
		}
	}
	for i, x := range xs {
		if x.Tier == TierL2 && len(whys[i]) > 0 {
			var l []string
			for _, w := range []string{"lecturas superadas", "imágenes ya vistas", "órdenes repetidas", "salidas largas recortadas", "piezas sin relación"} {
				if whys[i][w] {
					l = append(l, w)
				}
			}
			x.Reason = "retirado del contexto: " + strings.Join(l, ", ")
		}
	}
	return plan
}

// pruneRule: las reglas seguras de una parte de L2, o en L3 su línea indicativa.
func pruneRule(p *ocPart, m *ocMessage, tier pruneTier, n int, lastRead, lastWrite, lastCmd map[string]int, lastTodo int) (pruneEdit, bool) {
	if p.v.Type == "reasoning" {
		// El razonamiento de turnos anteriores, salvo si es lo único del mensaje (quedaría vacío y Gemini lo rechaza).
		for _, o := range m.Parts {
			if o.v.Type == "tool" || (o.v.Type == "text" && strings.TrimSpace(o.v.Text) != "") {
				return pruneEdit{Delete: true}, true
			}
		}
		return pruneEdit{}, false
	}
	if !p.isTool() {
		return pruneEdit{}, false
	}
	trim := p.v.Tool == "edit" || p.v.Tool == "write" || p.v.Tool == "patch" || p.v.Tool == "apply_patch" || p.v.Tool == "multiedit"
	if tier == TierL3 {
		if trim {
			return pruneEdit{TrimInput: true}, true
		}
		if len(p.v.State.Output) < pruneSmall && len(p.v.State.Attachments) == 0 {
			return pruneEdit{}, false
		}
		return pruneEdit{Clear: true}, true
	}
	out := p.output()
	switch p.v.Tool {
	case "edit", "write", "patch", "apply_patch", "multiedit":
		return pruneEdit{TrimInput: true}, true
	case "read":
		f := p.filePath()
		if (lastRead[f] > n) || lastWrite[f] > n {
			return pruneEdit{Clear: true, Why: "lecturas superadas"}, true
		}
		if len(p.v.State.Attachments) > 0 {
			return pruneEdit{Clear: true, Why: "imágenes ya vistas"}, true
		}
		return pruneEdit{}, false
	case "bash":
		if lastCmd[strings.TrimSpace(resumen(p.v.State.Input))] > n && len(p.v.State.Output) >= pruneSmall {
			return pruneEdit{Clear: true, Why: "órdenes repetidas"}, true
		}
	case "todowrite":
		if lastTodo > n && len(p.v.State.Output) >= pruneSmall {
			return pruneEdit{Clear: true}, true
		}
		return pruneEdit{}, false
	}
	if len(p.v.State.Attachments) > 0 {
		return pruneEdit{Clear: true, Why: "imágenes ya vistas"}, true
	}
	if c, ok := cutLong(out); ok {
		return pruneEdit{Output: &c, Why: "salidas largas recortadas"}, true
	}
	return pruneEdit{}, false
}

// cutLong: una salida larga con solo sus primeras y últimas líneas.
func cutLong(s string) (string, bool) {
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(ls) <= pruneLongLines && len(s) <= pruneLongChars {
		return "", false
	}
	cabeza, cola := pruneKeepLines, pruneKeepLines
	if len(ls) < cabeza+cola+5 { // pocas líneas muy largas: por caracteres
		h, t := s[:pruneKeepChars], s[len(s)-pruneKeepChars:]
		return strings.ToValidUTF8(h, "") + truncatedNote(len(s)-2*pruneKeepChars) + strings.ToValidUTF8(t, ""), true
	}
	h := recortaBytes(strings.Join(ls[:cabeza], "\n"), pruneKeepChars, false)
	t := recortaBytes(strings.Join(ls[len(ls)-cola:], "\n"), pruneKeepChars, true)
	return h + truncatedNote(len(s)-len(h)-len(t)) + t, true
}

func recortaBytes(s string, n int, final bool) string {
	if len(s) <= n {
		return s
	}
	if final {
		return strings.ToValidUTF8(s[len(s)-n:], "")
	}
	return strings.ToValidUTF8(s[:n], "")
}

// pruneSmall: por debajo de esto (caracteres), la salida se deja: la línea que la sustituye ocuparía lo mismo.
const pruneSmall = 400

// renderPart: la parte con su cambio (sobre el original; el resto de sus campos, igual).
// Si no cambia nada, devuelve el original.
func renderPart(orig string, e pruneEdit) (string, error) {
	d := json.NewDecoder(strings.NewReader(orig))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		return "", err
	}
	st, _ := m["state"].(map[string]any)
	if st == nil {
		return "", fmt.Errorf("parte sin estado")
	}
	cambia := false
	if o, _ := st["output"].(string); e.Output != nil && o != *e.Output {
		st["output"], cambia = *e.Output, true
	}
	if e.Clear && st["time"] == nil {
		st["time"] = map[string]any{}
	}
	if tm, _ := st["time"].(map[string]any); e.Clear && tm != nil && tm["compacted"] == nil {
		// La marca de OpenCode; con un valor fijo (su fin) para que el resultado sea siempre el mismo.
		v := tm["end"]
		if v == nil {
			v = json.Number("1")
		}
		tm["compacted"], cambia = v, true
	}
	if in, _ := st["input"].(map[string]any); e.TrimInput && in != nil {
		for _, k := range []string{"oldString", "newString", "content", "patchText", "patch", "edits"} {
			v, ok := in[k]
			if !ok {
				continue
			}
			s, _ := v.(string)
			if k == "edits" {
				b, _ := json.Marshal(v)
				s = string(b)
			}
			if len(s) <= 200 {
				continue
			}
			in[k] = ClearedText
			cambia = true
		}
	}
	if !cambia {
		return orig, nil
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// pruneChanges: lo que hay que cambiar en la base de datos para llegar al plan desde lo que hay ahora: partes que
// cambiar (id → data), que reponer (vuelven a su original) y que quitar; y los tokens que salen (negativo si se
// repone más de lo que se quita).
type pruneDiff struct {
	Update  map[string]string
	Restore []string
	Delete  []string
	Saved   int
}

func pruneChanges(win []*ocMessage, plan prunePlan) pruneDiff {
	d := pruneDiff{Update: map[string]string{}}
	for _, m := range win {
		for _, p := range m.Parts {
			cur := p.Data
			if p.Current != "" {
				cur = p.Current
			}
			e, ok := plan.Edits[p.ID]
			switch {
			case ok && e.Delete:
				d.Delete = append(d.Delete, p.ID)
				d.Saved += tokensOf(cur)
			case ok:
				nuevo, err := renderPart(p.Data, e)
				if err != nil || nuevo == cur {
					continue
				}
				if nuevo == p.Data { // nada que quitar: si estaba cambiada, vuelve a su original
					d.Restore = append(d.Restore, p.ID)
					d.Saved += tokensOf(cur) - tokensOf(p.Data)
					continue
				}
				d.Update[p.ID] = nuevo
				d.Saved += tokensOf(cur) - tokensOf(nuevo)
			case p.Current != "":
				d.Restore = append(d.Restore, p.ID)
				d.Saved += tokensOf(cur) - tokensOf(p.Data)
			}
		}
	}
	return d
}

// worthIt: si se barre ahora: solo si lo que sale (descontado lo que se repone) compensa perder la caché. Reponer
// por sí solo no basta: cambiar el historial por la mitad hace que todo lo posterior se mande sin caché, y eso
// cuesta más que la relectura que haría el agente si lo necesita (medido en el banco, prueba 7 con OpenCode: un
// turno que solo reponía una pieza costó 0,073 $ frente a 0,015 $). Lo que tenga que volver, vuelve con el
// siguiente barrido que compense.
func (d pruneDiff) worthIt() bool {
	return d.Saved >= pruneMinTokens && (len(d.Update) > 0 || len(d.Delete) > 0 || len(d.Restore) > 0)
}

// cosine: similitud entre dos vectores (recortados a las mismas dimensiones).
func cosine(a, b []float32) float64 {
	n := min(len(a), len(b))
	var p, na, nb float64
	for i := 0; i < n; i++ {
		p += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return p / math.Sqrt(na*nb)
}

// shortMessage: un mensaje que no dice de qué va («compila y actualiza»): hereda el anterior.
func shortMessage(s string) bool { return len(strings.Fields(s)) < pruneShortWords }

// PruneReport: lo que ve la interfaz al empezar el turno (Evento.Prune).
type PruneReport struct {
	Removed   int             `json:"retirados"`           // tokens que salen del historial en este turno
	Restored  int             `json:"repuestos,omitempty"` // piezas que vuelven («fallo de página»)
	Exchanges []PruneExchange `json:"intercambios"`        // los intercambios con algo fuera del contexto
}

// PruneExchange: un intercambio con algo fuera del contexto del agente (por su punto: el último mensaje del agente).
type PruneExchange struct {
	Point      string  `json:"punto"`
	Tier       string  `json:"nivel"` // L2 (algo retirado) | L3 (fuera)
	Reason     string  `json:"motivo"`
	Similarity float64 `json:"similitud,omitempty"`
}

// report: los intercambios del plan con algo fuera.
func (p prunePlan) report() []PruneExchange {
	l := []PruneExchange{}
	for _, x := range p.Exchanges {
		if x.Reason == "" || x.Point == "" {
			continue
		}
		e := PruneExchange{Point: x.Point, Tier: string(x.Tier), Reason: x.Reason}
		if x.Tier == TierL3 {
			e.Similarity = math.Round(x.Sim*100) / 100
		}
		l = append(l, e)
	}
	return l
}
