// ENRUTADOR DE INTENCIONES de pgocode: quién hace qué con cada mensaje del usuario. Es la referencia del código;
// la explicación completa (tabla, diagrama y ejemplos) está en docs/enrutador.md.
//
// Una sola decisión por mensaje, en un orden fijo: la primera etapa que encaja con seguridad gana.
//
//   DESTINOS (quién hace el trabajo)                                 coste
//   0. command  Orden de pgocode: /help, «descarta/reactiva/borra el     0 tokens (/plan, «plan N: …» y «ejecuta el
//               plan N» (las hace pgocode); /plan, «plan N: …» y          plan N» los redacta o ejecuta el agente)
//               «ejecuta el plan N» (pgocode prepara, el agente hace).
//   1. direct   Función directa: compila, tests, tipos, lint, commit y     0 tokens (el mensaje del commit, IA local)
//               preguntas de estado (direct.ts, servidor/directo.go).
//   2. aside    Pregunta aparte (no entra en la conversación): estado      estado 0 · suelta 0 con IA local (si no,
//               (pgocode), con contexto (copia de la sesión del agente)    modelo barato) · contexto: lo paga el agente
//               o suelta (IA local; respaldo: modelo barato de pago).
//   3. split    Reparto en subtareas: sesiones paralelas de pgocode,       lo de cada hija (sin repetir el contexto)
//               cada una en su worktree y rama pgo/…, unidas al acabar.
//   4. (IA local) No es un destino del mensaje principal: responde las preguntas aparte sueltas (ver aside) y
//               decide/redacta (clasificación, mensajes de commit). Ver «IA local como destino» en docs/enrutador.md.
//   5. agent    El agente de la sesión (Claude Code, OpenCode, Antigravity): ahora, a la cola o al turno en curso.
//
//   MOTORES DE DECISIÓN (quién decide), por orden de preferencia en cada etapa:
//     user (prefijo «>», «?», «//», la etiqueta o Alt+Intro) > rules (expresiones regulares, sin IA) > learned
//     (correcciones anteriores del usuario) > embeddings (Ollama embeddinggemma: partes independientes, skills,
//     subagentes) > local-ai (gemma3:4b: clasificación) > cheap-model (modelo barato de pago, solo de respaldo).
//     Si un motor no está o no responde a tiempo, la etapa cae a la siguiente (al final, el agente: en la duda,
//     a la conversación; que el agente no pierda una orden importa más que algo de ruido en su contexto).
//
//   MODIFICADORES (cambian cómo llega al agente, no a quién): queue (cola), turn (al turno en curso), plan (solo
//     lectura), skill-user («/skill …»), attachments, cut («Continuar desde aquí»); y al enviar: skill (elegida
//     por embeddings), subagents (sugerencia de subagentes, servidor), relay (relevo con memoria de trabajo);
//     la poda, la compactación, la continuación de OpenCode y el idioma los aplica siempre el servidor.
//
//   HERRAMIENTAS (no son destinos): Google Drive/Docs/Sheets y Gmail en solo lectura los usa el agente con
//     «pgo drive …» y «pgo gmail …»; un mensaje sobre Drive o el correo va al agente (sin función directa: ver
//     docs/enrutador.md).
//
// route() es síncrona y pura (para la etiqueta mientras se escribe y para el envío). Lo que necesita al servidor
// queda en needsServer y se resuelve al enviar con resolveClassify/resolveSplit (con tiempo máximo: withTimeout).
// Chat.tsx despacha según la decisión y la registra (logPayload → POST /api/ruta → registro de tiempos «ruta»).
// Pruebas: node --test src/router.test.ts (más de 60 mensajes con su ruta esperada).
import * as AP from './aparte.ts';
import { esAyuda } from './ayuda.ts';
import * as DR from './direct.ts';
import * as PL from './plan.ts';
import * as RP from './reparto.ts';
import type { QueueOrigin } from './cola.ts';

