// Compactación del historial del agente, gobernada por pgocode (ver internal/llm/compaction.go): a partir de
// cuántos miles de tokens de contexto compacta cada sesión y el contexto del último turno que se le pasa.
import type { Mensaje } from './api';

export const COMPACT_AT_DEFAULT_K = 80; // = llm.CompactAtDefault
export const COMPACT_AT_MIN_K = 20;
export const COMPACT_AT_MAX_K = 1000;

// compactAtTokens: el umbral de la sesión en tokens para el servidor (0: el valor por defecto).
export function compactAtTokens(k?: number): number {
  if (!k || !Number.isFinite(k)) return 0;
  return Math.min(Math.max(Math.round(k), COMPACT_AT_MIN_K), COMPACT_AT_MAX_K) * 1000;
}

// lastContext: los tokens de contexto de la última llamada del último turno con consumo (0 si no se sabe).
export function lastContext(m: Mensaje[]): number {
  for (let i = m.length - 1; i >= 0; i--) if (m[i].rol === 'assistant' && m[i].uso?.contexto) return m[i].uso!.contexto!;
  return 0;
}
