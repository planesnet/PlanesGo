// Memoria de trabajo: lo esencial de una conversación con el agente, sacado de lo que pgocode ya sabe (mensajes,
// herramientas y tareas), sin gastar tokens. Sirve para el relevo automático: cuando el contexto del agente crece
// demasiado (o la caché ya ha caducado), el siguiente mensaje va a una sesión nueva del agente con esta memoria en
// vez de reenviar todo el historial. Funciones puras para poder probarlas: node --test src/workingMemory.test.ts
import type { Herramienta, Mensaje } from './api';
import type { RelayMode } from './contextPolicy';

export const MEMORY_MAX = 12_000; // caracteres (~3k tokens)
// Lo que dura la caché implícita de los modelos (CACHE_MIN de costes.ts; aquí sin importarlo para las pruebas con node).
const CACHE_MINUTES = 5;

// Mismos criterios que aparte.ts (EDITA, ORDENES) para reconocer ediciones y órdenes de todos los agentes.
const EDITS = /^(write|edit|multiedit|notebookedit|patch|apply_patch|create_file|replace|write_file|edit_file|str_replace)/i;
const COMMANDS = /^(bash|shell|run|exec|run_command|terminal)/i;
const READS = /^(read|view|view_file|read_file|open_file|cat)$/i;
// Órdenes que importan para retomar el trabajo: pruebas, compilaciones, comprobaciones de tipos, git que cambia algo…
const RELEVANT = /\b(test|tests|vet|build|compile|make|tsc|pytest|jest|vitest|mocha|cargo|mvn|gradle|lint|eslint|ruff|mypy|go\s+run|npm\s+(?:run|ci|install)|pnpm|yarn|pip\s+install|docker|commit|merge|rebase|migrate|deploy)\b/i;
// Frases con decisiones o avisos (del usuario o del agente), sin modelo: reglas, restricciones, «importante»…
const DECISION = /^(?:no\s|nunca\b|siempre\b|importante|ojo\b|cuidado|recuerda|ten\s+en\s+cuenta|mejor\b|en\s+vez\s+de|en\s+lugar\s+de|hemos\s+decidido|he\s+decidido|decid|decisi[oó]n|nota:|aviso:|atenci[oó]n|don'?t\b|never\b|always\b|important|note:|warning:|decided)/i;

const squash = (t: string) => t.replace(/\s+/g, ' ').trim();
const cut = (t: string, n: number) => { const x = squash(t); return x.length > n ? x.slice(0, n - 1) + '…' : x; };
const tools = (m: Mensaje): Herramienta[] => (m.partes || []).filter(p => p.tipo === 'herramienta').map(p => (p as { h: Herramienta }).h);

function lastText(m: Mensaje): string {
  const ps = m.partes || [];
  for (let i = ps.length - 1; i >= 0; i--) {
    const p = ps[i];
    if (p.tipo === 'texto' && p.texto.trim()) return p.texto.trim();
  }
  return ps.length ? '' : (m.texto || '').trim();
}

// Los mensajes del usuario que cuentan como peticiones (sin las notas de pgocode ni los errores).
const requests = (ms: Mensaje[]) => ms.filter(m => m.rol === 'user' && (m.texto || '').trim());

export interface WorkingMemory {
  goal?: string;
  requests: string[];      // peticiones recientes del usuario (sin el objetivo), de la más antigua a la más reciente
  turns: { request: string; reply: string }[]; // cada petición con lo último que respondió el agente
  read: string[];          // ficheros consultados (leídos)
  files: string[];         // ficheros creados o editados (rutas tal cual las dio la herramienta)
  commands: { cmd: string; ok: boolean; noOutput?: boolean }[]; // noOutput: acabó «bien» pero sin salida (quizá no se ejecutó)
  tasks: { text: string; state: 'pendiente' | 'curso' | 'hecho' }[];
  decisions: string[];
  lastAnswer?: string;
}

// collectWorkingMemory: los datos de la memoria (sin formato).
// Lo que quedó fuera del contexto del agente con «Continuar desde aquí» no cuenta: el agente lo ha olvidado.
export function collectWorkingMemory(todos: Mensaje[]): WorkingMemory {
  // Tampoco las órdenes que hizo pgocode sin el agente (direct.ts): ni la orden ni su nota.
  const ms = todos.filter((m, i) => !m.fuera && !m.directo && !(m.rol === 'user' && todos[i + 1]?.directo)).map(vigentParts);
  const us = requests(ms);
  const goal = us[0]?.texto.trim();
  const files: string[] = [];
  const read: string[] = [];
  const turns: { request: string; reply: string }[] = [];
  const commands: WorkingMemory['commands'] = [];
  const decisions: string[] = [];
  let tasks: WorkingMemory['tasks'] = [];
  let lastAnswer: string | undefined;
  for (const m of ms) {
    if (m.rol === 'user') {
      if ((m.texto || '').trim()) turns.push({ request: m.texto.trim(), reply: '' });
      for (const f of sentences(m.texto || '')) if (DECISION.test(f)) decisions.push('Usuario: ' + cut(f, 200));
      continue;
    }
    if (m.nota) continue;
    for (const h of tools(m)) {
      const r = (h.resumen || '').trim();
      if (!r) continue;
      if (READS.test(h.nombre) && h.estado !== 'error') {
        if (!read.includes(r)) read.push(r);
      } else if (EDITS.test(h.nombre) && h.estado !== 'error') {
        const i = files.indexOf(r);
        if (i >= 0) files.splice(i, 1);
        files.push(r);
      } else if (COMMANDS.test(h.nombre) && RELEVANT.test(r) && h.estado !== 'curso') {
        const cmd = cut(r, 160);
        // Una orden repetida vale por su último resultado.
        const i = commands.findIndex(c => c.cmd === cmd);
        if (i >= 0) commands.splice(i, 1);
        // Sin salida no consta que se ejecutara (Antigravity da por acabada una orden denegada, sin salida): no es «ok».
        if (h.estado === 'ok' && !(h.salida || '').trim()) commands.push({ cmd, ok: false, noOutput: true });
        else commands.push({ cmd, ok: h.estado === 'ok' });
      }
    }
    const lista = m.tareas?.find(t => t.tipo === 'lista' && !t.padre);
    // Las canceladas, como hechas (como hasta ahora): la memoria de trabajo no cambia.
    if (lista?.elementos?.length) tasks = lista.elementos.map(e => ({ text: e.texto, state: e.estado === 'cancelada' ? 'hecho' : e.estado }));
    const t = lastText(m);
    if (t && !m.error) {
      lastAnswer = t;
      if (turns.length) turns[turns.length - 1].reply = t;
      for (const f of sentences(t)) if (DECISION.test(f.replace(/^[-*•\d.)\s]+/, ''))) decisions.push('Agente: ' + cut(f.replace(/^[-*•\d.)\s]+/, ''), 200));
    }
  }
  return {
    goal, requests: us.slice(1).map(m => m.texto.trim()), turns,
    files, read: read.filter(r => !files.includes(r)), commands, tasks, decisions: [...new Set(decisions)], lastAnswer,
  };
}

