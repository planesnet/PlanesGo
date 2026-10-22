package llm

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Antigravity: la vigencia sale de la conversación de pgocode; nada se aplica y lo que está fuera de vigencia
// cuenta como pendiente (lo que decide el relevo).
func TestChatVigencia(t *testing.T) {
	viejoDir, viejoNow := pruneDirFunc, pruneNow
	defer func() { pruneDirFunc, pruneNow = viejoDir, viejoNow }()
	d := t.TempDir()
	pruneDirFunc = func() string { return d }
	t0 := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	pruneNow = func() time.Time { return t0.Add(3 * time.Hour) }
	big := strings.Repeat("línea de código de prueba\n", 3000)
	xs := []ChatExchange{
		{Point: "m1", User: "Lee grande.py", Answer: "Tiene 700 funciones.", At: t0.UnixMilli(),
			Tools: []ChatTool{{ID: "m1:paso-2", Name: "view_file", Summary: "/p/grande.py", Output: big}}},
		{Point: "m3", User: "Ahora mira otra.py", Answer: "Vale.", At: t0.Add(time.Minute).UnixMilli(),
			Tools: []ChatTool{{ID: "m3:paso-2", Name: "view_file", Summary: "/p/otra.py", Output: "corto"}}},
		{Point: "m5", User: "Relee grande.py", Answer: "Igual.", At: t0.Add(2 * time.Minute).UnixMilli(),
			Tools: []ChatTool{{ID: "m5:paso-2", Name: "view_file", Summary: "/p/grande.py", Output: big}}},
		{Point: "m7", User: "Gracias", Answer: "De nada.", At: t0.Add(3 * time.Minute).UnixMilli()},
		{Point: "m9", User: "Adiós", Answer: "Hasta luego.", At: t0.Add(4 * time.Minute).UnixMilli()},
	}
	r := ChatVigencia(context.Background(), "chat-prueba1", xs, "otra cosa", false)
	if r == nil || r.Pending < 20000 {
		t.Fatalf("pendiente: %+v", r)
	}
	for _, p := range r.Pieces {
		if p.Out {
			t.Errorf("%s: nada se aplica en Antigravity", p.ID)
		}
		if p.ID == "m1:paso-2" && (p.Superseded != "lectura superada" || p.Value < vigCut) {
			t.Errorf("lectura superada: %+v", p)
		}
	}
	if r.Status == nil || r.Status.Engine != "reglas" {
		t.Errorf("estado: %+v", r.Status)
	}
	// Antigravity: sus lecturas solo traen el tamaño («1401 lines, 61315 bytes»); con él se estima lo que pesan.
	ys := []ChatExchange{
		{Point: "m1", User: "Lee alfa.py", Answer: "x * 70 + 123", Tools: []ChatTool{{ID: "m1:paso-2", Name: "read_file", Summary: "/p/alfa.py", Output: "1401 lines, 61315 bytes"}}},
		{Point: "m3", User: "Lee beta.py", Answer: "x * 43 + 45", Tools: []ChatTool{{ID: "m3:paso-7", Name: "read_file", Summary: "/p/beta.py", Output: "1401 lines, 61315 bytes"}}},
		{Point: "m5", User: "Relee alfa.py", Answer: "x * 19 + 600", Tools: []ChatTool{{ID: "m5:paso-12", Name: "read_file", Summary: "/p/alfa.py", Output: "1401 lines, 61315 bytes"}}},
		{Point: "m7", User: "Vale", Answer: "Vale"},
	}
	if r := ChatVigencia(context.Background(), "chat-prueba2", ys, "¿qué devuelve funcion_45?", false); r == nil || r.Pending < 15000 {
		t.Errorf("Antigravity: pendiente %+v", r)
	}
	if ChatVigencia(context.Background(), "../x", xs, "", false) != nil {
		t.Error("clave no válida")
	}
}
