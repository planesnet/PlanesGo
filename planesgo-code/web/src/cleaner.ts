// El hilo de limpieza (el recolector de vigencia de cada sesión, internal/llm/prune*.go) visto desde el chat: el
// indicador junto al redactor y su ventanita. El hilo no gasta tokens: son datos que ya llegan del agente (el
// contexto de cada llamada) y del propio recolector (que solo usa la IA local, Ollama); barrer, en cambio, puede
// costar caché (lo de después del prompt de sistema se reenvía una vez sin ella).
// Funciones puras: node --test src/cleaner.test.ts
import type { Mensaje, Parte } from './api';
import type { Calibration, PruneReport, Vigencia } from './prune';

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

// COMPACT_DEFAULT: el umbral de compactación por defecto (llm.CompactAtDefault), si el informe no trae el suyo.
export const COMPACT_DEFAULT = 150_000;

// compactAt: el contexto al que compacta el agente en esta sesión (con él se mide el %: la ventana del modelo, de
// 1 M en Gemini, engañaba: «9,4 % de 1,0 M» cuando se compacta a 150k).
export function compactAt(r?: PruneReport | null): number {
  return r?.umbral || COMPACT_DEFAULT;
}

// contextLine: «99 k de 150 k (66 %)».
export function contextLine(tokens: number, threshold: number): string {
  return `${k(tokens)} de ${k(threshold)} (${String(Math.round(pct(tokens, threshold))).replace('.', ',')} %)`;
}

// ContextPoint: un punto de la evolución del contexto. Cada turno sube hasta su contexto final; una limpieza (la
// poda al empezar el turno, un «Barrer ahora» o un relevo) es una caída vertical («drop») desde el punto anterior:
// así la gráfica sale en dientes de sierra.
export interface ContextPoint {
  tokens: number;
  pct: number;
  drop?: boolean;        // caída por una limpieza (mismo instante que el punto anterior)
  pruned?: number;       // tokens retirados en esa caída
  relay?: boolean;       // la caída es un relevo (conversación nueva del agente)
  live?: boolean;        // el paso en curso del turno que está trabajando (aún no ha acabado)
  step?: number;         // con live: el paso (llamadas al modelo del turno), si se sabe
}

// contextSeries: la evolución del contexto (los últimos «max» puntos): el final de cada turno y, delante, la caída
// de la limpieza que hubo al empezarlo.
export function contextSeries(ms: Mensaje[], window: number, max = 60): ContextPoint[] {
  const out: ContextPoint[] = [];
  let relay: number | null = null;
  const last = () => out[out.length - 1];
  const fall = (tokens: number, extra: Partial<ContextPoint>) => {
    const t = Math.max(Math.round(tokens), 0);
    out.push({ tokens: t, pct: pct(t, window), drop: true, ...extra });
  };
  ms.forEach((m, i) => {
    if (m.rol === 'user') { if (m.relevo) relay = m.relevo.a || 0; return; }
    // Un barrido pedido a mano entre turnos (sweepNote): cae desde el último punto.
    if (m.nota && m.pruned?.tokens && !m.uso?.contexto) {
      if (last()) fall(last()!.tokens - m.pruned.tokens, { pruned: m.pruned.tokens });
      return;
    }
    // El turno en curso (el último mensaje, sin acabar): el contexto de su paso actual.
    const live = i === ms.length - 1 && !m.fin && !m.error;
    const t = (live ? m.vivo?.contexto : 0) || m.uso?.contexto;
    if (!t) return;
    if (relay !== null && last()) fall(relay, { relay: true });
    else if (m.pruned?.tokens && last()) fall(last()!.tokens - m.pruned.tokens, { pruned: m.pruned.tokens });
    relay = null;
    out.push({ tokens: t, pct: pct(t, window), ...(live ? { live: true, ...(calls(m) ? { step: calls(m) } : {}) } : {}) });
  });
  const r = out.slice(-max);
  while (r.length && r[0].drop) r.shift(); // una caída sin su punto de antes no se puede dibujar
  return r;
}

// calls: las llamadas al modelo del turno, si el agente las da (Uso.Llamadas).
const calls = (m: Mensaje) => (m.uso as { llamadas?: number } | undefined)?.llamadas || 0;

const k = (n: number) => (n >= 1000 ? `${Math.round(n / 1000)} k` : String(n));

// pruneLabel: la caída de una limpieza («−18 k: 124 k → 106 k»).
export function pruneLabel(series: ContextPoint[], i: number): string {
  const p = series[i];
  if (!p?.drop || i === 0) return '';
  const antes = series[i - 1].tokens;
  return `${p.relay ? 'relevo' : '−' + k(p.pruned || antes - p.tokens)}: ${k(antes)} → ${k(p.tokens)}`;
}

