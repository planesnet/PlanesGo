package proceso

import (
	"os"
	"strings"
)

// CleanEnviron: el entorno de pgocode sin credenciales ni sesiones que un agente o un programa lanzado no deben
// heredar (docs/odoo-blindaje.md): las de Odoo (ODOO_*), el token de PlanesGo (que escribe en Odoo), el secreto de
// la app de escritorio y un token de agente de otro pgocode (si este se lanzó desde un agente).
func CleanEnviron() []string {
	return CleanEnv(os.Environ())
}

// CleanEnv: el entorno dado sin las variables secretas (ver CleanEnviron).
func CleanEnv(env []string) []string {
	l := make([]string, 0, len(env))
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if SecretVar(k) {
			continue
		}
		l = append(l, kv)
	}
	return l
}

// SecretVar: la variable no pasa a los agentes ni a los programas.
func SecretVar(k string) bool {
	switch strings.ToUpper(k) {
	case "PLANESGO_TOKEN", "ANTIGRAVITY_TOKEN", "PGO_TOKEN", "PGO_TOKEN_ESCRITORIO":
		return true
	}
	return strings.HasPrefix(strings.ToUpper(k), "ODOO_")
}
