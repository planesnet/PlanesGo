package router

import (
	"strings"
	"testing"
)

func TestTablaEnOrden(t *testing.T) {
	want := []string{Command, Direct, Aside, Split, Agent}
	if len(Routes) != len(want) {
		t.Fatalf("rutas: %d", len(Routes))
	}
	for i, r := range Routes {
		if r.Name != want[i] || r.Executor == "" || r.Cost == "" || r.Code == "" {
			t.Errorf("fila %d: %+v", i, r)
		}
	}
	if Known("local-ai") || !Known(Agent) {
		t.Error("la IA local no es un destino del mensaje principal")
	}
}

func TestValidate(t *testing.T) {
	ok := Decision{Sesion: "s1", Route: Aside, Executor: "local-ai", Engine: "rules", Reason: strings.Repeat("x", 500), Modifiers: []string{"queue"}, Ms: 12}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	if len([]rune(ok.Reason)) != 200 {
		t.Errorf("motivo sin recortar: %d", len(ok.Reason))
	}
	f := ok.Fields()
	if f["destino"] != Aside || f["ejecuta"] != "local-ai" || f["modificadores"] != "queue" || f["ms"] != int64(12) {
		t.Errorf("campos: %v", f)
	}
	malas := []Decision{
		{Route: Agent, Executor: "agent", Engine: "rules"},
		{Sesion: "s", Route: "local", Executor: "agent", Engine: "rules"},
		{Sesion: "s", Route: Agent, Executor: "otro", Engine: "rules"},
		{Sesion: "s", Route: Agent, Executor: "agent", Engine: "magia"},
		{Sesion: "s", Route: Agent, Executor: "agent", Engine: "rules", Modifiers: []string{"turbo"}},
	}
	for _, d := range malas {
		if d.Validate() == nil {
			t.Errorf("debería fallar: %+v", d)
		}
	}
	neg := Decision{Sesion: "s", Route: Direct, Executor: "pgocode", Engine: "rules", Ms: -5}
	if neg.Validate() != nil || neg.Ms != 0 {
		t.Errorf("ms negativos: %d", neg.Ms)
	}
}
