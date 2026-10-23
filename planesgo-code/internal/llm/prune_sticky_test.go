package llm

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Lo que pgocode ya retiró no vuelve solo porque su vigencia baje (en INTRADEMA volvieron de golpe 24 piezas y 49k,
// el contexto subió a 154k y OpenCode compactó): solo con una cita de algo que solo estaba ahí, y poco cada vez.
func TestStickyOut(t *testing.T) {
	s := &sesionPrueba{}
	s.user("instrucciones")
	s.assistant(text("ok"))
	// Tres lecturas grandes, ya retiradas por pgocode (Current con la marca nativa).
	var leidas []*ocPart
	for i := 0; i < 3; i++ {
		s.user(fmt.Sprintf("lee el fichero %d", i))
		cuerpo := fmt.Sprintf("func calculaTotal%d() {}\nvar tarifa_especial_%d = 3\n", i, i) + lineas(400)
		m := s.assistant(tool("read", map[string]any{"filePath": fmt.Sprintf("/p/f%d.go", i)}, cuerpo))
		p := m.Parts[0]
		cur, err := renderPart(p.Data, pruneEdit{Clear: true})
		if err != nil {
			t.Fatal(err)
		}
		p.Current = cur
		leidas = append(leidas, p)
	}
	s.user("sigue")
	s.assistant(text("vale"))
	// Un plan que ya no las retira (su vigencia ha bajado): sin señal, se quedan fuera.
	plan := prunePlan{Edits: map[string]pruneEdit{}}
	if back := stickyOut(s.msgs, &plan, "sigue con lo de antes"); len(back) != 0 {
		t.Fatalf("sin cita no vuelve nada: %v", back)
	}
	d := pruneChanges(s.msgs, plan)
	if len(d.Restore) != 0 || d.Back != 0 {
		t.Fatalf("no se repone: %+v", d)
	}
	// El agente cita dos identificadores que solo estaban en la lectura 1: vuelve esa (si cabe en el tope).
	plan = prunePlan{Edits: map[string]pruneEdit{}}
	back := stickyOut(s.msgs, &plan, "cambia calculaTotal1 y tarifa_especial_1")
	tok := tokensOf(leidas[1].Data) - tokensOf(leidas[1].Current)
	if tok <= pruneReviveMax {
		if len(back) != 1 || back[0] != leidas[1].ID {
			t.Fatalf("vuelve la citada: %v", back)
		}
	} else if len(back) != 0 {
		t.Fatalf("más grande que el tope (%d): no vuelve", tok)
	}
	if e := plan.Edits[leidas[0].ID]; !e.Keep {
		t.Fatal("las no citadas se quedan fuera")
	}
	d = pruneChanges(s.msgs, plan)
	if len(d.Restore) != len(back) {
		t.Fatalf("repuestas: %v", d.Restore)
	}
	if strings.Contains(d.String(), "-") || strings.Contains(d.pendingString(), "-") {
		t.Fatalf("registro: %q / %q", d.String(), d.pendingString())
	}
	neg := pruneDiff{Restore: []string{"x"}}
	neg.add(-40548)
	if neg.String() != "0 fuera · 40548 repuestos" || neg.pendingString() != "0 por retirar · 40548 por reponer" {
		t.Fatalf("una reposición sola: %q / %q", neg.String(), neg.pendingString())
	}
}

// El tope de lo que vuelve: muchas piezas citadas, solo las que caben en pruneReviveMax.
func TestStickyOutCap(t *testing.T) {
	s := &sesionPrueba{}
	s.user("instrucciones")
	s.assistant(text("ok"))
	var cita []string
	for i := 0; i < 24; i++ {
		s.user(fmt.Sprintf("lee %d", i))
		cuerpo := fmt.Sprintf("valorUnico%dA valorUnico%dB\n", i, i) + strings.Repeat("dato pequeño\n", 60)
		m := s.assistant(tool("read", map[string]any{"filePath": fmt.Sprintf("/p/g%d.txt", i)}, cuerpo))
		m.Parts[0].Current, _ = renderPart(m.Parts[0].Data, pruneEdit{Clear: true})
		cita = append(cita, fmt.Sprintf("valorUnico%dA valorUnico%dB", i, i))
	}
	plan := prunePlan{Edits: map[string]pruneEdit{}}
	stickyOut(s.msgs, &plan, strings.Join(cita, " "))
	d := pruneChanges(s.msgs, plan)
	if d.Back > pruneReviveMax || len(d.Restore) == 0 || len(d.Restore) == 24 {
		t.Fatalf("tope: %d repuestas, %d tokens", len(d.Restore), d.Back)
	}
}

// estTokens: los números van a ~1 token por dígito; el texto, a unos 3,5-4 caracteres.
func TestEstTokens(t *testing.T) {
	csv := strings.Repeat("2024-03-01,1234.56,0.25,98765\n", 1000)
	if r := float64(len(csv)) / float64(estTokens(csv)); r < 1.1 || r > 1.6 {
		t.Errorf("CSV numérico: %.2f caracteres por token", r)
	}
	prosa := strings.Repeat("El agente lee el fichero y propone un cambio pequeño en la función. ", 200)
	if r := float64(len(prosa)) / float64(estTokens(prosa)); r < 3.2 || r > 4.5 {
		t.Errorf("texto: %.2f caracteres por token", r)
	}
}

// La relación resta como mucho vigBigRelCap en las piezas grandes con muchos pasos encima (un CSV de 70k leído una
// vez seguía dentro con vigencia 2,1 porque todo el proyecto guardaba relación con él).
func TestBigPieceRelationCap(t *testing.T) {
	sim := 0.5
	f := vigFactors{Steps: pruneProtectSteps + 12, Sim: &sim}
	pequeña := vigencia(f, time.Time{})
	f.Size = 70000
	grande := vigencia(f, time.Time{})
	if pequeña >= vigCut || grande < vigCut {
		t.Fatalf("pequeña %.2f (sigue), grande %.2f (sale)", pequeña, grande)
	}
	// Con pocos pasos encima, la relación cuenta entera.
	f.Steps = pruneProtectSteps + 2
	if v := vigencia(f, time.Time{}); v >= vigCut {
		t.Fatalf("grande y reciente: %.2f", v)
	}
}

// Del primer intercambio solo es vital el texto: una lectura grande suya sale como las demás.
func TestFirstExchangeToolsNotPinned(t *testing.T) {
	s := &sesionPrueba{}
	s.user("lee datos.csv y resume")
	s.assistant(tool("read", map[string]any{"filePath": "/p/datos.csv"}, strings.Repeat("1,2,3,4\n", 4000)))
	s.assistant(text("resumen"))
	for i := 0; i < 6; i++ {
		s.user(fmt.Sprintf("otra cosa %d sin relación", i))
		for j := 0; j < 3; j++ {
			s.assistant(tool("bash", map[string]any{"command": fmt.Sprintf("ls %d %d", i, j)}, "x"))
		}
		s.assistant(text("hecho"))
	}
	plan := planPrune(s.msgs, nil, pruneYoung, time.Now())
	if plan.Exchanges[0].Tier == TierL3 || plan.Exchanges[0].V != 0 {
		t.Fatalf("el intercambio de las instrucciones sigue vital: %v %.2f", plan.Exchanges[0].Tier, plan.Exchanges[0].V)
	}
	for _, pv := range plan.Pieces {
		if pv.X == plan.Exchanges[0] && pv.F.Pinned {
			t.Fatal("sus herramientas no quedan fijadas")
		}
	}
}

// Por encima del objetivo, el corte baja para lo grande y viejo; lo pequeño y lo reciente siguen con el normal.
func TestBudgetCut(t *testing.T) {
	if budgetCut(2.5, 5000, 30, false) != 2.5 || budgetCut(2.5, 500, 30, false) != vigCut {
		t.Fatal("solo lo grande")
	}
	if budgetCut(2, 5000, pruneProtectSteps+1, true) != vigCut || budgetCut(2, 5000, 2*pruneProtectSteps, true) != 2 {
		t.Fatal("de los intercambios jóvenes, solo lo que tiene muchos pasos encima")
	}
	if budgetCut(vigCut, 50000, 50, false) != vigCut {
		t.Fatal("sin objetivo, el normal")
	}
}

// decideSweep baja el corte hasta quedar por debajo del objetivo.
func TestDecideSweepTarget(t *testing.T) {
	s := &sesionPrueba{}
	s.user("instrucciones")
	m0 := s.assistant(text("ok"))
	m0.Ctx = 15000
	for i := 0; i < 5; i++ {
		s.user(fmt.Sprintf("tarea %d sobre módulos distintos del proyecto", i))
		s.assistant(tool("read", map[string]any{"filePath": fmt.Sprintf("/p/m%d.go", i)}, lineas(600)))
		for j := 0; j < 4; j++ {
			s.assistant(tool("bash", map[string]any{"command": fmt.Sprintf("go test %d %d", i, j)}, "ok"))
		}
		s.assistant(text("hecho"))
	}
	last := s.msgs[len(s.msgs)-1]
	last.Ctx = 120000
	// Pesos con los que nada llega a 3 pero lo viejo pasa de 2: solo el objetivo lo saca.
	k := vigParams{AgeK: 1.2, StepK: 0, TimeK: 0, RelK: 0, RelPivot: 0.35}
	in := sweepInput{Win: s.msgs, Young: pruneYoung, Now: time.Now(), K: k, Msg: "otra tarea"}
	sin := decideSweep(in)
	in.Target = 60000
	con := decideSweep(in)
	if con.D.Saved <= sin.D.Saved || con.Cut >= vigCut {
		t.Fatalf("con objetivo sale más (%d frente a %d, corte %.1f)", con.D.Saved, sin.D.Saved, con.Cut)
	}
	if con.Rewrite != max(con.Ctx-con.System-con.D.Saved, 0) || con.System <= 0 {
		t.Fatalf("lo reenviado sin caché: todo lo de después del sistema (%d, sistema %d)", con.Rewrite, con.System)
	}
}
