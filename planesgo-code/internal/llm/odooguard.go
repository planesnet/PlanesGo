package llm

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"planesgo-code/internal/clients"
)

// Blindaje del Odoo de PlanesNet (docs/odoo-blindaje.md): en Odoo solo escribe la imputación de pgocode, nunca un
// agente. Aquí, la capa de los permisos de cada agente (no es la única: el servidor no atiende a los agentes fuera de
// /api/pgo, la sesión de Odoo no se guarda en disco, el entorno va sin credenciales y el MCP de imputación no se
// ofrece). Se deniega:
//   - la red hacia el Odoo y hacia el servidor de PlanesGo (que también escribe en Odoo): WebFetch y las órdenes que
//     nombren su host;
//   - leer los ficheros de datos de pgocode (sesiones, imputación) y las credenciales de PlanesGo;
//   - ejecutar los clientes de imputación de PlanesGo (pgo-mcp, planesgo-mcp, planesgo-track, sus hooks).
// Las reglas son fijas durante la ejecución de pgocode (no rompen la caché del prefijo).

// AgentGuard: lo que ningún agente de pgocode puede tocar. Lo rellena main a partir de su configuración.
type AgentGuard struct {
	// Hosts: host o host:puerto (de -odoo y -planesgo).
	Hosts []string
	// ReadDeny: rutas absolutas (admiten «*» y «**» al estilo gitignore) que los agentes no pueden leer.
	ReadDeny []string
	// CommandWords: trozos de orden que no se pueden ejecutar (ficheros secretos, clientes de imputación…).
	CommandWords []string
	// Programs: órdenes que no se pueden lanzar (al principio de la orden).
	Programs []string
}

var (
	guardMu sync.RWMutex
	guard   AgentGuard
)

// SetAgentGuard fija el blindaje de los agentes (al arrancar).
func SetAgentGuard(g AgentGuard) {
	guardMu.Lock()
	guard = g
	guardMu.Unlock()
}

func currentGuard() AgentGuard {
	guardMu.RLock()
	defer guardMu.RUnlock()
	return guard
}

// DesktopAppName: the desktop app's name (productName in electron/package.json), the folder of its data.
const DesktopAppName = "Planes Go Code"

// DefaultAgentGuard: el blindaje para un pgocode con Odoo en odooURL, PlanesGo en planesgoURL y sus datos en datos.
func DefaultAgentGuard(odooURL, planesgoURL, datos string) AgentGuard {
	g := AgentGuard{}
	for _, u := range []string{odooURL, planesgoURL} {
		if h := hostOf(u); h != "" && !contains(g.Hosts, h) {
			g.Hosts = append(g.Hosts, h)
		}
	}
	if datos != "" {
		// Arriba del todo, los ficheros de pgocode (sesiones, imputación, registros); no las subcarpetas, que son
		// las copias de trabajo de las ramas (ramas/) donde trabajan los agentes, y los datos de OpenCode.
		g.ReadDeny = append(g.ReadDeny, filepath.Join(datos, "*.json"), filepath.Join(datos, "*.json.*"),
			filepath.Join(datos, "*.log"), filepath.Join(datos, "*.log.*"), filepath.Join(datos, "tiempos", "**"),
			// The audit logs of the sessions' connections (SQL, WinRM, agent desktop): the orders of every session.
			filepath.Join(datos, "*.jsonl"),
			// The pairing secret: with it, an agent could pair itself as a client app (internal/clients).
			filepath.Join(datos, clients.SecretName))
	}
	if casa, err := os.UserHomeDir(); err == nil {
		g.ReadDeny = append(g.ReadDeny, filepath.Join(casa, ".planesgo_auth.json"))
	}
	// The desktop app's data (electron/main.js, userData): its client token (cliente.json, encrypted with a key that
	// the same system user can get) and the sessions of the integrated browser, Google's among them.
	// The access the person gives to an agent outside pgocode, in a session's folder (internal/servidor/externalaccess.go):
	// pgocode's own agents have their own token, and must not take that one (another session's, perhaps).
	g.ReadDeny = append(g.ReadDeny, "/**/.pgocode/acceso-externo.env")
	if conf, err := os.UserConfigDir(); err == nil {
		g.ReadDeny = append(g.ReadDeny, filepath.Join(conf, DesktopAppName, "**"))
	}
	g.CommandWords = []string{"imputacion-dia", "sesiones-reinicio", "partes-abiertos", "planesgo_auth",
		"/.claude/hooks/planesgo", "/.local/bin/pgo-mcp", "/.local/bin/planesgo-",
		clients.PairPath, "pgocode/" + clients.SecretName, ".pgocode/acceso-externo", ".config/" + DesktopAppName, `.config/Planes\ Go\ Code`}
	g.Programs = []string{"pgo-mcp", "planesgo-mcp", "planesgo-track"}
	return g
}

