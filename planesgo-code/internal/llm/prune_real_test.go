package llm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPruneReal: simulación sobre una sesión real de OpenCode (en una COPIA de su base de datos: HOME apunta a una
// carpeta con .local/share/pgocode/opencode/opencode.db). Para cada mensaje del usuario, lo que la poda quitaría
// del historial anterior. PGO_PRUEBA_PODA=<sesión> HOME=<copia> go test -run TestPruneReal -v ./internal/llm
func TestPruneReal(t *testing.T) {
	ses := os.Getenv("PGO_PRUEBA_PODA")
	if ses == "" {
		t.Skip()
	}
	var a *Agente
	for _, p := range Agentes() {
		if x, ok := p.(*Agente); ok && x.Clave == "opencode" {
			a = x
		}
	}
	dir := t.TempDir()
	pruneDirFunc = func() string { return dir }
	p := Peticion{Carpeta: os.TempDir(), Prune: PruneAuto}
	ctx, cancela := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancela()
	msgs, err := loadOpenCodeSession(ctx, a, p, ses)
	if err != nil {
		t.Fatal(err)
	}
	c := &pruneCollector{key: ses}
	c.load()
	var xs []*pruneExchange
	for _, x := range pruneExchanges(msgs) {
		if x.Messages[0].Parts != nil && x.Messages[0].Parts[0].v.Type != "compaction" && x.userText() != "" {
			xs = append(xs, x)
		}
	}
	t.Logf("%d mensajes, %d intercambios", len(msgs), len(xs))
	totalAntes, totalReglas, totalEmb := 0, 0, 0
	for k := 1; k < len(xs); k++ {
		// La sesión hasta antes del mensaje k.
		var pre []*ocMessage
		for _, m := range msgs {
			if m.ID < xs[k].UserID {
				pre = append(pre, m)
			}
		}
		win := pruneWindow(pre)
		antes := 0
		for _, m := range win {
			for _, pt := range m.Parts {
				antes += tokensOf(pt.Data)
			}
		}
		reglas := pruneChanges(win, planPrune(win, nil, pruneYoung))
		s := c.sims(ctx, win, xs[k].userText())
		plan := planPrune(win, s, pruneYoung)
		emb := pruneChanges(win, plan)
		l3 := 0
		for _, x := range plan.Exchanges {
			if x.Tier == TierL3 {
				l3++
			}
		}
		t.Logf("msg %2d %-50.50q historial %6d · reglas -%6d · con embeddings -%6d (L3 %d/%d)", k, xs[k].userText(), antes, reglas.Saved, emb.Saved, l3, len(plan.Exchanges))
		if os.Getenv("PGO_PRUEBA_PODA_DETALLE") == fmt.Sprint(k) {
			for _, x := range plan.Exchanges {
				t.Logf("  %s %s sim %.2f %q", x.Tier, x.UserID, x.Sim, x.Reason)
				for _, m := range x.Messages {
					for _, pt := range m.Parts {
						if e, ok := plan.Edits[pt.ID]; ok {
							o := ""
							if e.Output != nil {
								o = *e.Output
							}
							t.Logf("    %-9s %-5s %6d tok borra=%v limpia=%v ent=%v %.160q", pt.v.Type, pt.v.Tool, tokensOf(pt.Data), e.Delete, e.Clear, e.TrimInput, o)
						}
					}
				}
			}
		}
		totalAntes += antes
		totalReglas += reglas.Saved
		totalEmb += emb.Saved
	}
	t.Logf("TOTAL historial %d · reglas -%d (%.0f %%) · con embeddings -%d (%.0f %%)", totalAntes, totalReglas,
		100*float64(totalReglas)/float64(totalAntes), totalEmb, 100*float64(totalEmb)/float64(totalAntes))
}

// TestPruneOpenCodeReal: la poda con una sesión real de OpenCode (gasta tokens: unos céntimos). Cuatro turnos: leer
// un fichero grande, releerlo, una pregunta corta y, con la poda, una pregunta sobre el principio. Comprueba que la
// lectura superada sale del historial (OpenCode manda «[Old tool result content cleared]»), que el turno sigue
// funcionando y que el modelo mantiene lo que dedujo. PGO_PRUEBA_PODA_OC=1 go test -run TestPruneOpenCodeReal -v
func TestPruneOpenCodeReal(t *testing.T) {
	if os.Getenv("PGO_PRUEBA_PODA_OC") == "" {
		t.Skip()
	}
	var a *Agente
	for _, p := range Agentes() {
		if x, ok := p.(*Agente); ok && x.Clave == "opencode" {
			a = x
		}
	}
	dir := t.TempDir()
	pruneDirFunc = func() string { return dir }
	carpeta := t.TempDir()
	var b strings.Builder
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&b, "def funcion_%03d(x):\n    \"\"\"Calcula el valor %d de la tabla de ejemplo con un margen de seguridad.\"\"\"\n    return x * %d + %d\n\n", i, i, i, i%7)
	}
	os.WriteFile(filepath.Join(carpeta, "grande.py"), []byte(b.String()), 0o644)
	ctx, cancela := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancela()
	ses := ""
	turno := func(msg string) (string, []Uso, *PruneReport) {
		var texto strings.Builder
		var usos []Uso
		var rep *PruneReport
		err := a.Chat(ctx, Peticion{Carpeta: carpeta, SesionAgente: ses, Permisos: "lectura", Prune: PruneAuto, Idioma: "español",
			Mensajes: []Mensaje{{Rol: "user", Texto: msg}}}, func(e Evento) {
			texto.WriteString(e.Texto)
			if e.SesionAgente != "" {
				ses = e.SesionAgente
			}
			if e.Uso != nil {
				usos = append(usos, *e.Uso)
			}
			if e.Prune != nil {
				rep = e.Prune
			}
		})
		if err != nil {
			t.Fatalf("%q: %v", msg, err)
		}
		for i, u := range usos {
			t.Logf("  paso %d: contexto %d, caché leída %d", i+1, u.Contexto, u.CacheLeida)
		}
		t.Logf("%q → %.200q", msg, texto.String())
		return texto.String(), usos, rep
	}
	turno("Lee entero grande.py con tu herramienta de lectura (todas sus líneas) y dime cuántas funciones define y qué devuelve funcion_005. Responde en una línea.")
	turno("Vuelve a leer grande.py entero y dime el nombre de la última función. Una línea.")
	_, u3, _ := turno("¿Qué lenguaje es? Una palabra.")
	// La cuarta: aquí la poda quita la primera lectura (superada por la segunda).
	time.Sleep(3 * time.Second) // el marcado del turno anterior, en segundo plano
	r, u4, rep := turno("¿Qué devolvía funcion_005 según me dijiste en tu primera respuesta? Una línea.")
	if rep == nil || rep.Removed < pruneMinTokens {
		t.Fatalf("sin poda: %+v", rep)
	}
	t.Logf("poda: %d tokens fuera; intercambios %+v", rep.Removed, rep.Exchanges)
	t.Logf("contexto: último paso del turno 3 %d → primer paso del turno 4 %d", u3[len(u3)-1].Contexto, u4[0].Contexto)
	if u4[0].Contexto >= u3[len(u3)-1].Contexto {
		t.Fatal("el contexto no baja")
	}
	if !strings.Contains(r, "x * 5 + 5") && !strings.Contains(r, "5x + 5") && !strings.Contains(r, "x*5+5") && !strings.Contains(r, "5 * x + 5") {
		t.Logf("¡ojo! la respuesta no mantiene lo deducido: %q", r)
	}
	// En la base de datos: la primera lectura lleva la marca de OpenCode.
	msgs, err := loadOpenCodeSession(ctx, a, Peticion{Carpeta: carpeta}, ses)
	if err != nil {
		t.Fatal(err)
	}
	marcadas := 0
	for _, m := range msgs {
		for _, p := range m.Parts {
			if p.Current != "" && strings.Contains(p.Current, `"compacted"`) {
				marcadas++
			}
		}
	}
	t.Logf("sesión %s: %d partes marcadas", ses, marcadas)
	if marcadas == 0 {
		t.Fatal("ninguna parte marcada")
	}
}
