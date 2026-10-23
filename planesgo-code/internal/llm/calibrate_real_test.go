package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