export type Route = 'command' | 'direct' | 'aside' | 'split' | 'agent';
export type Executor = 'pgocode' | 'agent' | 'agent-copy' | 'local-ai' | 'cheap-model' | 'subsessions';
export type Engine = 'user' | 'rules' | 'learned' | 'embeddings' | 'local-ai' | 'cheap-model';
export type Modifier = 'queue' | 'turn' | 'now' | 'plan' | 'skill-user' | 'attachments' | 'cut' | 'skill' | 'subagents' | 'relay';
export type AsideKind = Exclude<AP.Destino, 'conversacion' | 'turno'>;

export interface RouteDecision {
  route: Route;
  executor: Executor;
  engine: Engine;
  reason: string;
  modifiers: Modifier[];
  text: string;                 // lo que se envía (sin el prefijo)
  detail?: string;              // la orden directa, el tipo de pregunta aparte, la orden de planes, «help»
  forced?: boolean;             // lo ha pedido el usuario (prefijo, etiqueta o Alt+Intro)
  needsServer?: 'classify' | 'split';
  direct?: DR.DirectKind;
  aside?: AsideKind;
  plan?: PL.OrdenPlan;
}

export interface RouteInput {
  text: string;
  isAgent: boolean;             // el proveedor es un agente (no un modelo por API)
  busy: boolean;                // el agente de la sesión está trabajando
  canInject?: boolean;          // el agente admite mensajes a mitad de turno (Claude Code)
  alwaysTurn?: boolean;         // opción «Siempre al turno»
  attachments?: number;
  hasConversation?: boolean;
  lastAgentText?: string;       // lo último que dijo el agente (si preguntó algo, se le está respondiendo)
  learned?: AP.Aprendidas;
  autoAside?: boolean;          // ajuste de la sesión «decidir aparte» (por defecto, sí)
  directOn?: boolean;           // ajuste de la sesión «órdenes directas» (por defecto, sí)
  permissions?: string;         // lectura · editar · auto
  child?: boolean;              // sesión hija de un reparto (no se vuelve a repartir)
  cutPending?: boolean;         // «Continuar desde aquí» pendiente
  canFork?: boolean;            // la sesión del agente se puede copiar para una pregunta aparte con contexto
  manual?: AP.Destino;          // elegido en la etiqueta o con Alt+Intro
  manualBy?: 'tag' | 'alt';
  // Envíos internos (no los escribe el usuario):
  raw?: boolean;                // el texto va tal cual (ejecución de un plan)
  noDirect?: boolean;           // al agente aunque parezca una orden directa (p. ej. pgocode no sabía hacerla)
  noSplit?: boolean;            // sin reparto (hijas, «Hacerlo en esta sesión»)
  noAside?: boolean;            // ya va a la conversación (al enviar o al salir de la cola): sin preguntas aparte
}

const ASIDE: AsideKind[] = ['estado', 'suelta', 'contexto'];
const isAside = (d: AP.Destino | undefined): d is AsideKind => ASIDE.includes(d as AsideKind);
const LOCAL_PLAN = new Set(['descartar', 'reactivar', 'borrar']);

// Quién responde cada tipo de pregunta aparte (la suelta, la IA local con el modelo barato de respaldo; el
// servidor dice al responder quién lo hizo de verdad).
export const asideExecutor = (k: AsideKind, canFork = true): Executor =>
  k === 'estado' ? 'pgocode' : k === 'suelta' ? 'local-ai' : canFork ? 'agent-copy' : 'cheap-model';

function aside(k: AsideKind, text: string, engine: Engine, reason: string, inp: RouteInput, forced = false): RouteDecision {
  return { route: 'aside', aside: k, detail: k, executor: asideExecutor(k, inp.canFork !== false), engine, reason, modifiers: [], text, forced };
}