// sparkTop: el máximo de la escala de la gráfica: el mayor punto o el objetivo (lo que sea mayor), con un poco de
// aire; el umbral de compactación entra si está cerca (si no, la gráfica quedaría aplastada abajo).
export function sparkTop(series: ContextPoint[], target = 0, threshold = 0): number {
  const top = Math.max(...series.map(p => p.tokens), target, 1);
  return threshold > 0 && threshold <= top * 1.5 ? Math.max(top, threshold) : top;
}

// sparkY: la altura de unos tokens en la gráfica (h de alto, escala hasta top).
export function sparkY(tokens: number, top: number, h: number): number {
  return Math.round((h - (tokens / Math.max(top, 1)) * (h - 2) - 1) * 10) / 10;
}

// sparkPoints: x e y de cada punto en un recuadro w×h. Las caídas van en la misma x que el punto anterior (línea
// vertical); la escala va de 0 a top (por defecto, el máximo).
export function sparkPoints(series: ContextPoint[], w: number, h: number, scaleTop?: number): { x: number; y: number }[] {
  if (!series.length) return [];
  const top = scaleTop || Math.max(...series.map(p => p.tokens), 1);
  const cols = Math.max(series.filter(p => !p.drop).length - 1, 1);
  let col = -1;
  return series.map(p => {
    if (!p.drop) col++;
    return { x: Math.round((Math.max(col, 0) / cols) * w * 10) / 10, y: sparkY(p.tokens, top, h) };
  });
}

export function sparkPath(series: ContextPoint[], w: number, h: number, scaleTop?: number): string {
  return sparkPoints(series, w, h, scaleTop).map((p, i) => `${i ? 'L' : 'M'}${p.x},${p.y}`).join(' ');
}

// turnTicks: la x de cada turno (un tic por punto que no es una caída).
export function turnTicks(series: ContextPoint[], w: number, h: number): number[] {
  const pts = sparkPoints(series, w, h);
  return series.map((p, i) => (p.drop ? -1 : pts[i].x)).filter(x => x >= 0);
}

// nowLabel: qué es el número grande: el contexto del turno anterior, o el del paso en curso si el agente trabaja
// (antes saltaba de uno a otro sin decirlo).
export function nowLabel(p?: ContextPoint, working = false): string {
  if (!p) return 'sin datos aún';
  if (p.live) return `contexto ahora (paso ${p.step || 'en curso'})`;
  return working ? 'contexto al acabar el turno anterior' : 'contexto';
}

// ---- «Barrer ahora» (barrido pedido por el usuario) ----

// sweepNote: el barrido pedido a mano entre turnos, como una nota del chat con lo retirado («✂ pgocode ha retirado
// 23k tokens del historial») y su marca en la evolución del contexto.
export function sweepNote(tokens: number, restored = 0, now = Date.now()): Mensaje {
  return { rol: 'assistant', texto: '', nota: true, fin: now, pruned: { tokens, ...(restored ? { repuestos: restored } : {}) } };
}

// cacheWarning: con la caché caliente, lo que cuesta barrer ahora («reenvía ~65 k sin caché una vez (a precio de
// entrada)»); «» si no hay caché que perder. Gemini y Claude Code solo reaprovechan el prefijo hasta el prompt de
// sistema: todo lo de después se vuelve a mandar (Gemini, a precio de entrada; Claude Code, de escritura).
export function cacheWarning(agent: string, r?: PruneReport | null): string {
  if (!r?.cache_caliente || !r.cache_resto) return '';
  return `reenvía ~${k(r.cache_resto)} sin caché una vez (a precio de ${agent === 'claude-code' ? 'escritura' : 'entrada'})`;
}

// inputPrice: dólares por millón de tokens de entrada sin caché (aproximado; 0: sin coste por token, Antigravity).
export function inputPrice(agent: string, model?: string): number {
  const m = (model || '').toLowerCase();
  if (agent === 'antigravity') return 0;
  if (m.includes('haiku')) return 1;
  if (m.includes('opus')) return 5;
  if (m.includes('flash')) return 0.3;
  if (agent === 'claude-code') return 3;
  return 2; // Gemini pro (OpenCode)
}

const usd = (x: number) => (x > 0 && x < 0.001 ? '<0,001' : x.toFixed(x < 0.01 ? 3 : 2).replace('.', ',')) + ' $';

