package odoo

import (
	"encoding/json"
	"testing"
)

func TestModelsCleanTaskNameAndOrigin(t *testing.T) {
	// 1. TimesheetEntry CleanTaskName & Detection
	ts := TimesheetEntry{
		TaskID: struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}{ID: 1, Name: "[CLAUDE] Tarea de refactorización"},
		Name: "Prueba con Claude",
	}

	if ts.CleanTaskName() != "Tarea de refactorización" {
		t.Errorf("CleanTaskName() esperado 'Tarea de refactorización', obtenido '%s'", ts.CleanTaskName())
	}

	if !ts.IsClaude() {
		t.Errorf("IsClaude() esperado true para prefijo [CLAUDE]")
	}

	if !ts.IsHoraMaquina() {
		t.Errorf("IsHoraMaquina() esperado true para tarea de Claude")
	}

	// 2. ActiveTimer CleanTaskName & Detection
	timerAgy := ActiveTimer{
		TaskName:    "[AGY] Análisis y diseño",
		Description: "Trabajo Antigravity",
		Source:      "antigravity",
	}
	if timerAgy.CleanTaskName() != "Análisis y diseño" {
		t.Errorf("CleanTaskName() esperado 'Análisis y diseño', obtenido '%s'", timerAgy.CleanTaskName())
	}
	if !timerAgy.IsAntigravity() {
		t.Errorf("IsAntigravity() esperado true para Source antigravity")
	}

	timerClaude := ActiveTimer{
		TaskName:    "[CLAUDE] Sesión de desarrollo",
		Description: "Trabajo Claude",
		Source:      "claude",
	}
	if timerClaude.CleanTaskName() != "Sesión de desarrollo" {
		t.Errorf("CleanTaskName() esperado 'Sesión de desarrollo', obtenido '%s'", timerClaude.CleanTaskName())
	}
	if !timerClaude.IsClaude() {
		t.Errorf("IsClaude() esperado true para Source claude")
	}

	// 3. JSON serialization of TimesheetEntry includes is_claude
	data, err := json.Marshal(ts)
	if err != nil {
		t.Fatalf("Error al serializar TimesheetEntry: %v", err)
	}
	var unmarshaled map[string]interface{}
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Error al deserializar JSON: %v", err)
	}
	if isClaude, ok := unmarshaled["is_claude"].(bool); !ok || !isClaude {
		t.Errorf("is_claude esperado true en JSON serializado, obtenido: %v", unmarshaled["is_claude"])
	}
}

func TestCreateTimeLocal(t *testing.T) {
	cases := []struct {
		name       string
		createDate string
		want       string
	}{
		{"UTC invierno a Madrid (+1h)", "2026-01-15 10:30:00", "11:30"},
		{"UTC verano a Madrid (+2h, DST)", "2026-07-15 10:30:00", "12:30"},
		{"vacío", "", ""},
		{"formato inválido", "no es una fecha", ""},
	}
	for _, c := range cases {
		ts := TimesheetEntry{CreateDate: c.createDate}
		if got := ts.CreateTimeLocal(); got != c.want {
			t.Errorf("%s: CreateTimeLocal() = %q, esperado %q", c.name, got, c.want)
		}
	}
}

// TestTicketUnmarshalOpenTicket reproduce el JSON real que Odoo devuelve para un
// ticket ABIERTO: closed_date y team_id vienen como "false" (booleano), no como
// string vacío ni objeto vacío. Un struct ingenuo con esos campos tipados como
// string/Many2One sin el manejo especial de Ticket.UnmarshalJSON falla el parseo
// en silencio, lo que CloseTicket confundía con "ticket no encontrado" (bug real
// detectado en producción: el ticket existía pero search_read nunca llegaba a
// devolver nada legible porque el propio Unmarshal reventaba antes).
func TestTicketUnmarshalOpenTicket(t *testing.T) {
	raw := []byte(`[{"id":24283,"name":"prueba","number":"HT24268","stage_id":[1,"New"],"team_id":false,"partner_id":false,"closed":false,"closed_date":false}]`)

	var tickets []Ticket
	if err := json.Unmarshal(raw, &tickets); err != nil {
		t.Fatalf("Unmarshal de un ticket abierto no debería fallar: %v", err)
	}
	if len(tickets) != 1 {
		t.Fatalf("esperado 1 ticket, obtenidos %d", len(tickets))
	}
	ticket := tickets[0]
	if ticket.ID != 24283 {
		t.Errorf("ID esperado 24283, obtenido %d", ticket.ID)
	}
	if ticket.Closed {
		t.Errorf("Closed esperado false para un ticket abierto")
	}
	if ticket.ClosedDate != "" {
		t.Errorf("ClosedDate esperado vacío, obtenido %q", ticket.ClosedDate)
	}
	if ticket.TeamID.ID != 0 {
		t.Errorf("TeamID.ID esperado 0 (sin equipo asignado), obtenido %d", ticket.TeamID.ID)
	}
}
