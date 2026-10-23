package llm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Una lectura de OpenCode («N: …»): principio, final y las líneas que faltan, para releerlas del propio fichero.
func TestOffloadTextRead(t *testing.T) {
	var b strings.Builder
	b.WriteString("<path>/p/grande.go</path>\n<type>file</type>\n<content>\n")
	for i := 1; i <= 500; i++ {
		fmt.Fprintf(&b, "%d: func f%d() int { return %d } // relleno de la línea\n", i, i, i)
	}
	b.WriteString("\n(End of file - total 500 lines)\n</content>")
	s := &sesionPrueba{}
	s.user("lee")
	m := s.assistant(tool("read", map[string]any{"filePath": "/p/grande.go"}, b.String()))
	txt, file, ok := offloadText(m.Parts[0], "/p/.pgocode/salidas", false)
	if !ok || file != "" {
		t.Fatalf("lectura: %v %q", ok, file)
	}
	if !strings.Contains(txt, "Lines 18-483 of /p/grande.go omitted") || !strings.Contains(txt, "offset=18 and limit=466") {
		t.Fatalf("aviso de la lectura: %q", txt)
	}
	if !strings.HasPrefix(txt, "<path>/p/grande.go</path>") || !strings.HasSuffix(txt, "</content>") || len(txt) > 4000 {
		t.Fatalf("principio y final (%d): %q", len(txt), txt)
	}
	// Claude Code («    12→…» o «12\t…»).
	if lineNum("    12→x") != 12 || lineNum("7\tx") != 7 || lineNum("sin número") != 0 {
		t.Fatal("números de línea")
	}
}

// Una orden: la ruta de la salida completa con el aviso nativo de cada agente; sin carpeta, nada.
func TestOffloadTextCommand(t *testing.T) {
	s := &sesionPrueba{}
	s.user("compila")
	m := s.assistant(tool("bash", map[string]any{"command": "make"}, lineas(400)))
	p := m.Parts[0]
	txt, file, ok := offloadText(p, "/p/.pgocode/salidas", false)
	if !ok || file != "/p/.pgocode/salidas/call_bash"+fmt.Sprint(len(p.output()))+".txt" {
		t.Fatalf("orden: %v %q", ok, file)
	}
	if !strings.Contains(txt, "Full output saved to: "+file) || !strings.Contains(txt, "400 lines") ||
		!strings.HasPrefix(txt, "línea 0 ") || !strings.HasSuffix(txt, "línea 399 de un fichero bastante largo con código") {
		t.Fatalf("aviso de OpenCode: %q", txt)
	}
	c, _, _ := offloadText(p, "/p/.pgocode/salidas", true)
	if !strings.HasPrefix(c, "<persisted-output>\nOutput too large (") || !strings.Contains(c, "Full output saved to: "+file) {
		t.Fatalf("aviso de Claude Code: %q", c)
	}
	if _, _, ok := offloadText(p, "", false); ok {
		t.Fatal("sin carpeta, una orden no va a fichero")
	}
	corta := s.assistant(tool("bash", map[string]any{"command": "ls"}, lineas(10)))
	if _, _, ok := offloadText(corta.Parts[0], "/p", false); ok {
		t.Fatal("lo corto se queda")
	}
}