// sweepCost: lo que da y lo que cuesta «Barrer ahora»: «retira 40 k · ahorra ~0,01 $/paso · cuesta ~0,12 $ ahora»
// (cada paso siguiente manda lo retirado de menos a precio de caché, 0,1; ahora, con la caché caliente, se reenvía
// lo de después del sistema: Gemini, a precio de entrada; Claude Code, de escritura a 1 h, 2). «» sin nada que
// retirar.
export function sweepCost(agent: string, model: string | undefined, r?: PruneReport | null): string {
  const n = r?.pendiente || 0;
  if (n <= 0) return '';
  const pr = inputPrice(agent, model);
  let t = `retira ${k(n)}`;
  if (!pr) return t;
  t += ` · ahorra ~${usd((n * pr * 0.1) / 1e6)}/paso`;
  const now = r?.cache_caliente && r.cache_resto ? (r.cache_resto * pr * (agent === 'claude-code' ? 1.9 : 0.9)) / 1e6 : 0;
  return t + (now > 0 ? ` · cuesta ~${usd(now)} ahora` : ' · caché fría: no cuesta caché');
}

// sweepAdvice: si conviene confirmar «Barrer ahora» (con la caché caliente no compensa: «» si compensa).
export function sweepAdvice(r?: PruneReport | null): string {
  return r?.cache_caliente && r.no_compensa ? 'Ahora no compensa: reenviar sin caché cuesta más de lo que ahorra en los próximos pasos. Mejor esperar al próximo turno.' : '';
}

// sweepHint: qué hará «Barrer ahora» en este agente (o por qué espera).
export function sweepHint(agent: string, r?: PruneReport | null, relayForced = false): string {
  if (agent === 'antigravity') return relayForced ? 'relevo pedido: se hará en el próximo envío' : 'no deja tocar su historial: relevará en el próximo envío';
  if (r?.forzado_pendiente) return 'pendiente: se aplica al empezar el próximo turno (la sesión está abierta)';
  if (!(r?.pendiente || 0)) return 'nada fuera de vigencia ahora';
  return '';
}

// pendingNote: lo que espera, para enseñarlo junto al botón (no plegado en Detalles): un barrido de Claude Code con
// la sesión abierta o el relevo pedido en Antigravity.
export function pendingNote(agent: string, r?: PruneReport | null, relayForced = false): string {
  if (agent === 'antigravity' && relayForced) return 'relevo pedido: se hará en el próximo envío';
  if (r?.forzado_pendiente) return 'pendiente: se aplica al empezar el próximo turno';
  return '';
}

// canSweep: si «Barrer ahora» tiene algo que hacer («» si sí; si no, por qué). En Antigravity el relevo solo llega
// si hay algo fuera de vigencia (shouldRelay), así que sin nada que retirar no se ofrece.
export function canSweep(agent: string, r?: PruneReport | null, relayForced = false): string {
  if (agent === 'antigravity' && relayForced) return 'relevo ya pedido';
  if (r?.forzado_pendiente) return 'ya pedido: espera al próximo turno';
  if (!(r?.pendiente || 0) && !(r?.por_reponer || 0)) return 'nada que retirar';
  return '';
}

// relayRemoved: lo que han retirado los relevos de la sesión (el contexto de antes menos la memoria con que siguió).
export function relayRemoved(ms: Mensaje[]): { tokens: number; count: number } {
  let tokens = 0, count = 0;
  for (const m of ms) if (m.rol === 'user' && m.relevo) { count++; tokens += Math.max((m.relevo.de || 0) - (m.relevo.a || 0), 0); }
  return { tokens, count };
}

// SweepShown: el resultado de un barrido pedido y cuándo se enseñó (para quitarlo cuando ya no vale).
export interface SweepShown { text: string; users: number; report: unknown; at: number }

// SWEEP_RESULT_MS: lo que se queda a la vista el resultado de un barrido como mucho.
export const SWEEP_RESULT_MS = 5 * 60_000;

// sweepResultVisible: el resultado sigue a la vista mientras no empiece otro turno, no cambie el informe (otra
// sesión, el marcado del turno) ni pase SWEEP_RESULT_MS; y nunca «nada que retirar» con algo pendiente.
export function sweepResultVisible(s: SweepShown | null, users: number, report: unknown, pending: number, now = Date.now()): boolean {
  if (!s?.text) return false;
  if (users !== s.users || report !== s.report || now - s.at > SWEEP_RESULT_MS) return false;
  return !(s.text === 'nada que retirar' && pending > 0);
}

export const userCount = (ms: Mensaje[]) => ms.reduce((n, m) => n + (m.rol === 'user' ? 1 : 0), 0);

// ---- calibración de la vigencia del proyecto ----

