package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Poda automática del historial del agente, decidida por pgocode (de momento, OpenCode).
//
// Los agentes reenvían todo su historial en cada paso. pgocode, que está por encima, sabe qué ya no importa y lo
// retira antes de cada turno, sin que el usuario marque nada ni el agente reciba instrucciones de más. Funciona como
// un recolector de basura: cada intercambio y cada pieza (lectura, salida, imagen, edición) tiene una vigencia
// (vigencia.go), un número ≥ 0 que sube con la antigüedad, el tiempo y la falta de relación con lo último, y salta
// si la pieza está superada. Los niveles de antes quedan como consecuencia:
//
//   - L1: el turno en curso y los dos últimos intercambios (vigencia 0 como intercambios: sus textos, razonamiento
//     y estructura no se tocan). Cada pieza suya tiene su propia vigencia (pasos del agente desde entonces, relación
//     con lo que se está haciendo ahora, tiempo, superada): las de los últimos pruneProtectSteps pasos no se tocan
//     nunca; las demás salen (solo su salida) con 3 o más, también a mitad de turno (midTurnPrune).
//   - L2: vigencia por debajo de 3. Sigue en el contexto, pero sin lo superado (que salta por encima de 3):
//     lecturas de un fichero que después se releyó o editó, imágenes ya vistas, salidas de órdenes repetidas, el
//     texto de las ediciones antiguas y el razonamiento; las salidas largas, con sus primeras y últimas líneas. Y
//     las lecturas o salidas grandes con vigencia propia (por su relación) de 3 o más (dos granos de embedding).
//   - L3: vigencia de 3 o más. Sus herramientas pasan a la marca nativa; los textos del usuario y del agente, que
//     son pequeños, se quedan. El original lo guarda pgocode: lo retirado no vuelve aunque su vigencia baje
//     (histéresis), solo si el agente o el usuario citan algo que solo estaba ahí, y como mucho pruneReviveMax tokens
//     por barrido (stickyOut): reponer de golpe reinflaba el contexto y el agente puede releer lo que necesite.
//
// Lo que se quita deja una línea que dice qué era y cómo recuperarlo: equivocarse cuesta una relectura, no un
// error. El marcado (embeddings y estado de cada intercambio) se hace en segundo plano según llega cada cosa
// (prune_opencode.go); el barrido (cambiar el historial del agente) al empezar el turno y, en OpenCode, entre pasos
// de un turno largo (midTurnPrune), solo si lo que sale compensa perder la caché (pruneMinTokens, midSweep). Lo
// anterior a la última compactación del agente sale como fuera («resumido por la compactación del agente»).

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
	// pruneThreshold: el umbral de la poda por niveles de antes de la vigencia (con él se calibra): similitud
	// mínima para seguir en el contexto, con vectores de 256 dimensiones (Matrioska de
	// embeddinggemma). Calibrado con la conversación de INTRADEMA: 0,30 con 768 dimensiones equivale a 0,38 con 256
	// (el 91 % de las decisiones coinciden; con 256 las similitudes salen de media 0,08 más altas).
	pruneThreshold = 0.38
	// pruneReviveMargin: lo que tiene que subir la relación de un intercambio de L3 para volver (evita que oscile de un
	// turno a otro y se pierda la caché cada vez).
	pruneReviveMargin = 0.04
	// pruneMinTokens: solo se barre si lo que sale pasa de esto; si no, no compensa perder la caché.
	pruneMinTokens = 15000
	// pruneShortWords: un mensaje más corto («compila y actualiza») hereda el anterior.
	pruneShortWords = 8
	// pruneYoung: intercambios de L1 (además del turno en curso).
	pruneYoung = 2
	// prunePieceTokens: tamaño a partir del que una pieza (lectura, salida) lleva su propio embedding. Las lecturas
	// de un trozo de fichero (100-250 líneas) rondan 1500-5000 caracteres: con 600 tokens (2400 caracteres), casi
	// todas llevan el suyo y su relación con lo que se está haciendo decide su vigencia dentro del turno.
	prunePieceTokens = 600
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
	Time             int64 // cuándo se creó (ms; 0 si no se sabe)
	Ctx              int   // del agente: el contexto de su llamada (tokens; 0 si no se sabe)
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

