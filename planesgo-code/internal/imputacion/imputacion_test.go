package imputacion

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// odooFalso apunta las llamadas y responde como Odoo.
type odooFalso struct {
	mu       sync.Mutex
	llamadas []string
	escrito  map[string]any
}

func (o *odooFalso) CallKW(_ context.Context, _, modelo, metodo string, args []any, _ map[string]any, res any) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.llamadas = append(o.llamadas, modelo+"."+metodo)
	var r any
	switch modelo + "." + metodo {
	case "project.task.search_read":
		r = []any{}
	case "project.task.create":
		r = 7
	case "account.analytic.line.create":
		r = 99
	case "account.analytic.line.write":
		if m, ok := args[1].(map[string]any); ok {
			for k, v := range m {
				o.escrito[k] = v
			}
		}
		r = true
	default:
		r = true
	}
	b, _ := json.Marshal(r)
	return json.Unmarshal(b, res)
}

func TestParteCompleto(t *testing.T) {
	o := &odooFalso{escrito: map[string]any{}}
	g := &Gestor{Odoo: o}
	fin := g.Empieza(Trabajo{Sesion: "s1", Proyecto: 447, UID: 2, Modelo: "Claude Code", Desc: Descripcion("Arreglo", "arregla el informe")})
	g.SumaUso("s1", 1000, 200, 0.05)
	time.Sleep(50 * time.Millisecond)
	ab, _ := g.Estado()
	if len(ab) != 1 || ab[0].LineaID != 99 || ab[0].TareaID != 7 || ab[0].Tarea != TareaDefecto {
		t.Fatalf("abierto: %+v", ab)
	}
	fin()
	g.CierraTodo()
	_, ce := g.Estado()
	if len(ce) != 1 || ce[0].Estado != "cerrado" || ce[0].Horas != 1.0/60 { // menos de un minuto: el mínimo
		t.Fatalf("cerrado: %+v", ce)
	}
	if fmt.Sprint(o.escrito["unit_amount"]) != fmt.Sprint(1.0/60) || o.escrito["ai_tokens_total"] != 1200 || o.escrito["ai_session_id"] != "pgocode-s1" {
		t.Errorf("escrito: %v", o.escrito)
	}
	want := []string{"project.task.search_read", "project.task.create", "account.analytic.line.create", "account.analytic.line.action_timer_start",
		"account.analytic.line.action_timer_stop", "account.analytic.line.write", "account.analytic.line.write"}
	if fmt.Sprint(o.llamadas) != fmt.Sprint(want) {
		t.Errorf("llamadas: %v", o.llamadas)
	}
}

// Cambiar de tarea en la sesión cierra el parte y abre otro.
func TestCambioDeTarea(t *testing.T) {
	o := &odooFalso{escrito: map[string]any{}}
	g := &Gestor{Odoo: o}
	g.Empieza(Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 5, Tarea: "A"})()
	g.Empieza(Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 6, Tarea: "B"})()
	time.Sleep(100 * time.Millisecond)
	ab, ce := g.Estado()
	if len(ab) != 1 || ab[0].TareaID != 6 || len(ce) != 1 || ce[0].TareaID != 5 {
		t.Errorf("abiertos %+v cerrados %+v", ab, ce)
	}
}
