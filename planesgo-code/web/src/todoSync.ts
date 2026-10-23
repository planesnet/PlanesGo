// Lista de tareas del agente sincronizada con lo que hace: los modelos la actualizan tarde (dejan «en curso» una tarea
// mientras ya trabajan en otra, marcan varias de golpe al final o ninguna). pgocode estima, con reglas y sin gastar
// tokens, a qué tarea corresponde cada acción del agente (ficheros, rutas, nombres de componentes, órdenes y palabras
// de la tarea) y lo muestra aparte («▷ en curso según lo que hace», «probablemente hecha», «sin hacer al acabar el
// turno»). Es solo una capa visual: no cambia nada de lo que se manda al agente. Cuando el agente actualiza su lista,
// manda la suya (la inferencia vuelve a empezar desde ahí). Prudente: si la coincidencia no es clara, no infiere.
// Funciones puras: node --test src/todoSync.test.ts
import type { Herramienta, Mensaje } from './api.ts';
import type { Elemento, Tarea } from './tareas.ts';

// La vista de cada elemento: «curso» (la que el agente marca), «inferida» (la que hace según sus acciones),
// «probable» (probablemente hecha), «sin-hacer» (al acabar el turno, sin marcar ni tocar).
export type ItemView = 'curso' | 'inferida' | 'pendiente' | 'hecha' | 'probable' | 'sin-hacer' | 'cancelada';
export interface SyncItem { i: number; texto: string; estado: Elemento['estado']; vista: ItemView; motivo?: string }
export interface SyncView {
  items: SyncItem[];
  ahora?: SyncItem;          // la que se hace ahora (marcada por el agente o inferida)
  siguiente?: SyncItem;      // sin ninguna en curso: la primera pendiente
  pendientes: SyncItem[];    // lo que falta (sin la de ahora)
  hechas: SyncItem[];        // hechas y probablemente hechas
  canceladas: SyncItem[];
  done: number;              // hechas (sin las probables)
  probables: number;
  total: number;             // sin las canceladas
  finished: boolean;         // no queda nada pendiente (contando las probables)
  inferred: boolean;         // algo de lo que se ve no lo ha dicho el agente
}
// acabado: el turno ha terminado; parado: lo paró el usuario (o falló); textoFinal: lo que dijo el agente al acabar;
// turno: todas las acciones del turno (al acabar, también cuentan las de antes de la última actualización de la lista:
// un agente que la actualiza tarde puede dejar «pendiente» algo que ya hizo).
export interface SyncOptions { acabado?: boolean; parado?: boolean; textoFinal?: string; turno?: Pick<Herramienta, 'nombre' | 'resumen' | 'entrada'>[] }

// ---- Palabras ----

const norm = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();

// Palabras que no distinguen una tarea de otra (artículos, verbos genéricos…).
const STOP = new Set(('a al algo ante bajo cada como con contra de del desde donde durante e el en entre es esta este esto ' +
  'hacia hasta la las le lo los mas muy no o para pero por que se segun si sin sobre solo su sus tambien tanto todo toda ' +
  'todos todas un una uno unos unas y ya etc nº n otro otra otros otras mismo misma nuevo nueva nuevos nuevas ' +
  'anadir crear hacer usar poner quitar implementar revisar actualizar cambiar mejorar integrar adaptar asegurar ' +
  'eliminar mover dejar dejandola permitir mostrar incluir aplicar definir convertir completo completa disponible ' +
  'the and or of to in on for with from into that this all new add create make use update fix check ensure the ' +
  'src internal cmd web lib app components componentes').split(/\s+/));

// Español → inglés de lo más común en código (vista ↔ View, proyecto ↔ Project…).
const ES_EN: Record<string, string> = {
  vista: 'view', proyecto: 'project', tarea: 'task', formulario: 'form', plantilla: 'template', ajuste: 'settings',
  configuracion: 'config', servidor: 'server', cliente: 'client', lista: 'list', busqueda: 'search', buscador: 'search',
  tema: 'theme', estilo: 'style', fichero: 'file', archivo: 'file', pantalla: 'screen', boton: 'button', informe: 'report',
  etapa: 'stage', nota: 'note', notificacion: 'notification', usuario: 'user', permiso: 'permission', pedido: 'order',
  calendario: 'calendar', tablero: 'board', filtro: 'filter', pagina: 'page', cabecera: 'header', barra: 'bar',
  campo: 'field', modulo: 'module', paquete: 'package', registro: 'log', imagen: 'image', enlace: 'link', ruta: 'route',
  prueba: 'test', menu: 'menu', panel: 'panel', modales: 'modal', modal: 'modal', grafico: 'chart', tabla: 'table',
  sesion: 'session', mensaje: 'message', error: 'error', fecha: 'date', idioma: 'lang', traduccion: 'i18n',
};

