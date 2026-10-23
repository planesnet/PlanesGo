// La caché del prompt de la sesión: cuándo caduca (lo que ve el usuario en la ventana del hilo de limpieza y en el
// aviso antes de enviar) y qué TTL se le pide a Claude Code en cada turno (internal/llm/cachettl.go).
//
// Precios relativos a la entrada (Claude): leer de la caché 0,1×; escribir 1,25× a 5 min y 2× a 1 h. Una petición a
// 1 h no lee lo escrito a 5 min (medido con Claude Code 2.1.309): pasar de 5 min a 1 h con la caché caliente obliga
// a reescribirla entera; de 1 h a 5 min, no.
//
// Medido con los turnos reales de Luis (1547 turnos de Claude, 14 días): el 80 % llega antes de 5 min, el 16 % entre
// 5 y 60 min y el 4 % después de 1 h. Con esa mezcla, todo a 1 h gasta un 16 % menos en caché que lo que hacía
// Claude Code por defecto (1 h en la primera llamada de cada turno y 5 min en las demás, que el turno siguiente
// reescribía) y un 34 % menos que todo a 5 min; elegirlo por sesión según sus pausas, un poco más.
// «Mantener caliente» (una llamada mínima antes de caducar, a 0,1× del contexto) no compensa frente a 1 h: con las
// mismas pausas sale un 12 % más caro (las de 5–60 min necesitan varias llamadas cada una).
import type { Mensaje } from './api';

export type ClaudeTTL = '5m' | '1h';

// Lo que dura la caché si el consumo no lo dice (mensajes de antes): Claude Code con suscripción, 1 h; Gemini,
// implícita (medido, cachettl.go): OpenCode, 5 min; Antigravity, 2 min.
const DEFAULT_TTL: Record<string, number> = { 'claude-code': 3600, opencode: 300, antigravity: 120 };

const READ = 0.1, WRITE_5M = 1.25, WRITE_1H = 2.0;
// Con menos pausas que estas en la sesión, 1 h (lo que sale mejor con el ritmo medido); se miran las últimas
// RECENT_GAPS.
const MIN_GAPS = 3, RECENT_GAPS = 12;

// TurnGap: la pausa antes de un turno (s), el contexto con el que acabó el anterior y lo que creció (tokens).
export interface TurnGap { gap: number; context: number; growth: number }

// turnGaps: las pausas entre turnos de la conversación (del fin de uno al principio del siguiente: su fin menos lo
// que duró), con el contexto de la última llamada del anterior.
export function turnGaps(messages: Mensaje[]): TurnGap[] {
  const turns = messages.filter(m => m.rol === 'assistant' && m.fin && m.uso?.contexto && !m.error);
  const out: TurnGap[] = [];
  for (let i = 1; i < turns.length; i++) {
    const a = turns[i - 1], b = turns[i];
    const start = b.fin! - (b.uso!.segundos || 0) * 1000;
    const gap = (start - a.fin!) / 1000;
    if (gap < 0) continue;
    out.push({ gap, context: a.uso!.contexto!, growth: Math.max(b.uso!.contexto! - a.uso!.contexto!, 0) });
  }
  return out;
}

// pauseCost: lo que cuesta en caché retomar tras la pausa (en tokens a precio de entrada).
function pauseCost(ttl: ClaudeTTL, g: TurnGap): number {
  const limit = ttl === '1h' ? 3600 : 300;
  return g.gap < limit ? READ * g.context : (ttl === '1h' ? WRITE_1H : WRITE_5M) * g.context;
}

// ttlCost: lo que costarían las pausas y lo nuevo de cada turno con ese TTL.
export function ttlCost(ttl: ClaudeTTL, gaps: TurnGap[]): number {
  return gaps.reduce((s, g) => s + pauseCost(ttl, g) + (ttl === '1h' ? WRITE_1H : WRITE_5M) * g.growth, 0);
}

// lastTurn: el último turno acabado con consumo (de él, cuándo acabó y su caché).
function lastTurn(messages: Mensaje[]): Mensaje | undefined {
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i];
    if (m.rol === 'assistant' && m.fin && m.uso?.contexto && !m.error) return m;
  }
  return undefined;
}

// chooseClaudeTTL: el TTL que se pide a Claude Code en el próximo turno de la sesión: el que menos cuesta con sus
// pausas recientes, con la de ahora (con pocas, 1 h). Si la caché sigue caliente a 5 min, se queda en 5 min (pasar
// a 1 h la reescribiría entera).
export function chooseClaudeTTL(messages: Mensaje[], now = Date.now()): ClaudeTTL {
  const last = lastTurn(messages);
  const all = turnGaps(messages);
  if (last) all.push({ gap: Math.max(now - last.fin!, 0) / 1000, context: last.uso!.contexto!, growth: 0 });
  const gaps = all.slice(-RECENT_GAPS);
  const want: ClaudeTTL = gaps.length < MIN_GAPS ? '1h' : ttlCost('1h', gaps) <= ttlCost('5m', gaps) ? '1h' : '5m';
  if (want === '1h' && last?.uso?.cache_ttl === 300 && now < last.fin! + 300_000) return '5m';
  return want;
}

// CacheState: la caché de la sesión ahora: lo que dura (s), cuándo caduca (ms), si sigue caliente y el contexto
// que se reenviará en el próximo mensaje.
export interface CacheState { ttl: number; expiresAt: number; warm: boolean; context: number }

export function cacheState(messages: Mensaje[], agent: string, now = Date.now()): CacheState | null {
  const last = lastTurn(messages);
  const ttl = last?.uso?.cache_ttl || DEFAULT_TTL[agent] || 0;
  if (!last || !ttl) return null;
  const expiresAt = last.fin! + ttl * 1000;
  return { ttl, expiresAt, warm: now < expiresAt, context: last.uso!.contexto! };
}

// duration: «menos de 1 min», «3 min», «1 h 5 min».
export function duration(ms: number): string {
  const min = Math.floor(ms / 60_000);
  if (min < 1) return 'menos de 1 min';
  if (min < 60) return `${min} min`;
  const h = Math.floor(min / 60), r = min % 60;
  return r ? `${h} h ${r} min` : `${h} h`;
}

const k = (n: number) => n >= 1000 ? `${Math.round(n / 1000)} k` : String(n);

// cacheLine: la línea de la ventana del hilo de limpieza (solo tokens, nada de dinero). Claude reescribe la caché
// (a precio de escritura); Gemini la vuelve a leer a precio de entrada.
export function cacheLine(st: CacheState | null, agent: string, now = Date.now()): string {
  if (!st) return '';
  const ttl = duration(st.ttl * 1000);
  if (st.warm) return `Caché caliente: caduca en ${duration(st.expiresAt - now)} (dura ${ttl} desde la última llamada).`;
  const what = agent === 'claude-code' ? `escribirá de nuevo en caché ${k(st.context)} tokens` : `reenviará ${k(st.context)} tokens sin caché`;
  return `Caché caducada hace ${duration(now - st.expiresAt)}: el próximo mensaje ${what}.`;
}

// cacheMinutes: lo que dura la caché de la sesión, en minutos (para los avisos de «parada sin caché»).
export function cacheMinutes(messages: Mensaje[], agent: string): number {
  const st = cacheState(messages, agent);
  return st ? st.ttl / 60 : 5;
}