// tokensOf: lo que ocupa lo que se manda al modelo de una parte (estimado con estTokens: los datos numéricos, como un
// CSV, salen a 1,2-1,5 caracteres por token con Gemini; el código, a unos 2,5; el texto, a unos 4). Lo que ocupa de
// verdad en cada sesión se ajusta con el uso real de sus llamadas (estimateRatio).
func tokensOf(data string) int {
	var v partView
	if json.Unmarshal([]byte(data), &v) != nil {
		return len(data) / 4
	}
	switch v.Type {
	case "tool":
		if v.State.Time.Compacted != 0 {
			return estTokens(string(v.State.Input)) + 10
		}
		out := string(v.State.Output)
		var s string
		if json.Unmarshal(v.State.Output, &s) == nil {
			out = s
		}
		return estTokens(string(v.State.Input)) + estTokens(out) + pruneImageTokens*len(v.State.Attachments)
	case "text", "reasoning":
		return estTokens(v.Text)
	}
	return 0
}

// estTokens: los tokens de un texto, por clases de caracteres (medido con Gemini y Claude): cada dígito, casi uno
// (los tokenizadores parten los números); la puntuación, 0,7; las letras y los espacios, uno cada 3,5.
func estTokens(s string) int {
	var dig, pun, otros int
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			dig++
		case r < 128 && (r == ' ' || r == '\n' || r == '\t' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')):
			otros++
		case r < 128:
			pun++
		default:
			otros++ // letras acentuadas y demás
		}
	}
	return int(float64(dig)*0.95 + float64(pun)*0.7 + float64(otros)/3.5)
}

// pruneExchange: un intercambio (mensaje del usuario y la respuesta del agente, con sus pasos).
type pruneExchange struct {
	UserID, Point string // Point: el último mensaje del agente (el «punto» de pgocode)
	Messages      []*ocMessage
	Tier          pruneTier
	Sim           float64 // -1: sin similitud
	Tail          bool    // los pasos que conservó la compactación del agente (sin su mensaje del usuario)
	Vig           vigFactors
	V             float64 // vigencia
}