// route: la decisión para un mensaje. Orden fijo: ayuda → lo que pide el usuario (prefijo o etiqueta) → órdenes de
// planes → función directa → pregunta aparte (reglas, aprendidas o clasificador) → reparto → agente.
export function route(inp: RouteInput): RouteDecision {
  const t = inp.text.trim();
  const att = inp.attachments || 0;
  const base: Modifier[] = [];
  if (att) base.push('attachments');
  if (inp.cutPending && inp.isAgent) base.push('cut');
  // 0. Ayuda: en la interfaz, sin agente.
  if (!inp.raw && esAyuda(t)) return { route: 'command', detail: 'help', executor: 'pgocode', engine: 'rules', reason: 'orden «/help»', modifiers: [], text: t };
  if (inp.raw) return agent(t, base, 'rules', 'envío interno de pgocode', true);

  // Lo que dicen las reglas de aparte.ts (prefijos «>», «?», «//», aprendidas, reglas con el agente ocupado).
  const ap: AP.Decision = inp.noAside ? { destino: 'conversacion', texto: t, motivo: inp.busy ? 'a la conversación' : 'el agente está libre' } : AP.decide(t, {
    ocupado: inp.busy, ultimo: inp.lastAgentText, hayConversacion: inp.hasConversation, adjuntos: att, aprendidas: inp.learned,
    activo: inp.autoAside !== false, ordenPlan: PL.interpreta(t) !== null, alTurno: Boolean(inp.alwaysTurn && inp.canInject), canInject: inp.canInject,
  });
  const manual = inp.manual;
  const userDest: AP.Destino | undefined = manual || (ap.forzado ? ap.destino : undefined);
  const text = manual ? (ap.forzado ? ap.texto : t) : ap.texto;
  const userReason = manual ? (inp.manualBy === 'alt' ? 'Alt+Intro' : 'elegido por ti en la etiqueta') : ap.motivo;
  // Aparte por decisión del usuario.
  if (isAside(userDest)) return aside(userDest, text, 'user', userReason, inp, true);
  const toConversation = userDest === 'conversacion' || userDest === 'turno';
  // Al turno en curso, pedido por el usuario (Alt+Intro, la etiqueta o «>» con el agente trabajando).
  if (userDest === 'turno') return agent(text, [...base, ...turnOrQueue(inp)], 'user', userReason, true);

  // 0. Órdenes de planes.
  const plan = PL.interpreta(text);
  if (plan) {
    const local = LOCAL_PLAN.has(plan.tipo);
    const mods: Modifier[] = local ? [] : ['plan'];
    if (inp.busy) mods.push('queue');
    return { route: 'command', detail: 'plan:' + plan.tipo, plan, executor: local ? 'pgocode' : 'agent', engine: toConversation ? 'user' : 'rules',
      reason: local ? 'orden de planes que hace pgocode' : plan.tipo === 'ejecutar' ? 'ejecutar un plan: lo hace el agente con permiso para editar' : 'el agente redacta el plan en solo lectura',
      modifiers: mods, text, forced: toConversation };
  }

  // 1. Función directa (solo si el mensaje entero es la orden; nunca con adjuntos ni si el usuario lo manda al agente).
  if (!toConversation && !inp.noDirect && inp.directOn !== false && inp.isAgent && !att) {
    const k = DR.detect(t);
    if (k) {
      const mods: Modifier[] = inp.busy ? [DR.isMechanical(k) ? 'queue' : 'now'] : [];
      return { route: 'direct', direct: k, detail: k, executor: 'pgocode', engine: 'rules', modifiers: mods, text: t,
        reason: inp.busy ? (DR.isMechanical(k) ? 'orden mecánica: la hace pgocode al acabar el turno' : 'pregunta de estado: la responde pgocode ya') : 'el mensaje entero es una orden que hace pgocode' };
    }
  }

  // 2. Pregunta aparte, decidida por pgocode (solo con el agente trabajando).
  if (!toConversation) {
    const engine: Engine = ap.motivo === 'como lo corregiste antes' ? 'learned' : 'rules';
    if (isAside(ap.destino)) return aside(ap.destino, text, engine, ap.motivo, inp);
    if (ap.duda) return { ...agent(text, [...base, ...turnOrQueue(inp, true)], 'rules', ap.motivo), needsServer: 'classify' };
    if (ap.destino === 'turno') return agent(text, [...base, 'turn'], engine, ap.motivo);
  }

  // 3. Reparto en subtareas: lo confirma el servidor (reglas y embeddings). Con el agente trabajando se decide al
  // salir de la cola.
  if (!toConversation && !inp.busy && !inp.noSplit && inp.isAgent && !att && !inp.child && !inp.cutPending && RP.candidata(text, inp.permissions)) {
    return { ...agent(text, base, 'rules', 'parece una lista de partes independientes: lo confirma el servidor'), needsServer: 'split' };
  }

  // 5. El agente.
  const mods: Modifier[] = [...base, ...(inp.busy ? ['queue' as const] : [])];
  return agent(text, mods, toConversation ? 'user' : ap.motivo === 'como lo corregiste antes' ? 'learned' : 'rules',
    toConversation ? userReason : ap.motivo, toConversation);
}