const dec = (n: number, d = 2) => String(Math.round(n * 10 ** d) / 10 ** d).replace('.', ',');

// calibProgress: lo que falta para calibrar con el historial del proyecto («71 de 150 piezas y 12 de 30 retiradas
// para calibrar»); «» si ya está calibrado o no se sabe.
export function calibProgress(c: Calibration): string {
  if (!c.generales || !c.min_piezas) return '';
  const l = [`${c.piezas || 0} de ${c.min_piezas} piezas`];
  if (c.min_retiradas) l.push(`${c.retiradas_generales || c.retiradas || 0} de ${c.min_retiradas} retiradas`);
  return l.join(' y ') + ' para calibrar';
}

// paramsLine: los pesos en una línea («antigüedad 0,12 · pasos 0,8 · tiempo 0,15 · relación 20 (0,5)»).
export function paramsLine(c: Calibration): string {
  const p = c.pesos;
  return `antigüedad ${dec(p.antiguedad)} · pasos ${dec(p.pasos)} · tiempo ${dec(p.tiempo)} · relación ${dec(p.relacion, 1)} (${dec(p.pivote)})`;
}

const pctTxt = (a: number, b: number) => (b > 0 ? dec((100 * a) / b, 1) : '0') + ' %';

// overLine: las relecturas por poda de más (con los pesos del proyecto y con los generales).
export function overLine(c: Calibration): string {
  return `${c.de_mas || 0} de ${c.retiradas || 0} retiradas (${pctTxt(c.de_mas || 0, c.retiradas || 0)}); con los generales, ${c.de_mas_generales || 0} de ${c.retiradas_generales || 0} (${pctTxt(c.de_mas_generales || 0, c.retiradas_generales || 0)})`;
}

// savedLine: los tokens que se habrían ahorrado con el historial del proyecto.
export function savedLine(c: Calibration): string {
  return `${k(c.tokens || 0)} tokens retirados (${k(c.tokens_paso || 0)} tokens·paso); con los generales, ${k(c.tokens_generales || 0)}`;
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

// HYSTERESIS: por qué algo con menos de 3 puede estar fuera.
export const HYSTERESIS = 'Sale con 3 o más y ya no vuelve aunque su vigencia baje: solo si el agente cita algo que solo estaba ahí (y poco cada vez); si lo necesita, lo vuelve a leer.';

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
  compactado?: boolean; // anterior a la última compactación del agente: ya solo tiene su resumen
}

// COMPACTED_MARK: el aviso del chat de que el agente ha compactado su historial (llm.compactionNotice).
export const COMPACTED_MARK = 'ha compactado su historial';

// lastCompaction: el último mensaje del agente con el aviso de su compactación (-1 si no hay).
export function lastCompaction(ms: Mensaje[], from = 0): number {
  for (let i = ms.length - 1; i >= from; i--) {
    const m = ms[i];
    if (m.rol !== 'assistant') continue;
    if ((m.texto || '').includes(COMPACTED_MARK) || (m.partes || []).some(p => p.tipo === 'texto' && p.texto.includes(COMPACTED_MARK))) return i;
  }
  return -1;
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
  const desde = relayStart(ms);
  const comp = lastCompaction(ms, desde);
  for (let i = desde; i < ms.length; i++) {
    const u = ms[i];
    if (u.rol !== 'user' || u.fuera) continue;
    const a = ms[i + 1];
    if (!a || a.rol !== 'assistant' || a.error || a.fuera || !a.uso) continue;
    const hs = (a.partes || []).filter((p): p is Extract<Parte, { tipo: 'herramienta' }> => p.tipo === 'herramienta').map(p => ({
      id: `m${i + 1}:${p.h.id}`, nombre: p.h.nombre, resumen: p.h.resumen, salida: p.h.salida,
      ...(p.h.capturas?.length ? { imagenes: p.h.capturas.length } : {}), ...(p.h.estado === 'error' ? { error: true } : {}),
      ...(p.h.tamano ? { tamano: p.h.tamano } : {}),
    }));
    out.push({ punto: `m${i + 1}`, usuario: u.texto || '', respuesta: lastText(a), ...(a.fin ? { en: a.fin } : {}), ...(hs.length ? { herramientas: hs } : {}),
      ...(i + 1 < comp ? { compactado: true } : {}) });
  }
  return out;
}

function vigOf(x: Vigencia): Vigencia {
  const v: Vigencia = { vigencia: x.vigencia };
  for (const c of ['hace', 'pasos', 'relacion', 'horas', 'superada', 'nota'] as const) if (x[c] !== undefined && x[c] !== '') (v as unknown as Record<string, unknown>)[c] = x[c];
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
