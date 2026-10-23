package llm

import "time"

// Caché del prompt (lo que el modelo ya tiene del historial y cobra a 0,1× al releerlo).
//
// Claude Code (2.1.309, medido con turnos reales): sin CLAUDE_CODE_PROMPT_CACHE_TTL, con suscripción, la primera
// llamada de cada turno escribe en la caché de 1 h y las demás del turno (el bucle de herramientas) en la de
// 5 min; y una petición de 1 h no lee lo escrito a 5 min. Así, cada turno reescribe a 2× todo lo que creció el
// turno anterior aunque siga en caché (en el historial de Luis: 15,7 M de tokens en 1277 turnos con la caché
// caliente, 12k por turno). Con la variable, todas las llamadas usan el mismo TTL y nada se reescribe: pgocode
// la pone siempre, con el TTL que decide la interfaz para cada sesión según el ritmo del usuario (cacheStatus.ts).
// Escribir cuesta 1,25× a 5 min y 2× a 1 h.
//
// Gemini (OpenCode, Antigravity): caché implícita, gratis al escribir y sin TTL que se pueda elegir. Medido el
// 23-10-2026: con OpenCode (gemini-3.1-flash-lite en Vertex) lo enviado seguía en caché a los 1, 3, 5 y 7 min y no
// a los 10; en las sesiones de Luis con gemini-4-argon, de los turnos que llegaron entre 5 y 10 min acertaron 3 de
// 7 y después de 10 min, ninguno: se cuentan 5 min (openCodeCacheTTL). Con Antigravity (agy, gemini-3.8-flash-low),
// a 1 y 2 min sí y a 3 y 5 min ya no (como mucho, lo fijo del agente): se cuentan 2 min (antigravityCacheTTL).

const (
	CacheTTL5m = "5m"
	CacheTTL1h = "1h"
)

// antigravityCacheTTL: lo que dura la caché implícita de Antigravity (medido: entre 2 y 3 min).
const antigravityCacheTTL = 2 * time.Minute

// claudeTTLOverride: PGOCODE_CLAUDE_CACHE_TTL (una duración de Go, para las pruebas: «1s» deja la caché «fría» sin
// esperar). Solo cambia lo que pgocode supone, no lo que pide a Claude Code.
var claudeTTLOverride time.Duration

// ClaudeCacheTTL: el TTL que pide pgocode a Claude Code para la petición («5m» si no dice otro válido).
func ClaudeCacheTTL(p Peticion) string {
	if p.CacheTTL == CacheTTL1h {
		return CacheTTL1h
	}
	return CacheTTL5m
}

// claudeTTL: lo que dura la caché que escriben las llamadas de la petición.
func claudeTTL(p Peticion) time.Duration {
	if claudeTTLOverride > 0 {
		return claudeTTLOverride
	}
	return ttlDuration(ClaudeCacheTTL(p))
}

func ttlDuration(ttl string) time.Duration {
	if ttl == CacheTTL1h {
		return time.Hour
	}
	return 5 * time.Minute
}

// claudeWriteFactor: el precio de escribir en la caché, relativo a la entrada.
func claudeWriteFactor(ttl time.Duration) float64 {
	if ttl > 5*time.Minute {
		return 2.0
	}
	return 1.25
}

// claudeCacheEnv: la variable que fija el TTL de todas las llamadas de la conversación principal.
func claudeCacheEnv(p Peticion) []string {
	return []string{"CLAUDE_CODE_PROMPT_CACHE_TTL=" + ClaudeCacheTTL(p)}
}

// claudeEnv: el entorno propio de Claude Code para la petición (compactación y caché).
func claudeEnv(p Peticion) []string {
	return append(claudeCompactEnv(p), claudeCacheEnv(p)...)
}

// cacheTTLSeconds: lo que dura la caché de lo que acaba de enviar el agente (para la interfaz; 0: no se sabe).
func cacheTTLSeconds(agent string, p Peticion) int {
	switch agent {
	case "claude-code":
		return int(claudeTTL(p).Seconds())
	case "opencode":
		return int(openCodeCacheTTL.Seconds())
	case "antigravity":
		return int(antigravityCacheTTL.Seconds())
	}
	return 0
}
