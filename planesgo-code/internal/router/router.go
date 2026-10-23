// Package router: la tabla de rutas del ENRUTADOR DE INTENCIONES de pgocode y su registro.
//
// La decisión de quién hace qué con cada mensaje la toma la interfaz (web/src/router.ts), que es donde están el
// mensaje, la sesión y el estado del agente; aquí no se repite esa lógica. Este paquete tiene lo que el servidor
// necesita para registrar y medir las decisiones: los nombres válidos (los mismos que router.ts), qué es cada ruta
// (quién la ejecuta, cuánto cuesta, dónde está el código) y los campos del suceso «ruta» del registro de tiempos.
// La documentación completa está en docs/enrutador.md.
package router

import (
	"errors"
	"strings"
)

// Destinos, en el orden en que se evalúan (el primero que encaja con seguridad gana).
const (
	Command = "command" // orden de pgocode (/help, órdenes de planes)
	Direct  = "direct"  // función directa: lo mecánico y las preguntas de estado (0 tokens)
	Aside   = "aside"   // pregunta aparte: no entra en la conversación
	Split   = "split"   // reparto en subtareas: sesiones paralelas en worktrees
	Agent   = "agent"   // el agente de la sesión
)

// Route: una fila de la tabla de rutas.
type Route struct {
	Name     string
	Executor string // quién hace el trabajo
	Cost     string // lo que cuesta
	Code     string // dónde está el código
}

// Routes: la tabla, en orden de evaluación. La IA local no es un destino del mensaje principal: responde las
// preguntas aparte sueltas y decide o redacta (ver docs/enrutador.md).
var Routes = []Route{
	{Command, "pgocode (o el agente para crear, revisar o ejecutar un plan)", "0 tokens", "web/src/ayuda.ts, web/src/plan.ts, Chat.tsx ordenPlan"},
	{Direct, "pgocode", "0 tokens (mensaje de commit: IA local o reglas)", "web/src/direct.ts, internal/servidor/directo.go"},
	{Aside, "pgocode (estado), copia de la sesión del agente (contexto), IA local o modelo barato (suelta)", "0 o lo de la copia o el modelo barato", "web/src/aparte.ts, internal/servidor/aparte.go"},
	{Split, "sesiones paralelas de pgocode (worktree y rama pgo/… por parte)", "lo de cada hija", "web/src/reparto.ts, internal/servidor/reparto.go, internal/repo/reparto.go, internal/llm/parallel.go"},
	{Agent, "el agente (Claude Code, OpenCode, Antigravity)", "lo que reenvía el agente en cada paso", "internal/llm/agentes.go, internal/servidor/servidor.go, inyecta.go"},
}

// Quién ejecuta, quién decide y los modificadores válidos (los mismos que router.ts).
var (
	Executors = set("pgocode", "agent", "agent-copy", "local-ai", "cheap-model", "subsessions")
	Engines   = set("user", "rules", "learned", "embeddings", "local-ai", "cheap-model")
	Modifiers = set("queue", "turn", "now", "plan", "skill-user", "attachments", "cut", "skill", "subagents", "relay")
)

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// Known: si es un destino de la tabla.
func Known(name string) bool {
	for _, r := range Routes {
		if r.Name == name {
			return true
		}
	}
	return false
}

// Decision: lo que manda la interfaz al registrar una decisión (POST /api/ruta).
type Decision struct {
	Sesion    string   `json:"sesion"`
	Route     string   `json:"destino"`
	Executor  string   `json:"ejecuta"`
	Engine    string   `json:"motor"`
	Reason    string   `json:"motivo"`
	Modifiers []string `json:"modificadores"`
	Detail    string   `json:"detalle"`
	Ms        int64    `json:"ms"`
}

// Validate: nombres conocidos y textos acotados (lo que llega de la interfaz se registra tal cual).
func (d *Decision) Validate() error {
	switch {
	case strings.TrimSpace(d.Sesion) == "":
		return errors.New("falta la sesión")
	case !Known(d.Route):
		return errors.New("destino desconocido: " + d.Route)
	case !Executors[d.Executor]:
		return errors.New("ejecutor desconocido: " + d.Executor)
	case !Engines[d.Engine]:
		return errors.New("motor desconocido: " + d.Engine)
	case len(d.Modifiers) > len(Modifiers):
		return errors.New("demasiados modificadores")
	}
	for _, m := range d.Modifiers {
		if !Modifiers[m] {
			return errors.New("modificador desconocido: " + m)
		}
	}
	if d.Ms < 0 || d.Ms > 10*60*1000 {
		d.Ms = 0
	}
	d.Reason = cut(d.Reason, 200)
	d.Detail = cut(d.Detail, 40)
	return nil
}

// Fields: los campos del suceso «ruta» del registro de tiempos (con AnotaAgente se añaden sesión, proyecto, tarea
// y agente).
func (d Decision) Fields() map[string]any {
	c := map[string]any{"destino": d.Route, "ejecuta": d.Executor, "motor": d.Engine, "motivo": d.Reason, "ms": d.Ms}
	if len(d.Modifiers) > 0 {
		c["modificadores"] = strings.Join(d.Modifiers, ",")
	}
	if d.Detail != "" {
		c["detalle"] = d.Detail
	}
	return c
}

func cut(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