// hostOf: host[:puerto] de una URL (el puerto solo si no es el de su esquema).
func hostOf(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	if h, p, err := net.SplitHostPort(u); err == nil && (p == "80" || p == "443") {
		return strings.ToLower(h)
	}
	return strings.ToLower(u)
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// bareHost: el host sin puerto.
func bareHost(h string) string {
	if x, _, err := net.SplitHostPort(h); err == nil {
		return x
	}
	return h
}

// claudeDenyRules: las reglas «deny» de Claude Code (permissions.deny en --settings).
func claudeDenyRules(g AgentGuard) []string {
	var r []string
	for _, h := range g.Hosts {
		r = append(r, "Bash(*"+h+"*)")
		// WebFetch por dominio (sin puerto); no para el propio equipo (dejaría sin WebFetch a todo lo local: un Odoo
		// simulado de pruebas en 127.0.0.1 queda cubierto por la regla de Bash y por no tener sesión).
		if b := bareHost(h); b != "localhost" && net.ParseIP(b) == nil {
			r = append(r, "WebFetch(domain:"+b+")")
		}
	}
	for _, f := range g.ReadDeny {
		// «//» delante: ruta absoluta para Claude Code.
		r = append(r, "Read(/"+f+")", "Edit(/"+f+")")
	}
	for _, w := range g.CommandWords {
		r = append(r, "Bash(*"+w+"*)")
	}
	for _, p := range g.Programs {
		r = append(r, "Bash("+p+")", "Bash("+p+" *)")
	}
	// Las herramientas de imputación del MCP de PlanesGo (no se le ofrece, pero por si llegase por otra config).
	r = append(r, "mcp__pgo__pgo_time", "mcp__pgo__pgo_project")
	return r
}

// openCodeBashDeny: los patrones de «bash» que se deniegan a OpenCode (gana la última regla que encaja; «*»,
// el permiso del modo, va primero al ordenar las claves).
func openCodeBashDeny(g AgentGuard) map[string]string {
	m := map[string]string{}
	for _, h := range g.Hosts {
		m["*"+h+"*"] = "deny"
	}
	for _, f := range g.ReadDeny {
		if b := filepath.Base(f); !strings.Contains(b, "*") {
			m["*"+b+"*"] = "deny"
		}
	}
	for _, w := range g.CommandWords {
		m["*"+w+"*"] = "deny"
	}
	for _, p := range g.Programs {
		m[p] = "deny"
		m[p+" *"] = "deny"
	}
	return m
}

// openCodeWebfetchDeny: las URL que OpenCode no puede pedir.
func openCodeWebfetchDeny(g AgentGuard) map[string]string {
	m := map[string]string{}
	for _, h := range g.Hosts {
		m["*://"+h+"*"] = "deny"
		m["*://"+h] = "deny"
	}
	return m
}

// isImputationMCP: un servidor MCP de imputación de PlanesGo (pgo-mcp / planesgo-mcp): no se ofrece a los agentes
// de pgocode, la imputación la hace pgocode.
func isImputationMCP(nombre string, cfg json.RawMessage) bool {
	if nombre == "pgo" {
		return true
	}
	s := string(cfg)
	return strings.Contains(s, "pgo-mcp") || strings.Contains(s, "planesgo-mcp")
}

// openCodeDisabledMCP: los servidores MCP de imputación de la configuración de OpenCode del usuario, desactivados
// (para OPENCODE_CONFIG_CONTENT, que se mezcla con la del usuario).
func openCodeDisabledMCP() map[string]any {
	m := map[string]any{}
	casa, err := os.UserHomeDir()
	if err != nil {
		return m
	}
	var cfg struct {
		MCP map[string]json.RawMessage `json:"mcp"`
	}
	for _, f := range []string{filepath.Join(casa, ".config", "opencode", "opencode.json"), filepath.Join(casa, ".config", "opencode", "opencode.jsonc")} {
		if b, err := os.ReadFile(f); err == nil && json.Unmarshal(b, &cfg) == nil {
			for k, v := range cfg.MCP {
				if isImputationMCP(k, v) {
					m[k] = map[string]any{"enabled": false}
				}
			}
		}
	}
	return m
}
