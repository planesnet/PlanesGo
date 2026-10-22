package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// supersededOf: por qué está superada una parte en el plan («» si no lo está o no es una pieza).
func supersededOf(plan prunePlan, id string) string {
	for _, p := range plan.Pieces {
		if p.PartID == id {
			return p.F.Superseded
		}
	}
	return ""
}

// Constructores de una sesión de OpenCode de prueba.
type sesionPrueba struct {
	msgs []*ocMessage
	n    int
}

func (s *sesionPrueba) id(pref string) string {
	s.n++
	return fmt.Sprintf("%s_%04dX", pref, s.n)
}

func (s *sesionPrueba) user(texto string) *ocMessage {
	m := &ocMessage{ID: s.id("msg"), Role: "user"}
	b, _ := json.Marshal(map[string]any{"type": "text", "text": texto})
	m.Parts = append(m.Parts, newPart(s.id("prt"), m.ID, string(b), ""))
	s.msgs = append(s.msgs, m)
	return m
}

func (s *sesionPrueba) assistant(parts ...map[string]any) *ocMessage {
	m := &ocMessage{ID: s.id("msg"), Role: "assistant"}
	for _, p := range parts {
		b, _ := json.Marshal(p)
		m.Parts = append(m.Parts, newPart(s.id("prt"), m.ID, string(b), ""))
	}
	s.msgs = append(s.msgs, m)
	return m
}

func tool(nombre string, input map[string]any, output string, extra ...func(st map[string]any)) map[string]any {
	st := map[string]any{"status": "completed", "input": input, "output": output, "title": input["filePath"], "time": map[string]any{"start": 1, "end": 2}}
	for _, f := range extra {
		f(st)
	}
	return map[string]any{"type": "tool", "tool": nombre, "callID": "call_" + nombre + fmt.Sprint(len(output)), "state": st,
		"metadata": map[string]any{"vertex": map[string]any{"thoughtSignature": "FIRMA"}}}
}

func text(t string) map[string]any      { return map[string]any{"type": "text", "text": t} }
func reasoning(t string) map[string]any { return map[string]any{"type": "reasoning", "text": t} }
func lineas(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "línea %d de un fichero bastante largo con código\n", i)
	}
	return b.String()
}

func conImagen(st map[string]any) {
	st["attachments"] = []any{map[string]any{"type": "file", "mime": "image/png", "url": "data:image/png;base64,AAAA"}}
}

func parteDe(t *testing.T, msgs []*ocMessage, tipo, herr string, n int) *ocPart {
	t.Helper()
	for _, m := range msgs {
		for _, p := range m.Parts {
			if p.v.Type == tipo && p.v.Tool == herr {
				if n == 0 {
					return p
				}
				n--
			}
		}
	}
	t.Fatalf("no hay %s %s", tipo, herr)
	return nil
}

// Una conversación de cinco intercambios: el primero lee app.go; el segundo lo vuelve a leer, ve una imagen, repite
// una orden con salida larga y edita; los demás, otras cosas.
func conversacion(n ...int) *sesionPrueba {
	app := 300
	if len(n) > 0 {
		app = n[0]
	}
	s := &sesionPrueba{}
	s.user("Lee app.go y explícame el arranque")
	s.assistant(reasoning("pienso en el arranque"), tool("read", map[string]any{"filePath": "/p/app.go"}, lineas(app)), text("Arranca en main."))
	s.user("Mira la captura y arregla el fallo de compilación")
	s.assistant(reasoning("veo la imagen"), tool("read", map[string]any{"filePath": "/p/captura.png"}, "Image read successfully", conImagen),
		tool("bash", map[string]any{"command": "go build ./..."}, lineas(200)),
		tool("read", map[string]any{"filePath": "/p/app.go"}, lineas(min(app, 300))),
		tool("edit", map[string]any{"filePath": "/p/app.go", "oldString": lineas(20), "newString": lineas(25)}, "Edit applied successfully."),
		tool("bash", map[string]any{"command": "go build ./..."}, "ok"))
	s.assistant(text("Arreglado."))
	s.user("Cambia el color del botón de la cabecera")
	s.assistant(tool("read", map[string]any{"filePath": "/p/web/cabecera.css"}, lineas(150)), text("Hecho."))
	s.user("Añade una prueba del arranque")
	s.assistant(reasoning("solo razono"))
	s.assistant(tool("read", map[string]any{"filePath": "/p/app_test.go"}, lineas(100)), text("Añadida."))
	s.user("Sube la versión")
	s.assistant(tool("read", map[string]any{"filePath": "/p/VERSION"}, "1.0"), text("Subida."))
	return s
}

