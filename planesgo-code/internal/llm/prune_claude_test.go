package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Constructor de un historial de Claude Code de prueba (con la forma de la versión 2.1.308).
type claudeFixture struct {
	ses     string
	lines   []map[string]any
	parent  any
	n       int
	t0      time.Time
	version string
}

func newClaudeFixture(ses string) *claudeFixture {
	return &claudeFixture{ses: ses, t0: time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC), version: "2.1.308"}
}

func (f *claudeFixture) add(m map[string]any) string {
	f.n++
	u := fmt.Sprintf("00000000-0000-4000-8000-%012d", f.n)
	m["uuid"], m["parentUuid"], m["sessionId"], m["version"] = u, f.parent, f.ses, f.version
	m["isSidechain"] = false
	m["timestamp"] = f.t0.Add(time.Duration(f.n) * time.Minute).Format(time.RFC3339Nano)
	f.parent = u
	f.lines = append(f.lines, m)
	return u
}

func (f *claudeFixture) user(content any) string {
	return f.add(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": content}})
}

// call: una llamada del agente con sus bloques (una línea por bloque, con el mismo message.id).
func (f *claudeFixture) call(ctx int, blocks ...map[string]any) string {
	id := fmt.Sprintf("msg_%d", f.n)
	u := ""
	for _, b := range blocks {
		u = f.add(map[string]any{"type": "assistant", "requestId": "req_1", "message": map[string]any{"id": id, "role": "assistant", "type": "message",
			"model": "claude-haiku-5-5", "content": []any{b}, "usage": map[string]any{"input_tokens": 3, "cache_read_input_tokens": ctx, "cache_creation_input_tokens": 100, "output_tokens": 50}}})
	}
	return u
}

func (f *claudeFixture) result(id string, content any, isError bool) string {
	return f.add(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": []any{
		map[string]any{"type": "tool_result", "tool_use_id": id, "content": content, "is_error": isError}}},
		"toolUseResult": map[string]any{"type": "text", "file": map[string]any{"content": "copia para la interfaz"}}})
}

func (f *claudeFixture) bytes() []byte {
	var b bytes.Buffer
	b.WriteString(`{"type":"queue-operation","operation":"enqueue","sessionId":"` + f.ses + `"}` + "\n")
	for _, l := range f.lines {
		x, _ := json.Marshal(l)
		b.Write(x)
		b.WriteByte('\n')
	}
	b.WriteString(`{"type":"api-request","sessionId":"` + f.ses + `"}` + "\n")
	return b.Bytes()
}

func useBlock(id, name string, input map[string]any) map[string]any {
	return map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}
}

func textBlock(t string) map[string]any { return map[string]any{"type": "text", "text": t} }

var bigFile = strings.Repeat("func f() { return 1234567890 } // línea de prueba\n", 2000) // ~100 KB

