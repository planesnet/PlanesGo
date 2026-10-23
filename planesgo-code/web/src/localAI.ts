// IA local (Propiedades → IA local): tipos de /api/ia-local y su estado en una línea. Funciones puras: node --test src/localAI.test.ts

export interface LocalAIState { url: string; api?: 'ollama' | 'openai'; listo: boolean; modelos?: string[]; error?: string; reserva?: boolean }
export interface LocalAITest { tipo: 'embeddings' | 'texto'; modelo?: string; ok: boolean; ms: number; detalle?: string; error?: string }
export interface LocalAIView { url: string; con_token: boolean; modelo_embeddings: string; modelo_texto?: string; estado: LocalAIState; pruebas?: LocalAITest[] }

// localAITestText: el resultado de probar un modelo, en una línea («✓ embeddinggemma · vector de 768 dimensiones · 45 ms»).
export function localAITestText(p: LocalAITest): string {
  const ms = p.ms >= 1000 ? `${(p.ms / 1000).toFixed(1).replace('.', ',')} s` : `${p.ms} ms`;
  if (!p.ok) return `✗ ${p.modelo || 'sin modelo'} · ${p.error || 'no responde'}`;
  return `✓ ${p.modelo || ''}${p.detalle ? ` · ${p.tipo === 'texto' ? '«' + p.detalle.slice(0, 40) + '»' : p.detalle}` : ''} · ${ms}`;
}

export const LOCAL_AI_DEFAULT_URL = 'http://127.0.0.1:11434';

// localAIStatusText: el estado en una línea.
export function localAIStatusText(e?: LocalAIState, model?: string): string {
  if (!e) return '—';
  if (e.listo) return `Conectado · ${e.api === 'openai' ? 'compatible con OpenAI' : 'Ollama'} · ${model || ''}`.replace(/ · $/, '');
  const err = e.error || 'no responde';
  return e.reserva ? `No responde (${err}): se usa el Ollama de este equipo` : `No responde (${err}): el hilo de limpieza sigue solo con reglas`;
}
