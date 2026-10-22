// El hilo de limpieza (el recolector de vigencia de cada sesión, internal/llm/prune*.go) visto desde el chat: el
// indicador junto al redactor y su ventanita. Nada de esto gasta tokens: son datos que ya llegan del agente (el
// contexto de cada llamada) y del propio recolector (que solo usa la IA local, Ollama).
// Funciones puras: node --test src/cleaner.test.ts
import type { Mensaje, Parte } from './api';
import type { PruneReport, Vigencia } from './prune';

// CollectorStatus: el estado del recolector (llm.CollectorStatus).
export interface CollectorStatus {
  fase: '' | 'marcando' | 'barriendo';
  pendientes?: number;          // piezas en las que está
  motor: 'ollama' | 'reglas';
  retirados_sesion?: number;    // tokens retirados en la sesión
  ultimos_retirados?: number;   // en el último barrido aplicado
  ultimo_barrido?: number;      // cuándo (ms)
}

// contextWindow: la ventana de contexto del modelo (tokens). Claude: 200k (1M con «[1m]»); Gemini: 1M.
export function contextWindow(agent: string, model?: string): number {
  const m = (model || '').toLowerCase();
  if (m.includes('[1m]')) return 1_000_000;
  if (agent === 'claude-code') return 200_000;
  if (agent === 'antigravity' || agent === 'opencode' || m.includes('gemini')) return 1_048_576;
  return 200_000;
}

export const pct = (tokens: number, window: number) => (window > 0 ? Math.round((tokens / window) * 1000) / 10 : 0);

// ContextPoint: el contexto de un turno (la última llamada) y, si al empezarlo se podó, cuánto.
export interface ContextPoint {
  tokens: number;
  pct: number;
  pruned?: number;       // tokens retirados al empezar el turno
  relay?: boolean;       // el turno fue un relevo (conversación nueva del agente)
}

// contextSeries: la evolución del contexto, turno a turno (los últimos «max»).
export function contextSeries(ms: Mensaje[], window: number, max = 40): ContextPoint[] {
  const out: ContextPoint[] = [];
  let relay = false;
  for (const m of ms) {
    if (m.rol === 'user') { if (m.relevo) relay = true; continue; }
    const t = m.uso?.contexto;
    if (!t) continue;
    const p: ContextPoint = { tokens: t, pct: pct(t, window) };
    if (m.pruned?.tokens) p.pruned = m.pruned.tokens;
    if (relay) { p.relay = true; relay = false; }
    out.push(p);
  }
  return out.slice(-max);
}

const k = (n: number) => (n >= 1000 ? `${Math.round(n / 1000)} k` : String(n));
const coma = (n: number) => String(n).replace('.', ',');

// pruneLabel: la marca de una poda en la evolución («−18 k, 62 % → 41 %»).
export function pruneLabel(series: ContextPoint[], i: number): string {
  const p = series[i];
  if (!p?.pruned) return '';
  const antes = i > 0 ? series[i - 1].pct : undefined;
  return `−${k(p.pruned)}` + (antes !== undefined ? `, ${coma(antes)} % → ${coma(p.pct)} %` : '');
}

// sparkPath: la línea (SVG) de la evolución, en un recuadro w×h; y de cada punto, su x e y.
export function sparkPoints(series: ContextPoint[], w: number, h: number): { x: number; y: number }[] {
  if (!series.length) return [];
  const top = Math.max(...series.map(p => p.pct), 1);
  const dx = series.length > 1 ? w / (series.length - 1) : 0;
  return series.map((p, i) => ({ x: Math.round(i * dx * 10) / 10, y: Math.round((h - (p.pct / top) * (h - 2) - 1) * 10) / 10 }));
}

export function sparkPath(series: ContextPoint[], w: number, h: number): string {
  return sparkPoints(series, w, h).map((p, i) => `${i ? 'L' : 'M'}${p.x},${p.y}`).join(' ');
}

// projected: el contexto que quedaría tras el próximo barrido (lo que está fuera de vigencia y sigue dentro).
export function projected(context: number, pending: number): number {
  return Math.max(context - Math.max(pending, 0), 0);
}

// pieceCounts: cuántas piezas (herramientas) hay en el contexto del agente y cuántas fuera.
export function pieceCounts(r?: PruneReport | null): { in: number; out: number } {
  const ps = r?.piezas || [];
  const out = ps.filter(p => p.fuera).length;
  return { in: ps.length - out, out };
}

// HOW: cómo se aplica la vigencia en cada agente.
export const HOW: Record<string, string> = {
  opencode: 'poda de su historial al empezar cada turno',
  'claude-code': 'poda en su historial (entre turnos)',
  antigravity: 'relevo por vigencia (no deja tocar su historial)',
};

// statusLine: el estado en una línea («al día», «marcando 3 piezas», «barriendo»).
export function statusLine(st?: CollectorStatus | null, busy = false): string {
  if (st?.fase === 'barriendo') return st.pendientes ? `barriendo (${st.pendientes} piezas)` : 'barriendo';
  if (st?.fase === 'marcando' || busy) return st?.pendientes ? `marcando ${st.pendientes} ${st.pendientes === 1 ? 'pieza' : 'piezas'}` : 'marcando';
  return 'al día';
}

export function engineLine(st?: CollectorStatus | null): string {
  return st?.motor === 'reglas' ? 'solo reglas (Ollama no responde)' : 'Ollama local · embeddinggemma';
}