// time: cuándo acabó el intercambio (su último mensaje; 0 si no se sabe).
func (x *pruneExchange) time() int64 {
	for i := len(x.Messages) - 1; i >= 0; i-- {
		if t := x.Messages[i].Time; t > 0 {
			return t
		}
	}
	return 0
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

// focusSteps: de cuántos pasos del agente sale «lo que se está haciendo ahora» (focusText).
const focusSteps = 6

// focusText: lo que el agente está haciendo ahora, para la relación de cada pieza: su último texto o razonamiento
// y las entradas de las herramientas de sus últimos pasos (ficheros, órdenes, búsquedas). «» si no hay nada.
func focusText(win []*ocMessage) string {
	var texto string
	var ents []string
	pasos := 0
	for i := len(win) - 1; i >= 0 && pasos < focusSteps; i-- {
		m := win[i]
		if m.Role != "assistant" {
			if texto != "" || len(ents) > 0 {
				break // hasta el último mensaje del usuario
			}
			continue
		}
		pasos++
		for j := len(m.Parts) - 1; j >= 0; j-- {
			p := m.Parts[j]
			switch {
			case (p.v.Type == "text" || p.v.Type == "reasoning") && texto == "" && strings.TrimSpace(p.v.Text) != "":
				texto = recorta(strings.TrimSpace(p.v.Text), 1000)
			case p.v.Type == "tool":
				ents = append(ents, p.v.Tool+" "+recortaLinea(resumen(p.v.State.Input), 160))
			}
		}
	}
	if texto == "" && len(ents) == 0 {
		return ""
	}
	return texto + "\nHerramientas: " + strings.Join(ents, "; ")
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

// compactedBefore: lo que el agente ya no tiene por su compactación: los mensajes que no están en su ventana, sin
// los de la compactación ni los resúmenes.
func compactedBefore(msgs, win []*ocMessage) []*ocMessage {
	if len(win) == len(msgs) {
		return nil
	}
	dentro := make(map[string]bool, len(win))
	for _, m := range win {
		dentro[m.ID] = true
	}
	var b []*ocMessage
	for _, m := range msgs {
		if dentro[m.ID] || m.Summary || isCompactionMsg(m) {
			continue
		}
		b = append(b, m)
	}
	return b
}

func isCompactionMsg(m *ocMessage) bool {
	for _, p := range m.Parts {
		if p.v.Type == "compaction" {
			return true
		}
	}
	return false
}

// pruneExchanges: los intercambios de la ventana (cada uno empieza en un mensaje del usuario).
func pruneExchanges(win []*ocMessage) []*pruneExchange {
	var xs []*pruneExchange
	for _, m := range win {
		if m.Role == "user" {
			xs = append(xs, &pruneExchange{UserID: m.ID, Messages: []*ocMessage{m}, Tier: TierL2, Sim: -1})
			continue
		}
		if len(xs) == 0 {
			// Los últimos pasos de un intercambio que conservó la compactación del agente: un intercambio sin
			// mensaje del usuario.
			u := &ocMessage{ID: m.ID + "~", Role: "user"}
			xs = append(xs, &pruneExchange{UserID: u.ID, Messages: []*ocMessage{u}, Tier: TierL2, Sim: -1, Tail: true})
		}
		x := xs[len(xs)-1]
		x.Messages = append(x.Messages, m)
		x.Point = m.ID
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
	// Keep: se queda como está ahora (lo que pgocode ya retiró no vuelve sin una señal clara; ver stickyOut).
	Keep bool
	// Offload: Output es el principio y el final con la ruta de la salida completa (prune_offload.go); File: el
	// fichero en que hay que guardarla antes de aplicar el cambio («» si es una lectura: está en el propio fichero).
	Offload bool
	File    string
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
	Pieces    []*pieceVig
	K         vigParams // los pesos de la vigencia
}

// pieceVig: la vigencia de una pieza del plan (una herramienta).
type pieceVig struct {
	PartID, CallID string
	X              *pruneExchange
	F              vigFactors
	V              float64
	Note           string // «salida recortada»
	Out            bool   // fuera del contexto con el plan
}

// planPrune decide qué se quita (sin tocar nada) con la vigencia de cada intercambio y pieza (vigencia.go): con
// vigCut o más, fuera. young: intercambios jóvenes (los últimos, vigencia 0). now: el momento del barrido. Con los
// pesos generales; planPruneK, con los del proyecto.
func planPrune(win []*ocMessage, sims *pruneSims, young int, now time.Time) prunePlan {
	return planPruneK(win, sims, young, now, defaultVig)
}

// readRange: las líneas que cubre una lectura (desde, hasta; hasta 0: hasta el final).
func (p *ocPart) readRange() (int, int) {
	m := p.input()
	num := func(k string) int {
		if f, ok := m[k].(float64); ok {
			return int(f)
		}
		return 0
	}
	desde, n := max(num("offset"), 1), num("limit")
	if n <= 0 {
		return desde, 0
	}
	return desde, desde + n
}

// covers: si la lectura a cubre todas las líneas de la b.
func covers(a, b *ocPart) bool {
	ad, ah := a.readRange()
	bd, bh := b.readRange()
	return ad <= bd && (ah == 0 || (bh != 0 && ah >= bh))
}

// pruneHistory: lo que pasa después de cada parte (lecturas, ediciones, órdenes, listas de tareas), para saber si
// está superada.
type pruneHistory struct {
	reads     map[string][]int // fichero → posiciones de sus lecturas
	lastWrite map[string]int
	lastCmd   map[string]int
	lastTodo  int
	parts     []*ocPart // las partes en orden
}

func newPruneHistory(parts []*ocPart) *pruneHistory {
	h := &pruneHistory{reads: map[string][]int{}, lastWrite: map[string]int{}, lastCmd: map[string]int{}, lastTodo: -1, parts: parts}
	for n, p := range parts {
		if p.v.Type != "tool" {
			continue
		}
		switch p.v.Tool {
		case "read":
			h.reads[p.filePath()] = append(h.reads[p.filePath()], n)
		case "edit", "write", "patch", "apply_patch", "multiedit":
			h.lastWrite[p.filePath()] = n
		case "bash":
			h.lastCmd[strings.TrimSpace(resumen(p.v.State.Input))] = n
		case "todowrite":
			h.lastTodo = n
		}
	}
	return h
}

// rereadAfter: si el mismo fichero se vuelve a leer después de n, cubriendo lo que leyó la parte n.
func (h *pruneHistory) rereadAfter(n int) bool {
	p := h.parts[n]
	for _, m := range h.reads[p.filePath()] {
		if m > n && covers(h.parts[m], p) {
			return true
		}
	}
	return false
}

// pruneYoungPieces: la vigencia propia de cada pieza de los intercambios jóvenes y su antigüedad en pasos (false:
// como antes, para medir la diferencia en las pruebas).
var pruneYoungPieces = true

// planPruneK: planPrune con los pesos k. Los intercambios jóvenes (el turno en curso y los anteriores) cuentan
// enteros como vitales, pero cada pieza suya tiene su propia vigencia (pasos desde entonces, relación con lo que se
// está haciendo ahora, tiempo y si está superada), salvo las de los últimos pruneProtectSteps pasos, que no se
// tocan. De esas piezas solo se vacía la salida (o el texto de una edición): el razonamiento, los textos y la
// estructura del turno siguen igual (Gemini necesita sus firmas en el turno en curso).
func planPruneK(win []*ocMessage, sims *pruneSims, young int, now time.Time, k vigParams) prunePlan {
	return planPruneCut(win, sims, young, now, k, vigCut)
}

// pruneBudgetBig y pruneBudgetMed: con el contexto por encima del objetivo de la sesión (contextTarget), el corte de
// vigencia baja primero (2,5 y 2) para lo más grande y viejo, y si aún sigue por encima (1,5; 1; 0,5), también para
// las piezas medianas (lecturas, búsquedas, ediciones y salidas de órdenes de una sesión larga) para evitar llegar a
// la compactación del agente.
const (
	pruneBudgetBig = 2000
	pruneBudgetMed = 250
)

func pieceSize(p *ocPart) int {
	n := estTokens(p.output()) + pruneImageTokens*len(p.v.State.Attachments)
	if isEditTool(p.v.Tool) {
		n = max(n, estTokens(string(p.v.State.Input)))
	}
	return n
}

// budgetCut: el corte de una pieza con el corte del presupuesto (cut): el normal salvo para lo grande y viejo, o lo
// mediano cuando el contexto sigue por encima del objetivo con cut < 2.
func budgetCut(cut float64, size, steps int, young bool) float64 {
	if cut >= vigCut {
		return vigCut
	}
	if cut >= 2 {
		if size < pruneBudgetBig || (young && steps < 2*pruneProtectSteps) {
			return vigCut
		}
		return cut
	}
	if size < pruneBudgetMed || (young && steps < pruneProtectSteps+4) {
		return vigCut
	}
	return cut
}

// planPruneCut: planPruneK con otro corte para lo grande y viejo (vigCut: el normal; ver budgetCut).
func planPruneCut(win []*ocMessage, sims *pruneSims, young int, now time.Time, k vigParams, cut float64) prunePlan {
	xs := pruneExchanges(win)
	plan := prunePlan{Edits: map[string]pruneEdit{}, Exchanges: xs, K: k}
	// Las partes en orden, con su intercambio, mensaje y los pasos del agente (sus llamadas al modelo) hasta ella.
	type pos struct{ x, m, p, step int }
	var orden []pos
	var parts []*ocPart
	pasos := 0
	for i, x := range xs {
		for j, m := range x.Messages {
			if m.Role == "assistant" {
				pasos++
			}
			for k := range m.Parts {
				orden = append(orden, pos{i, j, k, pasos})
				parts = append(parts, m.Parts[k])
			}
		}
	}
	h := newPruneHistory(parts)
	// La vigencia de cada intercambio: fuera (L3) con vigCut o más. Lo que ya estaba fuera necesita bajar un poco
	// más para volver (pruneReviveMargin: que no oscile de un turno a otro y se pierda la caché cada vez); sin su
	// relación (Ollama no responde), sigue fuera (reponerlo rompería la caché).
	for i, x := range xs {
		x.Vig = vigFactors{Ago: len(xs) - i, Young: young, At: x.time()}
		switch {
		case i >= len(xs)-young:
			x.Tier, x.Vig.Pinned = TierL1, true
		case i == 0 && !x.Tail:
			x.Tier, x.Vig.Pinned = TierL2, true
		default:
			corte := vigCut
			if sims != nil {
				if s, ok := sims.Exchange[x.UserID]; ok {
					x.Sim, x.Vig.Sim = s, simPtr(s)
				}
				if sims.PrevTier[x.UserID] == TierL3 {
					if x.Vig.Sim != nil {
						corte -= k.RelK * pruneReviveMargin
					} else {
						corte = 0
					}
				}
			}
			if k.of(x.Vig, now) >= corte {
				x.Tier = TierL3
			}
		}
		x.V = k.of(x.Vig, now)
	}
	for n, q := range orden {
		x := xs[q.x]
		m := x.Messages[q.m]
		p := parts[n]
		since := pasos - q.step
		if !pruneYoungPieces {
			since = 0 // como antes: sin pasos; lo joven, entero
		}
		if x.Tier == TierL1 {
			if pv := youngPiece(p, n, x, m, since, young, sims, h, now, k, cut); pv != nil {
				plan.Pieces = append(plan.Pieces, pv)
				if e, ok := pv.edit(p); ok {
					plan.Edits[p.ID] = e
				}
			}
			continue
		}
		if p.v.Type == "reasoning" {
			if e, ok := pruneReasoning(m); ok {
				plan.Edits[p.ID] = e
			}
			continue
		}
		if !p.isTool() {
			continue
		}
		// La pieza: su fecha, sus pasos, su relación propia si es grande (si no, la del intercambio) y si está
		// superada.
		f := x.Vig
		f.Steps, f.Size = since, pieceSize(p)
		if m.Time > 0 {
			f.At = m.Time
		}
		if f.Pinned && x.Tier != TierL1 && f.Size >= vigBigTokens {
			// El primer intercambio (las instrucciones): su texto es vital, pero sus lecturas enormes (un CSV de 70k)
			// tienen su vigencia como las demás. Las pequeñas siguen fijas: soltarlas todas hacía releer más (en la
			// simulación con las sesiones reales, 4 relecturas de más en una sesión de 100 piezas).
			f.Pinned = false
		}
		propia := false
		if sims != nil && p.isPiece() {
			if s, ok := sims.Piece[p.ID]; ok {
				f.Sim, f.Pinned, propia = simPtr(s), false, true
			}
		}
		e, superada := pruneSuperseded(p, n, h, false)
		f.Superseded = superada
		pv := &pieceVig{PartID: p.ID, CallID: p.v.CallID, X: x, F: f}
		pv.V = max(k.of(f, now), x.V)
		pc := budgetCut(cut, f.Size, f.Steps, false)
		switch {
		case superada != "":
			pv.Out = e.Clear || e.TrimInput
		case cut < 0 || x.Tier == TierL3 || (propia && k.of(f, now) >= pc) || (!f.Pinned && f.Steps > pruneProtectSteps && k.of(f, now) >= pc):
			// (cut < 0: «Vaciar contexto»: todo fuera, sin proteger lo reciente ni el primer intercambio.)
			// Fuera: la salida, a la marca nativa; las ediciones, sin su texto; lo pequeño se queda (la línea que lo
			// sustituye ocuparía lo mismo).
			e = outEdit(p)
			pv.Out = e.Clear || e.TrimInput
		case p.v.Tool != "read" && p.v.Tool != "todowrite":
			// Sigue, pero las salidas largas, con solo sus primeras y últimas líneas.
			if c, ok := cutLong(p.output()); ok {
				e = pruneEdit{Output: &c}
				pv.Note = "salida recortada"
			}
		}
		if e.Clear || e.TrimInput || e.Output != nil || e.Delete {
			plan.Edits[p.ID] = e
		}
		plan.Pieces = append(plan.Pieces, pv)
	}
	return plan
}

// outEdit: lo que se hace con una pieza fuera: la salida, a la marca nativa; las ediciones, sin su texto; lo
// pequeño se queda (la línea que lo sustituye ocuparía lo mismo).
func outEdit(p *ocPart) pruneEdit {
	switch {
	case isEditTool(p.v.Tool):
		return pruneEdit{TrimInput: true}
	case len(p.v.State.Output) < pruneSmall && len(p.v.State.Attachments) == 0:
		return pruneEdit{}
	}
	return pruneEdit{Clear: true}
}

// youngPiece: la vigencia de una pieza de un intercambio joven (nil si no es una herramienta). Vital (0) si es de
// los últimos pruneProtectSteps pasos; si no, la suya: los pasos desde entonces, su relación con lo que se está
// haciendo ahora (si tiene embedding), el tiempo y si está superada (solo por lo que no deja dudas: el mismo trozo
// del fichero leído otra vez, la misma orden, otra lista de tareas o una imagen ya vista; no la lectura de un fichero
// que se editó después, que el agente puede seguir usando para la edición siguiente).
func youngPiece(p *ocPart, n int, x *pruneExchange, m *ocMessage, since, young int, sims *pruneSims, h *pruneHistory, now time.Time, k vigParams, cut float64) *pieceVig {
	if !p.isTool() {
		return nil
	}
	f := vigFactors{Ago: x.Vig.Ago, Young: young, At: m.Time, Steps: since, Size: pieceSize(p)}
	if since < pruneProtectSteps {
		f = vigFactors{Pinned: true, Steps: since}
		return &pieceVig{PartID: p.ID, CallID: p.v.CallID, X: x, F: f}
	}
	if sims != nil {
		if s, ok := sims.Piece[p.ID]; ok {
			f.Sim = simPtr(s)
		}
	}
	_, f.Superseded = pruneSuperseded(p, n, h, true)
	pv := &pieceVig{PartID: p.ID, CallID: p.v.CallID, X: x, F: f}
	pv.V = k.of(f, now)
	pv.Out = pv.V >= budgetCut(cut, f.Size, since, true)
	return pv
}

// edit: lo que se hace con una pieza joven fuera (solo vaciarla).
func (pv *pieceVig) edit(p *ocPart) (pruneEdit, bool) {
	if !pv.Out {
		return pruneEdit{}, false
	}
	e := outEdit(p)
	pv.Out = e.Clear || e.TrimInput
	return e, pv.Out
}

func isEditTool(t string) bool {
	return t == "edit" || t == "write" || t == "patch" || t == "apply_patch" || t == "multiedit"
}

// pruneReasoning: el razonamiento de turnos anteriores fuera, salvo si es lo único del mensaje (quedaría vacío y
// Gemini lo rechaza).
func pruneReasoning(m *ocMessage) (pruneEdit, bool) {
	for _, o := range m.Parts {
		if o.v.Type == "tool" || (o.v.Type == "text" && strings.TrimSpace(o.v.Text) != "") {
			return pruneEdit{Delete: true}, true
		}
	}
	return pruneEdit{}, false
}

// pruneSuperseded: si una herramienta está superada, por qué y qué se hace con ella: lecturas de un trozo de un
// fichero que después se volvió a leer (o, fuera de los jóvenes, que se editó), imágenes ya vistas, órdenes
// repetidas, listas de tareas sustituidas y (fuera de los jóvenes) el texto de las ediciones antiguas. young: la
// pieza es de un intercambio joven.
func pruneSuperseded(p *ocPart, n int, h *pruneHistory, young bool) (pruneEdit, string) {
	if isEditTool(p.v.Tool) {
		if young {
			return pruneEdit{}, ""
		}
		return pruneEdit{TrimInput: true}, "edición antigua"
	}
	switch p.v.Tool {
	case "read":
		if h.rereadAfter(n) {
			return pruneEdit{Clear: true}, "lectura superada"
		}
		if !young && h.lastWrite[p.filePath()] > n {
			return pruneEdit{Clear: true}, "lectura superada"
		}
	case "bash":
		if h.lastCmd[strings.TrimSpace(resumen(p.v.State.Input))] > n && len(p.v.State.Output) >= pruneSmall {
			return pruneEdit{Clear: true}, "orden repetida"
		}
	case "todowrite":
		if h.lastTodo > n && len(p.v.State.Output) >= pruneSmall {
			return pruneEdit{Clear: true}, "lista de tareas superada"
		}
		return pruneEdit{}, ""
	}
	if len(p.v.State.Attachments) > 0 {
		return pruneEdit{Clear: true}, "imagen ya vista"
	}
	return pruneEdit{}, ""
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
	Saved   int // lo que sale menos lo que vuelve
	// Out y Back: los tokens que salen y los que vuelven (los dos ≥ 0; Saved = Out − Back).
	Out, Back int
}

// add: lo que cambia una parte (positivo: sale; negativo: vuelve).
func (d *pruneDiff) add(n int) {
	d.Saved += n
	if n >= 0 {
		d.Out += n
	} else {
		d.Back -= n
	}
}

// pendingString: lo de un barrido que no se aplica aún («12000 por retirar · 3000 por reponer»).
func (d pruneDiff) pendingString() string {
	t := fmt.Sprintf("%d por retirar", d.Out)
	if d.Back > 0 || len(d.Restore) > 0 {
		t += fmt.Sprintf(" · %d por reponer", d.Back)
	}
	return t
}

// scale: los tokens con la relación entre lo estimado y lo real de la sesión (estimateRatio).
func (d *pruneDiff) scale(r float64) {
	d.Out, d.Back = int(float64(d.Out)*r), int(float64(d.Back)*r)
	d.Saved = d.Out - d.Back
}

// String: «12k fuera · 3k repuestos» (para el registro: nunca «-N tokens fuera»).
func (d pruneDiff) String() string {
	t := fmt.Sprintf("%d fuera", d.Out)
	if d.Back > 0 || len(d.Restore) > 0 {
		t += fmt.Sprintf(" · %d repuestos", d.Back)
	}
	return t
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
			case ok && e.Keep:
				continue
			case ok && e.Delete:
				d.Delete = append(d.Delete, p.ID)
				d.add(tokensOf(cur))
			case ok:
				nuevo, err := renderPart(p.Data, e)
				if err != nil || nuevo == cur {
					continue
				}
				if nuevo == p.Data { // nada que quitar: si estaba cambiada, vuelve a su original
					d.Restore = append(d.Restore, p.ID)
					d.add(tokensOf(cur) - tokensOf(p.Data))
					continue
				}
				d.Update[p.ID] = nuevo
				d.add(tokensOf(cur) - tokensOf(nuevo))
			case p.Current != "":
				d.Restore = append(d.Restore, p.ID)
				d.add(tokensOf(cur) - tokensOf(p.Data))
			}
		}
	}
	return d
}

