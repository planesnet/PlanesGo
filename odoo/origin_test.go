package odoo

import "testing"

// Una tarea compartida ("Desarrollo") acumula las etiquetas Claude y Antigravity: cada parte
// debe mostrar solo su propio origen.
func TestOrigenPorParteConTareaCompartida(t *testing.T) {
	sharedTags := []Tag{{Name: "Antigravity"}, {Name: "Claude"}, {Name: "Cliente X"}}

	cases := []struct {
		name         string
		entry        TimesheetEntry
		wantAgy      bool
		wantClaude   bool
		wantTagNames []string
	}{
		{"claude por descripción del hook", TimesheetEntry{Name: "[Claude Code] Corrección: cálculo de horas", Tags: sharedTags}, false, true, []string{"Claude", "Cliente X"}},
		{"claude por descripción antigua", TimesheetEntry{Name: "Sesión Claude Code finalizada", Tags: sharedTags}, false, true, []string{"Claude", "Cliente X"}},
		{"claude por ai_model", TimesheetEntry{Name: "Desarrollo", AIModel: "Claude Code", Tags: sharedTags}, false, true, []string{"Claude", "Cliente X"}},
		{"antigravity por ai_model", TimesheetEntry{Name: "[Desarrollo] Login", AIModel: "Gemini 3.8 Flash", Tags: sharedTags}, true, false, []string{"Antigravity", "Cliente X"}},
		{"antigravity por prefijo", TimesheetEntry{Name: "[AGY] Login", Tags: sharedTags}, true, false, []string{"Antigravity", "Cliente X"}},
		{"sin señal propia: se usan las etiquetas de la tarea", TimesheetEntry{Name: "Trabajo", Tags: sharedTags}, true, true, []string{"Antigravity", "Claude", "Cliente X"}},
	}
	for _, c := range cases {
		agy, claude := c.entry.IsAntigravity(), c.entry.IsClaude()
		if agy != c.wantAgy || claude != c.wantClaude {
			t.Errorf("%s: IsAntigravity=%v IsClaude=%v, esperado %v/%v", c.name, agy, claude, c.wantAgy, c.wantClaude)
		}
		tags := c.entry.Tags
		if agy != claude {
			tags = withoutOriginTags(tags, agy)
		}
		var names []string
		for _, tg := range tags {
			names = append(names, tg.Name)
		}
		if len(names) != len(c.wantTagNames) {
			t.Errorf("%s: etiquetas %v, esperado %v", c.name, names, c.wantTagNames)
			continue
		}
		for i := range names {
			if names[i] != c.wantTagNames[i] {
				t.Errorf("%s: etiquetas %v, esperado %v", c.name, names, c.wantTagNames)
				break
			}
		}
	}
	if len(sharedTags) != 3 {
		t.Errorf("withoutOriginTags no debe modificar el slice de la tarea")
	}
}

func TestOrigenTemporizadorActivo(t *testing.T) {
	tags := []Tag{{Name: "Antigravity"}, {Name: "Claude"}}
	agy := ActiveTimer{Source: "antigravity", Description: "menciona Claude Code", Tags: tags}
	if !agy.IsAntigravity() || agy.IsClaude() {
		t.Errorf("Source antigravity: IsAntigravity=%v IsClaude=%v", agy.IsAntigravity(), agy.IsClaude())
	}
	cl := ActiveTimer{Source: "claude", Tags: tags}
	if cl.IsAntigravity() || !cl.IsClaude() {
		t.Errorf("Source claude: IsAntigravity=%v IsClaude=%v", cl.IsAntigravity(), cl.IsClaude())
	}
}
