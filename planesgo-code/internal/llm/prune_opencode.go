package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/depuracion"
	"planesgo-code/internal/selector"
)

// Poda en OpenCode: el recolector de cada sesión del agente (prune.go explica los niveles).
//
// Un recolector por historial: la clave es la sesión del agente (la principal de OpenCode o, más adelante, la hija
// de cada subagente, cuya referencia sería la tarea que le encargaron y sus últimos pasos). Cada uno tiene su
// propio estado (vectores, niveles, lo último que se aplicó) y su propio hilo, sin compartir nada con los demás:
//   - marcado continuo: cada salida de herramienta grande se embebe en cuanto llega (notePiece) y, al acabar el
//     turno, cada intercambio nuevo (mark); los vectores se guardan en ~/.local/share/pgocode/poda/<sesión>.json;
//   - barrido (sweep) solo al empezar el turno: con lo ya marcado, basta embeber el mensaje nuevo y comparar;
//     después se cambia la base de datos de OpenCode («opencode db», como reparaOpenCode).
//
// OpenCode manda al modelo, de cada herramienta acabada, su entrada (state.input) y su salida (state.output, o
// «[Old tool result content cleared]» si él mismo la vació) con los adjuntos (state.attachments: imágenes); del
// razonamiento, su texto con la firma de Gemini (metadata); y relee el historial de la base de datos en cada paso.
// Por eso basta cambiar esos campos de las partes (o quitar la parte de razonamiento) antes de lanzar el turno.
// El original de cada parte cambiada se guarda en la tabla pgocode_prune_original de la misma base de datos:
// «opencode db» solo ejecuta una sentencia por llamada y la línea de órdenes no admite argumentos de más de
// 128 KB, así que reponer una lectura grande desde fuera no cabría; con la tabla, reponer es una sentencia corta.

const pruneTable = "pgocode_prune_original"

var (
	idParteOC      = regexp.MustCompile(`^prt_[A-Za-z0-9]+$`)
	pruneArgMax    = 100 << 10 // tamaño máximo de una sentencia (el límite de un argumento es 128 KB)
	pruneDirFunc   = func() string { return filepath.Join(datosOpenCode(), "poda") }
	pruneMidSteps  = 0 // barrido entre pasos de un turno largo cada N pasos (0: nunca; ver midTurnWorth)
	tablaPreparada sync.Map
)

// ocDB ejecuta una sentencia con «opencode db» en la carpeta de datos de la sesión (en las pruebas, se sustituye).
var ocDB = ocDBRun

func ocDBRun(ctx context.Context, a *Agente, p Peticion, q string, asJSON bool) ([]byte, error) {
	args := []string{"db", q}
	if asJSON {
		args = append(args, "--format", "json")
	}
	cmd := exec.CommandContext(ctx, "opencode", args...)
	cmd.Dir = p.Carpeta
	cmd.Env = append(append(os.Environ(), a.EntornoDefecto...), "PWD="+p.Carpeta)
	if a.EntornoPara != nil {
		cmd.Env = append(cmd.Env, a.EntornoPara(p)...)
	}
	// La salida, a un fichero: por una tubería, OpenCode (bun) acaba antes de escribirla entera si es grande (se
	// cortaba en unos 4 MB).
	f, err := os.CreateTemp("", "pgocode-poda-*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	var errOut strings.Builder
	cmd.Stdout, cmd.Stderr = f, &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%v: %s", err, strings.TrimSpace(errOut.String()))
	}
	return os.ReadFile(f.Name())
}

// preparaTabla crea la tabla de originales (una vez por carpeta de datos).
func preparaTabla(ctx context.Context, a *Agente, p Peticion) error {
	clave := datosOpenCode()
	if _, ok := tablaPreparada.Load(clave); ok {
		return nil
	}
	_, err := ocDB(ctx, a, p, "CREATE TABLE IF NOT EXISTS "+pruneTable+" (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, data TEXT NOT NULL)", false)
	if err == nil {
		tablaPreparada.Store(clave, true)
	}
	return err
}

