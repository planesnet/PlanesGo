package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeSettings(t *testing.T) {
	var s struct {
		SkillOverrides map[string]string `json:"skillOverrides"`
	}
	if err := json.Unmarshal([]byte(claudeSettings()), &s); err != nil {
		t.Fatal(err)
	}
	for n, v := range map[string]string{"pgo-imputacion": "off", "loop": "off", "anthropic-skills:docs": "off",
		"anthropic-skills:docx": "name-only", "code-review": "name-only"} {
		if s.SkillOverrides[n] != v {
			t.Errorf("%s: %q, quería %q", n, s.SkillOverrides[n], v)
		}
	}
	// Las personales o del proyecto (y las de Odoo) no se tocan.
	if _, ok := s.SkillOverrides["odoo-19"]; ok {
		t.Error("odoo-19 no debería tener ajuste")
	}
	// Fijo entre llamadas: no rompe el prefijo en caché.
	if claudeSettings() != claudeSettings() {
		t.Error("claudeSettings cambia entre llamadas")
	}
	cl := Agentes()[0].(*Agente)
	if args := strings.Join(cl.Args("hola", Peticion{}), " "); !strings.Contains(args, "--settings {\"skillOverrides\"") {
		t.Errorf("sin --settings: %s", args)
	}
}

func write(t *testing.T, f string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIsOdooProject(t *testing.T) {
	base := t.TempDir()
	goProj := filepath.Join(base, "go")
	write(t, filepath.Join(goProj, "cmd", "main.go"))
	write(t, filepath.Join(goProj, "node_modules", "x", "__manifest__.py")) // no cuenta
	addons := filepath.Join(base, "addons")
	write(t, filepath.Join(addons, "custom", "mi_modulo", "__manifest__.py"))
	deep := filepath.Join(base, "deep")
	write(t, filepath.Join(deep, "a", "b", "c", "d", "__manifest__.py")) // demasiado hondo
	for dir, want := range map[string]bool{
		goProj: false, addons: true, filepath.Join(addons, "custom", "mi_modulo", "models"): true, deep: false,
	} {
		os.MkdirAll(dir, 0o755)
		if got := isOdooProject(dir); got != want {
			t.Errorf("%s: %v, quería %v", dir, got, want)
		}
	}
}

func TestPermisosOpenCodeSkills(t *testing.T) {
	base := t.TempDir()
	write(t, filepath.Join(base, "odoo", "mod", "__manifest__.py"))
	os.MkdirAll(filepath.Join(base, "go"), 0o755)
	sk := func(dir string) map[string]string {
		var p struct {
			Skill map[string]string `json:"skill"`
		}
		if err := json.Unmarshal([]byte(permisosOpenCode("auto", nil, dir)), &p); err != nil {
			t.Fatal(err)
		}
		return p.Skill
	}
	g := sk(filepath.Join(base, "go"))
	if g["odoo-*"] != "deny" || g["odoo-rpc-api"] != "allow" || g["docs"] != "deny" || g["pgo-imputacion"] != "deny" || g["*"] != "allow" {
		t.Errorf("fuera de Odoo: %v", g)
	}
	// La regla que conserva una skill va detrás de «odoo-*» (en OpenCode gana la última que encaja).
	p := permisosOpenCode("auto", nil, filepath.Join(base, "go"))
	if strings.Index(p, `"odoo-*"`) > strings.Index(p, `"odoo-rpc-api"`) {
		t.Errorf("orden: %s", p)
	}
	if o := sk(filepath.Join(base, "odoo")); o["odoo-*"] != "" || o["odoo-rpc-api"] != "" || o["antigravity-workflows"] != "deny" {
		t.Errorf("en Odoo: %v", o)
	}
}

func TestAntigravitySlashCommands(t *testing.T) {
	ag := Agentes()[1].(*Agente)
	if args := strings.Join(ag.Args("hola", Peticion{}), " "); !strings.Contains(args, "--disable-slash-commands") {
		t.Errorf("sin --disable-slash-commands: %s", args)
	}
	if args := strings.Join(ag.Args("/plan algo", Peticion{}), " "); strings.Contains(args, "--disable-slash-commands") {
		t.Errorf("una orden de barra no puede ir con --disable-slash-commands: %s", args)
	}
	// Con «/compact» delante (compactación de pgocode), tampoco.
	p := Peticion{SesionAgente: "c1", LastContext: 900000, CompactAt: 20000}
	if args := strings.Join(ag.Args("hola", p), " "); !strings.Contains(args, "--print=/compact") || strings.Contains(args, "--disable-slash-commands") {
		t.Errorf("«/compact» con --disable-slash-commands: %s", args)
	}
}