// ago: hace cuánto («hace 3 min»).
export function ago(ms: number | undefined, now = Date.now()): string {
  if (!ms) return 'nunca';
  const m = Math.max(0, Math.round((now - ms) / 60000));
  if (m < 1) return 'ahora mismo';
  if (m < 60) return `hace ${m} min`;
  const h = Math.round(m / 60);
  return h < 48 ? `hace ${h} h` : `hace ${Math.round(h / 24)} días`;
}

// ---- Antigravity: la vigencia sale de la conversación de pgocode (POST /api/chat/vigencia) ----

// ChatExchange: un intercambio para el servidor (llm.ChatExchange).
export interface ChatExchange {
  punto: string;
  usuario: string;
  respuesta: string;
  en?: number;
  herramientas?: { id: string; nombre: string; resumen?: string; salida?: string; imagenes?: number; error?: boolean; tamano?: number }[];
}

// relayStart: dónde empieza la conversación actual del agente (el último relevo; 0 si no hubo).
export function relayStart(ms: Mensaje[]): number {
  for (let i = ms.length - 1; i >= 0; i--) if (ms[i].rol === 'user' && ms[i].relevo) return i;
  return 0;
}

function lastText(m: Mensaje): string {
  const ps = m.partes || [];
  for (let i = ps.length - 1; i >= 0; i--) { const p = ps[i]; if (p.tipo === 'texto' && p.texto.trim()) return p.texto.trim(); }
  return (m.texto || '').trim();
}

// chatExchanges: los intercambios de la conversación actual del agente. Cada uno se reconoce por la posición de la
// respuesta en el chat («m12») y cada herramienta, por la posición y su id («m12:paso-3»: los de Antigravity se
// repiten de un turno a otro).
export function chatExchanges(ms: Mensaje[]): ChatExchange[] {
  const out: ChatExchange[] = [];
  for (let i = relayStart(ms); i < ms.length; i++) {
    const u = ms[i];
    if (u.rol !== 'user' || u.fuera) continue;
    const a = ms[i + 1];
    if (!a || a.rol !== 'assistant' || a.error || a.fuera || !a.uso) continue;
    const hs = (a.partes || []).filter((p): p is Extract<Parte, { tipo: 'herramienta' }> => p.tipo === 'herramienta').map(p => ({
      id: `m${i + 1}:${p.h.id}`, nombre: p.h.nombre, resumen: p.h.resumen, salida: p.h.salida,
      ...(p.h.capturas?.length ? { imagenes: p.h.capturas.length } : {}), ...(p.h.estado === 'error' ? { error: true } : {}),
      ...(p.h.tamano ? { tamano: p.h.tamano } : {}),
    }));
    out.push({ punto: `m${i + 1}`, usuario: u.texto || '', respuesta: lastText(a), ...(a.fin ? { en: a.fin } : {}), ...(hs.length ? { herramientas: hs } : {}) });
  }
  return out;
}

function vigOf(x: Vigencia): Vigencia {
  const v: Vigencia = { vigencia: x.vigencia };
  for (const c of ['hace', 'relacion', 'horas', 'superada', 'nota'] as const) if (x[c] !== undefined && x[c] !== '') (v as unknown as Record<string, unknown>)[c] = x[c];
  if (x.fuera) v.fuera = true;
  return v;
}

const same = (a?: Vigencia, b?: Vigencia) => JSON.stringify(a) === JSON.stringify(b);

// applyChatVigencia: la vigencia del informe en el chat (por las posiciones de chatExchanges). Lo que no cambia es
// el mismo objeto.
export function applyChatVigencia(ms: Mensaje[], r: PruneReport): Mensaje[] {
  const out = ms.slice();
  const piezas = new Map<number, Map<string, Vigencia>>();
  for (const p of r.piezas || []) {
    const mt = /^m(\d+):(.*)$/.exec(p.id);
    if (!mt) continue;
    const i = Number(mt[1]);
    if (!piezas.has(i)) piezas.set(i, new Map());
    piezas.get(i)!.set(mt[2], vigOf(p));
  }
  for (const x of r.intercambios) {
    const i = Number(/^m(\d+)$/.exec(x.punto)?.[1] ?? -1);
    if (i < 1 || i >= ms.length || ms[i].rol !== 'assistant' || ms[i - 1].rol !== 'user') continue;
    const v = vigOf(x);
    if (!same(out[i].vig, v)) out[i] = { ...out[i], vig: v };
    if (!same(out[i - 1].vig, v)) out[i - 1] = { ...out[i - 1], vig: v };
  }
  for (const [i, mp] of piezas) {
    const m = out[i];
    if (!m?.partes) continue;
    let cambia = false;
    const ps = m.partes.map((p): Parte => {
      if (p.tipo !== 'herramienta') return p;
      const v = mp.get(p.h.id);
      if (!v || same(p.h.vig, v)) return p;
      cambia = true;
      return { tipo: 'herramienta', h: { ...p.h, vig: v } };
    });
    if (cambia) out[i] = { ...m, partes: ps };
  }
  return out;
}

// markRelayedOut: al relevar, lo que quedó fuera de la memoria (fuera de vigencia) pasa a «fuera del contexto».
export function markRelayedOut(ms: Mensaje[]): Mensaje[] {
  return ms.map(m => (m.vig && m.vig.vigencia >= 3 && !m.vig.fuera ? { ...m, vig: { ...m.vig, fuera: true } } : m));
}