// VIG_OUT: con esta vigencia o más, fuera (VIG_CUT de prune.ts; aquí sin importarlo para las pruebas con node).
const VIG_OUT = 3;

// vigentOnly: solo lo vigente (vigencia por debajo de 3): sin los intercambios fuera de vigencia (el mensaje del
// usuario y la respuesta llevan la misma) ni las herramientas fuera de vigencia de los demás. Para la memoria del
// relevo por vigencia (Antigravity).
export function vigentOnly(ms: Mensaje[]): Mensaje[] {
  return ms.filter(m => !(m.vig && m.vig.vigencia >= VIG_OUT)).map(vigentParts);
}

function vigentParts(m: Mensaje): Mensaje {
  if (!m.partes?.some(p => p.tipo === 'herramienta' && (p.h.vig?.vigencia ?? 0) >= VIG_OUT)) return m;
  return { ...m, partes: m.partes.filter(p => !(p.tipo === 'herramienta' && (p.h.vig?.vigencia ?? 0) >= VIG_OUT)) };
}

function sentences(t: string): string[] {
  return t.split(/\n+|(?<=[.!?])\s+/).map(s => s.trim()).filter(s => s.length > 3);
}

// buildWorkingMemory: la memoria de trabajo de la conversación, en texto, para pasársela a una sesión nueva del
// agente (como mucho «max» caracteres; lo que sobra se recorta de lo menos importante).
export function buildWorkingMemory(ms: Mensaje[], max = MEMORY_MAX): string {
  const w = collectWorkingMemory(ms);
  if (!w.goal && !w.lastAnswer) return '';
  // Presupuesto por apartado (caracteres): el objetivo y lo último pesan más; ficheros y órdenes, la cola.
  const sec: string[] = [];
  if (w.goal) sec.push('## Objetivo (primera petición)\n' + cut(w.goal, 2000));
  // Peticiones con un extracto de lo que respondió el agente (la de la última, entera más abajo).
  const turns = w.turns.slice(-6);
  if (turns.length > 1 || (turns.length === 1 && w.turns.length > 1)) {
    sec.push('## Conversación (peticiones y respuestas del agente, extracto)\n' + turns.map((x, i) => {
      const req = w.turns.length - turns.length + i === 0 ? '(el objetivo)' : cut(x.request, 400);
      const last = i === turns.length - 1;
      return `- Usuario: ${req}` + (x.reply && !last ? `\n  Agente: ${cut(x.reply, 700)}` : '');
    }).join('\n'));
  }
  if (w.tasks.length) {
    const mark = { hecho: '[x]', curso: '[~]', pendiente: '[ ]' } as const;
    sec.push('## Lista de tareas\n' + w.tasks.slice(0, 30).map(t => `- ${mark[t.state]} ${cut(t.text, 160)}`).join('\n'));
  }
  if (w.files.length) {
    const fs = w.files.slice(-40);
    sec.push(`## Ficheros creados o editados (${w.files.length})\n` + (w.files.length > fs.length ? '- …\n' : '') + fs.map(f => '- ' + f).join('\n'));
  }
  if (w.read.length) sec.push(`## Ficheros consultados (${w.read.length})\n` + w.read.slice(-30).map(f => '- ' + f).join('\n'));
  if (w.commands.length) sec.push('## Órdenes relevantes (último resultado)\n' + w.commands.slice(-12).map(c => `- ${c.ok ? 'ok' : c.noOutput ? 'sin salida' : 'ERROR'}: \`${c.cmd}\``).join('\n'));
  if (w.decisions.length) sec.push('## Decisiones y avisos\n' + w.decisions.slice(-12).map(d => '- ' + d).join('\n'));
  if (w.lastAnswer) sec.push('## Última respuesta del agente (recortada)\n' + cutEnds(w.lastAnswer, 2500));
  let t = sec.join('\n\n');
  // Si no cabe, se recortan por el final los apartados más largos (salvo la última respuesta, que va al final).
  while (t.length > max) {
    const i = sec.reduce((a, x, k) => (k < sec.length - (w.lastAnswer ? 1 : 0) && x.length > sec[a].length ? k : a), 0);
    const exceso = t.length - max;
    if (sec[i].length - exceso < 200) { t = t.slice(0, max - 1) + '…'; break; }
    sec[i] = sec[i].slice(0, sec[i].length - exceso - 2) + '\n…';
    t = sec.join('\n\n');
  }
  return t;
}

