// Actividad en vivo: lo que el chat sabe de su sesión mientras el agente trabaja (estado, acción en curso, tiempos,
// tokens, contexto, cola, tareas…) y la lista compacta que pinta el panel Actividad, una línea por elemento.
// El chat publica una instantánea con cada evento del agente (como mucho 4 por segundo); el panel la recibe por el
// evento «pgo:actividad» y, si se abre a mitad de turno, la lee de aquí (antes salía «Libre» y vacío hasta el
// siguiente evento). Funciones puras: node --test src/liveActivity.test.ts
import type { Elemento, Tarea } from './tareas.ts';
import { tokensOfProgress } from './activityPanel.ts';
import type { SyncView } from './todoSync.ts';

export const ACTIVITY_EVENT = 'pgo:actividad';

export interface LiveSnapshot {
  sesion: string;
  tareas: Tarea[];
  trabajando: boolean;
  accion?: string;        // lo que hace ahora el agente principal (activity.ts: currentAction)
  inicio?: number;        // cuándo empezó el turno (ms)
  ultimoEvento?: number;  // último evento del agente (ms)
  tokensTurno?: number;   // consumidos en el turno hasta ahora
  contexto?: number;      // tokens de contexto de la última llamada
  umbral?: number;        // contexto al que compacta (el % se mide sobre él, como en el indicador del chat)
  enCola?: number;        // mensajes esperando en la cola
  colaParada?: boolean;   // la cola se detuvo (un turno falló)
  apartes?: { id: string; pregunta: string; inicio?: number }[]; // preguntas aparte en curso
  todo?: SyncView;        // la lista de tareas del agente sincronizada con lo que hace (todoSync.ts: sessionTodo)
}

// ---- Almacén (uno por pestaña): la última instantánea de cada sesión y el envío con límite de frecuencia ----

const latest = new Map<string, LiveSnapshot>();
const pending = new Map<string, ReturnType<typeof setTimeout>>();
const sentAt = new Map<string, number>();
export const MIN_INTERVAL_MS = 250;

// lastSnapshot: la última instantánea publicada de una sesión (para el panel que se abre a mitad de turno).
export const lastSnapshot = (sesion: string): LiveSnapshot | undefined => latest.get(sesion);

// publishSnapshot: guarda la instantánea y avisa al panel; si el último aviso fue hace menos de 250 ms, lo aplaza
// (sale siempre el último estado). Al cambiar de trabajando a libre, en el acto.
export function publishSnapshot(s: LiveSnapshot, now = Date.now()): void {
  const prev = latest.get(s.sesion);
  latest.set(s.sesion, s);
  const send = () => {
    pending.delete(s.sesion);
    sentAt.set(s.sesion, Date.now());
    const x = latest.get(s.sesion);
    if (x && typeof window !== 'undefined') window.dispatchEvent(new CustomEvent(ACTIVITY_EVENT, { detail: x }));
  };
  if (pending.has(s.sesion)) return;
  const wait = MIN_INTERVAL_MS - (now - (sentAt.get(s.sesion) || 0));
  if (wait <= 0 || prev?.trabajando !== s.trabajando) send();
  else pending.set(s.sesion, setTimeout(send, wait));
}

// ---- Textos ----

const cut = (t: string, n: number) => { const x = t.replace(/\s+/g, ' ').trim(); return x.length > n ? x.slice(0, n - 1).trimEnd() + '…' : x; };

// elapsed: «8 s», «2:05», «1 h 03 min» (como un monitor de procesos).
export function elapsed(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  if (s < 60) return s + ' s';
  if (s < 3600) return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0');
  return Math.floor(s / 3600) + ' h ' + String(Math.floor((s % 3600) / 60)).padStart(2, '0') + ' min';
}

// tokensShort: «850», «12 k», «1,2 M».
export const tokensShort = (n: number) => n >= 1_000_000 ? (n / 1_000_000).toFixed(1).replace('.', ',') + ' M' : n >= 1000 ? Math.round(n / 1000) + ' k' : String(n);

// ---- Lista de tareas del agente (TodoWrite / TaskCreate / todowrite) ----

export interface TodoView { done: number; total: number; current?: Elemento; next?: Elemento; items: Elemento[]; finished: boolean }

// todoView: la última lista de tareas del agente, con la que está en curso (o la siguiente pendiente) y el progreso.
export function todoView(tareas: Tarea[]): TodoView | undefined {
  const l = [...tareas].reverse().find(t => t.tipo === 'lista' && !t.padre);
  const items = l?.elementos || [];
  if (!items.length) return undefined;
  const done = items.filter(e => e.estado === 'hecho').length;
  return { done, total: items.length, current: items.find(e => e.estado === 'curso'), next: items.find(e => e.estado === 'pendiente'), items, finished: done === items.length };
}

// ---- Filas de la lista en vivo ----

export type Dot = 'curso' | 'ok' | 'error' | 'parada' | 'libre' | 'cola';
export interface Row { id: string; dot: Dot; kind: string; name: string; detail: string; time: string; tokens: string; title: string }