// En el plan: de un turno largo, lo grande pasados los pasos protegidos va a fichero (y lo protegido no); de los
// intercambios anteriores, las lecturas grandes y las salidas largas.
func TestOffloadPlan(t *testing.T) {
	s := &sesionPrueba{}
	s.user("Instrucciones")
	s.assistant(text("Vale."))
	s.user("Mira el registro antiguo")
	viejo := s.assistant(tool("read", map[string]any{"filePath": "/p/viejo.log"}, lineas(400)), tool("bash", map[string]any{"command": "git log"}, lineas(100)))
	s.assistant(text("Visto."))
	s.user("Ahora revisa todo")
	var ords []*ocPart
	for i := 0; i < 14; i++ {
		tl := tool("bash", map[string]any{"command": fmt.Sprintf("prueba %d", i)}, lineas(300))
		tl["callID"] = fmt.Sprintf("call_b%02d", i)
		ords = append(ords, s.assistant(tl).Parts[0])
	}
	pequeña := s.assistant(tool("bash", map[string]any{"command": "pequeña"}, lineas(5))).Parts[0]
	for i := 0; i < pruneProtectSteps; i++ {
		s.assistant(text("sigo"))
	}
	plan := planPrune(s.msgs, nil, pruneYoung, time.Time{})
	offloadPlan(&plan, s.msgs, "/p/.pgocode/salidas", false)
	// Las del turno con vigencia por debajo del corte y más de los pasos protegidos: a fichero.
	a := 0
	for _, p := range ords {
		e, ok := plan.Edits[p.ID]
		switch {
		case ok && e.Clear:
		case ok && e.Offload:
			a++
			if e.File == "" || !strings.Contains(*e.Output, e.File) {
				t.Fatalf("sin ruta: %+v", e)
			}
		default:
			t.Fatalf("%s (%d caracteres) sigue entera", p.v.State.Input, len(p.output()))
		}
	}
	if a == 0 {
		t.Fatal("ninguna a fichero")
	}
	if _, ok := plan.Edits[pequeña.ID]; ok {
		t.Fatal("lo pequeño no se toca")
	}
	for _, p := range viejo.Parts {
		if e := plan.Edits[p.ID]; !e.Clear && !e.Offload {
			t.Fatalf("lo antiguo, fuera o a fichero: %+v", e)
		}
	}
	// Lo de los últimos pasos protegidos no se toca aunque sea enorme.
	s.assistant(tool("bash", map[string]any{"command": "enorme"}, lineas(2000)))
	plan = planPrune(s.msgs, nil, pruneYoung, time.Time{})
	offloadPlan(&plan, s.msgs, "/p/.pgocode/salidas", false)
	if _, ok := plan.Edits[s.msgs[len(s.msgs)-1].Parts[0].ID]; ok {
		t.Fatal("una salida del último paso no se toca")
	}
	// De un intercambio anterior que sigue en el contexto: la lectura grande y la salida larga, a fichero; sin
	// carpeta, la salida se recorta como antes (sin ruta) y la lectura, igual que con ella.
	r := &sesionPrueba{}
	r.user("Instrucciones")
	r.assistant(text("Vale."))
	r.user("Mira el registro antiguo")
	ant := r.assistant(tool("read", map[string]any{"filePath": "/p/viejo.log"}, lineas(400)), tool("bash", map[string]any{"command": "git log"}, lineas(100)))
	r.assistant(text("Visto."))
	r.user("¿Y el registro de hoy?")
	r.assistant(text("Igual."))
	r.user("Gracias")
	r.assistant(text("De nada."))
	plan = planPrune(r.msgs, nil, pruneYoung, time.Time{})
	offloadPlan(&plan, r.msgs, "/p/.pgocode/salidas", false)
	if e := plan.Edits[ant.Parts[0].ID]; !e.Offload || e.File != "" || !strings.Contains(*e.Output, "Read /p/viejo.log with offset/limit") {
		t.Fatalf("lectura antigua: %+v", e)
	}
	if e := plan.Edits[ant.Parts[1].ID]; !e.Offload || e.File == "" {
		t.Fatalf("salida larga antigua: %+v", e)
	}
	plan = planPrune(r.msgs, nil, pruneYoung, time.Time{})
	offloadPlan(&plan, r.msgs, "", false)
	if e := plan.Edits[ant.Parts[1].ID]; e.Offload || e.Output == nil {
		t.Fatalf("sin carpeta: %+v", e)
	}
	if e := plan.Edits[ant.Parts[0].ID]; !e.Offload {
		t.Fatalf("sin carpeta, la lectura: %+v", e)
	}
}

// decideSweep: lo que va a fichero cuenta como lo demás que sale, y aplicar el mismo plan otra vez no cambia nada.
func TestOffloadSweepIdempotent(t *testing.T) {
	s, _ := longTurn(0)
	for i := 0; i < 12; i++ {
		tl := tool("bash", map[string]any{"command": fmt.Sprintf("go test %d", i)}, lineas(600))
		tl["callID"] = fmt.Sprintf("call_t%02d", i)
		s.assistant(tl)
	}
	in := sweepInput{Win: s.msgs, Young: pruneYoung, Now: time.Now(), K: defaultVig, Msg: "sigue", Dir: "/p/.pgocode/salidas"}
	sin := in
	sin.Dir = ""
	con, antes := decideSweep(in), decideSweep(sin)
	if con.D.Saved <= antes.D.Saved {
		t.Fatalf("a fichero ahorra más: %d frente a %d", con.D.Saved, antes.D.Saved)
	}
	// Como si se hubiera aplicado: la parte actual es la nueva.
	for _, m := range s.msgs {
		for _, p := range m.Parts {
			if nuevo, ok := con.D.Update[p.ID]; ok {
				p.Current = nuevo
			}
		}
	}
	if otra := decideSweep(in); otra.D.changes() {
		t.Fatalf("otra vez, sin cambios: %+v", otra.D)
	}
}