function turnOrQueue(inp: RouteInput, auto = false): Modifier[] {
  if (!inp.busy) return [];
  // Al turno solo si el agente lo admite y sin adjuntos (con adjuntos, a la cola). En automático, con «Siempre al turno».
  return inp.canInject && !inp.attachments && (!auto || inp.alwaysTurn) ? ['turn'] : ['queue'];
}

function agent(text: string, mods: Modifier[], engine: Engine, reason: string, forced = false): RouteDecision {
  const m = [...mods];
  if (text.startsWith('/') && !m.includes('skill-user')) m.push('skill-user');
  return { route: 'agent', executor: 'agent', engine, reason, modifiers: m, text, forced: forced || undefined };
}

// ——— Pasos con el servidor ———

// withTimeout: la promesa o null si tarda más de ms (o falla): la etapa cae a la siguiente.
export function withTimeout<T>(p: Promise<T>, ms: number): Promise<T | null> {
  return Promise.race([p.catch(() => null), new Promise<null>(res => setTimeout(() => res(null), ms))]);
}
export const CLASSIFY_MS = 5000;  // el servidor espera a la IA local hasta 6 s la primera vez (carga) y luego Gemini
export const SPLIT_MS = 3000;

// resolveClassify: lo que dijo el clasificador del servidor (IA local o, de respaldo, el modelo barato). Sin
// respuesta, a la conversación.
export function resolveClassify(d: RouteDecision, r: { tipo: AP.Destino; modelo?: string } | null, inp: RouteInput): RouteDecision {
  const { needsServer: _, ...rest } = d;
  if (!r) return { ...rest, reason: 'el clasificador no ha respondido: a la conversación' };
  const engine: Engine = r.modelo?.startsWith('local/') ? 'local-ai' : 'cheap-model';
  const who = engine === 'local-ai' ? `la IA local (${r.modelo!.slice(6)})` : 'el modelo barato';
  if (isAside(r.tipo) && r.tipo !== 'estado') return aside(r.tipo, d.text, engine, `lo ha decidido ${who}`, inp);
  return { ...rest, engine, reason: `lo ha decidido ${who}: es para el agente` };
}

// resolveSplit: la propuesta del servidor; con tres partes o más, reparto (con cuenta atrás); si no, al agente.
export function resolveSplit(d: RouteDecision, p: RP.Propuesta | null): RouteDecision {
  const { needsServer: _, ...rest } = d;
  if (p?.partes && p.partes.length >= 3) {
    return { ...rest, route: 'split', executor: 'subsessions', engine: 'embeddings', detail: String(p.partes.length),
      reason: `${p.partes.length} partes independientes (reglas y embeddings): una sesión paralela por parte` };
  }
  return { ...rest, reason: p ? `no se reparte: ${p.motivo || 'no son partes independientes'}` : 'el servidor no ha respondido a tiempo: al agente' };
}

