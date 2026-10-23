package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// loadDump: una sesión de OpenCode volcada a JSON con las mismas columnas que loadOpenCodeSession (mid, role,
// summary, parent, mt, ctx, pid, data, cur), leída en solo lectura de su base de datos.
func loadDump(t *testing.T, path string) []*ocMessage {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := parseSessionRows(b)
	if err != nil {
		t.Fatal(err)
	}
	return msgs
}

// dumpVec: los embeddings (Ollama local) con caché en una carpeta (los de cada sesión, en su fichero).
func dumpVec(t *testing.T, ses string) (vecFunc, func()) {
	dir := os.Getenv("PGO_PRUEBA_SIM_CACHE")
	if dir == "" || os.Getenv("PGO_PRUEBA_SIM_REGLAS") != "" {
		return nil, func() {}
	}
	drainCollectors()
	viejo := pruneDirFunc
	pruneDirFunc = func() string { return dir }
	c := &pruneCollector{key: ses}
	c.load()
	ctx := context.Background()
	return func(k, tx, task string) []float32 { return c.vector(ctx, k, tx, task) }, func() { c.save(); pruneDirFunc = viejo }
}

// TestPruneSimulationReal: la poda simulada sobre sesiones reales (volcadas en solo lectura): por tramo entre
// compactaciones del agente, lo que habría retirado y en qué paso habría llegado el contexto al umbral, y de lo
// retirado, cuánto se vuelve a usar y cuánto habría que releer.
// PGO_PRUEBA_SIM=<a.json,b.json> PGO_PRUEBA_SIM_CACHE=<carpeta> go test -run TestPruneSimulationReal -v ./internal/llm
func TestPruneSimulationReal(t *testing.T) {
	paths := os.Getenv("PGO_PRUEBA_SIM")
	if paths == "" {
		t.Skip()
	}
	for _, path := range strings.Split(paths, ",") {
		ses := strings.TrimSuffix(filepath.Base(path), ".json")
		msgs := loadDump(t, path)
		vec, fin := dumpVec(t, ses)
		t0 := time.Now()
		pruneYoungPieces = false
		antes := buildHistory(msgs, vec)
		pruneYoungPieces = true
		h := buildHistory(msgs, vec)
		fin()
		logResult(t, "antes (lo joven, entero)", antes.simulate(defaultVig, CompactAtDefault))
		t.Logf("%s: %d mensajes, %d pasos, %d piezas, %d puntos de barrido (%s)", ses, len(msgs), h.Steps, len(h.Pieces), len(h.Points), time.Since(t0).Round(time.Millisecond))
		simTargetPct = 0
		logResult(t, "generales sin objetivo de contexto", h.simulate(defaultVig, CompactAtDefault))
		simTargetPct = pruneTargetPct
		if n, err := strconv.Atoi(os.Getenv("PGO_PRUEBA_SIM_OBJETIVO")); err == nil { // % del umbral, para probar otros
			simTargetPct = n
			defer func() { simTargetPct = pruneTargetPct }()
		}
		r := h.simulate(defaultVig, CompactAtDefault)
		logResult(t, "generales", r)
		if kj := os.Getenv("PGO_PRUEBA_SIM_K"); kj != "" {
			var k vigParams
			if err := json.Unmarshal([]byte(kj), &k); err != nil {
				t.Fatal(err)
			}
			logResult(t, "calibrados", h.simulate(k, CompactAtDefault))
		}
		if os.Getenv("PGO_PRUEBA_SIM_DETALLE") != "" {
			parts := map[string]*ocPart{}
			for _, m := range msgs {
				for _, p := range m.Parts {
					parts[p.ID] = p
				}
			}
			for cp, at := range r.At {
				if cp.usedAfter(at) {
					p := parts[cp.ID]
					t.Logf("    usada tras retirarla: %s %s (paso %d, fuera en %d, usos %+v, de más %v)", p.v.Tool, recortaLinea(resumen(p.v.State.Input), 90), cp.Step, at, cp.Uses, cp.overPruned(at))
				}
			}
		}
	}
}

func logResult(t *testing.T, nombre string, r calibResult) {
	t.Logf("  %s: %d piezas retiradas (%d tokens, %d k tokens·paso); de ellas usadas después %d, podadas de más %d (%.1f %%); piezas usadas en el historial %d/%d",
		nombre, r.Cleared, r.Tokens, r.TokenSteps/1000, r.Used, r.Over, pctOf(r.Over, r.Cleared), r.UsedTotal, r.Labeled)
	t.Logf("    contexto de trabajo por paso: real media %d mediana %d → con la poda media %d mediana %d", r.MeanReal, r.MedReal, r.MeanSim, r.MedSim)
	for i, g := range r.Segs {
		if g.MaxReal == 0 {
			continue
		}
		t.Logf("    tramo %d (pasos %d-%d): retirados %6d · contexto máx. real %6d → %6d · umbral: real en el paso %s, con la poda %s",
			i+1, g.First, g.Last, g.Removed, g.MaxReal, g.MaxSim, stepOrNo(g.ReachReal), stepOrNo(g.ReachSim))
	}
}