const stem = (w: string) => w.endsWith('ciones') ? w.slice(0, -6) + 'cion' : w.length > 4 && w.endsWith('s') && !w.endsWith('ss') ? w.slice(0, -1) : w;
const word = (w: string) => { const s = stem(norm(w)); return ES_EN[s] || s; };

// Palabras sueltas de un texto (sin las vacías), normalizadas.
function plainWords(s: string): Set<string> {
  const r = new Set<string>();
  for (const w of norm(s).split(/[^a-z0-9ñ]+/)) if (w.length >= 3 && !STOP.has(w) && !/^\d+$/.test(w)) r.add(word(w));
  return r;
}

// Trozos de un identificador: ProjectsListView → projects list view; config_store.go → config store go.
const subwords = (id: string) => id.replace(/([a-z0-9])([A-Z])/g, '$1 $2').split(/[^A-Za-z0-9ñÑ]+/).filter(w => w.length >= 3).map(word);

// ---- Conceptos (verificar, commit, reiniciar): tareas que se cumplen con una orden y no con un fichero ----

const CONCEPTS: { name: string; task: RegExp; cmd: RegExp }[] = [
  { name: 'verificar', task: /\b(verific|compil|comprob|prueba|probar|test|tsc|lint|build|vet|e2e|playwright|typecheck)/,
    cmd: /\b(tsc|lint|eslint|build|test|vet|e2e|playwright|pytest|jest|vitest|make( dist| build| test)?|cargo (build|test|check)|go (build|test|vet)|npm (run|test)|node --test)\b/ },
  { name: 'commit', task: /\b(commit|confirm(ar|a) (los )?cambios)/, cmd: /\bgit commit\b/ },
  { name: 'reiniciar', task: /\b(reinici|arranc|despleg|desplieg|restart|deploy)/, cmd: /\b(systemctl|restart|docker compose up|deploy|pm2)\b/ },
];

// ---- Tareas y acciones como conjuntos de pistas ----

interface TaskClues { ids: string[]; paths: string[][]; words: Set<string>; concepts: Set<string> }

export function taskClues(texto: string): TaskClues {
  const ids: string[] = [], paths: string[][] = [];
  for (const raw of texto.match(/[A-Za-z0-9_./-]+/g) || []) {
    const t = raw.replace(/^[./-]+|[./-]+$/g, '');
    if (t.length < 3) continue;
    if (t.includes('/')) {
      const segs = norm(t).split('/').filter(Boolean);
      if (segs.length) paths.push(segs);
      const last = t.split('/').pop() || '';
      if (/\.[a-z0-9]{1,5}$/i.test(last)) ids.push(norm(last));
      continue;
    }
    // Identificadores: con mayúscula por dentro (ViewList), con extensión (index.css), con _ o con mayúscula inicial
    // (App, Makefile): se comparan con el nombre del fichero que toca la acción.
    if (/[a-z][A-Z]|_|\.[a-z0-9]{1,5}$/i.test(t) || /^[A-Z]/.test(t)) ids.push(norm(t));
  }
  const n = norm(texto);
  return { ids: [...new Set(ids)], paths, words: plainWords(texto), concepts: new Set(CONCEPTS.filter(c => c.task.test(n)).map(c => c.name)) };
}

// Las herramientas que solo leen o buscan cuentan menos; las de la propia lista no cuentan.
const LIST_TOOLS = /^(todowrite|todoread|taskcreate|taskupdate|tasklist|taskget|toolsearch|skill)$/i;
const READ_TOOLS = /^(read|grep|glob|list|ls|find|search|websearch|webfetch|view|read_file|list_dir|grep_search|find_by_name|codesearch)$/i;

interface ActionClues { files: { base: string; stem: string; segs: string[] }[]; small: Set<string>; content: Set<string>; words: Set<string>; concepts: Set<string>; weight: number }

