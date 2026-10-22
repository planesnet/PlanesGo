package llm

import (
	"errors"
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