func pctOf(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

func stepOrNo(s int) string {
	if s == 0 {
		return "no llega"
	}
	return itoa(s)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// TestCalibrationReal: la calibración de los pesos con sesiones reales (por proyecto y todas juntas).
// PGO_PRUEBA_CAL=<a.json,b.json;c.json> (cada grupo separado por «;» es un proyecto) PGO_PRUEBA_SIM_CACHE=<carpeta>
func TestCalibrationReal(t *testing.T) {
	grupos := os.Getenv("PGO_PRUEBA_CAL")
	if grupos == "" {
		t.Skip()
	}
	var todas []*calibHistory
	for _, g := range strings.Split(grupos, ";") {
		var hs []*calibHistory
		for _, path := range strings.Split(g, ",") {
			ses := strings.TrimSuffix(filepath.Base(path), ".json")
			vec, fin := dumpVec(t, ses)
			var msgs []*ocMessage
			if strings.HasPrefix(path, "claude:") { // una sesión de Claude Code (solo se lee)
				ses = strings.TrimPrefix(path, "claude:")
				fin()
				vec, fin = dumpVec(t, ses)
				var err error
				if msgs, err = loadClaudeHistory(ses); err != nil {
					t.Fatal(err)
				}
			} else {
				msgs = loadDump(t, path)
			}
			hs = append(hs, buildHistory(msgs, vec))
			fin()
		}
		todas = append(todas, hs...)
		base := simulateAll(hs, defaultVig)
		k, r := searchParams(hs, defaultVig)
		t.Logf("proyecto %s", g)
		logTotal(t, "  generales", defaultVig, base)
		logTotal(t, "  calibrados", k, r)
	}
	k, r := searchParams(todas, defaultVig)
	logTotal(t, "todos juntos: generales", defaultVig, simulateAll(todas, defaultVig))
	logTotal(t, "todos juntos: calibrados", k, r)
}

func logTotal(t *testing.T, nombre string, k vigParams, r calibTotal) {
	t.Logf("%s %+v: %d retiradas, %d tokens, %d k tokens·paso, usadas después %d, podadas de más %d (%.1f %%), piezas %d",
		nombre, k, r.Cleared, r.Tokens, r.TokenSteps/1000, r.Used, r.Over, pctOf(r.Over, r.Cleared), r.Labeled)
}

// TestStickyOutReal: el barrido sobre una sesión real (volcada en solo lectura) en un momento dado, con y sin la
// histéresis de lo retirado (stickyOut): cuánto volvería de golpe. El caso de INTRADEMA: a las 11:41:26 volvieron
// 24 piezas (~49k), el contexto subió a 154k y OpenCode compactó.
// PGO_PRUEBA_REPOS=<sesión.json> PGO_PRUEBA_REPOS_EN=<ms> PGO_PRUEBA_SIM_CACHE=<carpeta> go test -run TestStickyOutReal -v ./internal/llm
func TestStickyOutReal(t *testing.T) {
	path := os.Getenv("PGO_PRUEBA_REPOS")
	if path == "" {
		t.Skip()
	}
	en, _ := strconv.ParseInt(os.Getenv("PGO_PRUEBA_REPOS_EN"), 10, 64)
	ses := strings.TrimSuffix(filepath.Base(path), ".json")
	msgs := loadDump(t, path)
	var pre []*ocMessage
	for _, m := range msgs {
		if en == 0 || m.Time <= en {
			pre = append(pre, m)
		}
	}
	vec, fin := dumpVec(t, ses)
	defer fin()
	win := pruneWindow(pre)
	// PGO_PRUEBA_REPOS_ORIG: las partes con original guardado por pgocode (pgocode_prune_original): las que hoy
	// están repuestas se dan por retiradas, como antes de la reposición de golpe.
	if f := os.Getenv("PGO_PRUEBA_REPOS_ORIG"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		ids := map[string]bool{}
		for _, id := range strings.Fields(string(b)) {
			ids[id] = true
		}
		for _, m := range win {
			for _, p := range m.Parts {
				if ids[p.ID] && p.Current == "" && p.isTool() {
					p.Current, _ = renderPart(p.Data, outEdit(p))
					if p.Current == p.Data {
						p.Current = ""
					}
				}
			}
		}
	}
	msg := ""
	for i := len(win) - 1; i >= 0; i-- {
		if win[i].Role == "user" {
			if msg = strings.TrimSpace(quitaIdioma(userTextOf(win[i]))); msg != "" {
				break
			}
		}
	}
	var sims *pruneSims
	if vec != nil {
		sims = computeSims(win, msg, vec)
	}
	now := time.UnixMilli(win[len(win)-1].Time)
	retiradas := 0
	for _, m := range win {
		for _, p := range m.Parts {
			if p.Current != "" {
				retiradas++
			}
		}
	}
	plan := planPruneK(win, sims, pruneYoung+1, now, defaultVig)
	sin := pruneChanges(win, plan)
	o := decideSweep(sweepInput{Win: win, Sims: sims, Young: pruneYoung + 1, Now: now, K: defaultVig, Msg: msg, Target: CompactAtDefault * pruneTargetPct / 100})
	t.Logf("%d mensajes hasta %s, %d partes retiradas por pgocode; contexto %d (sistema %d, relación estimado/real %.2f)",
		len(pre), now.Format("15:04:05"), retiradas, o.Ctx, o.System, o.Ratio)
	t.Logf("  sin histéresis: %d repuestas (%d tokens estimados), %s", len(sin.Restore), sin.Back, sin)
	t.Logf("  con histéresis: %d repuestas, %s (corte %.1f, reenvío sin caché %d)", len(o.D.Restore), o.D, o.Cut, o.Rewrite)
	if o.D.Back > pruneReviveMax*2 {
		t.Errorf("vuelve demasiado: %d", o.D.Back)
	}
}