func TestPruneRules(t *testing.T) {
	s := conversacion()
	plan := planPrune(s.msgs, nil, pruneYoung, time.Time{})
	if len(plan.Exchanges) != 5 {
		t.Fatalf("intercambios: %d", len(plan.Exchanges))
	}
	tiers := ""
	for _, x := range plan.Exchanges {
		tiers += string(x.Tier) + " "
	}
	if tiers != "L2 L2 L2 L1 L1 " {
		t.Fatalf("niveles: %s", tiers)
	}
	// La primera lectura de app.go se releyó después: fuera, con su línea.
	r := parteDe(t, s.msgs, "tool", "read", 0)
	if e := plan.Edits[r.ID]; !e.Clear || supersededOf(plan, r.ID) != "lectura superada" {
		t.Fatalf("lectura superada: %+v", e)
	}
	// La segunda lectura de app.go se editó después: también fuera.
	if e := plan.Edits[parteDe(t, s.msgs, "tool", "read", 2).ID]; !e.Clear {
		t.Fatal("la lectura editada después sigue")
	}
	// La imagen ya vista: fuera (con la marca de OpenCode, que tampoco manda sus adjuntos).
	if r := parteDe(t, s.msgs, "tool", "read", 1); !plan.Edits[r.ID].Clear || supersededOf(plan, r.ID) != "imagen ya vista" {
		e := plan.Edits[r.ID]
		t.Fatalf("imagen: %+v", e)
	}
	// La orden que se repitió: fuera; la repetida (corta) se queda.
	if r := parteDe(t, s.msgs, "tool", "bash", 0); !plan.Edits[r.ID].Clear || supersededOf(plan, r.ID) != "orden repetida" {
		e := plan.Edits[r.ID]
		t.Fatalf("orden repetida: %+v", e)
	}
	if _, ok := plan.Edits[parteDe(t, s.msgs, "tool", "bash", 1).ID]; ok {
		t.Fatal("la orden corta no se toca")
	}
	// La edición: sin el texto.
	if e := plan.Edits[parteDe(t, s.msgs, "tool", "edit", 0).ID]; !e.TrimInput {
		t.Fatal("la edición conserva su texto")
	}
	// El razonamiento, fuera; en L1 nada se toca (ni el razonamiento que es lo único de su mensaje).
	if e := plan.Edits[parteDe(t, s.msgs, "reasoning", "", 0).ID]; !e.Delete {
		t.Fatal("razonamiento antiguo")
	}
	for _, x := range plan.Exchanges[3:] {
		for _, m := range x.Messages {
			for _, p := range m.Parts {
				if _, ok := plan.Edits[p.ID]; ok {
					t.Fatalf("L1 tocado: %s", p.Data)
				}
			}
		}
	}
	// Lo superado salta por encima del corte; el intercambio sigue (sin embeddings, solo antigüedad).
	for _, p := range plan.Pieces {
		if (p.F.Superseded != "") != (p.V >= vigCut) {
			t.Fatalf("vigencia de %s: %.2f (%q)", p.PartID, p.V, p.F.Superseded)
		}
	}
	if v := plan.Exchanges[1].V; v <= 0 || v >= 1 || plan.Exchanges[0].V != 0 || plan.Exchanges[4].V != 0 {
		t.Fatalf("vigencias: %.2f %.2f %.2f", plan.Exchanges[0].V, v, plan.Exchanges[4].V)
	}
	// La lectura del CSS (no se volvió a leer) se queda.
	if _, ok := plan.Edits[parteDe(t, s.msgs, "tool", "read", 3).ID]; ok {
		t.Fatal("lectura vigente retirada")
	}
}