const FILE_KEYS = ['file_path', 'filePath', 'FilePath', 'path', 'Path', 'TargetFile', 'AbsolutePath', 'notebook_path'];
const SMALL_KEYS = ['command', 'CommandLine', 'pattern', 'Pattern', 'query', 'Query', 'glob', 'include', 'description'];

export function actionClues(h: Pick<Herramienta, 'nombre' | 'resumen' | 'entrada'>): ActionClues | undefined {
  if (LIST_TOOLS.test(h.nombre)) return undefined;
  let entrada: Record<string, unknown> = {};
  try { const x = JSON.parse(h.entrada || '{}'); if (x && typeof x === 'object') entrada = x; } catch { /* recortada: se usa como texto */ }
  const str = (k: string) => (typeof entrada[k] === 'string' ? entrada[k] as string : '');
  const rutas = FILE_KEYS.map(str).filter(Boolean);
  if (!rutas.length && h.resumen && /^[~./]?[\w.-]*\//.test(h.resumen) && !/\s/.test(h.resumen)) rutas.push(h.resumen);
  const files = rutas.map(r => {
    const segs = norm(r).split('/').filter(Boolean);
    const base = segs[segs.length - 1] || '';
    return { base, stem: base.replace(/\.[a-z0-9]{1,5}$/, ''), segs };
  });
  const smallText = SMALL_KEYS.map(str).filter(Boolean).join(' ') || (h.nombre.match(/bash|shell|run_command|exec/i) ? h.resumen || '' : '');
  const small = new Set<string>();
  for (const t of smallText.split(/[\s"'`;|&()<>=,]+/)) if (t.length >= 3) { small.add(norm(t)); small.add(norm(t.split('/').pop() || '')); }
  const content = new Set<string>();
  for (const t of (h.entrada || '').slice(0, 2000).match(/[A-Za-z_][A-Za-z0-9_]{5,}/g) || []) content.add(norm(t));
  const words = new Set<string>();
  for (const f of rutas) for (const w of subwords(f.split('/').slice(-3).join(' '))) words.add(w);
  for (const w of plainWords(smallText)) words.add(w);
  const cmd = norm(smallText);
  const concepts = new Set(/bash|shell|run_command|exec/i.test(h.nombre) || str('command') ? CONCEPTS.filter(c => c.cmd.test(cmd)).map(c => c.name) : []);
  const weight = READ_TOOLS.test(h.nombre) ? 0.5 : 1;
  return { files, small, content, words, concepts, weight };
}

// score: cuánto apunta una acción a una tarea. 3 o más: coincidencia clara (el fichero, la ruta o la orden que nombra la
// tarea); 1-2: indicio (palabras sueltas: «vistas de proyecto» y ProjectManagementView).
export function score(t: TaskClues, a: ActionClues): number {
  let s = 0;
  for (const id of t.ids) {
    const sinExt = id.replace(/\.[a-z0-9]{1,5}$/, '');
    if (a.files.some(f => f.base === id || (sinExt.length >= 3 && f.stem === sinExt))) s += 3;
    else if (a.small.has(id)) s += 2;
    else if (id.length >= 6 && a.content.has(id)) s += 1;
  }
  for (const p of t.paths) {
    if (a.files.some(f => f.segs.some((_, k) => p.every((x, j) => f.segs[k + j] === x)))) s += 3;
  }
  let w = 0;
  for (const x of t.words) if (a.words.has(x)) w++;
  s += Math.min(w, 2);
  for (const c of t.concepts) if (a.concepts.has(c)) s += 3;
  return s;
}

// ---- La vista sincronizada ----

const CLAIM_DONE = /\b(he (completado|terminado|acabado|implementado|aplicado|hecho|realizado)|(se ha|se han|queda|quedan|esta|estan) (completad|terminad|hech|implementad|aplicad|verificad)|todo (listo|hecho|funciona)|completad[oa]s? y verificad|(all|everything) (is )?(done|complete)|i have (completed|finished)|(has|have) been (completed|implemented))/;
const CLAIM_PENDING = /\b(quedan? (pendiente|por)|siguen? pendiente|estan? pendiente|pendientes? de|faltan?\b|no he podido|no ha sido posible|todavia no|aun no|not yet|remaining)/;

// agentClaimsDone: el texto final del agente dice que ha terminado (y no que falte algo).
export const agentClaimsDone = (texto: string | undefined) => { const n = norm(texto || ''); return CLAIM_DONE.test(n) && !CLAIM_PENDING.test(n); };

const STRONG = 3;

// syncTodo: la lista del agente con lo que se infiere de las acciones hechas DESPUÉS de su última actualización.
export function syncTodo(elementos: Elemento[], acciones: Pick<Herramienta, 'nombre' | 'resumen' | 'entrada'>[] | undefined, o: SyncOptions = {}): SyncView {
  const clues = elementos.map(e => taskClues(e.texto));
  const open = (i: number) => elementos[i].estado === 'pendiente' || elementos[i].estado === 'curso';
  const agentCur = elementos.findIndex(e => e.estado === 'curso');
  let cur = agentCur;                                   // la que se hace (según el agente y, si cambia, según las acciones)
  const hits = elementos.map(() => 0);                  // acciones que la apoyan (las que solo leen, media)
  const mods = elementos.map(() => 0);                  // de ellas, las que cambian algo (editar, escribir, ejecutar)
  const last = elementos.map(() => -1);                 // la última acción que la apoyó
  const why = elementos.map(() => '');                  // la última acción clara (para el «por qué» al pasar el ratón)
  let toward = -1, towardW = 0;                         // candidata a «en curso» y lo acumulado hacia ella
  (acciones || []).forEach((h, k) => {
    const a = actionClues(h);
    if (!a) return;
    const s = elementos.map((_, i) => (open(i) ? score(clues[i], a) : 0));
    const apoya = (i: number) => { hits[i] += a.weight; if (a.weight >= 1) mods[i]++; last[i] = k; why[i] = h.resumen || h.nombre; };
    // Si apunta (aunque sea poco) a la que está en curso, el agente tiene razón: nada que inferir.
    if (cur >= 0 && s[cur] > 0) { apoya(cur); toward = -1; towardW = 0; return; }
    const orden = s.map((_, i) => i).sort((x, y) => s[y] - s[x]);
    const best = orden[0] ?? -1, second = orden.length > 1 ? s[orden[1]] : 0;
    if (best < 0 || s[best] < STRONG || s[best] < 2 * second) return; // no está claro: no se infiere
    if (cur < 0 || best === cur) { cur = best; apoya(best); return; }
    // Otra tarea: cambia cuando lo acumulado llega a una acción que cambia algo (o dos que solo leen).
    if (toward !== best) { toward = best; towardW = 0; }
    towardW += a.weight;
    apoya(best);
    if (towardW >= 1) { cur = best; toward = -1; towardW = 0; }
  });

  // Al acabar: las acciones de todo el turno que apuntan claramente a una tarea abierta (y que cambian algo).
  if (o.acabado && o.turno) {
    for (const h of o.turno.slice(0, Math.max(0, o.turno.length - (acciones?.length || 0)))) {
      const a = actionClues(h);
      if (!a || a.weight < 1) continue;
      const s = elementos.map((_, i) => (open(i) ? score(clues[i], a) : 0));
      const orden = s.map((_, i) => i).sort((x, y) => s[y] - s[x]);
      if (orden.length && s[orden[0]] >= STRONG && s[orden[0]] >= 2 * (orden.length > 1 ? s[orden[1]] : 0)) {
        mods[orden[0]]++;
        if (!why[orden[0]]) why[orden[0]] = h.resumen || h.nombre;
      }
    }
  }
  const claims = o.acabado && !o.parado && agentClaimsDone(o.textoFinal);
  const items: SyncItem[] = elementos.map((e, i) => {
    const base = { i, texto: e.texto, estado: e.estado };
    if (e.estado === 'hecho') return { ...base, vista: 'hecha' };
    if (e.estado === 'cancelada') return { ...base, vista: 'cancelada' };
    const porQue = why[i] ? `Según lo que ha hecho el agente (${why[i]})` : '';
    if (o.acabado) {
      if (o.parado) return { ...base, vista: 'pendiente' };
      if (mods[i] > 0) return { ...base, vista: 'probable', motivo: `${porQue}; el agente no la marcó al acabar` };
      if (claims) return { ...base, vista: 'probable', motivo: 'El agente dice que ha terminado, pero no la marcó' };
      return { ...base, vista: 'sin-hacer', motivo: e.estado === 'curso' ? 'El agente la dejó en curso al acabar el turno' : 'Sin marcar ni tocar al acabar el turno' };
    }
    if (i === cur) return i === agentCur ? { ...base, vista: 'curso' } : { ...base, vista: 'inferida', motivo: porQue };
    // Una que ya no se toca porque el agente ha pasado a otra: probablemente hecha.
    const movedOn = cur >= 0 && cur !== i && last[cur] > last[i];
    if (movedOn && (mods[i] > 0 || (i === agentCur && cur > i))) return { ...base, vista: 'probable', motivo: i === agentCur ? 'El agente la marca en curso, pero ya trabaja en otra' : porQue };
    return { ...base, vista: 'pendiente' };
  });
  const ahora = items.find(x => x.vista === 'curso' || x.vista === 'inferida');
  const pendientes = items.filter(x => x !== ahora && (x.vista === 'pendiente' || x.vista === 'sin-hacer'));
  const hechas = items.filter(x => x.vista === 'hecha' || x.vista === 'probable');
  const canceladas = items.filter(x => x.vista === 'cancelada');
  const done = items.filter(x => x.vista === 'hecha').length;
  const probables = items.filter(x => x.vista === 'probable').length;
  return {
    items, ahora, siguiente: ahora ? undefined : pendientes.find(x => x.vista === 'pendiente'), pendientes, hechas, canceladas,
    done, probables, total: items.length - canceladas.length, finished: !ahora && !pendientes.length,
    inferred: items.some(x => x.vista === 'inferida' || x.vista === 'probable' || x.vista === 'sin-hacer'),
  };
}

// ---- Desde los mensajes del chat ----

// mainList: la lista del agente principal (las de sus subagentes llevan padre y van aparte).
export const mainList = (tareas: Tarea[] | undefined) => (tareas || []).filter(t => t.tipo === 'lista' && !t.padre).pop();

// actionsOf: las herramientas del agente principal (sin las de sus subagentes) de un mensaje.
export const actionsOf = (m: Pick<Mensaje, 'partes'>): Herramienta[] =>
  (m.partes || []).flatMap(p => (p.tipo === 'herramienta' && !p.h.padre ? [p.h] : []));

// finalText: lo que dice el agente después de su última herramienta (su resumen al acabar).
export function finalText(m: Pick<Mensaje, 'partes' | 'texto'>): string {
  const ps = m.partes || [];
  const k = ps.map(p => p.tipo).lastIndexOf('herramienta');
  const t = ps.slice(k + 1).filter(p => p.tipo === 'texto').map(p => (p as { texto: string }).texto).join('\n');
  return t || (k < 0 ? m.texto : '');
}

// turnTodo: la lista de un turno (la del chat): con las acciones de después de su última actualización. Sin saber dónde
// se actualizó (mensajes de antes de esto), solo la presentación, sin inferir.
export function turnTodo(m: Mensaje, vivo: boolean): SyncView | undefined {
  const l = mainList(m.tareas);
  if (!l?.elementos?.length) return undefined;
  const acc = l.tras === undefined ? undefined : actionsOf(m).slice(l.tras);
  return syncTodo(l.elementos, acc, { acabado: !vivo && l.tras !== undefined, parado: m.error, textoFinal: finalText(m), turno: actionsOf(m) });
}

// sessionTodo: la última lista de la sesión (la de Actividad), con las acciones de después en ese turno y los siguientes.
export function sessionTodo(mensajes: Mensaje[], trabajando: boolean): SyncView | undefined {
  for (let k = mensajes.length - 1; k >= 0; k--) {
    const l = mainList(mensajes[k].tareas);
    if (!l?.elementos?.length) continue;
    const despues = mensajes.slice(k + 1).filter(m => m.rol === 'assistant');
    const acc = l.tras === undefined ? undefined : [...actionsOf(mensajes[k]).slice(l.tras), ...despues.flatMap(actionsOf)];
    const ultimo = despues[despues.length - 1] || mensajes[k];
    return syncTodo(l.elementos, acc, { acabado: !trabajando && l.tras !== undefined, parado: ultimo.error, textoFinal: finalText(ultimo),
      turno: [...actionsOf(mensajes[k]), ...despues.flatMap(actionsOf)] });
  }
  return undefined;
}
