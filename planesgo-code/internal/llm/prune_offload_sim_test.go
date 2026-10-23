package llm

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestOffloadSim: lo que añaden las salidas a fichero a la poda en las sesiones reales de OpenCode (en una COPIA de
// su base de datos: HOME apunta a una carpeta con .local/share/pgocode/opencode/opencode.db). Para cada mensaje del
// usuario de cada sesión, lo que la poda (solo reglas) quitaría del historial anterior con y sin ellas, desde el
// historial original. PGO_PRUEBA_SALIDAS_SIM=1 HOME=<copia> go test -run TestOffloadSim -v ./internal/llm
func TestOffloadSim(t *testing.T) {
	if os.Getenv("PGO_PRUEBA_SALIDAS_SIM") == "" {
		t.Skip()
	}
	var a *Agente
	for _, p := range Agentes() {
		if x, ok := p.(*Agente); ok && x.Clave == "opencode" {
			a = x
		}
	}
	p := Peticion{Carpeta: os.TempDir()}
	ctx := context.Background()
	out, err := ocDB(ctx, a, p, "SELECT id FROM session WHERE parent_id IS NULL", true)
	if err != nil {
		t.Fatal(err)
	}
	var sesiones []struct{ ID string }
	json.Unmarshal(out, &sesiones)
	minimo, _ := strconv.Atoi(os.Getenv("PGO_PRUEBA_SALIDAS_SIM_MIN"))
	var hist, sin, con, turnos, conPasos, sinPasos int
	for _, s := range sesiones {
		msgs, err := loadOpenCodeSession(ctx, a, p, s.ID)
		if err != nil || len(msgs) < minimo {
			continue
		}
		// Desde el original: sin lo que pgocode ya cambió.
		for _, m := range msgs {
			for _, pt := range m.Parts {
				pt.Current = ""
			}
		}
		xs := pruneExchanges(msgs)
		var sh, ss, sc int
		for k := 1; k < len(xs); k++ {
			var pre []*ocMessage
			for _, m := range msgs {
				if m.ID < xs[k].UserID {
					pre = append(pre, m)
				}
			}
			win := pruneWindow(pre)
			h := 0
			for _, m := range win {
				for _, pt := range m.Parts {
					h += tokensOf(pt.Data)
				}
			}
			// Los pasos del turno k (lo que se arrastra lo retirado al empezarlo).
			pasos := 0
			for _, m := range xs[k].Messages {
				if m.Role == "assistant" {
					pasos++
				}
			}
			in := sweepInput{Win: win, Young: pruneYoung, Now: time.Now(), K: defaultVig, Msg: xs[k].userText(), Dir: "/p/.pgocode/salidas"}
			offloadInSweep = false
			o1 := decideSweep(in)
			offloadInSweep = true
			o2 := decideSweep(in)
			sh += h
			ss += o1.D.Saved
			sc += o2.D.Saved
			sinPasos += o1.D.Saved * pasos
			conPasos += o2.D.Saved * pasos
			turnos++
		}
		if sh > 0 {
			t.Logf("%s: %3d intercambios · historial %8d · poda sin %8d (%4.1f %%) · con %8d (%4.1f %%)", s.ID, len(xs), sh, ss, 100*float64(ss)/float64(sh), sc, 100*float64(sc)/float64(sh))
		}
		hist += sh
		sin += ss
		con += sc
	}
	t.Logf("TOTAL %d turnos · historial %d · poda sin salidas a fichero %d (%.1f %%) · con %d (%.1f %%) · tokens·paso retirados: sin %d, con %d (+%.1f %%)",
		turnos, hist, sin, 100*float64(sin)/float64(hist), con, 100*float64(con)/float64(hist), sinPasos, conPasos, 100*float64(conPasos-sinPasos)/float64(max(sinPasos, 1)))
}