// sampleClaude: seis intercambios. El 1.º lee big.go (que se relee en el 4.º), edita, lanza un subagente y una
// orden que falla; el 2.º trae una imagen pegada; el 3.º, una salida grande de Bash.
func sampleClaude(ses string) *claudeFixture {
	f := newClaudeFixture(ses)
	f.user("Lee big.go y dime qué devuelve f.")
	f.call(15000, useBlock("toolu_read1", "Read", map[string]any{"file_path": "/p/big.go"}))
	f.result("toolu_read1", bigFile, false)
	f.call(45000, useBlock("toolu_edit1", "Edit", map[string]any{"file_path": "/p/big.go", "old_string": "a", "new_string": "b"}))
	f.result("toolu_edit1", "The file /p/big.go has been updated.", false)
	f.call(45500, useBlock("toolu_task1", "Task", map[string]any{"description": "explora", "prompt": "mira el proyecto"}))
	f.result("toolu_task1", []any{textBlock(strings.Repeat("informe del subagente ", 400))}, false)
	f.call(46000, useBlock("toolu_err1", "Bash", map[string]any{"command": "make prueba"}))
	f.result("toolu_err1", strings.Repeat("error: algo falla\n", 200), true)
	f.call(47000, textBlock("Devuelve 1234567890."))
	f.user([]any{textBlock("Mira esta captura"), map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": strings.Repeat("A", 8000)}}})
	f.call(49000, textBlock("Es una captura de la pantalla de entrada."))
	f.user("Lista los procesos")
	f.call(50000, useBlock("toolu_bash1", "Bash", map[string]any{"command": "ps aux"}))
	f.result("toolu_bash1", strings.Repeat("luis 1234 0.0 proceso de prueba con salida larga\n", 400), false)
	f.call(57000, textBlock("Hay 400 procesos."))
	f.user("Vuelve a leer big.go")
	f.call(58000, useBlock("toolu_read2", "Read", map[string]any{"file_path": "/p/big.go"}))
	f.result("toolu_read2", bigFile, false)
	f.call(88000, textBlock("Sigue igual."))
	f.user("¿Qué devolvía f al principio?")
	f.call(89000, textBlock("1234567890."))
	f.user("Gracias, eso es todo por ahora.")
	f.call(89500, textBlock("De nada."))
	return f
}

func TestParseClaudeSession(t *testing.T) {
	f := sampleClaude("s1")
	s, err := parseClaudeSession("s1", "x", f.bytes(), nil)
	if err != nil {
		t.Fatal(err)
	}
	xs := pruneExchanges(s.Win)
	if len(xs) != 6 {
		t.Fatalf("intercambios: %d", len(xs))
	}
	if xs[0].userText() != "Lee big.go y dime qué devuelve f." || xs[0].answerText() != "Devuelve 1234567890." {
		t.Errorf("primer intercambio: %q / %q", xs[0].userText(), xs[0].answerText())
	}
	// El punto de cada intercambio es el uuid de la última línea del agente (el «punto» de la interfaz).
	last := f.lines[len(f.lines)-1]["uuid"]
	if xs[5].Point != last {
		t.Errorf("punto %q, quiero %q", xs[5].Point, last)
	}
	p := partByID(s.Win, "toolu_read1")
	if p.v.Tool != "read" || p.filePath() != "/p/big.go" || !p.isTool() || len(p.output()) != len(bigFile) {
		t.Errorf("lectura: %+v", p.v)
	}
	if partByID(s.Win, "toolu_err1").isTool() {
		t.Error("un error no se toca")
	}
	if img := partByID(s.Win, imageID(f.lines[10]["uuid"].(string), 1)); len(img.v.State.Attachments) != 1 {
		t.Errorf("imagen pegada: %+v", img.v)
	}
}

func TestClaudeVersionAndFormat(t *testing.T) {
	f := sampleClaude("s2")
	f.lines[3]["version"] = "2.2.0"
	if _, err := parseClaudeSession("s2", "x", f.bytes(), nil); err == nil || !strings.Contains(err.Error(), "versión") {
		t.Errorf("versión desconocida: %v", err)
	}
	g := sampleClaude("s3")
	b := append(g.bytes(), []byte("{no es json\n")...)
	if _, err := parseClaudeSession("s3", "x", b, nil); err == nil {
		t.Error("una línea rota: no se poda")
	}
	h := sampleClaude("s4")
	h.lines[5]["parentUuid"] = "no-existe"
	if _, err := parseClaudeSession("s4", "x", h.bytes(), nil); err == nil {
		t.Error("cadena rota: no se poda")
	}
}

// Lo que se aplica en Claude Code: vaciar la lectura superada, la imagen ya vista y lo antiguo; nunca la edición, el
// informe del subagente ni el error. Las demás líneas, byte a byte.
func TestClaudePlanApply(t *testing.T) {
	f := sampleClaude("s5")
	data := f.bytes()
	s, err := parseClaudeSession("s5", "x", data, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := claudePlan(planPrune(s.Win, nil, pruneYoung, f.t0.Add(5*time.Hour)), s.Win)
	img := imageID(f.lines[10]["uuid"].(string), 1)
	for _, id := range []string{"toolu_read1", img} {
		if e, ok := plan.Edits[id]; !ok || !e.Clear {
			t.Errorf("%s: %+v", id, plan.Edits[id])
		}
	}
	for _, id := range []string{"toolu_edit1", "toolu_task1", "toolu_err1", "toolu_read2"} {
		if _, ok := plan.Edits[id]; ok {
			t.Errorf("%s no se toca", id)
		}
	}
	d := pruneChanges(s.Win, plan)
	if d.Saved < 20000 || len(d.Delete) > 0 {
		t.Fatalf("diff: %+v", d.Saved)
	}
	var clear []string
	for id := range d.Update {
		clear = append(clear, id)
	}
	out, orig, err := applyClaude(s, clear, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, b := bytes.Split(data, []byte("\n")), bytes.Split(out, []byte("\n"))
	if len(a) != len(b) {
		t.Fatalf("líneas: %d → %d", len(a), len(b))
	}
	cambiadas := 0
	for i := range a {
		if bytes.Equal(a[i], b[i]) {
			continue
		}
		cambiadas++
		var x, y map[string]any
		json.Unmarshal(a[i], &x)
		json.Unmarshal(b[i], &y)
		if x["uuid"] != y["uuid"] || x["parentUuid"] != y["parentUuid"] || x["timestamp"] != y["timestamp"] {
			t.Errorf("línea %d: cadena cambiada", i)
		}
		if _, ok := y["toolUseResult"]; ok {
			t.Errorf("línea %d: sigue con toolUseResult", i)
		}
		if !bytes.Contains(b[i], []byte(ClearedText)) {
			t.Errorf("línea %d sin la marca nativa", i)
		}
	}
	if cambiadas != len(clear) || len(orig) != len(clear) {
		t.Errorf("cambiadas %d, vaciados %d, originales %d", cambiadas, len(clear), len(orig))
	}
	// La imagen pegada pasa a un bloque de texto con la marca; el texto del mensaje se queda.
	if bytes.Contains(out, []byte(`"image"`)) || !bytes.Contains(out, []byte(`"Mira esta captura","type":"text"},{"type":"text","text":"[Old tool result content cleared]"}`)) {
		i := bytes.Index(out, []byte("Mira esta"))
		t.Errorf("imagen pegada sin vaciar: %s", out[max(i-100, 0):min(i+200, len(out))])
	}
	// Releído: lo vaciado está fuera y, con sus originales, se puede reponer (y queda como estaba).
	s2, err := parseClaudeSession("s5", "x", out, orig)
	if err != nil {
		t.Fatal(err)
	}
	p := partByID(s2.Win, "toolu_read1")
	if p.Current == "" || !appliedOut(p) || len(p.output()) != len(bigFile) {
		t.Errorf("releída: current=%q", p.Current)
	}
	// Un plan que ya no lo quita: se repone.
	d2 := pruneChanges(s2.Win, prunePlan{Edits: map[string]pruneEdit{}})
	if len(d2.Restore) != len(clear) || d2.Saved >= 0 {
		t.Fatalf("reponer: %+v", d2.Restore)
	}
	back, _, err := applyClaude(s2, nil, d2.Restore)
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range bytes.Split(back, []byte("\n")) {
		var x, y any
		json.Unmarshal(l, &x)
		json.Unmarshal(a[i], &y)
		if fmt.Sprint(x) != fmt.Sprint(y) {
			t.Errorf("línea %d no vuelve a su original", i)
		}
	}
	// Idempotente: con lo mismo vaciado, nada que cambiar.
	plan2 := claudePlan(planPrune(s2.Win, nil, pruneYoung, f.t0.Add(5*time.Hour)), s2.Win)
	if d3 := pruneChanges(s2.Win, plan2); len(d3.Update) > 0 || len(d3.Restore) > 0 {
		t.Errorf("segundo barrido: %d cambios, %d repuestos", len(d3.Update), len(d3.Restore))
	}
}

// claudeCache1h: la caché a 1 h (con la que se midió la receta).
var claudeCache1h = claudeCacheCost{TTL: time.Hour, Write: 2.0}

// La fórmula de la caché con los números de la receta (Y = 100k, X₁ = 30k).
func TestClaudeCacheWorth(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	s := &claudeSession{Pos: map[string]int{"a": 20000}, Cache: 95000, LastCall: now.Add(-10 * time.Minute)}
	d := pruneDiff{Saved: 30000, Update: map[string]string{"a": ""}}
	if ok, why := claudeCacheWorth(s, d, map[string]int{"a": 30000}, 0, now, claudeCache1h); ok {
		t.Errorf("P = 20k con caché caliente: no compensa (%s)", why)
	}
	s.Pos["a"] = 60000
	if ok, why := claudeCacheWorth(s, d, map[string]int{"a": 30000}, 0, now, claudeCache1h); !ok {
		t.Errorf("P = 60k: compensa en unas 2 llamadas (%s)", why)
	}
	// Con lo fijo del agente conocido (15k), se reescribe todo lo de después del sistema: ya no compensa.
	if ok, why := claudeCacheWorth(s, d, map[string]int{"a": 30000}, 15000, now, claudeCache1h); ok {
		t.Errorf("P = 60k pero sistema 15k: se reescribe desde 15k, no compensa (%s)", why)
	}
	// Lo del último turno (después de C): gratis.
	s.Pos["a"], s.Cache = 96000, 95000
	if ok, why := claudeCacheWorth(s, d, map[string]int{"a": 30000}, 0, now, claudeCache1h); !ok || !strings.Contains(why, "primera") {
		t.Errorf("después de C: %v %s", ok, why)
	}
	// Caliente y poco: no. Fría: sí, aunque sea poco.
	s.Pos["a"] = 20000
	small := pruneDiff{Saved: 5000, Update: map[string]string{"a": ""}}
	if ok, _ := claudeCacheWorth(s, small, map[string]int{"a": 5000}, 0, now, claudeCache1h); ok {
		t.Error("caliente y 5k: no")
	}
	s.LastCall = now.Add(-2 * time.Hour)
	if ok, _ := claudeCacheWorth(s, small, map[string]int{"a": 5000}, 0, now, claudeCache1h); !ok {
		t.Error("fría: sí")
	}
}

// claudeTestEnv: una carpeta de Claude Code y otra de pgocode de prueba, con el historial de muestra.
func claudeTestEnv(t *testing.T, ses string) (string, *claudeFixture) {
	home, datos := t.TempDir(), t.TempDir()
	viejoHome, viejoDir, viejoNow, viejoBusy := claudeHomeFunc, pruneDirFunc, pruneNow, claudeBusyFunc
	t.Cleanup(func() {
		// Lo que dejó en cola el recolector (el marcado al acabar el turno), antes de restaurar: si no, escribiría
		// en la carpeta de datos de verdad.
		drainCollectors()
		claudeHomeFunc, pruneDirFunc, pruneNow, claudeBusyFunc = viejoHome, viejoDir, viejoNow, viejoBusy
	})
	claudeHomeFunc = func() string { return home }
	pruneDirFunc = func() string { return datos }
	claudeBusyFunc = func(string) bool { return false }
	f := sampleClaude(ses)
	pruneNow = func() time.Time { return f.t0.Add(5 * time.Hour) }
	dir := filepath.Join(home, "projects", "-p")
	os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, ses+".jsonl")
	if err := os.WriteFile(path, f.bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, f
}

func TestSweepClaude(t *testing.T) {
	ses := "11111111-2222-4333-8444-555555555555"
	path, f := claudeTestEnv(t, ses)
	p := Peticion{SesionAgente: ses, Prune: PruneRules, Mensajes: []Mensaje{{Rol: "user", Texto: "sigue"}}}
	rep, bak := pruneClaude(context.Background(), p)
	if rep == nil || bak == nil || rep.Removed < 20000 || !rep.Podable {
		t.Fatalf("barrido: %+v %v", rep, bak)
	}
	now, _ := os.ReadFile(path)
	if !bytes.Contains(now, []byte(ClearedText)) {
		t.Fatal("el historial no ha cambiado")
	}
	if old, _ := os.ReadFile(bak.copy); !bytes.Equal(old, f.bytes()) {
		t.Fatal("la copia no es el original")
	}
	// La vigencia la ve la interfaz: la lectura superada, fuera.
	fuera := false
	for _, pc := range rep.Pieces {
		if pc.ID == "toolu_read1" {
			fuera = pc.Out && pc.Superseded != ""
		}
	}
	if !fuera {
		t.Errorf("piezas: %+v", rep.Pieces)
	}
	// La copia restaura el original.
	if err := bak.restore(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); !bytes.Equal(b, f.bytes()) {
		t.Fatal("restaurar")
	}
	// Con la sesión abierta en otro proceso, no se toca.
	claudeBusyFunc = func(string) bool { return true }
	rep, bak = pruneClaude(context.Background(), p)
	if bak != nil || rep.Removed != 0 || rep.Pending < 20000 {
		t.Errorf("abierta: %+v", rep)
	}
	// El estado del recolector, sin esperar.
	r := VigenciaReport(context.Background(), ses, false)
	if r == nil || r.Status == nil || r.Status.Phase != "" || r.Status.Removed < 20000 {
		t.Errorf("estado: %+v", r)
	}
}

// Si Claude Code no acepta el historial podado, se restaura la copia y se repite sin podar (con un «claude» falso
// que falla si encuentra la marca).
func TestChatClaudeRestoresBackup(t *testing.T) {
	ses := "22222222-2222-4333-8444-555555555555"
	path, f := claudeTestEnv(t, ses)
	bin := t.TempDir()
	script := `#!/bin/sh
f="` + path + `"
if grep -q "Old tool result content cleared" "$f"; then echo "No conversation found with session ID" >&2; exit 1; fi
echo '{"type":"system","subtype":"init","session_id":"` + ses + `"}'
echo '{"type":"assistant","uuid":"u-final","message":{"id":"m1","content":[{"type":"text","text":"1234567890"}],"usage":{"input_tokens":1}}}'
echo '{"type":"result","subtype":"success","is_error":false,"result":"1234567890","usage":{"input_tokens":1,"output_tokens":1}}'
`
	os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755)
	a := Agentes()[0].(*Agente)
	a.Programa = filepath.Join(bin, "claude")
	p := Peticion{SesionAgente: ses, Prune: PruneRules, Carpeta: t.TempDir(), Mensajes: []Mensaje{{Rol: "user", Texto: "¿qué devolvía f?"}}}
	var texto strings.Builder
	podado := 0
	err := a.Chat(context.Background(), p, func(e Evento) {
		texto.WriteString(e.Texto)
		if e.Prune != nil {
			podado = e.Prune.Removed
		}
	})
	if err != nil || !strings.Contains(texto.String(), "1234567890") {
		t.Fatalf("turno: %v %q", err, texto.String())
	}
	if podado == 0 {
		t.Error("no se llegó a podar")
	}
	if b, _ := os.ReadFile(path); !bytes.Equal(b, f.bytes()) {
		t.Error("no se ha restaurado la copia")
	}
}
