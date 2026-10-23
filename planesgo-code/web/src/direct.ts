// Órdenes directas: lo mecánico que pgocode hace sin el agente (compilar, pasar las pruebas, comprobar tipos o lint,
// commit) y las preguntas de estado que ya sabe responder (cambios en git, ficheros tocados, gasto, rama y carpeta,
// qué hizo el agente en el turno anterior). Cada turno del agente reenvía todo su contexto (decenas de miles de
// tokens): para esto no hace falta. La detección es estricta: solo si el mensaje ENTERO es la orden («compila»,
// «pasa los tests», «haz commit»…); con más palabras («compila y arregla los errores») va al agente, como siempre.
// Nunca push, reset ni nada destructivo. Lo ejecuta el servidor (POST /api/directo, internal/servidor/directo.go).
// Funciones puras para poder probarlas: node --test src/direct.test.ts
import type { Herramienta, Mensaje, Uso } from './api';
import { changedFiles, ultimoTexto } from './aparte.ts';
import * as CO from './costes.ts';
import { plural } from './plural.ts';

export type DirectKind = 'build' | 'test' | 'typecheck' | 'lint' | 'commit' | 'status' | 'diff' | 'files' | 'cost' | 'where' | 'last_turn';

// Las que ejecutan algo en la carpeta (con el agente trabajando, esperan en la cola a que acabe su turno).
const MECHANICAL = new Set<DirectKind>(['build', 'test', 'typecheck', 'lint', 'commit']);
export const isMechanical = (k: DirectKind) => MECHANICAL.has(k);
// Las que se responden en la interfaz sin preguntar al servidor (solo se anotan).
export const answeredHere = (k: DirectKind) => k === 'cost' || k === 'last_turn';

// Lo que guarda la nota de pgocode en la conversación.
export interface DirectNote {
  accion: DirectKind;
  curso?: boolean;      // ejecutándose
  ok?: boolean;
  orden?: string;
  origen?: string;
  segundos?: number;
  salida?: string;      // recortada (errores y final)
  errores?: string;     // solo lo relevante, para pasárselo al agente
  carpeta?: string;
  hash?: string;        // commit: el hash corto y su mensaje (para decírselo al agente en su próximo mensaje)
  mensaje?: string;
}

// Respuesta de /api/directo.
export interface DirectResult {
  accion: DirectKind; orden?: string; origen?: string; ok: boolean; codigo?: number; segundos?: number; salida?: string; errores?: string;
  mensaje?: string; hash?: string; via?: string; ficheros?: string[]; rama?: string; carpeta?: string; copia?: boolean; sin_orden?: boolean; motivo?: string;
}

const norm = (t: string) => t.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/\s+/g, ' ').trim();

// Cortesías al principio o al final que no cambian la orden.
const PREFIX = /^(?:(?:oye|venga|vale|ok|bueno|pgocode|por favor|porfa|please|ahora|y ahora)[\s,:]+)+/;
const SUFFIX = /(?:[\s,]+(?:por favor|porfa|please|pls|ya|ahora|de nuevo|otra vez|again))+$/;