const DOT_TASK: Record<string, Dot> = { curso: 'curso', ok: 'ok', error: 'error', parada: 'parada' };
const STATE_NAME: Record<Dot, string> = { curso: 'en marcha', ok: 'terminado', error: 'con error', parada: 'parado', libre: 'libre', cola: 'en cola' };

// agentRow: la línea del agente: estado, qué hace ahora, desde cuándo, tokens del turno y % de contexto.
export function agentRow(s: LiveSnapshot | undefined, now: number): Row {
  if (!s) return { id: 'agente', dot: 'libre', kind: 'Agente', name: 'libre', detail: '', time: '', tokens: '', title: 'El agente de la sesión no está trabajando' };
  const cola = s.enCola ? ` · ${s.enCola} en cola${s.colaParada ? ' (parada)' : ''}` : '';
  const ctx = s.contexto && s.umbral ? Math.round((s.contexto / s.umbral) * 100) : 0;
  const tokens = [s.trabajando && s.tokensTurno ? tokensShort(s.tokensTurno) : '', ctx ? ctx + ' %' : ''].filter(Boolean).join(' · ');
  const title = [
    s.trabajando && s.inicio ? `Turno en marcha desde hace ${elapsed(now - s.inicio)}` : '',
    s.trabajando && s.tokensTurno ? `${s.tokensTurno} tokens en este turno` : '',
    s.contexto ? `Contexto: ${tokensShort(s.contexto)} tokens${s.umbral ? ` (${ctx} % de ${tokensShort(s.umbral)}, donde compacta)` : ''}` : '',
    s.enCola ? `${s.enCola} mensajes esperan en la cola` : '',
  ].filter(Boolean).join('\n');
  if (!s.trabajando) return { id: 'agente', dot: s.enCola ? 'cola' : 'libre', kind: 'Agente', name: 'libre' + cola, detail: '', time: '', tokens, title };
  return { id: 'agente', dot: 'curso', kind: 'Agente', name: 'trabajando' + cola, detail: s.accion || '', time: s.inicio ? elapsed(now - s.inicio) : '', tokens, title };
}

// taskRow: un subagente o una orden en segundo plano: qué es, estado, su último paso o lo que devolvió, tiempo y tokens.
export function taskRow(t: Tarea, now: number): Row {
  const dot = DOT_TASK[t.estado || ''] || 'curso';
  const sub = t.tipo === 'subagente';
  const n = sub ? tokensOfProgress(t.progreso) : undefined;
  // El progreso de Claude: «Reading a.txt · Read · 1 herramientas · 27121 tokens»: sin los contadores.
  const paso = (t.progreso || '').split(' · ').filter(x => !/^\d+ (herramientas|tokens)$/.test(x))[0] || '';
  const detail = dot === 'curso' ? paso : cut(t.salida || '', 120) || STATE_NAME[dot];
  const time = t.inicio ? elapsed((dot === 'curso' ? now : t.fin || now) - t.inicio) : '';
  const name = t.titulo || t.subtipo || t.id;
  const title = [`${sub ? 'Subagente' : 'En segundo plano'}${t.subtipo ? ' · ' + t.subtipo : ''}: ${name}`, STATE_NAME[dot],
    t.progreso || '', t.salida ? cut(t.salida, 600) : '', t.detalle ? 'Salida completa: ' + t.detalle : ''].filter(Boolean).join('\n');
  return { id: t.id, dot, kind: sub ? 'subagente' : 'segundo plano', name, detail, time, tokens: n ? tokensShort(n) : '', title };
}

export interface LiveLists { subagents: Row[]; subagentsDone: Row[]; background: Row[]; backgroundDone: Row[]; asides: Row[] }

// liveLists: los subagentes y órdenes en segundo plano (en marcha primero; los terminados, aparte y los últimos
// primero) y las preguntas aparte en curso.
export function liveLists(s: LiveSnapshot | undefined, now: number, maxDone = 15): LiveLists {
  const ts = (s?.tareas || []).filter(t => t.tipo !== 'lista');
  const of = (sub: boolean) => ts.filter(t => (t.tipo === 'subagente') === sub);
  const running = (l: Tarea[]) => l.filter(t => t.estado === 'curso').map(t => taskRow(t, now));
  const done = (l: Tarea[]) => l.filter(t => t.estado !== 'curso').reverse().slice(0, maxDone).map(t => taskRow(t, now));
  const asides = (s?.apartes || []).map(a => ({ id: a.id, dot: 'curso' as Dot, kind: 'aparte', name: cut(a.pregunta, 80), detail: '',
    time: a.inicio ? elapsed(now - a.inicio) : '', tokens: '', title: 'Pregunta aparte (no entra en la conversación): ' + a.pregunta }));
  return { subagents: running(of(true)), subagentsDone: done(of(true)), background: running(of(false)), backgroundDone: done(of(false)), asides };
}

// needsPulse: si hay algo con el reloj en marcha (el panel se repinta cada segundo).
export const needsPulse = (s: LiveSnapshot | undefined) => Boolean(s && (s.trabajando || s.apartes?.length || s.tareas.some(t => t.tipo !== 'lista' && t.estado === 'curso')));
