package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// longTurn: un turno largo (el último intercambio): n pasos, cada uno lee un fichero distinto (salida grande).
func longTurn(n int) (*sesionPrueba, []*ocPart) {
	s := &sesionPrueba{}
	s.user("Instrucciones del proyecto")
	s.assistant(text("De acuerdo."))
	s.user("Revisa todos los módulos y arregla lo que falle")
	var reads []*ocPart
	for i := 0; i < n; i++ {
		t := tool("read", map[string]any{"filePath": fmt.Sprintf("/p/m%02d.go", i), "offset": 1, "limit": 250}, lineas(200))
		t["callID"] = fmt.Sprintf("call_r%02d", i)
		m := s.assistant(t)
		reads = append(reads, m.Parts[0])
	}
	return s, reads
}

func TestYoungPiecesOwnVigencia(t *testing.T) {
	s, reads := longTurn(30)
	// Entre pasos (el turno en curso es joven): sin embeddings, solo los pasos.
	plan := planPrune(s.msgs, nil, pruneYoung+1, time.Time{})
	por := map[string]*pieceVig{}
	for _, pv := range plan.Pieces {
		por[pv.PartID] = pv
	}
	for i, p := range reads {
		pv := por[p.ID]
		since := len(reads) - 1 - i
		switch {
		case since < pruneProtectSteps:
			if !pv.F.Pinned || pv.V != 0 || pv.Out {
				t.Fatalf("paso %d (hace %d): los últimos pasos no se tocan: %+v", i, since, pv)
			}
			if _, ok := plan.Edits[p.ID]; ok {
				t.Fatalf("paso %d tocado", i)
			}
		case pv.V >= vigCut:
			if !pv.Out || !plan.Edits[p.ID].Clear {
				t.Fatalf("paso %d (vigencia %.2f) no sale", i, pv.V)
			}
		}
		if pv.F.Steps != since {
			t.Fatalf("paso %d: %d pasos, esperaba %d", i, pv.F.Steps, since)
		}
	}
	if !por[reads[0].ID].Out {
		t.Fatalf("la lectura más vieja del turno sigue: %+v", por[reads[0].ID])
	}
	// La relación con lo que se hace ahora la mantiene; sin relación, sale antes.
	sims := &pruneSims{Exchange: map[string]float64{}, Piece: map[string]float64{}}
	sims.Piece[reads[0].ID] = 0.9
	sims.Piece[reads[20].ID] = 0.2
	plan = planPrune(s.msgs, sims, pruneYoung+1, time.Time{})
	for _, pv := range plan.Pieces {
		switch pv.PartID {
		case reads[0].ID:
			if pv.Out {
				t.Fatalf("lectura relacionada fuera: %+v", pv)
			}
		case reads[20].ID:
			if !pv.Out {
				t.Fatalf("lectura sin relación (hace 9 pasos) sigue: %+v", pv)
			}
		}
	}
	// El texto del usuario, el razonamiento y las respuestas no se tocan nunca en los jóvenes.
	for id, e := range plan.Edits {
		p := partByID(s.msgs, id)
		if p.v.Type != "tool" || !e.Clear {
			t.Fatalf("en un intercambio joven solo se vacían salidas: %s %+v", p.v.Type, e)
		}
	}
	// El primer intercambio (instrucciones) sigue fijo y el turno en curso entero cuenta como vital.
	if !plan.Exchanges[0].Vig.Pinned || plan.Exchanges[len(plan.Exchanges)-1].V != 0 {
		t.Fatal("intercambios fijos")
	}
}

func TestReadRangeSuperseded(t *testing.T) {
	s := &sesionPrueba{}
	s.user("Lee el fichero")
	s.assistant(tool("read", map[string]any{"filePath": "/p/a.go", "offset": 1, "limit": 100}, lineas(100)),
		tool("read", map[string]any{"filePath": "/p/a.go", "offset": 101, "limit": 100}, lineas(99)),
		tool("read", map[string]any{"filePath": "/p/b.go", "offset": 50, "limit": 20}, lineas(19)),
		tool("read", map[string]any{"filePath": "/p/b.go"}, lineas(150)), text("Leído."))
	for i := 0; i < 4; i++ {
		s.user(fmt.Sprintf("Otra cosa %d sin relación ninguna con lo anterior", i))
		s.assistant(text("Vale."))
	}
	plan := planPrune(s.msgs, nil, pruneYoung, time.Time{})
	if r := supersededOf(plan, parteDe(t, s.msgs, "tool", "read", 0).ID); r != "" {
		t.Fatalf("otro trozo del fichero no supera al primero: %q", r)
	}
	if r := supersededOf(plan, parteDe(t, s.msgs, "tool", "read", 2).ID); r != "lectura superada" {
		t.Fatalf("la lectura entera supera al trozo: %q", r)
	}
}