// ——— La cola ———

// standalone: lo que sale solo de la cola (cambia lo que hace pgocode con el mensaje): «/…», órdenes de planes y
// funciones directas.
// Lo encolado con «>» solo sale solo si es una orden de planes (al agente va igual, pero sin juntarse con otros).
export function standalone(t: string, directOn = true): boolean {
  const x = t.replace(/^>\s*/, '');
  return x.startsWith('/') || PL.interpreta(x) !== null || (directOn && DR.detect(t) !== null);
}

// fromQueue: lo que sale de la cola. Lo encolado con «>» (el usuario lo mandó al agente a propósito) va al agente
// aunque parezca una orden directa. origin: por qué se encoló (quién lo decidió y su motivo), para que el registro no
// lo pierda: lo forzado por el usuario manda; si no, el primero que lo sepa. generated: todo lo ha escrito pgocode.
export function fromQueue(texts: string[], origins: (QueueOrigin | undefined)[] = []): { text: string; forced: boolean; origin?: QueueOrigin; generated?: boolean } {
  const forced = texts.length > 0 && texts.every(x => x.startsWith('>'));
  const text = texts.map(x => (x.startsWith('>') ? x.slice(1).trim() : x)).filter(Boolean).join('\n\n');
  let origin = origins.find(o => o?.forced) || origins.find(Boolean);
  if (forced && !origin?.forced) origin = { engine: 'user', reason: 'lo mandaste a la conversación con «>»', forced: true };
  const generated = origins.length > 0 && origins.length === texts.length && origins.every(o => o?.generated);
  return { text, forced, ...(origin ? { origin } : {}), ...(generated ? { generated } : {}) };
}

// queueOrigin: lo que se guarda en la cola de una decisión (para registrarla igual al salir).
export const queueOrigin = (d: RouteDecision, generated = false): QueueOrigin =>
  ({ engine: d.engine, reason: d.reason, ...(d.forced ? { forced: true } : {}), ...(generated ? { generated } : {}) });

// fromOrigin: la decisión al salir de la cola, con quién decidió y por qué al encolarlo (no «el agente está libre»).
export function fromOrigin(d: RouteDecision, o: QueueOrigin | undefined): RouteDecision {
  if (!o) return d;
  return { ...d, engine: (ENGINE_IDS.includes(o.engine as Engine) ? o.engine : d.engine) as Engine, reason: o.reason, ...(o.forced ? { forced: true } : {}) };
}
const ENGINE_IDS: Engine[] = ['user', 'rules', 'learned', 'embeddings', 'local-ai', 'cheap-model'];

// manualStillApplies: el destino elegido a mano en la etiqueta sigue valiendo mientras se edita el mismo texto; si se
// vacía el cuadro o se cambia todo (nada en común al principio ni al final), se olvida.
export function manualStillApplies(base: string, now: string): boolean {
  const a = base.trim(), b = now.trim();
  if (!b) return false;
  if (!a) return true;
  return a[0] === b[0] || a[a.length - 1] === b[b.length - 1];
}

// ——— Lo que se ve ———

const PLAN_VERB: Record<string, string> = { descartar: 'descartar', reactivar: 'reactivar', borrar: 'borrar', ejecutar: 'ejecutar', revisar: 'revisar', nuevo: 'crear' };
const EXECUTOR: Record<Executor, string> = {
  pgocode: 'pgocode (sin el agente)', agent: 'el agente', 'agent-copy': 'una copia de la sesión del agente', 'local-ai': 'la IA local del equipo',
  'cheap-model': 'el modelo barato de pago', subsessions: 'sesiones paralelas de pgocode',
};
export const ENGINE: Record<Engine, string> = {
  user: 'tú', rules: 'reglas (sin IA)', learned: 'tus correcciones anteriores', embeddings: 'embeddings locales',
  'local-ai': 'la IA local', 'cheap-model': 'el modelo barato de pago',
};
const ASIDE_WHO: Record<Executor, string> = { pgocode: 'estado', 'agent-copy': 'copia de la sesión', 'local-ai': 'IA local', 'cheap-model': 'modelo barato', agent: '', subsessions: '' };

