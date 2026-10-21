package llm

import (
	"math"
	"os"
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

// Con tareas en segundo plano Claude emite varios «result», cada uno con el coste acumulado: la suma de lo
// emitido tiene que ser el coste total, no la suma de los acumulados.
func TestCosteClaudeAcumulado(t *testing.T) {
	ev := lee(t, "claude-1.jsonl", func(r *os.File, e func(Evento)) error { return leeClaudeCode(r, e) })
	n, total := 0, 0.0
	for _, e := range ev {
		if e.Uso != nil {
			n++
			total += e.Uso.Coste
		}
	}
	if n < 2 {
		t.Fatalf("la muestra debería tener varios result: %d", n)
	}
	if math.Abs(total-0.00933) > 0.00001 {
		t.Errorf("coste total %.5f, quiero 0.00933", total)
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
	if bg == nil || bg.Estado != "ok" || !bg.SegundoPlano || bg.Titulo == "" {
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
