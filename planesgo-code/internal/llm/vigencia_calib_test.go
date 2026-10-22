package llm

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"
	"time"
)

// loadSessionDump: una sesión de OpenCode volcada a JSON (filas con mid, mt, role, pid y pdata), como la de INTRADEMA.
func loadSessionDump(t *testing.T, path string) []*ocMessage {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var filas []struct {
		Mid, Role, Pid, Pdata string
		Mt                    int64
	}
	if err := json.Unmarshal(b, &filas); err != nil {
		t.Fatal(err)
	}
	sort.SliceStable(filas, func(i, j int) bool {
		a, b := filas[i], filas[j]
		if a.Mt != b.Mt {
			return a.Mt < b.Mt
		}
		if a.Mid != b.Mid {
			return a.Mid < b.Mid
		}
		return a.Pid < b.Pid
	})
	var msgs []*ocMessage
	for _, f := range filas {
		if len(msgs) == 0 || msgs[len(msgs)-1].ID != f.Mid {
			msgs = append(msgs, &ocMessage{ID: f.Mid, Role: f.Role, Time: f.Mt})
		}
		if f.Pid != "" {
			m := msgs[len(msgs)-1]
			m.Parts = append(m.Parts, newPart(f.Pid, f.Mid, f.Pdata, ""))
		}
	}
	return msgs
}

func editKind(e pruneEdit, ok bool) string {
	switch {
	case !ok:
		return "-"
	case e.Delete:
		return "borra"
	case e.Clear:
		return "fuera"
	case e.TrimInput:
		return "entrada"
	case e.Output != nil:
		return "recorta"
	}
	return "-"
}

// TestVigenciaCalibration: con una conversación real (la de INTRADEMA, 43 mensajes en 12 horas) y Ollama, para cada
// mensaje del usuario compara lo que decide la vigencia (corte en 3) con la poda por niveles de antes
// (legacyPlanPrune), por intercambio y por parte. Con PGO_PRUEBA_VIGENCIA_REJILLA=1, además busca las constantes.
// PGO_PRUEBA_VIGENCIA=<intradema.json> [PGO_PRUEBA_VIGENCIA_CACHE=<carpeta>] go test -run TestVigenciaCalibration -v ./internal/llm
func TestVigenciaCalibration(t *testing.T) {
	path := os.Getenv("PGO_PRUEBA_VIGENCIA")
	if path == "" {
		t.Skip()
	}
	dir := os.Getenv("PGO_PRUEBA_VIGENCIA_CACHE")
	if dir == "" {
		dir = t.TempDir()
	}
	pruneDirFunc = func() string { return dir }
	msgs := loadSessionDump(t, path)
	c := &pruneCollector{key: "calibracion"}
	c.load()
	defer c.save()
	ctx, cancela := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancela()
	var xs []*pruneExchange
	for _, x := range pruneExchanges(msgs) {
		if len(x.Messages[0].Parts) > 0 && x.Messages[0].Parts[0].v.Type != "compaction" && x.userText() != "" {
			xs = append(xs, x)
		}
	}
	type rec struct {
		ago, young int
		hours, sim float64
		old        bool
	}
	var recs []rec
	xIgual, xTotal, xOld, xNew := 0, 0, 0, 0
	pIgual, pTotal := 0, 0
	tOld, tNew := 0, 0
	distintas := map[string]int{}
	for k := 1; k < len(xs); k++ {
		var pre []*ocMessage
		for _, m := range msgs {
			if m.ID < xs[k].UserID {
				pre = append(pre, m)
			}
		}
		win := pruneWindow(pre)
		now := time.UnixMilli(xs[k].Messages[0].Time)
		s := c.sims(ctx, win, xs[k].userText())
		if s == nil {
			t.Fatal("sin Ollama")
		}
		s.PrevTier = nil // sin histéresis: cada barrido por sí solo
		old := legacyPlanPrune(win, s, pruneYoung)
		oldTier := map[string]pruneTier{}
		for _, x := range old.Exchanges {
			oldTier[x.UserID] = x.Tier
		}
		neu := planPrune(win, s, pruneYoung, now)
		for i, x := range neu.Exchanges {
			if x.Tier == TierL1 || i == 0 {
				continue
			}
			o := oldTier[x.UserID] == TierL3
			xTotal++
			if o == (x.Tier == TierL3) {
				xIgual++
			}
			if o {
				xOld++
			}
			if x.Tier == TierL3 {
				xNew++
			}
			if x.Vig.Sim != nil {
				recs = append(recs, rec{x.Vig.Ago, x.Vig.Young, vigHours(x.Vig.At, now), *x.Vig.Sim, o})
			}
		}
		for _, m := range win {
			for _, p := range m.Parts {
				eo, oko := old.Edits[p.ID]
				en, okn := neu.Edits[p.ID]
				ko, kn := editKind(eo, oko), editKind(en, okn)
				if ko == "-" && kn == "-" {
					continue
				}
				pTotal++
				if ko == kn {
					pIgual++
				} else {
					distintas[ko+"→"+kn]++
				}
			}
		}
		do, dn := pruneChanges(win, old), pruneChanges(win, neu)
		tOld += do.Saved
		tNew += dn.Saved
	}
	t.Logf("intercambios: %d de %d coinciden (%.1f %%); fuera antes %d, ahora %d", xIgual, xTotal, 100*float64(xIgual)/float64(xTotal), xOld, xNew)
	t.Logf("partes con cambios: %d de %d coinciden (%.1f %%); diferencias %v", pIgual, pTotal, 100*float64(pIgual)/float64(pTotal), distintas)
	t.Logf("tokens retirados (suma de los barridos): antes %d, ahora %d", tOld, tNew)
	if os.Getenv("PGO_PRUEBA_VIGENCIA_REJILLA") == "" {
		return
	}
	// La coincidencia según lo empinada que sea la relación (con la antigüedad y el tiempo elegidos).
	for _, kr := range []float64{8, 10, 12, 15, 20, 25, 30} {
		best, bp := 0, 0.0
		for piv := 0.38; piv <= 0.9; piv += 0.01 {
			ok := 0
			for _, r := range recs {
				v := vigAge(r.ago-r.young) + vigTimeK*math.Log1p(r.hours) + kr*(piv-r.sim)
				if (v >= vigCut) == r.old {
					ok++
				}
			}
			if ok > best {
				best, bp = ok, piv
			}
		}
		t.Logf("relación %.0f → pivote %.2f: %.1f %%", kr, bp, 100*float64(best)/float64(len(recs)))
	}
	// Rejilla: con la antigüedad y el tiempo dados, la relación que mejor reproduce el corte de antes.
	for _, ka := range []float64{0.06, 0.09, 0.12, 0.15, 0.2, 0.25} {
		for _, kt := range []float64{0.1, 0.15, 0.2, 0.25} {
			best, bk, bp := 0, 0.0, 0.0
			for kr := 4.0; kr <= 30; kr += 1 {
				for piv := 0.38; piv <= 0.9; piv += 0.01 {
					ok := 0
					for _, r := range recs {
						v := ka*math.Log1p(float64(max(r.ago-r.young, 0)))/math.Log(phi) + kt*math.Log1p(r.hours) + kr*(piv-r.sim)
						if (v >= vigCut) == r.old {
							ok++
						}
					}
					if ok > best {
						best, bk, bp = ok, kr, piv
					}
				}
			}
			t.Logf("antigüedad %.2f tiempo %.2f → relación %.0f pivote %.2f: %.1f %%", ka, kt, bk, bp, 100*float64(best)/float64(len(recs)))
		}
	}
}
