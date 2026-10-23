// Qué está haciendo el agente ahora mismo, para «Trabajando…»: la herramienta en curso (leyendo X, ejecutando Y…),
// si razona o escribe la respuesta, y los subagentes o tareas en segundo plano que siguen. Con lo que ya llega del
// agente: sin gastar tokens. Funciones puras: node --test src/activity.test.ts
import type { Herramienta, Mensaje } from './api';

const base = (r: string) => r.split('/').filter(Boolean).pop() || r;
const cut = (t: string, n: number) => { const x = t.replace(/\s+/g, ' ').trim(); return x.length > n ? x.slice(0, n - 1) + '…' : x; };

// Verbo de cada herramienta (los nombres de Claude Code, OpenCode y Antigravity).
const VERBOS: [RegExp, string, boolean][] = [
  [/^(read|read_file|view_file|view)$/i, 'leyendo', true],
  [/^(edit|multiedit|patch|apply_patch|str_replace|replace|edit_file|replace_file_content|multi_replace_file_content)$/i, 'editando', true],
  [/^(write|write_file|create_file|write_to_file)$/i, 'escribiendo', true],
  [/^(grep|search|grep_search|codebase_search|find_by_name)$/i, 'buscando', false],
  [/^(glob|list|ls|list_dir|list_files)$/i, 'mirando ficheros', false],
  [/^(bash|shell|run|run_command|exec|terminal)$/i, 'ejecutando', false],
  [/^(webfetch|web_fetch|read_url_content|fetch)$/i, 'consultando', false],
  [/^(websearch|web_search|search_web)$/i, 'buscando en internet', false],
  [/^(task|agent|invoke_subagent)$/i, 'lanzando un subagente', false],
  [/^(todowrite|taskcreate|taskupdate|tasklist|taskget)$/i, 'organizando sus tareas', false],
  [/^(skill)$/i, 'cargando una skill', false],
];

export function herramientaEnCurso(h: Herramienta): string {
  const nombre = h.nombre.replace(/^mcp__[^_]+__/, '');
  for (const [re, verbo, fichero] of VERBOS) {
    if (re.test(nombre)) {
      if (verbo === 'organizando sus tareas' || verbo === 'cargando una skill') return h.skill ? `cargando la skill ${h.skill}` : verbo;
      const r = h.resumen ? (fichero ? base(h.resumen) : cut(h.resumen, 60)) : '';
      return r ? `${verbo} ${r}` : verbo;
    }
  }
  return 'usando ' + nombre + (h.resumen ? ' ' + cut(h.resumen, 50) : '');
}

// actividad: lo que hace ahora (texto corto) o '' si aún no se sabe.
export function actividad(m: Mensaje | undefined): string {
  if (!m || m.rol !== 'assistant') return '';
  const partes = m.partes || [];
  const curso = partes.filter(p => p.tipo === 'herramienta' && p.h.estado === 'curso').map(p => (p as { h: Herramienta }).h);
  const vivas = (m.tareas || []).filter(t => t.tipo !== 'lista' && t.estado === 'curso');
  const l: string[] = [];
  const principal = curso.filter(h => !h.padre);
  if (principal.length) l.push(herramientaEnCurso(principal[principal.length - 1]) + (principal.length > 1 ? ` (y ${principal.length - 1} más)` : ''));
  else {
    const u = partes[partes.length - 1];
    if (u?.tipo === 'pensamiento') l.push('pensando');
    else if (u?.tipo === 'texto') l.push('escribiendo la respuesta');
  }
  if (vivas.length) {
    const t = vivas[vivas.length - 1];
    const quien = t.tipo === 'subagente' ? 'subagente' : 'tarea';
    l.push(vivas.length === 1 ? `${quien} «${cut(t.titulo || t.subtipo || t.id, 40)}» en marcha` : `${vivas.length} subagentes o tareas en marcha`);
  }
  return l.join(' · ');
}