// writeOffloads: la salida completa al fichero, con .pgocode/ fuera de git; lo que ya está no se reescribe.
func TestWriteOffloads(t *testing.T) {
	carpeta := t.TempDir()
	if exec.Command("git", "-C", carpeta, "init", "-q").Run() != nil {
		t.Skip("sin git")
	}
	s := &sesionPrueba{}
	s.user("x")
	m := s.assistant(tool("bash", map[string]any{"command": "make"}, lineas(400)))
	p := m.Parts[0]
	dir := offloadDirOf(carpeta)
	txt, file, _ := offloadText(p, dir, false)
	plan := prunePlan{Edits: map[string]pruneEdit{p.ID: {Output: &txt, Offload: true, File: file}}}
	viejo := filepath.Join(dir, "vieja.txt")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(viejo, []byte("x"), 0o600)
	hace := time.Now().AddDate(0, 0, -offloadKeepDays-1)
	os.Chtimes(viejo, hace, hace)
	if err := writeOffloads(plan, s.msgs, map[string]string{p.ID: "nuevo"}); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(file); err != nil || string(b) != p.output() {
		t.Fatalf("salida completa: %v", err)
	}
	if _, err := os.Stat(viejo); err == nil {
		t.Fatal("las salidas antiguas se borran")
	}
	if b, _ := os.ReadFile(filepath.Join(carpeta, ".git", "info", "exclude")); !strings.Contains(string(b), ".pgocode/") {
		t.Fatalf("exclude: %q", b)
	}
	// Sin cambio de esa parte, no se escribe nada.
	os.Remove(file)
	if writeOffloads(plan, s.msgs, map[string]string{}) != nil {
		t.Fatal("error")
	}
	if _, err := os.Stat(file); err == nil {
		t.Fatal("escrita sin aplicarse")
	}
}

// Claude Code: la salida a fichero se aplica con el aviso <persisted-output>, al volver a leer el historial se
// reconoce (con su original) y se puede reponer.
func TestOffloadClaude(t *testing.T) {
	f := sampleClaude("s9")
	data := f.bytes()
	s, err := parseClaudeSession("s9", "x", data, nil)
	if err != nil {
		t.Fatal(err)
	}
	o := decideSweep(sweepInput{Win: s.Win, Young: pruneYoung, Now: f.t0.Add(5 * time.Hour), K: defaultVig, Msg: "sigue", Claude: true, Dir: "/p/.pgocode/salidas"})
	eb, er := o.Plan.Edits["toolu_bash1"], o.Plan.Edits["toolu_read2"]
	if !eb.Offload || !strings.HasPrefix(*eb.Output, "<persisted-output>") || !er.Offload || !strings.Contains(*er.Output, "Read /p/big.go with offset/limit") {
		t.Fatalf("a fichero: %+v %+v", eb, er)
	}
	for _, id := range []string{"toolu_task1", "toolu_edit1", "toolu_err1"} {
		if _, ok := o.Plan.Edits[id]; ok {
			t.Fatalf("%s no se toca", id)
		}
	}
	var clear []string
	texts := map[string]string{}
	for id := range o.D.Update {
		clear = append(clear, id)
		if e := o.Plan.Edits[id]; e.Offload {
			texts[id] = *e.Output
		}
	}
	out, orig, err := applyClaude(s, clear, nil, texts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "persisted-output") {
		t.Fatal("sin aviso en el historial")
	}
	s2, err := parseClaudeSession("s9", "x", out, orig)
	if err != nil {
		t.Fatal(err)
	}
	b := partByID(s2.Win, "toolu_bash1")
	if b.Current == "" || len(b.output()) != len(strings.Repeat("luis 1234 0.0 proceso de prueba con salida larga\n", 400)) {
		t.Fatalf("releído: el original y la actual: %d %q", len(b.output()), b.Current)
	}
	o2 := decideSweep(sweepInput{Win: s2.Win, Young: pruneYoung, Now: f.t0.Add(5 * time.Hour), K: defaultVig, Msg: "sigue", Claude: true, Dir: "/p/.pgocode/salidas"})
	if o2.D.changes() {
		t.Fatalf("otra vez, sin cambios: %+v", o2.D)
	}
	back, _, err := applyClaude(s2, nil, []string{"toolu_bash1"}, nil)
	if err != nil || strings.Count(string(back), "persisted-output") >= strings.Count(string(out), "persisted-output") {
		t.Fatalf("repuesta: %v", err)
	}
}
