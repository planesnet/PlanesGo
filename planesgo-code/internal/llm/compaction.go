package llm

import (
	"fmt"
	"strconv"
	"strings"
)

// Compactación del historial gobernada por pgocode, solo como red de seguridad: lo que acota el historial es la poda
// (OpenCode) o el relevo (Claude Code con la caché caducada, Antigravity; ver web/src/contextPolicy.ts). Compactar
// pronto salió más caro en el banco de consumo (prueba 7: un 5 % más a 80k), así que el umbral por defecto es alto.
// Cada agente lo hace con lo que ofrece, sin turnos de más:
//   - Claude Code: su auto-compactación con la ventana y el porcentaje adelantados por variables de entorno
//     (CLAUDE_CODE_AUTO_COMPACT_WINDOW, que no baja de 100k, y CLAUDE_AUTOCOMPACT_PCT_OVERRIDE para bajar más);
//   - OpenCode: su compactación automática con un «contexto» del modelo a medida (OPENCODE_CONFIG_CONTENT);
//   - Antigravity: no tiene umbral configurable en modo print; pgocode le antepone «/compact» al mensaje cuando
//     el contexto del turno anterior pasa del umbral (resume y contesta en el mismo turno).
//
// Con la gestión automática del contexto desactivada (CompactAt = CompactAgentOwn), nada de eso: la compactación,
// la que traiga el agente.
const (
	CompactAtDefault = 150000
	CompactAgentOwn  = -1
	CompactAtMin     = 20000 // por debajo, las instrucciones y herramientas del agente casi lo llenan solas
	CompactAtMax     = 1000000
)

// agentOwnCompaction: sin compactación adelantada por pgocode.
func agentOwnCompaction(p Peticion) bool { return p.CompactAt < 0 }

// compactThreshold: el umbral de la petición, con su valor por defecto y dentro de los límites.
func compactThreshold(p Peticion) int {
	n := p.CompactAt
	if n <= 0 {
		n = CompactAtDefault
	}
	return min(max(n, CompactAtMin), CompactAtMax)
}

// Claude Code 2.1: compacta al llegar a min(ventana·pct/100, ventana − 13k), con ventana = la de
// CLAUDE_CODE_AUTO_COMPACT_WINDOW (de 100k a 1M) menos la reserva de salida (min(salida del modelo, 20k)).
const (
	claudeOutputReserve = 20000
	claudeCompactBuffer = 13000
	claudeMinWindow     = 100000
)

// claudeCompactEnv: las variables que hacen que Claude Code compacte a partir del umbral.
func claudeCompactEnv(p Peticion) []string {
	if agentOwnCompaction(p) {
		return nil
	}
	n := compactThreshold(p)
	w := max(claudeMinWindow, n+claudeOutputReserve+claudeCompactBuffer)
	pct := float64(n) * 100 / float64(w-claudeOutputReserve)
	return []string{"CLAUDE_CODE_AUTO_COMPACT_WINDOW=" + strconv.Itoa(w),
		"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=" + strconv.FormatFloat(min(pct, 100), 'f', 3, 64)}
}

// OpenCode 1.19 compacta cuando los tokens de la última llamada llegan a limit.context − min(limit.output, 32k):
// el «contexto» que se le declara es el umbral más esa reserva de salida (0: el del modelo).
const openCodeOutput = 32000

func openCodeContextLimit(p Peticion) int {
	if agentOwnCompaction(p) {
		return 0
	}
	return compactThreshold(p) + openCodeOutput
}

// antigravityMessage: con el contexto del turno anterior por encima del umbral, el mensaje va tras «/compact»
// (Antigravity resume la conversación y después contesta al mensaje, en el mismo turno).
func antigravityMessage(msg string, p Peticion) string {
	if agentOwnCompaction(p) || p.SesionAgente == "" || p.Aparte || p.LastContext < compactThreshold(p) || strings.HasPrefix(strings.TrimSpace(msg), "/") {
		return msg
	}
	return "/compact " + msg
}

// compactionNotice: el aviso en el chat de que el agente ha compactado su historial (pre y post: tokens de
// contexto antes y después, 0 si no se saben).
func compactionNotice(agent string, pre, post int) string {
	t := "\n\n_(" + agent + " ha compactado su historial"
	switch {
	case pre > 0 && post > 0:
		t += fmt.Sprintf(": de %s a %s tokens", kTokens(pre), kTokens(post))
	case pre > 0:
		t += fmt.Sprintf(": tenía %s tokens", kTokens(pre))
	}
	return t + ".)_\n\n"
}

func kTokens(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	return strconv.Itoa((n+500)/1000) + "k"
}