// consumoPorAccion: lo que costó cada acción (herramienta) del turno, en tokens: lo acumulado de la conversación al
// lanzarla menos lo de la acción anterior (o el inicio del turno). Los tokens se gastan por llamada al modelo: si
// en una llamada lanza varias acciones, la primera lleva el consumo y las demás nada.
export function consumoPorAccion(partes: { tipo: string; h?: Herramienta }[], inicio: number): Map<string, number> {
  const r = new Map<string, number>();
  let prev = inicio;
  for (const p of partes) {
    const t = p.tipo === 'herramienta' ? p.h?.tokens : undefined;
    if (!t || !p.h) continue;
    if (t > prev) r.set(p.h.id, t - prev);
    prev = Math.max(prev, t);
  }
  return r;
}

// toolStatusTitle: qué significa la bolita de una acción (al pasar el ratón): hecha, en curso o por qué falló (la
// primera línea con contenido de su salida, que es el error del agente).
export function toolStatusTitle(h: Pick<Herramienta, 'estado' | 'salida'>): string {
  if (h.estado === 'curso') return 'En curso';
  if (h.estado !== 'error') return 'Hecha';
  const motivo = (h.salida || '').split('\n').map(l => l.trim()).find(l => l) || '';
  return 'Ha fallado' + (motivo ? ': ' + (motivo.length > 200 ? motivo.slice(0, 199) + '…' : motivo) : ' (el agente no dio el motivo)') + '. El agente suele reintentarlo.';
}

// silenceLabel: si el agente lleva un rato sin mandar nada (un minuto o más), cuánto: así se ve que sigue vivo y que
// la espera es del modelo (OpenCode solo avisa de una acción cuando ha terminado; un fichero grande tarda minutos).
// Con una acción en curso (m: el mensaje del agente) la espera es de esa acción, no del modelo: «la orden sigue en
// marcha (N min)».
export function silenceLabel(ms: number, m?: Mensaje): string {
  if (!(ms >= 60_000)) return '';
  const min = Math.floor(ms / 60_000);
  const curso = (m?.partes || []).filter(p => p.tipo === 'herramienta' && p.h.estado === 'curso' && !p.h.padre).map(p => (p as { h: Herramienta }).h);
  if (curso.length) {
    const h = curso[curso.length - 1];
    const nombre = h.nombre.replace(/^mcp__[^_]+__/, '');
    const que = /^(bash|shell|run|run_command|exec|terminal)$/i.test(nombre) ? 'la orden' : /^(task|agent|invoke_subagent)$/i.test(nombre) ? 'el subagente' : `«${nombre}»`;
    return ` · ${que} sigue en marcha (${min} min)`;
  }
  return ` · sin novedades desde hace ${min} min (el modelo está generando o esperando respuesta)`;
}

// currentAction: lo que hace el agente principal ahora mismo, en pocas palabras (la línea del agente en Actividad):
// la acción en curso («editando X», «ejecutando make test»), «pensando», «escribiendo la respuesta» o, si lo último
// ya acabó, «esperando al modelo». Con un minuto o más sin eventos, cuánto (así se ve que sigue vivo).
export function currentAction(m: Mensaje | undefined, silenceMs = 0): string {
  const partes = m?.rol === 'assistant' ? m.partes || [] : [];
  const min = silenceMs >= 60_000 ? ` · ${Math.floor(silenceMs / 60_000)} min sin novedades` : '';
  const curso = partes.filter(p => p.tipo === 'herramienta' && p.h.estado === 'curso' && !p.h.padre).map(p => (p as { h: Herramienta }).h);
  if (curso.length) {
    const h = curso[curso.length - 1];
    const sub = /^(task|agent|invoke_subagent)$/i.test(h.nombre);
    const que = sub ? 'esperando al subagente' + (h.resumen ? ` «${cut(h.resumen, 50)}»` : '') : herramientaEnCurso(h);
    return que + (curso.length > 1 ? ` (y ${curso.length - 1} más)` : '') + min;
  }
  const u = partes[partes.length - 1];
  if (u?.tipo === 'pensamiento' && silenceMs < 30_000) return 'pensando';
  if (u?.tipo === 'texto' && silenceMs < 30_000) return 'escribiendo la respuesta';
  return 'esperando al modelo' + min;
}
