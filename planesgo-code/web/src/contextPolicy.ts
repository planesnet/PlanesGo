// Gestión automática del contexto: lo que hace pgocode con el historial de cada agente, en un solo sitio. Cada agente
// reenvía todo su historial en cada paso; pgocode lo acota con lo que cada uno permite:
//   - OpenCode: la poda (internal/llm/prune.go): al empezar cada turno retira lo que ya no importa.
//   - Claude Code: la misma poda, en su historial (internal/llm/prune_claude.go). El relevo con la caché caducada
//     (workingMemory.ts) queda de reserva, solo si su historial no se puede podar (informe sin «podable»).
//   - Antigravity: no deja tocar su historial (lo guarda cifrado): pgocode calcula la vigencia con su propio chat
//     (POST /api/chat/vigencia) y, cuando lo que está fuera de vigencia pesa bastante, releva (modo «vig»).
//     Su vigencia sale del chat (vigSource «chat»); la de los demás, del recolector de su historial («agent»).
//   - Los tres: la compactación de reserva (internal/llm/compaction.go, llm.CompactAtDefault = 150k).
// Con el ajuste de la sesión «Gestión automática del contexto» desactivado, nada de eso: sin poda, sin relevo y la
// compactación, la que traiga el agente. Funciones puras: node --test src/contextPolicy.test.ts
import type { Mensaje } from './api';

// RelayMode: cuándo va el mensaje a una sesión nueva del agente con la memoria de trabajo (shouldRelay).
// off: nunca; idle: solo con la caché caducada; on: también al pasar del umbral (RELAY_THRESHOLD; el banco de
// consumo); vig: también cuando lo que está fuera de vigencia pasa de PRUNE_MIN_TOKENS (Antigravity).
export type RelayMode = 'off' | 'idle' | 'on' | 'vig';

// VigSource: de dónde sale la vigencia: del recolector del historial del agente o de la conversación de pgocode.
export type VigSource = 'agent' | 'chat' | 'none';

export interface ContextPolicy {
  prune: boolean;     // poda del historial del agente al empezar cada turno (OpenCode y Claude Code)
  relay: RelayMode;
  vig: VigSource;
  compactAt: number;  // para el servidor: 0, la compactación de reserva de pgocode; -1, la del agente
}

// COMPACT_AGENT_OWN: sin compactación adelantada (llm.CompactAgentOwn).
export const COMPACT_AGENT_OWN = -1;

const POLICIES: Record<string, Omit<ContextPolicy, 'compactAt'>> = {
  opencode: { prune: true, relay: 'off', vig: 'agent' },
  'claude-code': { prune: true, relay: 'idle', vig: 'agent' },
  antigravity: { prune: false, relay: 'vig', vig: 'chat' },
};

const OFF: ContextPolicy = { prune: false, relay: 'off', vig: 'none', compactAt: COMPACT_AGENT_OWN };

// autoContextOn: si la sesión gestiona el contexto (por defecto, sí).
export const autoContextOn = (s: { autoContext?: boolean }) => s.autoContext !== false;

// contextPolicy: lo que hace pgocode con el contexto del agente de la sesión.
export function contextPolicy(agent: string, s: { autoContext?: boolean }): ContextPolicy {
  if (!autoContextOn(s)) return OFF;
  return { ...(POLICIES[agent] ?? { prune: false, relay: 'off', vig: 'none' }), compactAt: 0 };
}

// relayAsBackup: con poda, el relevo solo queda de reserva: si el último informe dice que el historial se puede
// podar, no hace falta (la poda con la caché fría sale gratis y no pierde nada).
export function relayAsBackup(p: ContextPolicy, prunable: boolean | undefined): RelayMode {
  return p.prune && prunable ? 'off' : p.relay;
}

// lastContext: los tokens de contexto de la última llamada del último turno con consumo (0 si no se sabe).
export function lastContext(m: Mensaje[]): number {
  for (let i = m.length - 1; i >= 0; i--) if (m[i].rol === 'assistant' && m[i].uso?.contexto) return m[i].uso!.contexto!;
  return 0;
}