// pruneReviveMax: lo que puede volver en un barrido (tokens). Reponer de golpe reinfla el contexto (en INTRADEMA,
// 24 piezas y 49k que lo subieron a 154k y lo hicieron compactar); el agente puede volver a leer lo que necesite, y
// una relectura cuesta solo esa pieza.
const pruneReviveMax = 5000

// stickyOut: lo que pgocode ya retiró se queda fuera aunque su vigencia vuelva a bajar (histéresis: sale con 3 o
// más y no vuelve por la vigencia), salvo con una señal fuerte: lo último del usuario o del agente (recent) cita dos
// o más identificadores que solo estaban en esa pieza; y aun así, como mucho pruneReviveMax tokens por barrido (las
// más citadas primero). Cambia el plan (Keep en lo que se queda fuera) y devuelve lo que vuelve.
func stickyOut(win []*ocMessage, plan *prunePlan, recent string) []string {
	type cand struct {
		p          *ocPart
		score, tok int
	}
	var cs []cand
	for _, m := range win {
		for _, p := range m.Parts {
			if p.Current == "" {
				continue
			}
			if e, ok := plan.Edits[p.ID]; ok {
				if e.Delete || e.Keep {
					continue
				}
				if nuevo, err := renderPart(p.Data, e); err == nil && tokensOf(nuevo) <= tokensOf(p.Current) {
					continue // sale igual o más
				}
			}
			cs = append(cs, cand{p: p, tok: max(tokensOf(p.Data)-tokensOf(p.Current), 0)})
		}
	}
	if len(cs) == 0 {
		return nil
	}
	citado := map[string]bool{}
	for _, w := range distinctiveWords(recent) {
		citado[w] = true
	}
	if len(citado) > 0 {
		// Lo que el agente tiene a la vista ahora (lo que no está retirado): ahí la cita no prueba nada.
		visto := map[string]bool{}
		for _, m := range win {
			for _, p := range m.Parts {
				if p.Current != "" && appliedOut(p) {
					continue
				}
				var t string
				switch p.v.Type {
				case "tool":
					t = string(p.v.State.Input) + "\n" + p.output()
				case "text", "reasoning":
					t = p.v.Text
				}
				for _, w := range distinctiveWords(t) {
					visto[w] = true
				}
			}
		}
		for i := range cs {
			uno := map[string]bool{}
			for _, w := range distinctiveWords(pieceContent(cs[i].p)) {
				if citado[w] && !visto[w] && !uno[w] {
					uno[w] = true
					cs[i].score++
				}
			}
		}
	}
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].score != cs[j].score {
			return cs[i].score > cs[j].score
		}
		return cs[i].tok < cs[j].tok
	})
	queda := pruneReviveMax
	var vuelve []string
	fuera := map[string]bool{}
	for _, c := range cs {
		if c.score >= 2 && c.tok <= queda {
			queda -= c.tok
			vuelve = append(vuelve, c.p.ID)
			continue
		}
		plan.Edits[c.p.ID] = pruneEdit{Keep: true}
		fuera[c.p.ID] = appliedOut(c.p)
	}
	for _, pv := range plan.Pieces {
		if o, ok := fuera[pv.PartID]; ok {
			pv.Out = o
		}
	}
	return vuelve
}