// loadOpenCodeSession lee la sesión: sus mensajes con sus partes, con el original de las que cambió pgocode.
func loadOpenCodeSession(ctx context.Context, a *Agente, p Peticion, ses string) ([]*ocMessage, error) {
	if !idSesionOC.MatchString(ses) {
		return nil, errors.New("sesión no válida")
	}
	if err := preparaTabla(ctx, a, p); err != nil {
		return nil, err
	}
	q := `SELECT m.id AS mid, json_extract(m.data,'$.role') AS role, CASE WHEN json_type(m.data,'$.summary')='true' THEN 1 ELSE 0 END AS summary, ` +
		`coalesce(json_extract(m.data,'$.parentID'),'') AS parent, coalesce(p.id,'') AS pid, coalesce(o.data,p.data,'') AS data, ` +
		`CASE WHEN o.data IS NULL OR o.data=p.data THEN '' ELSE p.data END AS cur ` +
		`FROM message m LEFT JOIN part p ON p.message_id=m.id LEFT JOIN ` + pruneTable + ` o ON o.id=p.id ` +
		`WHERE m.session_id='` + ses + `' ORDER BY m.time_created, m.id, p.id`
	out, err := ocDB(ctx, a, p, q, true)
	if err != nil {
		return nil, err
	}
	return parseSessionRows(out)
}