func TestPruneReasoningAlone(t *testing.T) {
	s := &sesionPrueba{}
	s.user("uno")
	s.assistant(reasoning("solo esto"))
	s.assistant(text("hecho"))
	for i := 0; i < 3; i++ {
		s.user("otro")
		s.assistant(text("vale"))
	}
	plan := planPrune(s.msgs, nil, pruneYoung, time.Time{})
	if _, ok := plan.Edits[parteDe(t, s.msgs, "reasoning", "", 0).ID]; ok {
		t.Fatal("un mensaje con solo razonamiento se quedaría vacío")
	}
}

func TestPruneLong(t *testing.T) {
	c, ok := cutLong(lineas(200))
	if !ok || !strings.Contains(c, "línea 0 ") || !strings.Contains(c, "línea 199 ") || strings.Contains(c, "línea 100 ") ||
		!strings.Contains(c, "[Tool output truncated for compaction: omitted ") {
		t.Fatalf("recorte: %q", c)
	}
	if _, ok := cutLong(lineas(30)); ok {
		t.Fatal("una salida corta no se recorta")
	}
	c, ok = cutLong(strings.Repeat("x", 9000))
	if !ok || len(c) > 3200 {
		t.Fatalf("una línea muy larga: %d", len(c))
	}
	s := &sesionPrueba{}
	s.user("busca")
	s.assistant(tool("grep", map[string]any{"pattern": "x"}, lineas(500)), text("ya"))
	for i := 0; i < 3; i++ {
		s.user("otro")
		s.assistant(text("vale"))
	}
	plan := planPrune(s.msgs, nil, pruneYoung, time.Time{})
	if e := plan.Edits[parteDe(t, s.msgs, "tool", "grep", 0).ID]; e.Output == nil || !strings.Contains(*e.Output, "[Tool output truncated for compaction") {
		t.Fatalf("salida larga: %+v", e)
	}
}

func TestPruneRelevance(t *testing.T) {
	s := conversacion()
	xs := pruneExchanges(s.msgs)
	sims := &pruneSims{Exchange: map[string]float64{}, Piece: map[string]float64{}, PrevTier: map[string]pruneTier{}}
	for _, x := range xs {
		sims.Exchange[x.UserID] = 0.1 // nada se parece
	}
	plan := planPrune(s.msgs, sims, pruneYoung, time.Time{})
	if plan.Exchanges[0].Tier != TierL2 || plan.Exchanges[1].Tier != TierL3 || plan.Exchanges[2].Tier != TierL3 || plan.Exchanges[3].Tier != TierL1 {
		t.Fatalf("el primero y los dos últimos no se sacan: %v %v %v %v", plan.Exchanges[0].Tier, plan.Exchanges[1].Tier, plan.Exchanges[2].Tier, plan.Exchanges[3].Tier)
	}
	if x := plan.Exchanges[2]; x.V < vigCut || x.Vig.Sim == nil || *x.Vig.Sim != 0.1 {
		t.Fatalf("vigencia: %+v", x.Vig)
	}
	// En L3, las herramientas pasan a una línea; los textos se quedan.
	css := parteDe(t, s.msgs, "tool", "read", 3)
	if e := plan.Edits[css.ID]; !e.Clear {
		t.Fatalf("L3: %+v", e)
	}
	for _, m := range plan.Exchanges[2].Messages {
		for _, p := range m.Parts {
			if _, ok := plan.Edits[p.ID]; ok && p.v.Type == "text" {
				t.Fatal("texto retirado")
			}
		}
	}
	// Vuelve a parecerse lo justo para quedar por debajo del corte: estaba en L3, así que necesita el margen.
	sims.PrevTier[xs[2].UserID] = TierL3
	limite := vigRelPivot - (vigCut-vigAge(1))/vigRelK // la relación con la que el intercambio 2 queda en 3
	sims.Exchange[xs[2].UserID] = limite + 0.01
	if planPrune(s.msgs, sims, pruneYoung, time.Time{}).Exchanges[2].Tier != TierL3 {
		t.Fatal("sin margen, vuelve")
	}
	sims.Exchange[xs[2].UserID] = limite + pruneReviveMargin + 0.01
	if planPrune(s.msgs, sims, pruneYoung, time.Time{}).Exchanges[2].Tier != TierL2 {
		t.Fatal("con margen, no vuelve")
	}
	// Sin similitudes (Ollama no responde): lo que estaba fuera sigue fuera.
	if p := planPrune(s.msgs, &pruneSims{PrevTier: map[string]pruneTier{xs[2].UserID: TierL3}}, pruneYoung, time.Time{}); p.Exchanges[2].Tier != TierL3 || p.Exchanges[1].Tier != TierL2 {
		t.Fatal("sin Ollama")
	}
	// Por piezas: en un intercambio relacionado, una lectura grande sin relación sale sola.
	sims = &pruneSims{Exchange: map[string]float64{}, Piece: map[string]float64{css.ID: 0.05}}
	for _, x := range xs {
		sims.Exchange[x.UserID] = 0.9
	}
	plan = planPrune(s.msgs, sims, pruneYoung, time.Time{})
	if e := plan.Edits[css.ID]; plan.Exchanges[2].Tier != TierL2 || !e.Clear || supersededOf(plan, css.ID) != "" {
		t.Fatalf("pieza: %v %+v", plan.Exchanges[2].Tier, e)
	}
}

