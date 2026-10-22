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
