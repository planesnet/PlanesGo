// Gestión automática del contexto: lo que hace pgocode con el historial de cada agente, en un solo sitio. Cada agente
// reenvía todo su historial en cada paso; pgocode lo acota con lo que cada uno permite:
//   - OpenCode: la poda (internal/llm/prune.go): al empezar cada turno retira lo que ya no importa.
//   - Claude Code: el relevo (workingMemory.ts), solo con la caché caducada, que es cuando compensa siempre; con la
//     caché caliente sigue la misma sesión (hasta que tenga poda).
//   - Antigravity: el relevo, que no deja tocar su historial.
//   - Los tres: la compactación de reserva (internal/llm/compaction.go, llm.CompactAtDefault = 150k).
// Con el ajuste de la sesión «Gestión automática del contexto» desactivado, nada de eso: sin poda, sin relevo y la
// compactación, la que traiga el agente. Funciones puras: node --test src/contextPolicy.test.ts
import type { Mensaje } from './api';

// RelayMode: cuándo va el mensaje a una sesión nueva del agente con la memoria de trabajo (shouldRelay).
// off: nunca; idle: solo con la caché caducada; on: también al pasar del umbral (RELAY_THRESHOLD).
export type RelayMode = 'off' | 'idle' | 'on';

export interface ContextPolicy {
  prune: boolean;     // poda del historial al empezar cada turno (el servidor solo la sabe hacer en OpenCode)
  relay: RelayMode;
  compactAt: number;  // para el servidor: 0, la compactación de reserva de pgocode; -1, la del agente
}

// COMPACT_AGENT_OWN: sin compactación adelantada (llm.CompactAgentOwn).
export const COMPACT_AGENT_OWN = -1;

const POLICIES: Record<string, Omit<ContextPolicy, 'compactAt'>> = {
  opencode: { prune: true, relay: 'off' },
  'claude-code': { prune: false, relay: 'idle' },
  antigravity: { prune: false, relay: 'on' },
};

const OFF: ContextPolicy = { prune: false, relay: 'off', compactAt: COMPACT_AGENT_OWN };

// autoContextOn: si la sesión gestiona el contexto (por defecto, sí).
export const autoContextOn = (s: { autoContext?: boolean }) => s.autoContext !== false;

// contextPolicy: lo que hace pgocode con el contexto del agente de la sesión.
export function contextPolicy(agent: string, s: { autoContext?: boolean }): ContextPolicy {
  if (!autoContextOn(s)) return OFF;
  return { ...(POLICIES[agent] ?? { prune: false, relay: 'off' }), compactAt: 0 };
}

// lastContext: los tokens de contexto de la última llamada del último turno con consumo (0 si no se sabe).
export function lastContext(m: Mensaje[]): number {
  for (let i = m.length - 1; i >= 0; i--) if (m[i].rol === 'assistant' && m[i].uso?.contexto) return m[i].uso!.contexto!;
  return 0;
}