// tagText: la etiqueta encima del cuadro de texto antes de enviar.
export function tagText(d: RouteDecision, busy: boolean): string {
  if (d.needsServer === 'classify') return '→ Conversación o aparte: lo decide pgocode al enviar';
  if (d.needsServer === 'split') return '→ Conversación o reparto en subtareas: lo decide pgocode al enviar';
  switch (d.route) {
    case 'command': {
      if (d.detail === 'help') return '→ Ayuda de pgocode (0 tokens)';
      const p = d.plan!;
      const n = 'numero' in p ? ` ${p.numero}` : '';
      if (d.executor === 'pgocode') return `→ pgocode: ${PLAN_VERB[p.tipo]} el plan${n} (0 tokens)${busy ? ', al acabar el turno' : ''}`;
      return `→ Agente: ${PLAN_VERB[p.tipo]} ${p.tipo === 'nuevo' ? 'un plan (solo lectura)' : 'el plan' + n}${busy ? ' (a la cola)' : ''}`;
    }
    case 'direct': return DR.tagText(d.direct!, busy);
    case 'aside': return d.aside === 'estado' ? '→ Estado (sin tokens)' : `→ Aparte · ${AP.NOMBRES[d.aside!].toLowerCase()} (${ASIDE_WHO[d.executor]})`;
    case 'split': return `→ Reparto en ${d.detail} subtareas`;
    default:
      if (d.modifiers.includes('turn')) return '→ Al turno en curso (el agente lo lee ya)';
      return d.modifiers.includes('queue') ? '→ Conversación (a la cola)' : '→ Conversación';
  }
}

// tagTitle: al pasar el ratón por la etiqueta: quién lo hace, quién lo ha decidido y por qué.
export function tagTitle(d: RouteDecision): string {
  const who = d.needsServer ? 'lo decide pgocode al enviar' : `lo hace ${EXECUTOR[d.executor]}`;
  return `${who} · decide: ${ENGINE[d.engine]} · ${d.reason}`;
}

// ——— Lo que queda en cada mensaje ———

export interface RouteStamp { route: Route; executor: Executor; engine: Engine; reason: string; modifiers: Modifier[]; detail?: string }
export const stamp = (d: RouteDecision): RouteStamp => ({ route: d.route, executor: d.executor, engine: d.engine, reason: d.reason, modifiers: d.modifiers, ...(d.detail ? { detail: d.detail } : {}) });

// addModifier: un modificador que se sabe al enviar (skill elegida, relevo, subagentes sugeridos).
export const addModifier = (s: RouteStamp | undefined, m: Modifier): RouteStamp | undefined =>
  !s || s.modifiers.includes(m) ? s : { ...s, modifiers: [...s.modifiers, m] };

const MOD_BADGE: Partial<Record<Modifier, string>> = { now: 'aparte', queue: 'cola', turn: 'al turno', plan: 'plan', 'skill-user': 'skill', skill: 'skill elegida', subagents: 'subagentes sugeridos', relay: 'relevo', cut: 'desde aquí' };

