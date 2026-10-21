package llm

import (
	"math"
	"os"
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
