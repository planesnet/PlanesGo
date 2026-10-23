package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// conGuard fija el blindaje de una prueba (y lo deja como estaba).
func conGuard(t *testing.T, g AgentGuard) {
	antes := currentGuard()
	SetAgentGuard(g)
	t.Cleanup(func() { SetAgentGuard(antes) })
}

func TestDefaultAgentGuard(t *testing.T) {
	g := DefaultAgentGuard("https://planesnet.autopyme.com", "https://planesgo.autopyme.com/", "/d/pgocode")
	if !slices.Equal(g.Hosts, []string{"planesnet.autopyme.com", "planesgo.autopyme.com"}) {
		t.Errorf("hosts: %v", g.Hosts)
	}
	// El host sale de la configuración (otro Odoo, con puerto: el simulado de las pruebas).
	if g := DefaultAgentGuard("http://127.0.0.1:8070", "", ""); !slices.Equal(g.Hosts, []string{"127.0.0.1:8070"}) {
		t.Errorf("hosts del Odoo simulado: %v", g.Hosts)
	}
	if !slices.Contains(g.ReadDeny, "/d/pgocode/*.json") || slices.ContainsFunc(g.ReadDeny, func(s string) bool { return strings.Contains(s, "ramas") }) {
		t.Errorf("lecturas denegadas: %v", g.ReadDeny)
	}
}

