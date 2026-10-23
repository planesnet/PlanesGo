package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"planesgo-code/internal/skills"
)

// Lo fijo de cada llamada al modelo: el prompt de sistema y las definiciones de las herramientas (con la lista de
// skills: nombre y descripción de cada una) van en TODAS las llamadas. pgocode quita de ahí lo que en sus sesiones
// no sirve (medido en docs/herramientas.md):
//   - Claude Code: con el ajuste skillOverrides (por --settings, solo en las sesiones de pgocode): «off» para las
//     skills que no se pueden usar sin nadie delante o sin los conectores de claude.ai; «name-only» (solo el
//     nombre, sin la descripción) para las genéricas cuyo nombre ya dice para qué son (el cuerpo se carga al
//     usarlas, como antes). Las skills personales y del proyecto no se tocan.
//   - OpenCode: las skills denegadas no le llegan al modelo; se deniegan las de Claude que necesitan sus conectores,
//     las propias de Antigravity y, fuera de un proyecto de Odoo, las de Odoo.
//   - Antigravity: sin la lista de órdenes de barra (--disable-slash-commands) salvo si el mensaje es una.

// claudeSkillsOff: skills que no se usan en una sesión de pgocode (sin nadie delante, sin conectores de claude.ai,
// o cuyo trabajo hace pgocode: la imputación y la configuración del arnés).
var claudeSkillsOff = []string{
	"design", "design-sync", "update-config", "fewer-permission-prompts", "doctor", "loop", "schedule",
	"workflow-authoring", "run-skill-generator", "batch", "plugin-authoring", "keybindings-help",
	"anthropic-skills:morning", "anthropic-skills:import-memory", "anthropic-skills:docs", "anthropic-skills:google-workspace",
}

// claudeSkillsNameOnly: skills genéricas que se listan solo con su nombre (probado con haiku: «crea un documento
// Word…» sigue usando anthropic-skills:docx).
var claudeSkillsNameOnly = []string{
	"deep-research", "dataviz", "verify", "debug", "code-review", "simplify", "claude-api", "run",
	"anthropic-skills:docx", "anthropic-skills:pdf", "anthropic-skills:pptx", "anthropic-skills:xlsx",
	"anthropic-skills:skill-creator",
}

// claudeSettings: el JSON de --settings de las sesiones de Claude Code de pgocode (fijo: no cambia entre turnos,
// así no rompe el prefijo en caché).
func claudeSettings() string {
	o := map[string]string{}
	for _, n := range claudeSkillsNameOnly {
		o[n] = "name-only"
	}
	for _, n := range append(append([]string(nil), claudeSkillsOff...), skills.Sustituidas...) {
		o[n] = "off"
	}
	cfg := map[string]any{"skillOverrides": o}
	// Y en el mismo --settings (Claude Code toma solo uno), el límite de salidas de órdenes (prune_offload.go).
	if s := claudeOffloadSettings(); s != "" {
		var extra map[string]any
		if json.Unmarshal([]byte(s), &extra) == nil {
			for k, v := range extra {
				cfg[k] = v
			}
		}
	}
	b, _ := json.Marshal(cfg)
	return string(b)
}

// openCodeSkillsOff: skills que OpenCode ve (las lee también de ~/.claude/skills y ~/.agents/skills) y que no puede
// usar: las de Claude que necesitan sus conectores o su aplicación, y las propias de Antigravity.
var openCodeSkillsOff = []string{
	"morning", "import-memory", "docs", "google-workspace",
	"antigravity-agent-manager", "antigravity-skill-orchestrator", "antigravity-workflows",
}

// odooSkillsKept: las skills de Odoo que sirven también fuera de un módulo de Odoo (hablar con Odoo por su API).
var odooSkillsKept = map[string]bool{"odoo-rpc-api": true}

// openCodeHiddenSkills: los patrones de skill que se deniegan a OpenCode en esta carpeta.
func openCodeHiddenSkills(carpeta string) []string {
	l := append([]string(nil), openCodeSkillsOff...)
	if carpeta != "" && !isOdooProject(carpeta) {
		// «odoo-*» deniega todas; las que se conservan se permiten en permisosOpenCode (en OpenCode gana la última
		// regla que encaja, y json.Marshal ordena las claves: «odoo-*» va antes que «odoo-rpc-api»).
		l = append(l, "odoo-*")
	}
	return l
}

// isOdooProject: la carpeta es (o está dentro de) un proyecto de Odoo: un __manifest__.py en ella o en sus
// subcarpetas (hasta 3 niveles), en una carpeta superior cercana (dentro de un módulo), o un odoo-bin.
func isOdooProject(dir string) bool {
	d := filepath.Clean(dir)
	for i := 0; i < 3; i++ { // dentro de un módulo: el manifiesto está más arriba
		if fileExists(filepath.Join(d, "__manifest__.py")) || fileExists(filepath.Join(d, "odoo-bin")) {
			return true
		}
		p := filepath.Dir(d)
		if p == d {
			break
		}
		d = p
	}
	vistos := 0
	var busca func(d string, nivel int) bool
	busca = func(d string, nivel int) bool {
		es, err := os.ReadDir(d)
		if err != nil {
			return false
		}
		for _, e := range es {
			if vistos++; vistos > 5000 { // carpetas enormes: no se recorren enteras
				return false
			}
			n := e.Name()
			if !e.IsDir() {
				if n == "__manifest__.py" || n == "odoo-bin" {
					return true
				}
				continue
			}
			if nivel < 3 && !strings.HasPrefix(n, ".") && n != "node_modules" && n != "vendor" && n != "venv" && n != "__pycache__" {
				if busca(filepath.Join(d, n), nivel+1) {
					return true
				}
			}
		}
		return false
	}
	return busca(filepath.Clean(dir), 1)
}

func fileExists(f string) bool {
	st, err := os.Stat(f)
	return err == nil && !st.IsDir()
}
