package llm

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

var vigT0 = time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)

func TestVigenciaFormula(t *testing.T) {
	base := vigFactors{Ago: 5, Young: 2, At: vigT0.UnixMilli()}
	if v := vigencia(vigFactors{Pinned: true, Ago: 40, Sim: simPtr(0)}, vigT0); v != 0 {
		t.Fatalf("fija: %v", v)
	}
	if v := vigencia(vigFactors{Pinned: true, Superseded: "lectura superada"}, vigT0); v != vigSupersededJump {
		t.Fatalf("fija y superada: %v", v)
	}
	// Antigüedad: sube poco a poco con cada intercambio.
	a, b := vigencia(base, vigT0), vigencia(vigFactors{Ago: 6, Young: 2, At: base.At}, vigT0)
	if a != vigAgeStep*3 || b <= a || b-a > 0.1 {
		t.Fatalf("antigüedad: %v → %v", a, b)
	}
	// Tiempo: suave (logarítmico en horas).
	h1, h3, d1 := vigencia(base, vigT0.Add(time.Hour)), vigencia(base, vigT0.Add(3*time.Hour)), vigencia(base, vigT0.Add(24*time.Hour))
	if !(a < h1 && h1 < h3 && h3 < d1) || d1-a > 0.6 || (h3-h1)/2 >= h1-a || (d1-h3)/21 >= (h3-h1)/2 {
		t.Fatalf("tiempo: %v %v %v %v", a, h1, h3, d1)
	}
	// Relación: mucha la baja (sin bajar de 0); poca la sube por encima del corte.
	mucha, poca := base, base
	mucha.Sim, poca.Sim = simPtr(0.8), simPtr(0.2)
	if v := vigencia(mucha, vigT0); v != 0 {
		t.Fatalf("mucha relación: %v", v)
	}
	if v := vigencia(poca, vigT0); v < vigCut {
		t.Fatalf("poca relación: %v", v)
	}
	// Superada: salto grande, por encima del corte.
	s := base
	s.Superseded = "orden repetida"
	if v := vigencia(s, vigT0); v < vigCut {
		t.Fatalf("superada: %v", v)
	}
	// Sin fecha, el tiempo no cuenta.
	if v := vigencia(vigFactors{Ago: 5, Young: 2}, vigT0.Add(1000*time.Hour)); v != a {
		t.Fatalf("sin fecha: %v", v)
	}
}

// conFechas: la conversación de prueba con un intercambio por hora (el último, en vigT0).
func conFechas(s *sesionPrueba) {
	xs := pruneExchanges(s.msgs)
	for i, x := range xs {
		for _, m := range x.Messages {
			m.Time = vigT0.Add(time.Duration(i-len(xs)+1) * time.Hour).UnixMilli()
		}
	}
}

// El tiempo resta vigencia: el mismo plan unas horas después (fecha forzada) da vigencias más altas, sin cambiar
// lo joven; y con relación justo por encima del corte, el tiempo acaba sacando el intercambio.
func TestVigenciaTime(t *testing.T) {
	s := conversacion()
	conFechas(s)
	xs := pruneExchanges(s.msgs)
	sims := &pruneSims{Exchange: map[string]float64{}, Piece: map[string]float64{}}
	for _, x := range xs {
		sims.Exchange[x.UserID] = 0.36
	}
	ahora := planPrune(s.msgs, sims, pruneYoung, vigT0)
	luego := planPrune(s.msgs, sims, pruneYoung, vigT0.Add(3*time.Hour))
	for i := range ahora.Exchanges {
		a, l := ahora.Exchanges[i].V, luego.Exchanges[i].V
		if i == 0 || i >= len(xs)-pruneYoung {
			if a != 0 || l != 0 {
				t.Fatalf("fijo %d: %v %v", i, a, l)
			}
			continue
		}
		if l <= a {
			t.Fatalf("intercambio %d: %v → %v", i, a, l)
		}
	}
	// Una relación que deja el intercambio 2 (hace 3 intercambios, 2 horas) justo por debajo del corte.
	h := vigHours(xs[2].time(), vigT0)
	sims.Exchange[xs[2].UserID] = vigRelPivot - (vigCut-0.05-vigAgeStep-vigTimeK*math.Log1p(h))/vigRelK
	if x := planPrune(s.msgs, sims, pruneYoung, vigT0).Exchanges[2]; x.Tier != TierL2 || x.V >= vigCut {
		t.Fatalf("ahora: %v %v", x.Tier, x.V)
	}
	if x := planPrune(s.msgs, sims, pruneYoung, vigT0.Add(48*time.Hour)).Exchanges[2]; x.Tier != TierL3 {
		t.Fatalf("dos días después: %v %v", x.Tier, x.V)
	}
}

