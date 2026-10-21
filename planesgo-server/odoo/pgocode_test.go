package odoo

import (
	"encoding/json"
	"testing"
)

// Partes de pgocode: manda ai_model (máquina con modelo, hombre sin él), aunque la tarea tenga etiqueta de IA;
// y un ai_model u otros campos vacíos (false en Odoo) no rompen la lectura.
func TestPartesPgocode(t *testing.T) {
	var l []TimesheetEntry
	raw := `[{"id":1,"name":"x","ai_model":"OpenCode google-vertex/gemini-4-argon","ai_session_id":"pgocode-s1-m"},
		{"id":2,"name":"x","ai_model":false,"ai_session_id":"pgocode-s1-h"},
		{"id":3,"name":"x","ai_model":"Claude Code opus","ai_session_id":"pgocode-s2-m"},
		{"id":4,"name":"x","ai_model":false,"ai_session_id":false}]`
	if err := json.Unmarshal([]byte(raw), &l); err != nil {
		t.Fatal(err)
	}
	for i := range l {
		l[i].Tags = []Tag{{Name: "Claude"}}
	}
	if !l[0].IsHoraMaquina() || l[0].IsAntigravity() || l[0].IsClaude() {
		t.Error("OpenCode: máquina, ni Antigravity ni Claude")
	}
	if l[1].IsHoraMaquina() || !l[1].IsHoraHombre() || l[1].IsClaude() {
		t.Error("línea de hombre de pgocode: Hora Hombre aunque la tarea tenga la etiqueta Claude")
	}
	if !l[2].IsClaude() || !l[2].IsHoraMaquina() {
		t.Error("Claude Code de pgocode: Claude y máquina")
	}
	if !l[3].IsClaude() {
		t.Error("los partes que no son de pgocode siguen como antes (etiqueta de la tarea)")
	}
}