func TestPruneChanges(t *testing.T) {
	s := conversacion()
	plan := planPrune(s.msgs, nil, pruneYoung, time.Time{})
	d := pruneChanges(s.msgs, plan)
	if len(d.Update) == 0 || len(d.Delete) != 2 || len(d.Restore) != 0 || d.Saved < 5000 {
		t.Fatalf("cambios: %d %d %d %d", len(d.Update), len(d.Delete), len(d.Restore), d.Saved)
	}
	// Lo cambiado conserva lo demás (la firma de Gemini de la llamada) y es JSON válido.
	r := parteDe(t, s.msgs, "tool", "read", 0)
	var v map[string]any
	if err := json.Unmarshal([]byte(d.Update[r.ID]), &v); err != nil || !strings.Contains(d.Update[r.ID], "FIRMA") ||
		v["state"].(map[string]any)["input"].(map[string]any)["filePath"] != "/p/app.go" ||
		v["state"].(map[string]any)["time"].(map[string]any)["compacted"] != 2.0 || !strings.Contains(d.Update[r.ID], "línea 299") {
		t.Fatalf("parte cambiada: %v %s", err, d.Update[r.ID])
	}
	e := parteDe(t, s.msgs, "tool", "edit", 0)
	if !strings.Contains(d.Update[e.ID], `"oldString":"[Old tool result content cleared]"`) || !strings.Contains(d.Update[e.ID], "Edit applied") {
		t.Fatalf("edición: %s", d.Update[e.ID])
	}
	// Con lo ya aplicado, no hay nada más que hacer.
	for _, m := range s.msgs {
		for _, p := range m.Parts {
			if n, ok := d.Update[p.ID]; ok {
				p.Current = n
			}
		}
	}
	if d2 := pruneChanges(s.msgs, plan); len(d2.Update) != 0 || d2.worthIt() { // las partes borradas siguen en s.msgs (en la base de datos, no)
		t.Fatalf("repetir: %d %v", len(d2.Update), d2.worthIt())
	}
	// Si el plan ya no quita una parte cambiada, se repone.
	delete(plan.Edits, r.ID)
	d3 := pruneChanges(s.msgs, plan)
	if len(d3.Restore) != 1 || d3.Restore[0] != r.ID || d3.worthIt() {
		t.Fatalf("reponer (solo, no compensa perder la caché): %+v", d3.Restore)
	}
	// Con un barrido que compensa, lo repuesto va con él.
	if !(pruneDiff{Restore: []string{"a"}, Update: map[string]string{"b": ""}, Saved: pruneMinTokens}).worthIt() {
		t.Fatal("reponer con un barrido")
	}
	// Por debajo del mínimo no compensa.
	if (pruneDiff{Update: map[string]string{"a": ""}, Saved: pruneMinTokens - 1}).worthIt() {
		t.Fatal("poco")
	}
}