func parseSessionRows(out []byte) ([]*ocMessage, error) {
	var filas []struct {
		Mid, Role, Parent, Pid, Data, Cur string
		Summary                           any
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(out, &filas); err != nil {
		return nil, err
	}
	var msgs []*ocMessage
	for _, f := range filas {
		if len(msgs) == 0 || msgs[len(msgs)-1].ID != f.Mid {
			s := false
			switch v := f.Summary.(type) {
			case float64:
				s = v != 0
			case bool:
				s = v
			}
			msgs = append(msgs, &ocMessage{ID: f.Mid, Role: f.Role, Parent: f.Parent, Summary: s})
		}
		if f.Pid != "" {
			m := msgs[len(msgs)-1]
			m.Parts = append(m.Parts, newPart(f.Pid, f.Mid, f.Data, f.Cur))
		}
	}
	return msgs, nil
}

// sqlText: un literal de texto de SQLite.
func sqlText(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func sqlIDs(ids []string) string {
	l := make([]string, len(ids))
	for i, id := range ids {
		l[i] = "'" + id + "'"
	}
	return "(" + strings.Join(l, ",") + ")"
}

// pruneStatements: las sentencias que aplican el cambio (guardar originales, cambiar, reponer y quitar), cada una
// por debajo de pruneArgMax (las grandes, por tandas).
func pruneStatements(d pruneDiff) ([]string, error) {
	var tocadas []string
	for id := range d.Update {
		tocadas = append(tocadas, id)
	}
	sort.Strings(tocadas)
	tocadas = append(tocadas, d.Delete...)
	for _, id := range append(append([]string(nil), tocadas...), d.Restore...) {
		if !idParteOC.MatchString(id) {
			return nil, fmt.Errorf("parte no válida: %q", id)
		}
	}
	var qs []string
	porTandas := func(ids []string, cabe func([]string) string) {
		for i := 0; i < len(ids); {
			j := i + 1
			for j < len(ids) && len(cabe(ids[i:j+1])) < pruneArgMax {
				j++
			}
			qs = append(qs, cabe(ids[i:j]))
			i = j
		}
	}
	porTandas(tocadas, func(ids []string) string {
		return "INSERT OR IGNORE INTO " + pruneTable + " (id, session_id, data) SELECT id, session_id, data FROM part WHERE id IN " + sqlIDs(ids)
	})
	upd := tocadas[:len(tocadas)-len(d.Delete)]
	porTandas(upd, func(ids []string) string {
		var b strings.Builder
		b.WriteString("UPDATE part SET data = CASE id")
		for _, id := range ids {
			b.WriteString(" WHEN '" + id + "' THEN " + sqlText(d.Update[id]))
		}
		b.WriteString(" ELSE data END WHERE id IN " + sqlIDs(ids))
		return b.String()
	})
	porTandas(d.Restore, func(ids []string) string {
		return "UPDATE part SET data = (SELECT o.data FROM " + pruneTable + " o WHERE o.id = part.id) WHERE id IN " + sqlIDs(ids) +
			" AND id IN (SELECT id FROM " + pruneTable + ")"
	})
	porTandas(d.Delete, func(ids []string) string { return "DELETE FROM part WHERE id IN " + sqlIDs(ids) })
	return qs, nil
}

// ---- recolector de cada sesión ----

// pruneState: lo que el recolector guarda de su sesión (en ~/.local/share/pgocode/poda/<sesión>.json).
type pruneState struct {
	// Vectores (256 dimensiones): «x:<hash del texto>» de los intercambios y las consultas, «p:<callID>» de las piezas.
	Vectors map[string][]float32 `json:"vectores"`
	// Tiers: el nivel de cada intercambio (por su mensaje del usuario) en el último barrido aplicado.
	Tiers map[string]pruneTier `json:"niveles"`
	// Applied: lo que ve la interfaz (lo que está fuera ahora en el historial del agente).
	Applied []PruneExchange `json:"aplicado"`
}

type pruneCollector struct {
	key   string // la sesión del agente
	jobs  chan func()
	state pruneState
	// snapshot: la sesión leída al acabar el último turno (el barrido la usa si sigue valiendo).
	snapshot []*ocMessage
}

var (
	muCollectors sync.Mutex
	collectors   = map[string]*pruneCollector{}
)

// collectorFor: el recolector de una sesión del agente (con su hilo; se crea la primera vez).
func collectorFor(key string) *pruneCollector {
	muCollectors.Lock()
	defer muCollectors.Unlock()
	if c := collectors[key]; c != nil {
		return c
	}
	c := &pruneCollector{key: key, jobs: make(chan func(), 256)}
	c.load()
	collectors[key] = c
	go c.loop()
	return c
}

// loop: el hilo del recolector. Hace los trabajos en orden; tras un rato sin nada, suelta la sesión leída.
func (c *pruneCollector) loop() {
	t := time.NewTimer(10 * time.Minute)
	for {
		select {
		case f := <-c.jobs:
			f()
			t.Reset(10 * time.Minute)
		case <-t.C:
			c.snapshot = nil
		}
	}
}

// do: un trabajo en el hilo del recolector, esperando a que acabe (o a ctx).
func (c *pruneCollector) do(ctx context.Context, f func()) error {
	hecho := make(chan struct{})
	select {
	case c.jobs <- func() { f(); close(hecho) }:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-hecho:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// later: un trabajo en el hilo del recolector, sin esperar (si la cola está llena, se pierde: ya se hará al marcar).
func (c *pruneCollector) later(f func()) {
	select {
	case c.jobs <- f:
	default:
	}
}

func (c *pruneCollector) file() string { return filepath.Join(pruneDirFunc(), c.key+".json") }

func (c *pruneCollector) load() {
	c.state = pruneState{}
	if b, err := os.ReadFile(c.file()); err == nil {
		json.Unmarshal(b, &c.state)
	}
	if c.state.Vectors == nil {
		c.state.Vectors = map[string][]float32{}
	}
	if c.state.Tiers == nil {
		c.state.Tiers = map[string]pruneTier{}
	}
}

func (c *pruneCollector) save() {
	if err := os.MkdirAll(pruneDirFunc(), 0o700); err != nil {
		return
	}
	b, _ := json.Marshal(c.state)
	tmp := c.file() + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, c.file())
	}
}

// ---- embeddings (Ollama local, 256 dimensiones) ----

var (
	muOllamaPoda   sync.Mutex
	ollamaCaidoHas time.Time
	clientePoda    = &http.Client{Timeout: 20 * time.Second}
	// pruneEmbed calcula un vector (en las pruebas se sustituye).
	pruneEmbed = func(ctx context.Context, texto, tarea string) ([]float32, error) {
		muOllamaPoda.Lock()
		caido := time.Now().Before(ollamaCaidoHas)
		muOllamaPoda.Unlock()
		if caido {
			return nil, errors.New("ollama no responde")
		}
		v, err := selector.Ollama(ctx, clientePoda, texto, tarea)
		if err != nil {
			muOllamaPoda.Lock()
			ollamaCaidoHas = time.Now().Add(30 * time.Second)
			muOllamaPoda.Unlock()
			return nil, err
		}
		if len(v) > pruneDims {
			v = v[:pruneDims]
		}
		return v, nil
	}
)

func hashKey(prefijo, texto string) string {
	h := sha256.Sum256([]byte(texto))
	return prefijo + hex.EncodeToString(h[:12])
}

// vector: el de la clave, de la caché o calculándolo (nil si Ollama no responde).
func (c *pruneCollector) vector(ctx context.Context, clave, texto, tarea string) []float32 {
	if v, ok := c.state.Vectors[clave]; ok {
		return v
	}
	v, err := pruneEmbed(ctx, texto, tarea)
	if err != nil {
		return nil
	}
	c.state.Vectors[clave] = v
	return v
}

// notePiece: una herramienta acaba de terminar en el turno (marcado continuo): si su salida es grande, se embebe ya.
func (c *pruneCollector) notePiece(callID, tool, res, out string) {
	if callID == "" || len(out)/4 < prunePieceTokens || tool == "edit" || tool == "write" {
		return
	}
	texto := pieceEmbedText(tool, res, out)
	c.later(func() {
		ctx, cancela := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancela()
		c.vector(ctx, "p:"+callID, texto, "RETRIEVAL_DOCUMENT")
	})
}

// mark: al acabar un turno, lee la sesión y embebe lo que falte (intercambios y piezas grandes). Así el barrido del
// turno siguiente solo tiene que embeber el mensaje nuevo.
func (c *pruneCollector) mark(ctx context.Context, a *Agente, p Peticion, embeddings bool) {
	msgs, err := loadOpenCodeSession(ctx, a, p, c.key)
	if err != nil {
		depuracion.Log(a.Clave, "poda: leer la sesión %s: %v", c.key, err)
		c.snapshot = nil
		return
	}
	c.snapshot = msgs
	if embeddings {
		c.embedAll(ctx, pruneWindow(msgs))
	}
	c.save()
}

func (c *pruneCollector) embedAll(ctx context.Context, win []*ocMessage) {
	for _, x := range pruneExchanges(win) {
		if c.vector(ctx, hashKey("x:", x.embedText()), x.embedText(), "RETRIEVAL_DOCUMENT") == nil {
			return // sin Ollama: solo reglas
		}
		for _, m := range x.Messages {
			for _, pt := range m.Parts {
				if pt.isPiece() && pt.v.CallID != "" {
					c.vector(ctx, "p:"+pt.v.CallID, pt.pieceText(), "RETRIEVAL_DOCUMENT")
				}
			}
		}
	}
}

// sims: las similitudes de cada intercambio y pieza frente al mensaje nuevo (si es corto, también frente al
// anterior del usuario: se queda la mayor). Nil si no hay embeddings.
func (c *pruneCollector) sims(ctx context.Context, win []*ocMessage, msg string) *pruneSims {
	xs := pruneExchanges(win)
	msg = strings.TrimSpace(quitaIdioma(msg))
	q := c.vector(ctx, hashKey("q:", msg), msg, "RETRIEVAL_QUERY")
	if q == nil {
		return nil
	}
	qs := [][]float32{q}
	if shortMessage(msg) && len(xs) > 0 {
		prev := strings.TrimSpace(xs[len(xs)-1].userText())
		if v := c.vector(ctx, hashKey("q:", prev), prev, "RETRIEVAL_QUERY"); v != nil {
			qs = append(qs, v)
		}
	}
	mejor := func(v []float32) float64 {
		s := -1.0
		for _, q := range qs {
			s = max(s, cosine(q, v))
		}
		return s
	}
	r := &pruneSims{Exchange: map[string]float64{}, Piece: map[string]float64{}, PrevTier: c.state.Tiers}
	for _, x := range xs {
		v := c.vector(ctx, hashKey("x:", x.embedText()), x.embedText(), "RETRIEVAL_DOCUMENT")
		if v == nil {
			return nil
		}
		r.Exchange[x.UserID] = mejor(v)
		for _, m := range x.Messages {
			for _, pt := range m.Parts {
				if pt.isPiece() && pt.v.CallID != "" {
					if v := c.vector(ctx, "p:"+pt.v.CallID, pt.pieceText(), "RETRIEVAL_DOCUMENT"); v != nil {
						r.Piece[pt.ID] = mejor(v)
					}
				}
			}
		}
	}
	return r
}

// sweep: el barrido al empezar el turno. Decide con lo marcado y, si compensa, cambia el historial del agente.
// young: intercambios de L1 (2 al empezar el turno; 3 entre pasos, para no tocar el turno en curso).
func (c *pruneCollector) sweep(ctx context.Context, a *Agente, p Peticion, msg string, young int) (*PruneReport, error) {
	msgs := c.snapshot
	c.snapshot = nil // tras el barrido (o el turno) ya no vale
	if msgs == nil {
		var err error
		if msgs, err = loadOpenCodeSession(ctx, a, p, c.key); err != nil {
			return nil, err
		}
	}
	win := pruneWindow(msgs)
	var s *pruneSims
	if p.Prune == PruneAuto {
		ce, cancela := context.WithTimeout(ctx, 15*time.Second)
		s = c.sims(ce, win, msg)
		cancela()
		if s == nil && len(c.state.Tiers) > 0 {
			s = &pruneSims{PrevTier: c.state.Tiers}
		}
	}
	plan := planPrune(win, s, young)
	d := pruneChanges(win, plan)
	rep := &PruneReport{Exchanges: c.state.Applied}
	if !d.worthIt() {
		depuracion.Log(a.Clave, "poda %s: %d tokens pendientes (no compensa aún)", c.key, d.Saved)
		if rep.Exchanges == nil {
			rep.Exchanges = []PruneExchange{}
		}
		return rep, nil
	}
	qs, err := pruneStatements(d)
	if err != nil {
		return nil, err
	}
	for _, q := range qs {
		if _, err := ocDB(ctx, a, p, q, false); err != nil {
			return nil, err
		}
	}
	tiers := map[string]pruneTier{}
	for _, x := range plan.Exchanges {
		if x.Tier == TierL3 {
			tiers[x.UserID] = TierL3
		}
	}
	c.state.Tiers, c.state.Applied = tiers, plan.report()
	c.save()
	depuracion.Log(a.Clave, "poda %s: %d tokens fuera (%d partes cambiadas, %d repuestas, %d quitadas)", c.key, d.Saved,
		len(d.Update), len(d.Restore), len(d.Delete))
	return &PruneReport{Removed: max(d.Saved, 0), Restored: len(d.Restore), Exchanges: c.state.Applied}, nil
}

// pruneOpenCode: el barrido al empezar el turno de OpenCode (en el hilo del recolector de su sesión).
func pruneOpenCode(ctx context.Context, a *Agente, p Peticion) *PruneReport {
	if p.Prune == PruneOff || p.Aparte || !idSesionOC.MatchString(p.SesionAgente) || len(p.Mensajes) == 0 {
		return nil
	}
	c := collectorFor(p.SesionAgente)
	var rep *PruneReport
	ce, cancela := context.WithTimeout(ctx, 60*time.Second)
	defer cancela()
	err := c.do(ce, func() {
		var err error
		if rep, err = c.sweep(ce, a, p, p.Mensajes[len(p.Mensajes)-1].Texto, pruneYoung); err != nil {
			depuracion.Log(a.Clave, "poda %s: %v", p.SesionAgente, err)
		}
	})
	if err != nil {
		depuracion.Log(a.Clave, "poda %s: %v", p.SesionAgente, err)
	}
	return rep
}

// markOpenCode: al acabar el turno, el marcado de lo nuevo (en segundo plano).
func markOpenCode(a *Agente, p Peticion, ses string) {
	if p.Prune == PruneOff || p.Aparte || !idSesionOC.MatchString(ses) {
		return
	}
	c := collectorFor(ses)
	c.later(func() {
		ctx, cancela := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancela()
		c.mark(ctx, a, p, p.Prune == PruneAuto)
	})
}

// forgetSnapshot: la sesión ha cambiado fuera del turno (corte, reparación): el barrido tiene que releerla.
func forgetSnapshot(ses string) {
	muCollectors.Lock()
	c := collectors[ses]
	muCollectors.Unlock()
	if c != nil {
		c.later(func() { c.snapshot = nil })
	}
}

// midTurnWorth: si compensa barrer entre pasos de un turno largo (preparado, desactivado: pruneMidSteps = 0).
// OpenCode relee el historial de la base de datos en cada paso, así que un barrido entre pasos surte efecto en el
// siguiente. Con Gemini, escribir en caché no se cobra, pero desde el primer cambio el historial deja de estar en
// caché una vez: se paga ese resto a precio de entrada (1) en vez de caché (cacheRatio, 0,1 en Gemini) en un
// paso, y a cambio cada paso siguiente manda «removed» tokens menos (a precio de caché). Compensa si
// removed·cacheRatio·pasosRestantes > resto·(1−cacheRatio).
func midTurnWorth(removed, suffix, stepsLeft int, cacheRatio float64) bool {
	return float64(removed)*cacheRatio*float64(stepsLeft) > float64(suffix)*(1-cacheRatio)
}

func init() {
	if n, err := strconv.Atoi(os.Getenv("PGOCODE_PRUNE_STEPS")); err == nil && n > 0 {
		pruneMidSteps = n
	}
}

// midTurnPrune: el barrido entre pasos de un turno muy largo, cada pruneMidSteps pasos (desactivado por defecto:
// PGOCODE_PRUNE_STEPS=N lo activa). No espera: lo hace el hilo del recolector mientras el agente sigue; el turno en
// curso y los dos intercambios anteriores (L1) no se tocan.
func midTurnPrune(a *Agente, p Peticion, ses string, pasos int) {
	if pruneMidSteps <= 0 || pasos%pruneMidSteps != 0 || p.Prune == PruneOff || p.Aparte || !idSesionOC.MatchString(ses) || len(p.Mensajes) == 0 {
		return
	}
	c := collectorFor(ses)
	q := p
	q.SesionAgente = ses
	c.later(func() {
		ctx, cancela := context.WithTimeout(context.Background(), time.Minute)
		defer cancela()
		if _, err := c.sweep(ctx, a, q, p.Mensajes[len(p.Mensajes)-1].Texto, pruneYoung+1); err != nil {
			depuracion.Log(a.Clave, "poda entre pasos %s: %v", ses, err)
		}
	})
}
