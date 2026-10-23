package llm

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// «Barrer ahora» en OpenCode: lo que está fuera de vigencia sale aunque no llegue al mínimo.
func TestForceSweepOpenCode(t *testing.T) {
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
	viejoDB, viejoDir, viejoEmb := ocDB, pruneDirFunc, pruneEmbed
	defer func() { drainCollectors(); ocDB, pruneDirFunc, pruneEmbed = viejoDB, viejoDir, viejoEmb }()
	dir := t.TempDir()
	pruneDirFunc = func() string { return dir }
	pruneEmbed = func(context.Context, string, string) ([]float32, error) { return nil, os.ErrNotExist }
	tablaPreparada.Store(datosOpenCode(), true)
	ocDB = func(_ context.Context, _ *Agente, _ Peticion, q string, _ bool) ([]byte, error) {
		if strings.HasPrefix(q, "SELECT") {
			return rows, nil
		}
		escritas++
		return nil, nil
	}
	carpeta := t.TempDir()
	p := Peticion{Carpeta: carpeta, SesionAgente: "ses_forzado1", Prune: PruneRules, Mensajes: []Mensaje{{Rol: "user", Texto: "sigue"}}}
	if rep := pruneOpenCode(context.Background(), &Agente{Clave: "opencode"}, p); rep == nil || rep.Removed != 0 || escritas != 0 || rep.Pending <= 0 {
		t.Fatalf("sin forzar, por debajo del mínimo no se toca: %+v", rep)
	}
	rep, err := ForceSweep(context.Background(), "opencode", "ses_forzado1", carpeta, "sigue", false)
	if err != nil || rep.Removed <= 0 || escritas == 0 || rep.Status == nil {
		t.Fatalf("forzado: %+v %v (%d escrituras)", rep, err, escritas)
	}
	if _, err := ForceSweep(context.Background(), "antigravity", "x", carpeta, "", false); err == nil {
		t.Fatal("Antigravity no se barre en el servidor")
	}
}

// En Claude Code, con la sesión abierta queda pendiente y se aplica al empezar el turno siguiente.
func TestForceSweepClaude(t *testing.T) {
	drainCollectors()
	ses := "33333333-2222-4333-8444-555555555555"
	_, f := claudeTestEnv(t, ses)
	viejoEmb := pruneEmbed
	t.Cleanup(func() { drainCollectors(); pruneEmbed = viejoEmb })
	pruneEmbed = func(context.Context, string, string) ([]float32, error) { return nil, os.ErrNotExist }
	pruneNow = func() time.Time { return f.t0.Add(40 * time.Minute) } // caché caliente
	claudeBusyFunc = func(string) bool { return true }
	rep, err := ForceSweep(context.Background(), "claude-code", ses, "/p", "sigue", true)
	if err != nil || rep.Removed != 0 || !rep.ForcePending {
		t.Fatalf("abierta: %+v %v", rep, err)
	}
	if !rep.CacheWarm || rep.CacheSuffix <= 0 {
		t.Errorf("con la caché caliente, lo que deja de estar en ella: %+v", rep)
	}
	claudeBusyFunc = func(string) bool { return false }
	rep2, bak := pruneClaude(context.Background(), Peticion{SesionAgente: ses, Prune: PruneRules, Mensajes: []Mensaje{{Rol: "user", Texto: "sigue"}}})
	if rep2 == nil || bak == nil || rep2.Removed <= 0 || rep2.ForcePending {
		t.Fatalf("pendiente al empezar el turno: %+v", rep2)
	}
}