func TestPruneWindow(t *testing.T) {
	s := &sesionPrueba{}
	s.user("antiguo")
	s.assistant(text("viejo"))
	cola := s.user("cola")
	s.assistant(text("sigue"))
	comp := &ocMessage{ID: s.id("msg"), Role: "user"}
	b, _ := json.Marshal(map[string]any{"type": "compaction", "auto": true, "tail_start_id": cola.ID})
	comp.Parts = []*ocPart{newPart(s.id("prt"), comp.ID, string(b), "")}
	s.msgs = append(s.msgs, comp)
	sum := s.assistant(text("resumen"))
	sum.Summary, sum.Parent = true, comp.ID
	s.user("nuevo")
	s.assistant(text("ok"))
	var ids []string
	for _, m := range pruneWindow(s.msgs) {
		ids = append(ids, m.Parts[0].v.Text)
	}
	if strings.Join(ids, ",") != "cola,sigue,nuevo,ok" {
		t.Fatalf("ventana: %v", ids)
	}
	// Sin resumen acabado, la compactación aún no cuenta.
	sum.Summary = false
	if len(pruneWindow(s.msgs)) != len(s.msgs) {
		t.Fatal("compactación sin resumen")
	}
}

func TestPruneStatements(t *testing.T) {
	grande := strings.Repeat("y", 60<<10)
	d := pruneDiff{Update: map[string]string{"prt_a": `{"x":"it's"}`, "prt_b": grande, "prt_c": grande}, Restore: []string{"prt_r"}, Delete: []string{"prt_d"}}
	qs, err := pruneStatements(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 5 { // guardar, 2 tandas de cambios, reponer, quitar
		t.Fatalf("%d sentencias: %v", len(qs), qs)
	}
	for _, q := range qs {
		if len(q) > 128<<10 {
			t.Fatalf("sentencia de %d bytes", len(q))
		}
	}
	if !strings.Contains(qs[0], "('prt_a','prt_b','prt_c','prt_d')") || !strings.Contains(qs[1], `'{"x":"it''s"}'`) ||
		!strings.HasPrefix(qs[3], "UPDATE part SET data = (SELECT o.data") || qs[4] != "DELETE FROM part WHERE id IN ('prt_d')" {
		t.Fatalf("sentencias: %.200s", qs)
	}
	if _, err := pruneStatements(pruneDiff{Delete: []string{"prt_x' OR 1=1"}}); err == nil {
		t.Fatal("id no válido")
	}
}

func TestParseSessionRows(t *testing.T) {
	out := `[{"mid":"msg_1","role":"user","summary":0,"parent":"","pid":"prt_1","data":"{\"type\":\"text\",\"text\":\"hola\"}","cur":""},
{"mid":"msg_2","role":"assistant","summary":1,"parent":"msg_1","pid":"prt_2","data":"{\"type\":\"text\",\"text\":\"a\"}","cur":"{\"type\":\"text\",\"text\":\"b\"}"},
{"mid":"msg_3","role":"assistant","summary":0,"parent":"msg_1","pid":"","data":"","cur":""}]`
	ms, err := parseSessionRows([]byte(out))
	if err != nil || len(ms) != 3 || !ms[1].Summary || ms[0].Parts[0].v.Text != "hola" || ms[1].Parts[0].Current == "" || len(ms[2].Parts) != 0 {
		t.Fatalf("%v %+v", err, ms)
	}
}

func TestShortMessageAndCosine(t *testing.T) {
	if !shortMessage("compila y actualiza") || shortMessage("añade una prueba del arranque que compruebe el puerto de escucha") {
		t.Fatal("mensaje corto")
	}
	if c := cosine([]float32{1, 0}, []float32{1, 0}); c < 0.999 {
		t.Fatal(c)
	}
	if !midTurnWorth(30000, 50000, 20, 0.1) || midTurnWorth(5000, 80000, 3, 0.1) {
		t.Fatal("entre pasos")
	}
}

// El barrido completo con la base de datos y los embeddings simulados.
func TestPruneSweep(t *testing.T) {
	s := conversacion(3000) // una primera lectura grande, que se relee después: pasa del mínimo
	// La sesión, como la devuelve «opencode db».
	var filas []map[string]any
	for _, m := range s.msgs {
		for _, p := range m.Parts {
			filas = append(filas, map[string]any{"mid": m.ID, "role": m.Role, "summary": 0, "parent": "", "pid": p.ID, "data": p.Data, "cur": ""})
		}
	}
	rows, _ := json.Marshal(filas)
	var mu sync.Mutex
	var sentencias []string
	drainCollectors()
	viejoDB, viejoEmb, viejoDir := ocDB, pruneEmbed, pruneDirFunc
	defer func() { drainCollectors(); ocDB, pruneEmbed, pruneDirFunc = viejoDB, viejoEmb, viejoDir }()
	dir := t.TempDir()
	pruneDirFunc = func() string { return dir }
	tablaPreparada.Store(datosOpenCode(), true)
	ocDB = func(_ context.Context, _ *Agente, _ Peticion, q string, asJSON bool) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		if strings.HasPrefix(q, "SELECT") {
			return rows, nil
		}
		sentencias = append(sentencias, q)
		return nil, nil
	}
	// Vectores: «arranque» se parece a lo del arranque; lo demás, no.
	pruneEmbed = func(_ context.Context, texto, _ string) ([]float32, error) {
		if strings.Contains(texto, "arranque") || strings.Contains(texto, "app.go") {
			return []float32{1, 0}, nil
		}
		return []float32{0, 1}, nil
	}
	a := &Agente{Clave: "opencode"}
	p := Peticion{Carpeta: t.TempDir(), SesionAgente: "ses_prueba1", Prune: PruneAuto, Mensajes: []Mensaje{{Rol: "user", Texto: "Explícame otra vez el arranque de la aplicación con detalle"}}}
	rep := pruneOpenCode(context.Background(), a, p)
	if rep == nil {
		t.Fatal("sin informe")
	}
	mu.Lock()
	n := len(sentencias)
	mu.Unlock()
	if rep.Removed < pruneMinTokens {
		t.Fatalf("no aplicado: %+v", rep)
	}
	{
		if n == 0 || !strings.HasPrefix(sentencias[0], "INSERT OR IGNORE INTO "+pruneTable) {
			t.Fatalf("sentencias: %v", sentencias)
		}
		var l3 bool
		for _, x := range rep.Exchanges {
			if x.Out && x.Value >= vigCut && x.Sim != nil && *x.Sim == 0 {
				l3 = true
			}
		}
		if !l3 {
			t.Fatalf("informe: %+v", rep)
		}
	}
	// El estado queda guardado con sus vectores.
	c := collectorFor("ses_prueba1")
	c.do(context.Background(), func() {})
	if len(c.state.Vectors) == 0 {
		t.Fatal("sin vectores")
	}
}