// Claude Code: las reglas «deny» van en su --settings (red al Odoo y a PlanesGo, ficheros de datos, clientes de
// imputación) y la orden /pgo y la skill de imputación, desactivadas.
func TestClaudeGuardRules(t *testing.T) {
	conGuard(t, DefaultAgentGuard("https://planesnet.autopyme.com", "https://planesgo.autopyme.com", "/home/x/.local/share/pgocode"))
	a := Agentes()[0].(*Agente)
	args := a.Args("hola", Peticion{Permisos: "auto", Carpeta: t.TempDir()})
	i := slices.Index(args, "--settings")
	if i < 0 {
		t.Fatalf("sin --settings: %v", args)
	}
	var cfg struct {
		Permissions struct{ Deny []string } `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(args[i+1]), &cfg); err != nil {
		t.Fatal(err)
	}
	for _, r := range []string{"WebFetch(domain:planesnet.autopyme.com)", "Bash(*planesnet.autopyme.com*)",
		"WebFetch(domain:planesgo.autopyme.com)", "Bash(*planesgo.autopyme.com*)",
		"Read(//home/x/.local/share/pgocode/*.json)", "Bash(*imputacion-dia*)", "Bash(*sesiones-reinicio*)",
		"Bash(*planesgo_auth*)", "Bash(pgo-mcp *)", "Bash(planesgo-mcp *)", "mcp__pgo__pgo_time"} {
		if !slices.Contains(cfg.Permissions.Deny, r) {
			t.Errorf("falta la regla %s en %v", r, cfg.Permissions.Deny)
		}
	}
	j := strings.Join(args, " ")
	for _, sk := range []string{"Skill(pgo)", "Skill(pgo-imputacion)"} {
		if !strings.Contains(j, "--disallowedTools "+sk) {
			t.Errorf("sin --disallowedTools %s", sk)
		}
	}
	// El Odoo simulado en local: Bash con su host:puerto, sin cerrar WebFetch a todo 127.0.0.1.
	conGuard(t, DefaultAgentGuard("http://127.0.0.1:8070", "", ""))
	d := claudeDenyRules(currentGuard())
	if !slices.Contains(d, "Bash(*127.0.0.1:8070*)") || slices.ContainsFunc(d, func(s string) bool { return strings.HasPrefix(s, "WebFetch(domain:127") }) {
		t.Errorf("reglas con el Odoo simulado: %v", d)
	}
}

// OpenCode: en «auto», bash con las órdenes denegadas; webfetch sin los hosts; en los demás modos, bash denegado.
func TestOpenCodeGuard(t *testing.T) {
	conGuard(t, DefaultAgentGuard("https://planesnet.autopyme.com", "https://planesgo.autopyme.com", "/home/x/.local/share/pgocode"))
	var p struct {
		Bash     any               `json:"bash"`
		Webfetch map[string]string `json:"webfetch"`
	}
	if err := json.Unmarshal([]byte(permisosOpenCode("auto", nil, "")), &p); err != nil {
		t.Fatal(err)
	}
	bash, _ := p.Bash.(map[string]any)
	for _, k := range []string{"*planesnet.autopyme.com*", "*planesgo.autopyme.com*", "*imputacion-dia*", "*planesgo_auth*", "pgo-mcp *"} {
		if bash[k] != "deny" {
			t.Errorf("bash sin %s denegado: %v", k, bash)
		}
	}
	if bash["*"] != "allow" || p.Webfetch["*"] != "allow" || p.Webfetch["*://planesnet.autopyme.com*"] != "deny" {
		t.Errorf("auto: bash %v, webfetch %v", bash, p.Webfetch)
	}
	if err := json.Unmarshal([]byte(permisosOpenCode("editar", nil, "")), &p); err != nil || p.Bash != "deny" {
		t.Errorf("editar: bash %v", p.Bash)
	}
}

// El MCP de imputación de PlanesGo («pgo») no se ofrece: ni a Claude Code (--mcp-config) ni a OpenCode (se
// desactiva en OPENCODE_CONFIG_CONTENT), aunque se pida.
func TestImputationMCPNotOffered(t *testing.T) {
	casa := t.TempDir()
	t.Setenv("HOME", casa)
	os.WriteFile(filepath.Join(casa, ".claude.json"), []byte(`{"mcpServers":{
		"pgo":{"type":"stdio","command":"/x/.local/bin/pgo-mcp"},
		"otro-nombre":{"type":"stdio","command":"/x/bin/planesgo-mcp"},
		"freecad":{"type":"stdio","command":"uvx","args":["freecad-mcp"]}}}`), 0o600)
	f := configMCP(t.TempDir(), []string{"pgo", "otro-nombre", "freecad"})
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "pgo-mcp") || strings.Contains(string(b), "planesgo-mcp") || !strings.Contains(string(b), "freecad") {
		t.Errorf("configuración MCP: %s", b)
	}
	os.MkdirAll(filepath.Join(casa, ".config", "opencode"), 0o755)
	os.WriteFile(filepath.Join(casa, ".config", "opencode", "opencode.json"), []byte(`{"mcp":{"pgo":{"type":"local","command":["/x/pgo-mcp"],"enabled":true}}}`), 0o600)
	e := entornoOpenCode(Peticion{Permisos: "auto", Modelo: "google-vertex/gemini-4-argon"})
	if !strings.Contains(strings.Join(e, " "), `"mcp":{"pgo":{"enabled":false}}`) {
		t.Errorf("OpenCode con el MCP de imputación: %v", e)
	}
}

// El agente no hereda credenciales de Odoo ni de PlanesGo, ni el secreto de la app de escritorio, ni el token de
// agente de otro pgocode; sí PLANESGO_GESTOR=pgocode (PlanesGo dormido) y su propio PGO_TOKEN.
func TestAgentEnvWithoutCredentials(t *testing.T) {
	for k, v := range map[string]string{"ODOO_URL": "https://planesnet.autopyme.com", "ODOO_PASSWORD": "secreto",
		"ODOO_SESSION_ID": "s", "PLANESGO_TOKEN": "t", "ANTIGRAVITY_TOKEN": "t", "PGO_TOKEN_ESCRITORIO": "e", "PGO_TOKEN": "de-otro"} {
		t.Setenv(k, v)
	}
	dir := t.TempDir()
	salida := filepath.Join(dir, "entorno.txt")
	a := &Agente{Clave: "falso", Nombre: "Falso", Programa: "sh", Args: func(string, Peticion) []string {
		return []string{"-c", "env > " + salida + "; echo listo"}
	}, Lee: leeTexto}
	err := a.Chat(context.Background(), Peticion{Carpeta: dir, Mensajes: []Mensaje{{Rol: "user", Texto: "hola"}},
		Entorno: []string{"PGO_TOKEN=propio", "PLANESGO_GESTOR=pgocode"}}, func(Evento) {})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(salida)
	env := "\n" + string(b)
	for _, k := range []string{"ODOO_URL=", "ODOO_PASSWORD=", "ODOO_SESSION_ID=", "PLANESGO_TOKEN=", "ANTIGRAVITY_TOKEN=", "PGO_TOKEN_ESCRITORIO=", "PGO_TOKEN=de-otro"} {
		if strings.Contains(env, "\n"+k) {
			t.Errorf("el agente hereda %s", k)
		}
	}
	for _, k := range []string{"PGO_TOKEN=propio", "PLANESGO_GESTOR=pgocode"} {
		if !strings.Contains(env, "\n"+k) {
			t.Errorf("al agente le falta %s", k)
		}
	}
}
