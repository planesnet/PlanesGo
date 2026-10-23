package llm

import (
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Muestras reales de los agentes (testdata/agentes), grabadas al investigar subagentes y tareas en segundo plano.
func lee(t *testing.T, f string, lector func(r *os.File, emite func(Evento)) error) []Evento {
	t.Helper()
	fh, err := os.Open("testdata/agentes/" + f)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	var l []Evento
	lector(fh, func(e Evento) { l = append(l, e) })
	return l
}

// Al acabar la respuesta con solo una orden en segundo plano (muestra real: «sleep 20; echo FIN-BG»), el lector
// termina ahí (FinConFondo) con esa orden, en vez de esperarla y gastar el turno extra que haría Claude.
func TestClaudeFinConFondo(t *testing.T) {
	fh, _ := os.Open("testdata/agentes/claude-1.jsonl")
	defer fh.Close()
	usos := 0
	err := leeClaudeCode(fh, func(e Evento) {
		if e.Uso != nil {
			usos++
		}
	})
	var fin *FinConFondo
	if !errors.As(err, &fin) || len(fin.Ordenes) != 1 || !strings.Contains(fin.Ordenes[0].Orden, "sleep 20") || fin.Ordenes[0].ID != "b7zdr3wrl" {
		t.Fatalf("fin: %v %+v", err, fin)
	}
	if usos != 1 {
		t.Errorf("consumos: %d (solo el de la respuesta)", usos)
	}
}

// total_cost_usd es acumulado: con varios «result» se emite solo la diferencia.
func TestCosteClaudeAcumulado(t *testing.T) {
	l := `{"type":"result","total_cost_usd":0.01,"usage":{"input_tokens":1,"output_tokens":1}}
{"type":"result","total_cost_usd":0.025,"usage":{"input_tokens":1,"output_tokens":1}}
`
	total := 0.0
	leeClaudeCode(strings.NewReader(l), func(e Evento) {
		if e.Uso != nil {
			total += e.Uso.Coste
		}
	})
	if math.Abs(total-0.025) > 1e-9 {
		t.Errorf("coste %.4f, quiero 0.025", total)
	}
}

// Ninguna herramienta de Antigravity se queda «en curso» al acabar, tampoco los subagentes.
func TestAntigravitySinPasosColgados(t *testing.T) {
	for _, f := range []string{"agy-1.jsonl", "agy-2.jsonl", "agy-3-fin.jsonl"} {
		estado := map[string]string{}
		nombre := map[string]string{}
		for _, e := range lee(t, f, func(r *os.File, e func(Evento)) error { return leeAntigravity(r, e) }) {
			if h := e.Herramienta; h != nil {
				estado[h.ID] = h.Estado
				if h.Nombre != "" {
					nombre[h.ID] = h.Nombre
				}
			}
		}
		sub := false
		for id, s := range estado {
			if s == "curso" {
				t.Errorf("%s: %s (%s) sigue en curso", f, id, nombre[id])
			}
			sub = sub || nombre[id] == "invoke_subagent"
		}
		if f == "agy-2.jsonl" && !sub {
			t.Errorf("%s: no aparece el subagente", f)
		}
	}
}

// tareas: el estado final de cada tarea (fusionando las actualizaciones por ID, como la interfaz).
func tareas(ev []Evento) map[string]Tarea {
	m := map[string]Tarea{}
	for _, e := range ev {
		if e.Tarea == nil {
			continue
		}
		t, n := m[e.Tarea.ID], *e.Tarea
		for _, c := range []struct {
			d *string
			v string
		}{{&t.Tipo, n.Tipo}, {&t.Herramienta, n.Herramienta}, {&t.Titulo, n.Titulo},
			{&t.Subtipo, n.Subtipo}, {&t.Estado, n.Estado}, {&t.Progreso, n.Progreso}, {&t.Salida, n.Salida}, {&t.Detalle, n.Detalle}, {&t.SesionHija, n.SesionHija}} {
			if c.v != "" {
				*c.d = c.v
			}
		}
		t.ID = n.ID
		t.SegundoPlano = t.SegundoPlano || n.SegundoPlano
		if n.Elementos != nil {
			t.Elementos = n.Elementos
		}
		m[n.ID] = t
	}
	return m
}

func TestTareasClaude(t *testing.T) {
	ev := lee(t, "claude-1.jsonl", func(r *os.File, e func(Evento)) error { return leeClaudeCode(r, e) })
	ts := tareas(ev)
	var sub, bg *Tarea
	for _, x := range ts {
		x := x
		switch x.Tipo {
		case "subagente":
			sub = &x
		case "segundo_plano":
			bg = &x
		}
	}
	if sub == nil || sub.Estado != "ok" || sub.Subtipo != "general-purpose" || sub.Herramienta == "" || sub.Detalle == "" {
		t.Errorf("subagente: %+v", sub)
	}
	// La orden en segundo plano sigue en marcha al acabar la respuesta: la adopta pgocode.
	if bg == nil || bg.Estado != "curso" || !bg.SegundoPlano || bg.Titulo == "" {
		t.Errorf("segundo plano: %+v", bg)
	}
	// Las herramientas del subagente van dentro de él (Padre = la herramienta que lo lanzó).
	dentro := 0
	for _, e := range ev {
		if e.Herramienta != nil && e.Herramienta.Padre != "" {
			if e.Herramienta.Padre != sub.Herramienta {
				t.Errorf("herramienta con otro padre: %+v", e.Herramienta)
			}
			dentro++
		}
	}
	if dentro == 0 {
		t.Error("no hay herramientas dentro del subagente")
	}
	// Parar una tarea: queda «parada».
	ts = tareas(lee(t, "claude-2.jsonl", func(r *os.File, e func(Evento)) error { return leeClaudeCode(r, e) }))
	parada := false
	for _, x := range ts {
		parada = parada || x.Estado == "parada"
	}
	if !parada {
		t.Errorf("claude-2: ninguna tarea parada: %+v", ts)
	}
}

func TestTareasAntigravity(t *testing.T) {
	ts := tareas(lee(t, "agy-2.jsonl", func(r *os.File, e func(Evento)) error { return leeAntigravity(r, e) }))
	n := 0
	for _, x := range ts {
		if x.Tipo == "subagente" {
			n++
			if x.Estado != "ok" || x.Subtipo != "research" || x.SesionHija == "" || !strings.HasSuffix(x.Detalle, "transcript.jsonl") {
				t.Errorf("subagente: %+v", x)
			}
		}
	}
	if n != 1 {
		t.Errorf("subagentes: %d (%+v)", n, ts)
	}
}

func TestTareasOpenCode(t *testing.T) {
	ts := tareas(lee(t, "opencode-1.jsonl", func(r *os.File, e func(Evento)) error { return leeOpenCode(r, e) }))
	l, ok := ts["lista"]
	if !ok || len(l.Elementos) != 2 || l.Elementos[0].Estado != "hecho" || l.Elementos[1].Estado != "pendiente" {
		t.Errorf("lista: %+v", l)
	}
	sub := 0
	for _, x := range ts {
		if x.Tipo == "subagente" {
			sub++
			if x.SesionHija == "" || x.Salida == "" || x.Estado != "ok" {
				t.Errorf("subagente: %+v", x)
			}
		}
	}
	if sub == 0 {
		t.Error("sin subagente")
	}
}

// listasClaude: las sucesivas Tareas «lista» de una muestra de Claude Code, con la lista guardada de cada sesión
// en carpeta (vacía: nada guardado).
func listasClaude(t *testing.T, f, carpeta string) []Tarea {
	t.Helper()
	antes := carpetaTareasClaude
	t.Cleanup(func() { carpetaTareasClaude = antes })
	carpetaTareasClaude = func(s string) string { return filepath.Join(carpeta, s) }
	var l []Tarea
	for _, e := range lee(t, f, func(r *os.File, e func(Evento)) error { return leeClaudeCode(r, e) }) {
		if e.Tarea != nil && e.Tarea.Tipo == "lista" {
			l = append(l, *e.Tarea)
		}
	}
	return l
}

func estadosLista(t Tarea) string {
	var b strings.Builder
	for _, e := range t.Elementos {
		b.WriteString(e.Texto + "=" + e.Estado + ";")
	}
	return t.Estado + ":" + b.String()
}

// Claude Code 2.1 (con CLAUDE_CODE_ENABLE_TODO_TOOLS) lleva la lista con TaskCreate/TaskUpdate, a trozos: se monta
// entera en cada cambio, como la de TodoWrite.
func TestListaClaudeTaskCreate(t *testing.T) {
	ls := listasClaude(t, "claude-8-lista.jsonl", t.TempDir())
	if len(ls) != 9 { // 3 altas y 6 cambios de estado
		t.Fatalf("listas: %d", len(ls))
	}
	for i, q := range map[int]string{
		2: "ok:Crear a.txt=pendiente;Crear b.txt=pendiente;Crear c.txt=pendiente;",
		3: "curso:Crear a.txt=curso;Crear b.txt=pendiente;Crear c.txt=pendiente;",
		5: "curso:Crear a.txt=hecho;Crear b.txt=curso;Crear c.txt=pendiente;",
		8: "ok:Crear a.txt=hecho;Crear b.txt=hecho;Crear c.txt=hecho;",
	} {
		if g := estadosLista(ls[i]); g != q {
			t.Errorf("lista %d: %s, quería %s", i, g, q)
		}
	}
}

// Al continuar la sesión, la lista parte de la guardada por Claude Code (las tareas de los turnos anteriores).
func TestListaClaudeResume(t *testing.T) {
	ls := listasClaude(t, "claude-9-lista-resume.jsonl", "testdata/agentes/claude-tareas")
	q := "ok:Crear a.txt=hecho;Crear b.txt=hecho;Crear c.txt=hecho;Crear d.txt=hecho;Borrar b.txt=pendiente;"
	if len(ls) == 0 || estadosLista(ls[len(ls)-1]) != q {
		t.Fatalf("listas: %+v", ls)
	}
	// Sin la lista guardada (pregunta aparte, sesión nueva): solo lo de este turno.
	ls = listasClaude(t, "claude-9-lista-resume.jsonl", t.TempDir())
	if g := estadosLista(ls[len(ls)-1]); g != "ok:Crear d.txt=hecho;Borrar b.txt=pendiente;" {
		t.Errorf("sin guardada: %s", g)
	}
}

// TodoWrite (Claude Code anterior o con otros modelos) trae la lista entera; un TaskUpdate que falla no cambia nada.
func TestListaClaudeTodoWrite(t *testing.T) {
	l := &listaClaude{}
	tw := l.usa("t1", "TodoWrite", []byte(`{"todos":[{"content":"Uno","status":"completed","activeForm":"Uno"},{"content":"Dos","status":"in_progress","activeForm":"Dos"}]}`))
	if tw == nil || estadosLista(*tw) != "curso:Uno=hecho;Dos=curso;" {
		t.Errorf("TodoWrite: %+v", tw)
	}
	l.usa("t2", "TaskUpdate", []byte(`{"taskId":"1","status":"completed"}`))
	if x := l.resultado("t2", true, "Task not found", nil); x != nil {
		t.Errorf("TaskUpdate fallido: %+v", x)
	}
	l.usa("t3", "TaskCreate", []byte(`{"subject":"Tres","description":"x"}`))
	if x := l.resultado("t3", false, "Task #7 created successfully: Tres", nil); x == nil || estadosLista(*x) != "ok:Tres=pendiente;" {
		t.Errorf("TaskCreate sin tool_use_result: %+v", x)
	}
	l.usa("t4", "TaskUpdate", []byte(`{"taskId":"7","status":"deleted"}`))
	if x := l.resultado("t4", false, "Updated task #7 deleted", nil); x != nil {
		t.Errorf("lista vacía: %+v", x)
	}
}

// Consumo real con un subagente (muestra real): «usage» del result solo cuenta el agente principal (113.731 de
// entrada); el consumo del turno sale de «modelUsage» y lleva también el del subagente. El último «vivo»
// coincide con la suma de los consumos y cada herramienta lleva lo consumido hasta ella.
func TestClaudeConsumoConSubagente(t *testing.T) {
	var suma Uso
	var ultimo *Uso
	var herr []int
	for _, e := range lee(t, "claude-10-subagente-consumo.jsonl", func(r *os.File, emite func(Evento)) error { return leeClaudeCode(r, emite) }) {
		if e.Uso != nil {
			suma.Entrada += e.Uso.Entrada
			suma.Salida += e.Uso.Salida
		}
		if e.Vivo != nil {
			ultimo = e.Vivo
		}
		if e.Herramienta != nil && e.Herramienta.Estado == "curso" {
			herr = append(herr, e.Herramienta.Tokens)
		}
	}
	if suma.Entrada != 170275 || suma.Salida != 608 {
		t.Errorf("consumo del turno: %+v", suma)
	}
	if ultimo == nil || ultimo.Entrada != suma.Entrada || ultimo.Salida != suma.Salida {
		t.Errorf("último vivo %+v, consumo %+v", ultimo, suma)
	}
	if len(herr) != 2 || herr[0] <= 0 || herr[1] <= herr[0] {
		t.Errorf("tokens de las herramientas: %v", herr)
	}
}

// Sesión retomada con --resume (muestra real, haiku): «modelUsage» y «total_cost_usd» son lo acumulado de toda la
// sesión, no del proceso. El segundo turno ha de contar solo lo suyo (3.447 de entrada y 43 de salida), con el
// acumulado guardado del turno anterior y también sin él (sesión de antes de guardarlo).
func TestClaudeConsumoAlRetomar(t *testing.T) {
	turno := func(f string) Uso {
		var u Uso
		for _, e := range lee(t, f, func(r *os.File, emite func(Evento)) error { return leeClaudeCode(r, emite) }) {
			if e.Uso != nil {
				u.Entrada += e.Uso.Entrada
				u.Salida += e.Uso.Salida
				u.Coste += e.Uso.Coste
			}
		}
		return u
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	acumulados = nil
	if u := turno("claude-11-resume-1.jsonl"); u.Entrada != 3361 || u.Salida != 55 || math.Abs(u.Coste-6.129e-05) > 1e-9 {
		t.Errorf("primer turno: %+v", u)
	}
	if u := turno("claude-11-resume-2.jsonl"); u.Entrada != 3447 || u.Salida != 43 || math.Abs(u.Coste-(0.00011744-6.129e-05)) > 1e-9 {
		t.Errorf("segundo turno, con el acumulado guardado: %+v", u)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // sin acumulado guardado
	acumulados = nil
	u := turno("claude-11-resume-2.jsonl")
	if u.Entrada != 3447 || u.Salida != 43 {
		t.Errorf("segundo turno, sin acumulado: %+v", u)
	}
	if real := 0.00011744 - 6.129e-05; math.Abs(u.Coste-real) > real*0.25 { // proporcional: aproximado
		t.Errorf("coste sin acumulado %.8f, real %.8f", u.Coste, real)
	}
}

// Compactación automática (muestras reales, haiku, Claude Code 2.1.308, umbral de 20k por entorno): la sesión
// sigue con el mismo id, «modelUsage» y «total_cost_usd» siguen acumulando y llevan la llamada del resumen, que no
// sale en la salida. Con el acumulado guardado, el turno que compacta cuenta su resumen (46.681 de entrada, aunque
// la única llamada visible sea de 5.775); y una sesión nueva que compacta a mitad de turno lo cuenta todo
// (80.268, de los que solo se ven 50.570).
func TestClaudeConsumoAlCompactar(t *testing.T) {
	turno := func(f string, read func(io.Reader, func(Evento)) error) Uso {
		var u Uso
		for _, e := range lee(t, f, func(r *os.File, emite func(Evento)) error { return read(r, emite) }) {
			if e.Uso != nil {
				u.Entrada += e.Uso.Entrada
				u.Salida += e.Uso.Salida
				u.Coste += e.Uso.Coste
				u.Contexto = e.Uso.Contexto
			}
		}
		return u
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	acumulados = nil
	if u := turno("claude-12-compacta-1.jsonl", leeClaudeCodeNew); u.Entrada != 71144 || u.Salida != 490 || !near(u.Coste, 0.00545779) {
		t.Errorf("primer turno: %+v", u)
	}
	if u := turno("claude-12-compacta-2.jsonl", leeClaudeCode); u.Entrada != 46681 || u.Salida != 1487 ||
		!near(u.Coste, 0.0080869-0.00545779) || u.Contexto != 5775 {
		t.Errorf("turno que compacta: %+v", u)
	}
	if u := turno("claude-12-compacta-3.jsonl", leeClaudeCode); u.Entrada != 6094 || u.Salida != 7 || !near(u.Coste, 0.00857313-0.0080869) {
		t.Errorf("turno después de compactar: %+v", u)
	}
	acumulados = nil
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if u := turno("claude-13-compacta-nueva.jsonl", leeClaudeCodeNew); u.Entrada != 80268 || u.Salida != 3431 || !near(u.Coste, 0.00922649) {
		t.Errorf("sesión nueva que compacta: %+v", u)
	}
}

// agy 1.4.6 en accept-edits sin consola (muestra real): la orden se da por acabada sin salida, la respuesta llega
// vacía y el resultado trae denied_actions «command». La herramienta sale en error y hay aviso.
func TestAntigravityDenegada(t *testing.T) {
	var h *Herramienta
	texto := ""
	for _, e := range lee(t, "agy-4-denegada.jsonl", func(r *os.File, e func(Evento)) error { return leeAntigravity(r, e) }) {
		if e.Herramienta != nil {
			h = e.Herramienta
		}
		texto += e.Texto
	}
	if h == nil || h.Estado != "error" || !strings.Contains(h.Salida, "no permite órdenes") || h.Nombre != "run_command" {
		t.Errorf("herramienta: %+v", h)
	}
	if !strings.Contains(texto, "sin responder") || !strings.Contains(texto, "RunCommand") {
		t.Errorf("aviso: %q", texto)
	}
	// Con respuesta, sin aviso.
	texto = ""
	for _, e := range lee(t, "agy-1.jsonl", func(r *os.File, e func(Evento)) error { return leeAntigravity(r, e) }) {
		texto += e.Texto
	}
	if strings.Contains(texto, "sin responder") {
		t.Errorf("aviso de más: %q", texto)
	}
}

// Los bloques de texto de un turno (p. ej. antes y después de los subagentes en segundo plano) van separados por una
// línea en blanco: sin ella se pegan («…cuando lleguen.El informe…») y «## Informe» no es un encabezado.
func TestBloquesDeTextoSeparados(t *testing.T) {
	l := `{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"Lanzo los subagentes; sigo cuando lleguen."}]}}
{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"a"}]}}
{"type":"assistant","message":{"id":"m2","content":[{"type":"text","text":"## Informe\nTodo bien."}]}}
{"type":"assistant","message":{"id":"m2","content":[{"type":"text","text":"\n` + "`devolucion`" + ` ya existía."}]}}
`
	texto := ""
	leeClaudeCode(strings.NewReader(l), func(e Evento) { texto += e.Texto })
	if want := "Lanzo los subagentes; sigo cuando lleguen.\n\n## Informe\nTodo bien.\n\n`devolucion` ya existía."; texto != want {
		t.Errorf("claude: %q", texto)
	}
	var b textBlocks
	if r := b.next("a") + b.next("  ") + b.note("\n\n_(aviso)_\n\n") + b.next("b") + b.next("c"); r != "a\n\n_(aviso)_\n\nb\n\nc" {
		t.Errorf("bloques: %q", r)
	}
}