// Un segundo barrido sin cambios en la sesión no vuelve a tocar nada (y la interfaz recibe lo aplicado).
func TestPruneSweepPending(t *testing.T) {
	s := conversacion(20)
	var filas []map[string]any
	for _, m := range s.msgs {
		for _, p := range m.Parts {
			filas = append(filas, map[string]any{"mid": m.ID, "role": m.Role, "summary": 0, "parent": "", "pid": p.ID, "data": p.Data, "cur": ""})
		}
	}
	rows, _ := json.Marshal(filas)
	escritas := 0
	drainCollectors()
	viejoDB, viejoDir := ocDB, pruneDirFunc
	defer func() { drainCollectors(); ocDB, pruneDirFunc = viejoDB, viejoDir }()
	dir := t.TempDir()
	pruneDirFunc = func() string { return dir }
	tablaPreparada.Store(datosOpenCode(), true)
	ocDB = func(_ context.Context, _ *Agente, _ Peticion, q string, _ bool) ([]byte, error) {
		if strings.HasPrefix(q, "SELECT") {
			return rows, nil
		}
		escritas++
		return nil, nil
	}
	p := Peticion{Carpeta: t.TempDir(), SesionAgente: "ses_prueba2", Prune: PruneRules, Mensajes: []Mensaje{{Rol: "user", Texto: "sigue"}}}
	rep := pruneOpenCode(context.Background(), &Agente{Clave: "opencode"}, p)
	if rep == nil || rep.Removed != 0 || escritas != 0 || rep.Exchanges == nil {
		t.Fatalf("por debajo del mínimo no se toca nada: %+v, %d escrituras", rep, escritas)
	}
	if pruneOpenCode(context.Background(), &Agente{Clave: "opencode"}, Peticion{SesionAgente: "ses_prueba2", Mensajes: p.Mensajes}) != nil {
		t.Fatal("sin poda, sin informe")
	}
}