// El principio y el final de un texto largo (lo que suele importar de una respuesta: el plan y la conclusión).
function cutEnds(t: string, n: number): string {
  const x = t.trim();
  if (x.length <= n) return x;
  const h = Math.floor(n * 0.4);
  return x.slice(0, h).trimEnd() + '\n[…]\n' + x.slice(x.length - (n - h - 5)).trimStart();
}

// relayMessage: lo que recibe la sesión nueva del agente: la memoria de trabajo y el mensaje del usuario. Si el
// mensaje empieza por una orden («/skill …»), la memoria va detrás para no romperla.
export function relayMessage(memory: string, message: string): string {
  const head = '[Memoria de trabajo: pgocode ha renovado tu contexto para ahorrar tokens. Esto es lo esencial de la conversación anterior en esta misma carpeta; si te falta algo, mira los ficheros o el historial de git]\n\n' + memory + '\n\n[Fin de la memoria de trabajo]';
  return message.trimStart().startsWith('/') ? `${message}\n\n${head}` : `${head}\n\n${message}`;
}

// RELAY_THRESHOLD: con el relevo siempre activo (Antigravity, ver contextPolicy.ts), el contexto a partir del cual el
// mensaje va a una sesión nueva aunque la caché siga caliente.
export const RELAY_THRESHOLD = 60_000;
// PRUNE_MIN_TOKENS: con el relevo por vigencia (modo «vig»), lo que tiene que pesar lo que está fuera de vigencia
// para relevar (el mismo mínimo que la poda: llm.pruneMinTokens).
export const PRUNE_MIN_TOKENS = 15_000;