const PATTERNS: [DirectKind, RegExp][] = [
  ['build', /^(?:compila(?:lo)?|recompila(?:lo)?|build|rebuild|compile(?: it| the project)?|(?:haz|lanza|ejecuta|corre|run|do) (?:el |un |a |the )?build|construye(?: el proyecto)?|compila (?:el proyecto|todo|el codigo|la app|la aplicacion)|build (?:it|the project|everything|all))$/],
  ['test', /^(?:tests?|testea(?:lo)?|run (?:the |all the )?(?:unit )?tests|(?:pasa|ejecuta|lanza|corre|execute) (?:(?:los|las|el|todos los|todas las) )?(?:tests?|pruebas|tests unitarios|pruebas unitarias|test unitarios))$/],
  ['typecheck', /^(?:typecheck|type-check|type check|check (?:the )?types|(?:comprueba|verifica|chequea|pasa) (?:los )?tipos|(?:pasa|ejecuta|lanza|corre|run) (?:el |the )?(?:typecheck|type-check|tsc|vet))$/],
  ['lint', /^(?:lint|lintea(?:lo)?|(?:pasa|ejecuta|lanza|corre|run) (?:el |the )?(?:lint|linter|eslint))$/],
  ['commit', /^(?:commit|commitea(?:lo)?(?: todo| los cambios)?|haz (?:un |el )?commit(?: de (?:todo|los cambios))?|git commit|commit (?:it|all|everything|the changes|changes)|(?:make|do) a commit)$/],
  ['status', /^(?:git status|estado de git|estado del repo(?:sitorio)?|(?:que )?cambios hay(?: sin (?:commitear|confirmar)| pendientes)?|que cambios hay(?: sin (?:commitear|confirmar)| pendientes)?|hay cambios(?: sin (?:commitear|confirmar)| pendientes)?|que (?:hay|queda|falta) (?:sin|por) commitear|(?:are there )?any (?:uncommitted )?changes|(?:uncommitted|pending) changes|what(?:'s| is) uncommitted)$/],
  ['diff', /^(?:git diff|diff|el diff|(?:ensename|muestrame|dame|ver|show(?: me)?) (?:el |the )?diff)$/],
  ['files', /^(?:que (?:ficheros|archivos) (?:has (?:tocado|cambiado|modificado|editado)|hemos (?:tocado|cambiado|modificado)|se han (?:tocado|cambiado|modificado)|ha (?:tocado|cambiado|modificado) el agente|han cambiado)(?: hasta ahora)?|(?:ficheros|archivos) (?:cambiados|tocados|modificados)|(?:which|what) files (?:did you|have you|has the agent) (?:change|changed|touch|touched|edit|edited|modify|modified)(?: so far)?|(?:which|what) files (?:changed|were changed|have changed))$/],
  ['cost', /^(?:cuanto (?:llevamos|llevo|hemos|he|has|llevas) gastad[oa]s?(?: hasta ahora| en (?:esta|la) sesion)?|cuanto (?:llevamos|va|vamos) de gasto|cuanto (?:ha costado|cuesta|lleva costando) (?:esto|la sesion|esta sesion)|cuantos tokens (?:llevamos|hemos gastado|has gastado|van|llevas|se han gastado|gastados)(?: hasta ahora| en (?:esta|la) sesion)?|(?:gasto|coste|costo) de (?:la|esta) sesion|how much (?:have we|did we|have i|did i) spen[dt](?: so far)?|how many tokens (?:have we used|did we use|so far|used)(?: so far)?)$/],
  ['where', /^(?:en que rama (?:estamos|estoy|estas|trabajas|trabajamos)|que rama (?:es esta|es|estamos usando|usamos)|rama actual|en que (?:carpeta|directorio) (?:estamos|estoy|estas|trabajas|trabajamos)|en que rama y carpeta (?:estamos|estas|trabajas)|(?:which|what) branch(?: (?:are we on|am i on|is this))?|current branch|pwd)$/],
  ['last_turn', /^(?:que (?:has hecho|hiciste)(?: en (?:el )?(?:ultimo turno|turno anterior))?|que hizo el agente(?: en (?:el )?(?:ultimo turno|turno anterior))?|resumen del (?:ultimo turno|turno anterior)|que acciones (?:has hecho|hiciste)|what did you (?:just )?do(?: in the last turn)?|what have you done)$/],
];

// detect: la orden directa si el mensaje entero lo es; null si no (o si lleva un prefijo de los de siempre).
export function detect(texto: string): DirectKind | null {
  const t = texto.trim();
  if (!t || t.length > 80 || /\n/.test(t) || /^(?:>|\/|\?)/.test(t)) return null;
  let n = norm(t).replace(/^[¿¡\s]+/, '').replace(/[?!.…\s]+$/, '').replace(/^[¿¡\s]+/, '');
  n = n.replace(PREFIX, '').replace(SUFFIX, '').replace(/[?!.,…\s]+$/, '').trim();
  if (!n) return null;
  for (const [k, re] of PATTERNS) if (re.test(n)) return k;
  return null;
}

const LABELS: Record<DirectKind, string> = {
  build: 'Compilar', test: 'Pruebas', typecheck: 'Comprobar tipos', lint: 'Lint', commit: 'Commit', status: 'Cambios sin commitear',
  diff: 'Diff', files: 'Ficheros tocados', cost: 'Gasto de la sesión', where: 'Rama y carpeta', last_turn: 'Turno anterior',
};
export const label = (k: DirectKind) => LABELS[k];

const RUNNING: Partial<Record<DirectKind, string>> = {
  build: 'compilando', test: 'pasando las pruebas', typecheck: 'comprobando los tipos', lint: 'pasando el lint', commit: 'haciendo el commit',
};
export const FOOTER = 'Lo ha hecho pgocode sin el agente: 0 tokens';

// runningText: la nota mientras se ejecuta.
export const runningText = (k: DirectKind) => `⏳ pgocode está ${RUNNING[k] || 'mirándolo'}…`;

// tagText: la etiqueta encima del cuadro de texto antes de enviar.
export const tagText = (k: DirectKind, busy: boolean) =>
  busy && isMechanical(k) ? `→ pgocode, al acabar el turno: ${label(k).toLowerCase()} (0 tokens)` : `→ pgocode, sin el agente: ${label(k).toLowerCase()} (0 tokens)`;

const dur = (s?: number) => (s === undefined ? '' : s < 1 ? `${Math.round(s * 1000)} ms` : s < 60 ? `${s.toFixed(1).replace('.', ',')} s` : `${Math.floor(s / 60)} min ${Math.round(s % 60)} s`);
const code = (t: string) => '`' + t.replace(/`/g, "'") + '`';
const list = (fs: string[], max = 30) => fs.slice(0, max).map(f => '- ' + code(f)).join('\n') + (fs.length > max ? `\n- … y ${fs.length - max} más` : '');

// resultNote: la nota de pgocode con el resultado del servidor (texto Markdown y datos para la tarjeta).
export function resultNote(r: DirectResult): { texto: string; directo: DirectNote } {
  const directo: DirectNote = { accion: r.accion, ok: r.ok, orden: r.orden, origen: r.origen, segundos: r.segundos, carpeta: r.carpeta };
  const l: string[] = [];
  switch (r.accion) {
    case 'build': case 'test': case 'typecheck': case 'lint': {
      l.push(`${r.ok ? '✓' : '✗'} **${label(r.accion)}**: ${code(r.orden || '')} ${r.ok ? 'ha ido bien' : `ha fallado${r.codigo ? ` (código ${r.codigo})` : ''}`} · ${dur(r.segundos)}`);
      if (r.origen) l.push(`<small>Orden sacada de ${r.origen}.</small>`);
      directo.salida = r.salida; directo.errores = r.errores;
      break;
    }
    case 'commit':
      if (r.ok && r.hash) {
        l.push(`✓ **Commit** ${code(r.hash)} en ${code(r.rama || '')}: «${r.mensaje}»`);
        directo.hash = r.hash; directo.mensaje = r.mensaje;
        l.push(`${plural(r.ficheros?.length || 0, 'fichero')} · mensaje ${r.via?.startsWith('local/') ? `redactado por la IA local (${r.via.slice(6)})` : 'por reglas (sin IA local)'}. No se ha subido nada (sin push).`);
        directo.salida = r.salida;
      } else if (r.ok) l.push('✓ **Commit**: no hay cambios que commitear.');
      else { l.push(`✗ **Commit**: ha fallado (${code(r.orden || 'git commit')}).`); directo.salida = r.salida; directo.errores = r.errores || r.salida; }
      break;
    case 'status':
      if (!r.ok) { l.push(`✗ ${r.motivo || 'No se ha podido leer el estado de git.'}`); break; }
      l.push(r.ficheros?.length ? `**Cambios sin commitear** en ${code(r.rama || '')} (${r.ficheros.length}):\n\n${list(r.ficheros)}` : `**Sin cambios**: todo está commiteado en ${code(r.rama || '')}.`);
      break;
    case 'diff':
      if (!r.ok) { l.push(`✗ ${r.motivo || 'No se ha podido leer el diff.'}`); break; }
      l.push(r.ficheros?.length ? `**Diff** frente al último commit (${plural(r.ficheros.length, 'fichero')}):` : '**Sin cambios** frente al último commit.');
      if (r.ficheros?.length) directo.salida = r.salida;
      break;
    case 'where':
      l.push(`**Carpeta:** ${code(r.carpeta || '')}${r.copia ? ' (copia de trabajo de la rama)' : ''}`);
      l.push(r.rama ? `**Rama:** ${code(r.rama)}` : 'Sin rama (no es un repositorio git o HEAD suelto).');
      break;
    default:
      l.push(r.motivo || '');
  }
  return { texto: l.join('\n\n'), directo };
}

// filesAnswer: los ficheros que ha tocado el agente en la conversación (sus herramientas de edición) y los que
// tienen cambios sin commitear según git (si se sabe).
export function filesAnswer(ms: Mensaje[], git?: string[]): string {
  const deAgente = [...new Set(ms.filter(m => m.rol === 'assistant' && !m.nota && !m.fuera).flatMap(changedFiles))];
  const l: string[] = [];
  l.push(deAgente.length ? `**Ficheros que ha tocado el agente en esta conversación (${deAgente.length}):**\n\n${list(deAgente)}` : 'El agente no ha editado ficheros en esta conversación.');
  if (git) l.push(git.length ? `**Con cambios sin commitear según git (${git.length}):**\n\n${list(git)}` : 'Git: no hay cambios sin commitear.');
  return l.join('\n\n');
}

// costAnswer: lo gastado en la sesión (de costes.ts) y cuántas veces lo ha resuelto pgocode sin el agente.
export function costAnswer(ms: Mensaje[], aparte?: Uso): string {
  const g = CO.gasto(ms);
  const l: string[] = [];
  l.push(`**Gasto de la conversación:** ${CO.dinero(g.coste)} · ${CO.tokens(g.entrada + g.salida)} tokens (${CO.tokens(g.entrada)} de entrada, ${CO.tokens(g.salida)} de salida) en ${g.turnos} ${g.turnos === 1 ? 'turno' : 'turnos'} del agente.`);
  if (aparte && (aparte.entrada || aparte.salida)) l.push(`Preguntas aparte: ${CO.dinero(aparte.coste || 0)} · ${CO.tokens(aparte.entrada + aparte.salida)} tokens.`);
  const ult = [...ms].reverse().find(m => m.rol === 'assistant' && m.uso?.contexto);
  if (ult?.uso?.contexto) l.push(`Contexto del agente en su último turno: ${CO.tokens(ult.uso.contexto)} tokens (lo que reenvía en cada paso).`);
  const directas = ms.filter(m => m.directo).length;
  if (directas) l.push(`pgocode ha resuelto ${directas} ${directas === 1 ? 'orden' : 'órdenes'} sin el agente en esta conversación.`);
  return l.join('\n\n');
}

// lastTurnAnswer: lo que hizo el agente en su último turno, de sus acciones (sin IA).
export function lastTurnAnswer(ms: Mensaje[]): string {
  const m = [...ms].reverse().find(x => x.rol === 'assistant' && !x.nota && !x.fuera);
  if (!m) return 'El agente aún no ha hecho ningún turno en esta conversación.';
  const hs = (m.partes || []).filter(p => p.tipo === 'herramienta').map(p => (p as { h: Herramienta }).h);
  const l: string[] = [];
  const u = m.uso ? ` · ${CO.tokens(m.uso.entrada + m.uso.salida)} tokens${m.uso.coste ? ' · ' + CO.dinero(m.uso.coste) : ''}` : '';
  l.push(`**Último turno del agente:** ${hs.length} ${hs.length === 1 ? 'acción' : 'acciones'}${hs.some(h => h.estado === 'error') ? ` (${hs.filter(h => h.estado === 'error').length} con error)` : ''}${u}.`);
  if (hs.length) {
    const max = 25;
    l.push(hs.slice(0, max).map((h, i) => `${i + 1}. ${h.nombre}${h.resumen ? ' ' + code(cut(h.resumen, 100)) : ''}${h.estado === 'error' ? ' ✗' : ''}`).join('\n') + (hs.length > max ? `\n\n… y ${hs.length - max} más.` : ''));
  }
  const fs = changedFiles(m);
  if (fs.length) l.push(`**Ficheros cambiados:** ${fs.map(code).join(', ')}`);
  const t = ultimoTexto(m);
  if (t) l.push(`**Lo último que dijo:** «${cut(t, 400)}»`);
  return l.join('\n\n');
}

// agentMessage: lo que se le pasa al agente con «Pásaselo al agente»: la orden y solo los errores relevantes.
export function agentMessage(n: DirectNote): string {
  const errs = (n.errores || n.salida || '').trim();
  const que = n.accion === 'commit' ? 'el commit (git add -A y git commit)' : `${code(n.orden || '')}`;
  return `He ejecutado ${que} en la carpeta de la sesión y ha fallado.${errs ? ` Estos son los errores:\n\n\`\`\`\n${cut(errs, 4000, false)}\n\`\`\`` : ''}\n\nArréglalo${n.accion === 'commit' ? ' y vuelve a intentar el commit' : ` y vuelve a ejecutar ${code(n.orden || '')} para comprobarlo`}.`;
}

function cut(t: string, n: number, oneLine = true) {
  const x = oneLine ? t.replace(/\s+/g, ' ').trim() : t;
  return x.length > n ? x.slice(0, n - 1) + '…' : x;
}

const SHORT: Partial<Record<DirectKind, string>> = { build: 'compilar', test: 'pruebas', typecheck: 'tipos', lint: 'lint' };

// sinceLastTurn: lo que ha hecho pgocode sin el agente (commits, compilar, pruebas…) desde el último turno del agente,
// en una línea corta para su próximo mensaje («pgocode, desde tu último turno: commit abc123 "…"; pruebas ✓»), para
// que no razone con un estado viejo (p. ej. creer que algo está sin commitear). Vacío si no ha hecho nada. Lo que ya
// va en el mensaje (la orden de «Pásaselo al agente») no se repite.
export function sinceLastTurn(ms: Mensaje[], message = ''): string {
  let i = ms.length - 1;
  while (i >= 0 && !(ms[i].rol === 'assistant' && !ms[i].nota && !ms[i].error && !ms[i].fuera)) i--;
  const l: string[] = [];
  for (const m of ms.slice(i + 1)) {
    const d = m.directo;
    if (!d || d.curso || d.ok === undefined || !isMechanical(d.accion)) continue;
    if (d.accion === 'commit') {
      if (d.ok && d.hash) l.push(`commit ${d.hash} "${cut(d.mensaje || '', 80)}"`);
      else if (!d.ok) l.push('commit ✗');
      continue;
    }
    if (d.orden && message.includes(d.orden)) continue;
    l.push(`${SHORT[d.accion]} ${d.ok ? '✓' : '✗'}${d.orden ? ` (${code(d.orden)})` : ''}`);
  }
  return l.length ? `(pgocode, desde tu último turno: ${l.slice(-6).join('; ')}.)` : '';
}