// badge: el distintivo de un mensaje: la ruta y, detrás, sus modificadores («agente · cola · skill elegida»).
export function badge(s: RouteStamp): string {
  const main = s.route === 'agent' ? 'agente'
    : s.route === 'direct' ? 'pgocode'
      : s.route === 'command' ? (s.executor === 'pgocode' ? 'pgocode' : 'agente · orden de plan')
        : s.route === 'split' ? `reparto${s.detail ? ' en ' + s.detail : ''}`
          : 'aparte · ' + ({ pgocode: 'estado', 'agent-copy': 'copia', 'local-ai': 'IA local', 'cheap-model': 'modelo barato' } as Record<string, string>)[s.executor];
  const mods = s.modifiers.map(m => MOD_BADGE[m]).filter((x): x is string => Boolean(x) && !(s.route === 'command' && x === 'plan'));
  return [main, ...mods].join(' · ');
}
export const badgeTitle = (s: RouteStamp) => `Lo hace ${EXECUTOR[s.executor]} · decide: ${ENGINE[s.engine]} · ${s.reason}`;

// ——— Registro y resumen ———

// logPayload: lo que se manda a POST /api/ruta (queda en el registro de tiempos como suceso «ruta»).
export const logPayload = (sesion: string, s: RouteStamp, ms: number) => ({
  sesion, destino: s.route, ejecuta: s.executor, motor: s.engine, motivo: s.reason, modificadores: s.modifiers, detalle: s.detail || '', ms: Math.max(0, Math.round(ms)),
});

export type RouteTally = Record<string, number>;
// tally: cuenta una decisión (por destino y, las preguntas aparte, también por quién las respondió).
export function tally(t: RouteTally, s: RouteStamp): RouteTally {
  const n = { ...t, [s.route]: (t[s.route] || 0) + 1 };
  if (s.route === 'aside') n['aside:' + s.executor] = (t['aside:' + s.executor] || 0) + 1;
  if (s.route === 'command' && s.executor === 'pgocode') n['command:pgocode'] = (t['command:pgocode'] || 0) + 1;
  return n;
}

// summary: «Esta sesión: 12 al agente · 3 directas · 2 aparte (1 IA local) · 1 reparto · 2 órdenes»; y cuántos no han
// gastado tokens de pago.
export function summary(t: RouteTally): string {
  const total = (['command', 'direct', 'aside', 'split', 'agent'] as Route[]).reduce((s, r) => s + (t[r] || 0), 0);
  if (!total) return 'Esta sesión: aún ningún mensaje.';
  const parts: string[] = [];
  if (t.agent) parts.push(`${t.agent} al agente`);
  if (t.direct) parts.push(`${t.direct} ${t.direct === 1 ? 'directa' : 'directas'}`);
  if (t.aside) {
    const sub = (['pgocode', 'local-ai', 'agent-copy', 'cheap-model'] as Executor[]).filter(e => t['aside:' + e]).map(e => `${t['aside:' + e]} ${ASIDE_WHO[e]}`);
    parts.push(`${t.aside} aparte${sub.length ? ` (${sub.join(', ')})` : ''}`);
  }
  if (t.split) parts.push(`${t.split} reparto${t.split === 1 ? '' : 's'}`);
  if (t.command) parts.push(`${t.command} ${t.command === 1 ? 'orden' : 'órdenes'} de pgocode`);
  const free = (t.direct || 0) + (t['command:pgocode'] || 0) + (t['aside:pgocode'] || 0) + (t['aside:local-ai'] || 0);
  return `Esta sesión: ${parts.join(' · ')}. ${free} de ${total} sin tokens de pago.`;
}

// Persistencia del recuento por sesión (solo en este navegador: el registro de verdad es el del servidor).
const KEY = (sesion: string) => 'pgo:rutas:' + sesion;
export function readTally(sesion: string): RouteTally {
  try { const x = JSON.parse(localStorage.getItem(KEY(sesion)) || '{}'); return x && typeof x === 'object' ? x as RouteTally : {}; } catch { return {}; }
}
export function saveTally(sesion: string, t: RouteTally) {
  try { localStorage.setItem(KEY(sesion), JSON.stringify(t)); } catch { /* sin almacenamiento */ }
}