export interface RelayInput {
  agent: string;
  mode: RelayMode;        // contextPolicy(agente).relay
  context?: number;       // tokens de la última llamada del agente (uso.contexto)
  idleMinutes: number;    // lo que lleva parada la conversación
  cacheMinutes?: number;  // lo que dura su caché (cacheStatus.ts; por defecto, CACHE_MINUTES)
  hasAgentSession: boolean;
  attachments?: number;
  planOrder?: boolean;    // orden de planes («/plan …») o ejecución de un plan
  memoryChars: number;    // tamaño de la memoria de trabajo
  threshold?: number;     // con el modo «on», otro umbral (el banco de consumo, estrategia «relevo:N»)
  outTokens?: number;     // con el modo «vig», lo que está fuera de vigencia (tokens estimados)
  outMin?: number;        // con el modo «vig», otro mínimo (para probarlo)
  forced?: boolean;       // con el modo «vig», el usuario lo ha pedido («Barrer ahora»): con algo fuera de vigencia basta
}

// idleMinimum: con la caché caducada, el contexto a partir del cual el relevo sale a cuenta. Seguir la sesión
// obliga a volver a escribir en caché TODO el historial; el relevo, solo lo fijo del agente (instrucciones y
// herramientas: unos 17k en Claude Code, menos en los demás) más la memoria. Medido con el banco de consumo: con la
// caché caliente el relevo cuesta más que seguir (escribe caché que no hacía falta).
export function idleMinimum(agent: string): number {
  return agent === 'claude-code' ? 25_000 : 20_000;
}

// shouldRelay: por qué el próximo mensaje debe ir a una sesión nueva con la memoria de trabajo (null: no).
// Con la caché caducada, si el historial pasa claramente de lo fijo del agente (idleMinimum); con el modo «on»,
// también al pasar de RELAY_THRESHOLD. Nunca con adjuntos, órdenes de planes o sin sesión del agente, ni si la
// memoria no ahorra al menos la mitad.
export function shouldRelay(i: RelayInput): string | null {
  if (i.mode === 'off' || !i.hasAgentSession || i.attachments || i.planOrder || !i.context || !i.memoryChars) return null;
  if (i.mode === 'vig' && i.forced && (i.outTokens || 0) > 0) return `barrido pedido: ${fmt(i.outTokens!)} de ${fmt(i.context)} están fuera de vigencia`;
  if (i.memoryChars / 4 > i.context / 2) return null;
  if (i.idleMinutes >= (i.cacheMinutes ?? CACHE_MINUTES) && i.context >= idleMinimum(i.agent)) return `la conversación lleva ${Math.round(i.idleMinutes)} min parada (sin caché) con ${fmt(i.context)} de contexto`;
  const limit = i.threshold || RELAY_THRESHOLD;
  if (i.mode === 'on' && i.context >= limit) return `el contexto (${fmt(i.context)}) pasa del umbral (${fmt(limit)})`;
  const outMin = i.outMin || PRUNE_MIN_TOKENS;
  if (i.mode === 'vig' && (i.outTokens || 0) >= outMin) return `${fmt(i.outTokens!)} de ${fmt(i.context)} están fuera de vigencia`;
  return null;
}

export const estimateTokens = (chars: number) => Math.ceil(chars / 4);
const fmt = (n: number) => n >= 1000 ? Math.round(n / 1000) + 'k' : String(n);