func TestCompactedReported(t *testing.T) {
	s := &sesionPrueba{}
	s.user("antiguo")
	viejo := s.assistant(tool("read", map[string]any{"filePath": "/p/x.go"}, lineas(300)), text("visto"))
	viejo.Parts[0].v.CallID = "call_viejo"
	cola := s.user("cola")
	colaA := s.assistant(text("sigue"))
	comp := &ocMessage{ID: s.id("msg"), Role: "user"}
	b, _ := json.Marshal(map[string]any{"type": "compaction", "auto": true, "tail_start_id": colaA.ID})
	comp.Parts = []*ocPart{newPart(s.id("prt"), comp.ID, string(b), "")}
	s.msgs = append(s.msgs, comp)
	sum := s.assistant(text("resumen"))
	sum.Summary, sum.Parent = true, comp.ID
	s.user("nuevo")
	s.assistant(text("ok"))
	win := pruneWindow(s.msgs)
	before := compactedBefore(s.msgs, win)
	if len(before) != 3 || before[2].ID != cola.ID {
		t.Fatalf("antes de la compactación: %d mensajes", len(before))
	}
	// La cola conservada (sin su mensaje del usuario) es un intercambio de la ventana.
	xs := pruneExchanges(win)
	if len(xs) != 2 || !xs[0].Tail || xs[0].Point != colaA.ID {
		t.Fatalf("intercambios de la ventana: %+v", xs)
	}
	st := vigSnapshot(planPrune(win, nil, pruneYoung, time.Time{}), func(*pruneExchange) bool { return false }, func(*pieceVig) bool { return false })
	st.addCompacted(before)
	xr, pr := st.report(time.Time{})
	var punto string
	for _, x := range pruneExchanges(before) {
		punto = x.Point
		break
	}
	found := false
	for _, x := range xr {
		if x.Point == punto {
			found = true
			if !x.Out || x.Value < vigCut || x.Superseded != compactedNote {
				t.Fatalf("intercambio compactado: %+v", x)
			}
		}
	}
	if !found {
		t.Fatal("el intercambio compactado no sale en el informe")
	}
	ok := false
	for _, p := range pr {
		if p.ID == "call_viejo" {
			ok = p.Out && p.Value >= vigCut
		}
	}
	if !ok {
		t.Fatalf("la pieza compactada no sale fuera: %+v", pr)
	}
}

func TestClaudeBeforeCompaction(t *testing.T) {
	f := newClaudeFixture("ses-comp")
	f.user("Lee viejo.go")
	f.call(1000, useBlock("toolu_viejo", "Read", map[string]any{"file_path": "/p/viejo.go"}))
	f.result("toolu_viejo", bigFile, false)
	viejo := f.call(2000, textBlock("Leído."))
	f.parent = nil
	f.add(map[string]any{"type": "system", "subtype": "compact_boundary", "content": "Conversation compacted"})
	f.add(map[string]any{"type": "user", "isCompactSummary": true, "message": map[string]any{"role": "user", "content": "Resumen de lo anterior"}})
	f.call(3000, textBlock("Sigo."))
	f.user("Nuevo")
	f.call(4000, textBlock("Hecho."))
	s, err := parseClaudeSession("ses-comp", "/x", f.bytes(), nil)
	if err != nil {
		t.Fatal(err)
	}
	st := &vigState{}
	st.addCompacted(s.Before)
	xr, pr := st.report(time.Time{})
	if len(xr) != 1 || xr[0].Point != viejo || !xr[0].Out || len(pr) != 1 || pr[0].ID != "toolu_viejo" || !pr[0].Out {
		t.Fatalf("antes de la compactación: %+v %+v", xr, pr)
	}
	for _, m := range s.Win {
		if m.ID == viejo {
			t.Fatal("lo compactado sigue en la ventana")
		}
	}
}

func TestChatCompactedAndSteps(t *testing.T) {
	xs := []ChatExchange{
		{Point: "m1", User: "viejo", Answer: "ok", Compacted: true, Tools: []ChatTool{{ID: "m1:a", Name: "view_file", Summary: "/p/a.go", Output: lineas(300)}}},
		{Point: "m3", User: "nuevo", Answer: "ok", Tools: []ChatTool{{ID: "m3:b", Name: "view_file", Summary: "/p/b.go", Output: "x"}, {ID: "m3:c", Name: "run_command", Summary: "ls", Output: "y"}}},
	}
	win := chatWindow(xs[1:])
	if n := len(win); n != 4 || win[3].ID != "m3" || win[1].Role != "assistant" {
		t.Fatalf("cada herramienta, un paso: %d mensajes", n)
	}
	drainCollectors()
	dir := t.TempDir()
	viejo := pruneDirFunc
	pruneDirFunc = func() string { return dir }
	defer func() { drainCollectors(); pruneDirFunc = viejo }()
	r := ChatVigencia(t.Context(), "chat-comp1", "/p", xs, "", false)
	out := false
	for _, x := range r.Exchanges {
		if x.Point == "m1" {
			out = x.Out && x.Value >= vigCut
		}
	}
	if !out {
		t.Fatalf("intercambio compactado de Antigravity: %+v", r.Exchanges)
	}
}

func TestMidSweepWorth(t *testing.T) {
	d := pruneDiff{Saved: 20000, Update: map[string]string{}}
	s := &sesionPrueba{}
	s.user("x")
	m := s.assistant(tool("read", map[string]any{"filePath": "/p/a"}, strings.Repeat("a", 90000)))
	s.assistant(tool("read", map[string]any{"filePath": "/p/b"}, strings.Repeat("b", 300000)))
	d.Update[m.Parts[0].ID] = `{"type":"tool","state":{"output":""}}`
	if suf := changedSuffix(s.msgs, d); suf < 100000 {
		t.Fatalf("resto: %d", suf)
	}
	// Lejos del umbral: el resto (100k sin caché) no se amortiza con 20k menos en 10 pasos.
	if (&midSweep{Context: 50000, Threshold: 150000}).worth(d, s.msgs) {
		t.Fatal("no compensa")
	}
	// A mitad del umbral de compactación, sí.
	if !(&midSweep{Context: 80000, Threshold: 150000}).worth(d, s.msgs) {
		t.Fatal("cerca de compactar compensa")
	}
}
