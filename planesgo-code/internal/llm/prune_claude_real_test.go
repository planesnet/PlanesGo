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

// TestPruneClaudeReal: Claude Code de verdad (gasta: unos céntimos con haiku). Siete turnos que leen y releen dos
// ficheros grandes; antes del 5.º se simula que Claude Code no acepta el historial podado (la copia restaura el
// original y el turno se repite sin podar); antes del 6.º se poda de verdad (con la caché «fría»: el reloj de la
// poda va 2 h por delante) y se mide el contexto; el 6.º y el 7.º preguntan por lo del principio. Con el entorno
// limpio (sin CLAUDE*, ANTHROPIC*…):
//
//	PGO_PRUEBA_CLAUDE=<carpeta dentro de /home> go test -run TestPruneClaudeReal -v -timeout 30m ./internal/llm
func TestPruneClaudeReal(t *testing.T) {
	dir := os.Getenv("PGO_PRUEBA_CLAUDE")
	if dir == "" {
		t.Skip()
	}
	os.MkdirAll(dir, 0o755)
	gen := func(nombre string, k int) {
		var b strings.Builder
		b.WriteString("# Módulo de prueba " + nombre + "\n")
		for i := 0; i < 700; i++ {
			fmt.Fprintf(&b, "def funcion_%d(x):\n    return x * %d + %d  # cálculo número %d de la tabla %s\n", i, (i*k)%113, i, i, nombre)
		}
		os.WriteFile(filepath.Join(dir, nombre), []byte(b.String()), 0o644)
	}
	gen("alfa.py", 7)
	gen("beta.py", 11)
	datos := t.TempDir()
	viejoDir, viejoNow := pruneDirFunc, pruneNow
	defer func() { pruneDirFunc, pruneNow, claudeAfterWrite = viejoDir, viejoNow, nil }()
	pruneDirFunc = func() string { return datos }
	a := Agentes()[0].(*Agente)
	ses := ""
	turno := func(n int, msg string) (string, int, *PruneReport) {
		p := Peticion{Carpeta: dir, Modelo: "haiku", SesionAgente: ses, Prune: PruneAuto, Mensajes: []Mensaje{{Rol: "user", Texto: msg}}}
		var txt strings.Builder
		ctxTok := 0
		var rep *PruneReport
		ctx, cancela := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancela()
		err := a.Chat(ctx, p, func(e Evento) {
			txt.WriteString(e.Texto)
			if e.SesionAgente != "" {
				ses = e.SesionAgente
			}
			if e.Uso != nil && e.Uso.Contexto > 0 {
				ctxTok = e.Uso.Contexto
			}
			if e.Prune != nil {
				rep = e.Prune
			}
		})
		if err != nil {
			t.Fatalf("turno %d: %v", n, err)
		}
		// Que acabe el marcado (en segundo plano) antes del siguiente.
		VigenciaReport(context.Background(), ses, true)
		r := strings.TrimSpace(txt.String())
		t.Logf("turno %d: contexto %d · %q", n, ctxTok, recorta(r, 300))
		return r, ctxTok, rep
	}
	turno(1, "Lee alfa.py con Read (entero) y dime qué devuelve funcion_123 (la expresión exacta). Una línea.")
	turno(2, "Lee beta.py con Read (entero) y dime qué devuelve funcion_45. Una línea.")
	turno(3, "Vuelve a leer alfa.py con Read y dime qué devuelve funcion_600. Una línea.")
	_, c4, _ := turno(4, "Gracias. Responde solo: vale.")
	// 5.º: Claude Code no acepta el historial podado (aquí, el fichero desaparece tras escribirlo): la copia lo
	// restaura y el turno se repite sin podar.
	pruneNow = func() time.Time { return time.Now().Add(2 * time.Hour) }
	path, err := claudeSessionFile(ses)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(path)
	claudeAfterWrite = func(p string) { os.Rename(p, p+".oculto") }
	r5, c5, rep5 := turno(5, "Responde solo: sigo.")
	claudeAfterWrite = nil
	os.Remove(path + ".oculto")
	now, _ := os.ReadFile(path)
	if rep5 == nil || rep5.Removed == 0 {
		t.Errorf("turno 5: no se llegó a podar (%+v)", rep5)
	}
	if strings.Contains(string(now), ClearedText) || !strings.HasPrefix(string(now), string(original)) || r5 == "" {
		t.Errorf("turno 5: la copia no ha restaurado el original")
	}
	t.Logf("turno 5 restaurado: contexto %d (antes %d)", c5, c4)
	// 6.º: poda de verdad.
	r6, c6, rep6 := turno(6, "Sin usar herramientas: según lo que leíste al principio, ¿qué devuelve funcion_123 de alfa.py? Una línea.")
	if rep6 == nil || rep6.Removed == 0 {
		t.Fatalf("turno 6: sin poda (%+v)", rep6)
	}
	t.Logf("PODA: retirados %d tokens estimados; contexto %d → %d", rep6.Removed, c5, c6)
	if c6 >= c5 {
		t.Errorf("el contexto no baja: %d → %d", c5, c6)
	}
	if !strings.Contains(r6, "70") || !strings.Contains(r6, "123") {
		t.Errorf("turno 6: respuesta inesperada %q", r6)
	}
	r7, c7, _ := turno(7, "Sin usar herramientas: ¿qué devolvía funcion_45 de beta.py? ¿Y cuántos ficheros has leído en total? Breve.")
	if !strings.Contains(r7, "45") {
		t.Errorf("turno 7: %q", r7)
	}
	t.Logf("contextos: 4.º %d · 5.º %d · 6.º %d · 7.º %d; sesión %s", c4, c5, c6, c7, ses)
}