// La vigencia de cada pieza: lo superado por encima del corte, lo demás con la de su intercambio, y lo joven a 0.
func TestVigenciaPieces(t *testing.T) {
	s := conversacion()
	conFechas(s)
	plan := planPrune(s.msgs, nil, pruneYoung, vigT0)
	n := 0
	for _, p := range plan.Pieces {
		if p.V < p.X.V {
			t.Fatalf("pieza por debajo de su intercambio: %+v", p)
		}
		if (p.F.Superseded != "") != p.Out {
			t.Fatalf("fuera sin estar superada (sin embeddings): %+v", p)
		}
		if p.Out {
			n++
		}
	}
	if n == 0 {
		t.Fatal("nada superado")
	}
	// Informe: todas los intercambios con punto y todas las herramientas con id; una hora después, más altas.
	st := vigSnapshot(plan, func(x *pruneExchange) bool { return x.Tier == TierL3 }, func(p *pieceVig) bool { return p.Out })
	xs, ps := st.report(vigT0)
	if len(xs) != 5 || len(ps) == 0 {
		t.Fatalf("informe: %d %d", len(xs), len(ps))
	}
	xs2, _ := st.report(vigT0.Add(time.Hour))
	if xs2[1].Value <= xs[1].Value || xs[4].Value != 0 || xs2[4].Value != 0 {
		t.Fatalf("una hora después: %v → %v", xs[1].Value, xs2[1].Value)
	}
	if xs[1].Ago != 4 || xs[1].Hours != 3 {
		t.Fatalf("porqué: %+v", xs[1].Vigencia)
	}
	b, _ := json.Marshal(PruneReport{Exchanges: xs[:2], Pieces: ps[:1]})
	for _, k := range []string{`"punto":`, `"vigencia":`, `"hace":4`, `"horas":3`, `"piezas":[{"id":"call_`} {
		if !strings.Contains(string(b), k) {
			t.Fatalf("JSON sin %s: %s", k, b)
		}
	}
}

// Sin embeddings (Ollama no está o solo reglas): sin el término de relación. Solo con antigüedad y tiempo no llega
// al corte ni con un mes (como antes, sin embeddings no se saca nada entero; las reglas siguen).
func TestVigenciaWithoutEmbeddings(t *testing.T) {
	s := conversacion(60)
	plan := planPrune(s.msgs, nil, pruneYoung, vigT0)
	if x := plan.Exchanges[1]; x.Vig.Sim != nil || x.Tier != TierL2 {
		t.Fatalf("sin relación: %+v", x.Vig)
	}
	for _, m := range s.msgs {
		m.Time = vigT0.Add(-24 * 30 * time.Hour).UnixMilli()
	}
	plan = planPrune(s.msgs, nil, pruneYoung, vigT0)
	if x := plan.Exchanges[1]; x.Tier != TierL2 || x.V < 0.9 || x.V >= vigCut {
		t.Fatalf("un mes: %v %v", x.Tier, x.V)
	}
}

// En segundo plano: tras marcar, VigenciaReport da la vigencia con la hora actual (aquí, forzada: el último turno
// «tras una espera»), sin barrer.
func TestVigenciaReportBackground(t *testing.T) {
	s := conversacion()
	conFechas(s)
	var filas []map[string]any
	for _, m := range s.msgs {
		for _, p := range m.Parts {
			filas = append(filas, map[string]any{"mid": m.ID, "role": m.Role, "summary": 0, "parent": "", "mt": m.Time, "pid": p.ID, "data": p.Data, "cur": ""})
		}
	}
	rows, _ := json.Marshal(filas)
	viejoDB, viejoDir, viejoNow, viejoEmb := ocDB, pruneDirFunc, pruneNow, pruneEmbed
	defer func() { ocDB, pruneDirFunc, pruneNow, pruneEmbed = viejoDB, viejoDir, viejoNow, viejoEmb }()
	dir := t.TempDir()
	pruneDirFunc = func() string { return dir }
	ocDB = func(_ context.Context, _ *Agente, _ Peticion, q string, _ bool) ([]byte, error) {
		if strings.HasPrefix(q, "SELECT") {
			return rows, nil
		}
		return nil, nil
	}
	pruneEmbed = func(_ context.Context, _, tarea string) ([]float32, error) {
		if tarea == "RETRIEVAL_QUERY" {
			return []float32{1, 0}, nil
		}
		return []float32{1, 2}, nil // relación 0,45
	}
	ahora := vigT0
	pruneNow = func() time.Time { return ahora }
	if VigenciaReport(context.Background(), "ses_vigencia1") != nil {
		t.Fatal("sin estado, sin informe")
	}
	if VigenciaReport(context.Background(), "../x") != nil {
		t.Fatal("sesión no válida")
	}
	markOpenCode(&Agente{Clave: "opencode"}, Peticion{Carpeta: t.TempDir(), Prune: PruneAuto}, "ses_vigencia1")
	r1 := VigenciaReport(context.Background(), "ses_vigencia1")
	if r1 == nil || len(r1.Exchanges) != 5 || r1.Exchanges[1].Sim == nil {
		t.Fatalf("informe: %+v", r1)
	}
	ahora = vigT0.Add(3 * time.Hour)
	r2 := VigenciaReport(context.Background(), "ses_vigencia1")
	if r2.Exchanges[1].Value <= r1.Exchanges[1].Value || r2.Exchanges[1].Hours != r1.Exchanges[1].Hours+3 {
		t.Fatalf("tres horas después: %+v → %+v", r1.Exchanges[1].Vigencia, r2.Exchanges[1].Vigencia)
	}
}