// recentText: lo último del usuario y del agente (para la señal de stickyOut).
func recentText(win []*ocMessage, msg string) string {
	return msg + "\n" + focusText(win)
}

// worthIt: si se barre ahora: solo si lo que sale (descontado lo que se repone) compensa perder la caché. Reponer
// por sí solo no basta: cambiar el historial por la mitad hace que todo lo posterior se mande sin caché, y eso
// cuesta más que la relectura que haría el agente si lo necesita (medido en el banco, prueba 7 con OpenCode: un
// turno que solo reponía una pieza costó 0,073 $ frente a 0,015 $). Lo que tenga que volver, vuelve con el
// siguiente barrido que compense.
// changes: si hay algo que cambiar.
func (d pruneDiff) changes() bool {
	return len(d.Update) > 0 || len(d.Delete) > 0 || len(d.Restore) > 0
}

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

// PruneReport: lo que ve la interfaz al empezar el turno (Evento.Prune) y, en segundo plano, al volver a calcular
// la vigencia (VigenciaReport): la de cada intercambio y pieza.
type PruneReport struct {
	Removed   int             `json:"retirados"`           // tokens que salen del historial en este turno
	Restored  int             `json:"repuestos,omitempty"` // piezas que vuelven («fallo de página»)
	Exchanges []PruneExchange `json:"intercambios"`        // todos los intercambios (por su punto)
	Pieces    []PrunePiece    `json:"piezas"`              // las herramientas de los intercambios
	// Podable: el historial del agente se puede podar (con Claude Code, si no, la interfaz recurre al relevo).
	Podable bool `json:"podable,omitempty"`
	// Pending: tokens (estimados) de lo que está fuera de vigencia y sigue en el contexto del agente (Antigravity:
	// lo que decide el relevo).
	Pending int `json:"pendiente,omitempty"`
	// Status: el estado del recolector (el indicador del chat).
	Status *CollectorStatus `json:"estado,omitempty"`
	// Barrer a mano: con la caché caliente, lo que dejaría de estar en ella desde el primer cambio (tokens); si el
	// barrido forzado espera al próximo turno (Claude Code con la sesión abierta).
	CacheWarm    bool `json:"cache_caliente,omitempty"`
	CacheSuffix  int  `json:"cache_resto,omitempty"`
	ForcePending bool `json:"forzado_pendiente,omitempty"`
	// NoPays: con la caché caliente, barrer ahora cuesta más de lo que ahorra (la interfaz pide confirmación).
	NoPays bool `json:"no_compensa,omitempty"`
	// Back: tokens que volverían con el próximo barrido (piezas retiradas que el agente ha vuelto a citar).
	Back int `json:"por_reponer,omitempty"`
	// Threshold: el umbral al que compacta el agente; Target: el objetivo de contexto de la sesión (por encima, el
	// hilo retira también lo grande y viejo con menos vigencia); Context: el contexto real de la última llamada
	// (tras un barrido, lo que queda). Tokens; 0 si no se saben.
	Threshold int `json:"umbral,omitempty"`
	Target    int `json:"objetivo,omitempty"`
	Context   int `json:"contexto,omitempty"`
	// Calibration: los pesos de la vigencia del proyecto y su calibración.
	Calibration *CalibrationInfo `json:"calibracion,omitempty"`
}

// PruneExchange: la vigencia de un intercambio (por su punto: el último mensaje del agente).
type PruneExchange struct {
	Point string `json:"punto"`
	Vigencia
}
