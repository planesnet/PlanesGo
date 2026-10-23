package llm

import (
	"strings"
	"testing"
)

// Una orden en primer plano que Claude Code avisa también como tarea (local_bash, is_backgrounded:false) no sale
// como «En segundo plano»; la lanzada con run_in_background, sí.
func TestTareaPrimerPlano(t *testing.T) {
	in := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"make lento"}}]}}
{"type":"system","subtype":"task_started","task_id":"b1","tool_use_id":"t1","description":"Run make lento target in foreground","is_backgrounded":false,"task_type":"local_bash"}
{"type":"system","subtype":"task_notification","task_id":"b1","status":"completed","summary":"hecho"}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"sleep 9","run_in_background":true}}]}}
{"type":"system","subtype":"task_started","task_id":"b2","tool_use_id":"t2","description":"Sleep","is_backgrounded":true,"task_type":"local_bash"}`
	var tareas []*Tarea
	if err := leeClaudeCode(strings.NewReader(in), func(e Evento) {
		if e.Tarea != nil {
			tareas = append(tareas, e.Tarea)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if len(tareas) != 1 || tareas[0].ID != "b2" || tareas[0].Tipo != "segundo_plano" {
		for _, x := range tareas {
			t.Logf("%+v", *x)
		}
		t.Errorf("tareas: %d", len(tareas))
	}
}
