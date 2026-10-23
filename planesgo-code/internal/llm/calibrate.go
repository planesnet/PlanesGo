package llm

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Simulación de la poda sobre un historial real y etiquetas automáticas de cada pieza (para medir y calibrar la
// vigencia sin que nadie marque nada).
//
// El historial se recorre como lo vivió el agente: en cada punto en que pgocode barrería (al empezar cada turno y
// cada pruneMidSteps pasos dentro de él) se calcula la vigencia de lo que tenía en su ventana (con sus
// compactaciones), y lo que saldría se da por retirado desde ese paso (si el barrido compensa). Las etiquetas salen
// de lo que hizo el agente después:
//   - USADA en el paso u: el agente vuelve a leer ese trozo del fichero (relectura), edita el fichero leído, o cita en
//     su texto o en las entradas de sus herramientas identificadores, rutas o líneas que solo estaban en esa pieza
//     (dos o más). Cada uso es de la última pieza que tenía eso (la copia más reciente es la que mira el agente).
//   - NO USADA: nada de eso mientras sigue en su contexto (hasta su compactación).
//   - PODADA DE MÁS: retirada en el paso t y lo primero que hace después con ella el agente (antes de que salga
//     del contexto) no es leerla otra vez, sino usarla: sin la pieza tendría que haberla releído. Si lo primero es
//     una relectura que el agente hizo de todos modos, retirarla no costó nada. Si pgocode ya la había retirado de
//     verdad, cualquier relectura posterior cuenta (puede ser por la poda).

// calibUse: un uso de una pieza: en qué paso y si fue volver a leerla.
type calibUse struct {
	Step   int
	Reread bool
}

// calibPiece: una pieza del historial con sus usos.
type calibPiece struct {
	ID   string
	Step int  // el paso del agente en que llegó
	End  int  // el paso en que salió del contexto por una compactación del agente (o el final + 1)
	Real bool // pgocode ya la había retirado de verdad
	Uses []calibUse
	Seg  int // el tramo (entre compactaciones del agente) en que llegó
}

// calibCand: una pieza que podría salir en un punto de barrido, con lo que su vigencia necesita.
type calibCand struct {
	P      *calibPiece
	Tokens int // lo que sale si sale
	F, XF  vigFactors
	H, XH  float64 // horas desde la pieza y desde su intercambio
	Young  bool
	Propia bool
}

// out: si sale con los pesos k (lo mismo que decide planPruneK, sin el margen para volver de lo ya retirado).
func (c *calibCand) out(k vigParams) bool { return c.outCut(k, vigCut) }

// outCut: out con el corte del objetivo de contexto para lo grande y viejo (planPruneCut).
func (c *calibCand) outCut(k vigParams, cut float64) bool {
	pv := k.raw(c.F, c.H)
	pc := budgetCut(cut, c.F.Size, c.F.Steps, c.Young)
	if c.Young {
		return pv >= pc
	}
	if pv >= pc && (c.F.Superseded != "" || (!c.F.Pinned && (c.Propia || c.F.Steps > pruneProtectSteps))) {
		return true
	}
	return k.raw(c.XF, c.XH) >= vigCut
}

// calibPoint: un punto de barrido.
type calibPoint struct {
	Step  int
	Mid   bool // entre pasos de un turno (si no, al empezarlo)
	Ctx   int  // el contexto de la última llamada (tokens; 0 si no se sabe)
	Sys   int  // lo fijo del agente (systemTokens)
	Cold  bool // al empezar el turno, con la caché ya fría (más de openCodeCacheTTL desde la llamada anterior)
	Seg   int
	Cands []calibCand
}

// calibHistory: un historial preparado para simular.
type calibHistory struct {
	Pieces []*calibPiece
	Points []calibPoint
	Steps  int // pasos del agente en total
	// Ctx: el contexto real de cada paso (tokens) y su tramo.
	Ctx    map[int]int
	SegOf  map[int]int
	NSegs  int
	byPart map[string]*calibPiece
}

// buildHistory: prepara un historial (todos sus mensajes, como los guarda el agente) para simular la poda: sus
// piezas con sus usos y, en cada punto de barrido, lo que podría salir. vec: los embeddings (nil: solo reglas).
func buildHistory(msgs []*ocMessage, vec vecFunc) *calibHistory {
	h := &calibHistory{Ctx: map[int]int{}, SegOf: map[int]int{}, byPart: map[string]*calibPiece{}}
	h.labelUses(msgs)
	// Los puntos de barrido: al empezar cada turno y cada pruneMidSteps pasos dentro de él.
	step, enTurno, turnMsg := 0, 0, ""
	mid := max(pruneMidSteps, 4)
	for i, m := range msgs {
		if m.Role == "user" {
			if m.Summary || isCompactionMsg(m) {
				continue
			}
			if t := strings.TrimSpace(quitaIdioma(userTextOf(m))); t != "" && t != textoContinuacion && !strings.HasPrefix(t, "Has llegado al límite de pasos") {
				turnMsg = t
			}
			if turnMsg != "" && i > 0 {
				n := len(h.Points)
				h.addPoint(msgs[:i], turnMsg, pruneYoung, step, false, vec)
				if len(h.Points) > n && m.Time > 0 {
					h.Points[n].Cold = m.Time-lastCallUntil(msgs[:i], 0) > openCodeCacheTTL.Milliseconds()
				}
			}
			enTurno = 0
			continue
		}
		step++
		enTurno++
		if enTurno%mid == 0 && turnMsg != "" {
			h.addPoint(msgs[:i+1], turnMsg, pruneYoung+1, step, true, vec)
		}
	}
	return h
}

// userTextOf: el texto de un mensaje del usuario (sin los sintéticos).
func userTextOf(m *ocMessage) string {
	var b strings.Builder
	for _, p := range m.Parts {
		if p.v.Type == "text" && !p.v.Synthetic {
			b.WriteString(p.v.Text)
		}
	}
	return b.String()
}

func (h *calibHistory) addPoint(prefix []*ocMessage, msg string, young, step int, mid bool, vec vecFunc) {
	win := pruneWindow(prefix)
	if len(win) == 0 {
		return
	}
	var sims *pruneSims
	if vec != nil {
		sims = computeSims(win, msg, vec)
	}
	var now time.Time
	ctx := 0
	for i := len(prefix) - 1; i >= 0; i-- {
		if now.IsZero() && prefix[i].Time > 0 {
			now = time.UnixMilli(prefix[i].Time)
		}
		if ctx == 0 && prefix[i].Role == "assistant" && prefix[i].Ctx > 0 {
			ctx = prefix[i].Ctx
		}
		if !now.IsZero() && ctx > 0 {
			break
		}
	}
	plan := planPruneK(win, sims, young, now, defaultVig)
	pt := calibPoint{Step: step, Mid: mid, Ctx: ctx, Sys: systemTokens(win), Seg: h.SegOf[step]}
	parts := map[string]*ocPart{}
	for _, m := range win {
		for _, p := range m.Parts {
			parts[p.ID] = p
		}
	}
	for _, pv := range plan.Pieces {
		cp := h.byPart[pv.PartID]
		if cp == nil || (pv.F.Pinned && pv.F.Superseded == "" && pv.X.Tier == TierL1) {
			continue
		}
		p := parts[pv.PartID]
		young := pv.X.Tier == TierL1
		e := outEdit(p)
		if pe, ok := plan.Edits[p.ID]; ok && !young && pv.F.Superseded != "" {
			e = pe // lo superado: lo que decide su regla
		}
		if !e.Clear && !e.TrimInput {
			continue
		}
		cur, err := renderPart(p.Data, e)
		if err != nil {
			continue
		}
		tok := tokensOf(p.Data) - tokensOf(cur)
		if tok <= 0 {
			continue
		}
		propia := false
		if sims != nil && !young && p.isPiece() {
			_, propia = sims.Piece[p.ID]
		}
		pt.Cands = append(pt.Cands, calibCand{P: cp, Tokens: tok, F: pv.F, XF: pv.X.Vig, H: vigHours(pv.F.At, now),
			XH: vigHours(pv.X.Vig.At, now), Young: young, Propia: propia})
	}
	h.Points = append(h.Points, pt)
}

// ---- etiquetas ----

// distinctiveWord: palabras que identifican algo (rutas, identificadores con _ . / o mayúsculas por dentro,
// números de línea con su contexto no: solo identificadores).
var reWord = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:[./-][A-Za-z0-9_]+)*`)

func distinctiveWords(s string) []string {
	var out []string
	for _, w := range reWord.FindAllString(s, -1) {
		if len(w) < 6 {
			continue
		}
		dist := strings.ContainsAny(w, "_./")
		for i, r := range w {
			if i > 0 && (unicode.IsUpper(r) || unicode.IsDigit(r)) {
				dist = true
				break
			}
		}
		if dist {
			out = append(out, w)
		}
	}
	return out
}

// pieceContent: lo que la pieza le da al agente (la salida; de una edición, su texto nuevo).
func pieceContent(p *ocPart) string {
	if isEditTool(p.v.Tool) {
		in := p.input()
		var b strings.Builder
		for _, k := range []string{"newString", "new_string", "content"} {
			if s, ok := in[k].(string); ok {
				b.WriteString(s)
			}
		}
		return b.String()
	}
	return p.output()
}

// labelUses: las piezas de todo el historial con sus usos, los pasos y tramos, y cuándo sale cada una del
// contexto por una compactación del agente.
func (h *calibHistory) labelUses(msgs []*ocMessage) {
	// Las compactaciones: en qué paso y desde qué mensaje conservan la cola.
	type comp struct {
		step int
		tail string
		idx  int
	}
	var comps []comp
	step := 0
	stepOfMsg := make([]int, len(msgs))
	for i, m := range msgs {
		if m.Role == "assistant" {
			step++
		}
		stepOfMsg[i] = step
		if m.Role == "user" {
			for _, p := range m.Parts {
				if p.v.Type == "compaction" {
					comps = append(comps, comp{step: step, tail: p.v.TailStartID, idx: i})
				}
			}
		}
	}
	h.Steps = step
	seg := 0
	ci := 0
	seen := map[string]bool{}          // palabras que el agente ya tiene en sus textos o entradas
	owner := map[string]*calibPiece{}  // palabra → la última pieza que la trae
	lastRead := map[string][]*ocPart{} // fichero → sus lecturas (las partes)
	pieceOf := map[*ocPart]*calibPiece{}
	for i, m := range msgs {
		for ci < len(comps) && comps[ci].idx <= i {
			seg++
			ci++
		}
		s := stepOfMsg[i]
		if m.Role == "assistant" {
			h.SegOf[s] = seg
			if m.Ctx > 0 {
				h.Ctx[s] = m.Ctx
			}
		}
		// Lo que el agente escribe en este paso (sus textos y las entradas de sus herramientas): los usos.
		if m.Role == "assistant" && !m.Summary {
			var dicho strings.Builder
			for _, p := range m.Parts {
				switch {
				case p.v.Type == "text" || p.v.Type == "reasoning":
					dicho.WriteString(p.v.Text + "\n")
				case p.v.Type == "tool":
					dicho.Write(p.v.State.Input)
					dicho.WriteString("\n")
				}
			}
			cuenta := map[*calibPiece]int{}
			for _, w := range distinctiveWords(dicho.String()) {
				if cp := owner[w]; cp != nil {
					cuenta[cp]++
					delete(owner, w) // desde ahora lo tiene en su propio texto
				}
				seen[w] = true
			}
			for cp, n := range cuenta {
				if n >= 2 {
					cp.Uses = append(cp.Uses, calibUse{Step: s})
				}
			}
			// Relecturas y ediciones de un fichero leído.
			for _, p := range m.Parts {
				if p.v.Type != "tool" {
					continue
				}
				f := p.filePath()
				if f == "" {
					continue
				}
				switch {
				case p.v.Tool == "read":
					for _, r := range lastRead[f] {
						a1, a2 := r.readRange()
						b1, b2 := p.readRange()
						if (a2 == 0 || b1 < a2) && (b2 == 0 || a1 < b2) { // se solapan
							if cp := pieceOf[r]; cp != nil {
								cp.Uses = append(cp.Uses, calibUse{Step: s, Reread: true})
							}
						}
					}
				case isEditTool(p.v.Tool):
					if l := lastRead[f]; len(l) > 0 {
						if cp := pieceOf[l[len(l)-1]]; cp != nil {
							cp.Uses = append(cp.Uses, calibUse{Step: s})
						}
					}
				}
			}
		}
		// Las piezas que llegan en este paso.
		for _, p := range m.Parts {
			if p.v.Type != "tool" || p.v.State.Status != "completed" {
				continue
			}
			cp := &calibPiece{ID: p.ID, Step: s, End: h.Steps + 1, Seg: seg, Real: p.Current != "" && appliedOut(p)}
			for _, c := range comps {
				if c.idx > i && (c.tail == "" || m.ID < c.tail) {
					cp.End = c.step + 1
					break
				}
			}
			h.Pieces = append(h.Pieces, cp)
			h.byPart[p.ID] = cp
			pieceOf[p] = cp
			for _, w := range distinctiveWords(pieceContent(p)) {
				if !seen[w] {
					owner[w] = cp
				}
			}
			if p.v.Tool == "read" {
				if f := p.filePath(); f != "" {
					lastRead[f] = append(lastRead[f], p)
				}
			}
		}
	}
	h.NSegs = seg + 1
	// Los usos, solo mientras la pieza sigue en el contexto.
	for _, cp := range h.Pieces {
		var us []calibUse
		for _, u := range cp.Uses {
			if u.Step > cp.Step && u.Step < cp.End {
				us = append(us, u)
			}
		}
		cp.Uses = us
	}
}

// ---- simulación ----

// calibResult: lo que habría hecho la poda con unos pesos.
type calibResult struct {
	Cleared    int   // piezas retiradas
	Tokens     int   // tokens retirados
	TokenSteps int64 // tokens·paso fuera del contexto (lo que se deja de reenviar)
	Used       int   // retiradas que el agente usa después (mientras seguirían en su contexto)
	Over       int   // podadas de más (habría tenido que releerlas)
	Labeled    int   // piezas del historial
	UsedTotal  int   // piezas usadas después de llegar (etiqueta USADA)
	// El contexto de trabajo (tokens por paso, real y con la poda simulada): la media y la mediana.
	MeanReal, MeanSim, MedReal, MedSim int
	// Por tramo: tokens retirados y el contexto máximo real y simulado.
	Segs []calibSeg
	At   map[*calibPiece]int // en qué paso salió cada pieza
}

// calibSeg: un tramo entre compactaciones del agente.
type calibSeg struct {
	Removed       int
	MaxReal       int
	MaxSim        int
	ReachReal     int // primer paso del tramo con el contexto ≥ el umbral (0: no llega)
	ReachSim      int
	First, Last   int // primer y último paso
	OutAtLastStep int // tokens fuera en el último paso del tramo
}

// overPruned: si retirar la pieza en el paso t obliga a releerla.
func (cp *calibPiece) overPruned(t int) bool {
	for _, u := range cp.Uses {
		if u.Step <= t {
			continue
		}
		return !u.Reread || cp.Real
	}
	return false
}

func (cp *calibPiece) usedAfter(t int) bool {
	for _, u := range cp.Uses {
		if u.Step > t {
			return true
		}
	}
	return false
}

// simulate: la poda con los pesos k en cada punto de barrido (con la economía de cada uno: al empezar el turno,
// pruneMinTokens; entre pasos, además, el contexto por encima de la mitad del umbral). threshold: el contexto al
// que compacta el agente (para el tramo: en qué paso habría llegado).
func (h *calibHistory) simulate(k vigParams, threshold int) calibResult {
	r := h.sweepSim(k, threshold)
	h.segments(&r, threshold)
	return r
}

// simTargetPct: el objetivo de contexto en la simulación, en % del umbral (en las pruebas, 0 para comparar sin él).
var simTargetPct = pruneTargetPct

// sweepSim: lo que sale en cada punto de barrido con los pesos k (sin el detalle por tramo).
func (h *calibHistory) sweepSim(k vigParams, threshold int) calibResult {
	r := calibResult{At: map[*calibPiece]int{}, Labeled: len(h.Pieces)}
	for _, cp := range h.Pieces {
		if len(cp.Uses) > 0 {
			r.UsedTotal++
		}
	}
	target := threshold * simTargetPct / 100
	for _, pt := range h.Points {
		fuera := 0
		for i := range pt.Cands {
			if _, ya := r.At[pt.Cands[i].P]; ya {
				fuera += pt.Cands[i].Tokens
			}
		}
		sel := func(cut float64) (int, []*calibCand) {
			nuevo := 0
			var sale []*calibCand
			for i := range pt.Cands {
				c := &pt.Cands[i]
				if _, ya := r.At[c.P]; !ya && c.outCut(k, cut) {
					nuevo += c.Tokens
					sale = append(sale, c)
				}
			}
			return nuevo, sale
		}
		nuevo, sale := sel(vigCut)
		ctx := max(pt.Ctx-fuera, 0)
		if pt.Ctx > 0 {
			ctx = max(ctx, 1) // conocido (lo estimado fuera puede pasar del real): no es «sin contexto»
		}
		if target > 0 && pt.Ctx > 0 && ctx > target {
			for _, cut := range []float64{2.5, 2} {
				if ctx-nuevo <= target {
					break
				}
				nuevo, sale = sel(cut)
			}
		}
		if len(sale) == 0 {
			continue
		}
		o := sweepOutcome{Ctx: ctx, System: pt.Sys, D: pruneDiff{Saved: nuevo, Out: nuevo, Delete: []string{""}}}
		if pt.Ctx > 0 {
			o.Rewrite = max(ctx-pt.Sys-nuevo, 0)
		}
		if !o.pays(pt.Cold, pt.Mid, threshold, target) {
			continue
		}
		for _, c := range sale {
			r.At[c.P] = pt.Step
			r.Cleared++
			r.Tokens += c.Tokens
			r.TokenSteps += int64(c.Tokens) * int64(max(min(c.P.End, h.Steps+1)-pt.Step-1, 0))
			if c.P.usedAfter(pt.Step) {
				r.Used++
			}
			if c.P.overPruned(pt.Step) {
				r.Over++
			}
		}
	}
	return r
}

// segments: por tramo, el contexto real y el que habría quedado.
func (h *calibHistory) segments(r *calibResult, threshold int) {
	tok := map[*calibPiece]int{}
	for _, pt := range h.Points {
		for _, c := range pt.Cands {
			if s, ok := r.At[c.P]; ok && s == pt.Step {
				tok[c.P] = c.Tokens
			}
		}
	}
	r.Segs = make([]calibSeg, h.NSegs)
	var reales, sims []int
	defer func() { r.MeanReal, r.MedReal = meanMedian(reales); r.MeanSim, r.MedSim = meanMedian(sims) }()
	for s := 1; s <= h.Steps; s++ {
		g := &r.Segs[h.SegOf[s]]
		if g.First == 0 {
			g.First = s
		}
		g.Last = s
		ctx, ok := h.Ctx[s]
		if !ok {
			continue
		}
		out := 0
		for cp, at := range r.At {
			if at < s && cp.End > s {
				out += tok[cp]
			}
		}
		sim := max(ctx-out, 0)
		reales, sims = append(reales, ctx), append(sims, sim)
		g.MaxReal, g.MaxSim, g.OutAtLastStep = max(g.MaxReal, ctx), max(g.MaxSim, sim), out
		if threshold > 0 && ctx >= threshold && g.ReachReal == 0 {
			g.ReachReal = s
		}
		if threshold > 0 && sim >= threshold && g.ReachSim == 0 {
			g.ReachSim = s
		}
	}
	for cp, at := range r.At {
		r.Segs[h.SegOf[max(at, 1)]].Removed += tok[cp]
	}
}

// meanMedian: la media y la mediana.
func meanMedian(v []int) (int, int) {
	if len(v) == 0 {
		return 0, 0
	}
	s := append([]int(nil), v...)
	sort.Ints(s)
	t := 0
	for _, x := range s {
		t += x
	}
	return t / len(s), s[len(s)/2]
}

// ---- búsqueda de los pesos ----

// calibMaxOver: lo que se tolera de podadas de más: el 2 % de las piezas retiradas. Releer cuesta un paso y la
// pieza entera otra vez (sin caché), y el agente puede equivocarse mientras tanto; retirar de menos solo cuesta
// reenviarla a precio de caché. Por eso es una restricción, no un término que se compense con lo ahorrado.
const calibMaxOver = 0.02

// calibGrid: los valores que se prueban de cada peso (búsqueda por coordenadas: uno cada vez, con los demás fijos,
// hasta que ninguno mejora).
var calibGrid = [][]float64{
	{0, 0.06, 0.12, 0.2, 0.3, 0.5},               // antigüedad (intercambios)
	{0, 0.2, 0.4, 0.6, 0.8, 1, 1.3, 1.6, 2, 2.5}, // pasos
	{0, 0.075, 0.15, 0.3, 0.5},                   // tiempo
	{5, 10, 15, 20, 25, 30, 40},                  // relación
	{0.35, 0.4, 0.45, 0.5, 0.55, 0.6, 0.65},      // pivote
}

func (k *vigParams) coord(i int) *float64 {
	return [...]*float64{&k.AgeK, &k.StepK, &k.TimeK, &k.RelK, &k.RelPivot}[i]
}

// calibTotal: los resultados de varios historiales juntos.
type calibTotal struct {
	Cleared, Tokens, Over, Used, Labeled int
	TokenSteps                           int64
}

func simulateAll(hs []*calibHistory, k vigParams) calibTotal {
	var t calibTotal
	for _, h := range hs {
		r := h.sweepSim(k, CompactAtDefault)
		t.Cleared += r.Cleared
		t.Tokens += r.Tokens
		t.Over += r.Over
		t.Used += r.Used
		t.Labeled += r.Labeled
		t.TokenSteps += r.TokenSteps
	}
	return t
}

// excess: las podadas de más por encima de lo tolerado (0 si cumple).
func (t calibTotal) excess() float64 {
	return max(float64(t.Over)-calibMaxOver*float64(t.Cleared), 0)
}

// better: si a es mejor que b: primero cumplir (o pasarse menos), después más tokens·paso fuera.
func (a calibTotal) better(b calibTotal) bool {
	if ea, eb := a.excess(), b.excess(); ea != eb {
		return ea < eb
	}
	return a.TokenSteps > b.TokenSteps
}

// searchParams: los pesos que más retiran (tokens·paso fuera del contexto) con las podadas de más por debajo del
// 2 %, empezando por base.
func searchParams(hs []*calibHistory, base vigParams) (vigParams, calibTotal) {
	// Primero, la rejilla entera de los tres que más pesan (pasos, relación y pivote), con los demás como en base:
	// desde unos pesos que se pasan de podadas de más, cambiar uno solo no suele bastar.
	type sem struct {
		k vigParams
		t calibTotal
	}
	sems := []sem{{base, simulateAll(hs, base)}}
	for _, st := range calibGrid[1] {
		for _, rk := range calibGrid[3] {
			for _, pv := range calibGrid[4] {
				k := base
				k.StepK, k.RelK, k.RelPivot = st, rk, pv
				sems = append(sems, sem{k, simulateAll(hs, k)})
			}
		}
	}
	sort.SliceStable(sems, func(i, j int) bool { return sems[i].t.better(sems[j].t) })
	best, bestT := sems[0].k, sems[0].t
	// Después, por coordenadas desde las tres mejores.
	for _, s := range sems[:min(3, len(sems))] {
		if k, t := descend(hs, s.k, s.t); t.better(bestT) {
			best, bestT = k, t
		}
	}
	return best, bestT
}

// descend: búsqueda por coordenadas desde k.
func descend(hs []*calibHistory, best vigParams, bestT calibTotal) (vigParams, calibTotal) {
	for pasada := 0; pasada < 4; pasada++ {
		mejora := false
		for i := range calibGrid {
			for _, v := range calibGrid[i] {
				k := best
				*k.coord(i) = v
				if k == best {
					continue
				}
				if t := simulateAll(hs, k); t.better(bestT) {
					best, bestT, mejora = k, t, true
				}
			}
		}
		if !mejora {
			break
		}
	}
	return best, bestT
}
