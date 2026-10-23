package servidor

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Tokens de los agentes (PGO_TOKEN) por sesión y turno: se crean al lanzar el agente y se revocan al acabar su
// turno. Cada token dice de qué proyecto y sesión de pgocode es; el servidor lo comprueba en cada orden «pgo» y
// solo atiende las de la lista blanca (agentEndpoints). Ninguna toca Odoo: Odoo lo escribe solo la imputación de
// pgocode (internal/imputacion), nunca un agente (docs/odoo-blindaje.md).

// agentTokenTTL: lo más que vale un token aunque el turno no acabe (un turno colgado no deja una llave eterna).
const agentTokenTTL = 12 * time.Hour

// agentGrant: a quién pertenece un token de agente.
type agentGrant struct {
	project int
	session string
	expires time.Time
}

// agentTokens: los tokens vivos (solo en memoria: no sobreviven a un reinicio).
type agentTokens struct {
	mu  sync.Mutex
	m   map[string]agentGrant
	now func() time.Time
}

func (a *agentTokens) clock() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// issue crea el token de un turno; revoke lo anula.
func (a *agentTokens) issue(project int, session string) (token string, revoke func()) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", func() {}
	}
	token = hex.EncodeToString(b)
	a.mu.Lock()
	if a.m == nil {
		a.m = map[string]agentGrant{}
	}
	// De paso, fuera los caducados.
	ahora := a.clock()
	for k, g := range a.m {
		if !ahora.Before(g.expires) {
			delete(a.m, k)
		}
	}
	a.m[token] = agentGrant{project: project, session: session, expires: ahora.Add(agentTokenTTL)}
	a.mu.Unlock()
	return token, func() {
		a.mu.Lock()
		delete(a.m, token)
		a.mu.Unlock()
	}
}

// lookup: el permiso del token, si existe y no ha caducado.
func (a *agentTokens) lookup(token string) (agentGrant, bool) {
	if token == "" {
		return agentGrant{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	g, ok := a.m[token]
	if !ok {
		return agentGrant{}, false
	}
	if !a.clock().Before(g.expires) {
		delete(a.m, token)
		return agentGrant{}, false
	}
	return g, true
}

// agentEndpoints: la lista blanca de órdenes del agente (/api/pgo/<orden>) y su método. Todo lo demás, 403.
// Ninguna lee ni escribe en Odoo; añadir aquí una que lo haga rompe TestAgentEndpointsNoOdoo.
var agentEndpoints = map[string]string{
	"ejecutar": http.MethodPost,
	"parar":    http.MethodPost,
	"procesos": http.MethodGet,
	"salida":   http.MethodGet,
	"abrir":    http.MethodPost,
	// Google en solo lectura, a través del navegador integrado (internal/gbrowser).
	"drive-buscar": http.MethodGet,
	"drive-listar": http.MethodGet,
	"drive-leer":   http.MethodGet,
	"gmail-buscar": http.MethodGet,
	"gmail-listar": http.MethodGet,
	"gmail-leer":   http.MethodGet,
}

// agentEndpointAllowed: la orden está en la lista blanca con ese método.
func agentEndpointAllowed(orden, metodo string) bool {
	m, ok := agentEndpoints[orden]
	return ok && m == metodo
}

// agentToken: el token de agente de la petición y su permiso (si vale).
func (s *Servidor) agentToken(r *http.Request) (agentGrant, bool) {
	return s.agentes.lookup(strings.TrimSpace(r.Header.Get("X-PGO-Agente")))
}

// issueAgentToken: el token del turno de un agente (vacío si los agentes no tienen la orden «pgo»).
func (s *Servidor) issueAgentToken(project int, session string) (string, func()) {
	if s.TokenAgente == "" || project <= 0 {
		return "", func() {}
	}
	return s.agentes.issue(project, session)
}
