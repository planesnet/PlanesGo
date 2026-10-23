// Actividad de cada proyecto para la lista de proyectos: si alguna de sus sesiones tiene al agente trabajando (o
// subagentes, órdenes en segundo plano…) y si hay respuestas del agente que el usuario aún no ha visto. Los proyectos con
// algo de eso se quedan arriba en la lista aunque se cambie de proyecto. Funciones puras + un almacén en memoria con
// aviso (evento) para la interfaz. Pruebas: node --test src/projectActivity.test.ts

export interface ProjectState { working: string[]; unread: string[] } // ids de sesión

export const PROJECT_ACTIVITY = 'pgo:actividad-proyectos';
let estado: Record<number, ProjectState> = {};

const sin = (l: string[], id: string) => l.filter(x => x !== id);
const con = (l: string[], id: string) => (l.includes(id) ? l : [...l, id]);

// apply: el estado tras un cambio en una sesión (puro).
export function apply(s: Record<number, ProjectState>, proyecto: number, sesion: string, c: { working?: boolean; unread?: boolean }): Record<number, ProjectState> {
  const p = s[proyecto] || { working: [], unread: [] };
  const n: ProjectState = {
    working: c.working === undefined ? p.working : c.working ? con(p.working, sesion) : sin(p.working, sesion),
    unread: c.unread === undefined ? p.unread : c.unread ? con(p.unread, sesion) : sin(p.unread, sesion),
  };
  const r = { ...s };
  if (!n.working.length && !n.unread.length) delete r[proyecto];
  else r[proyecto] = n;
  return r;
}

// busy: el proyecto tiene algo que mantenerlo arriba.
export const busy = (s: Record<number, ProjectState>, proyecto: number) => Boolean(s[proyecto]);

// order: los proyectos con actividad primero (en su orden), luego el resto.
export function order<T extends { id: number }>(l: T[], s: Record<number, ProjectState>): T[] {
  return [...l.filter(p => busy(s, p.id)), ...l.filter(p => !busy(s, p.id))];
}

export function mark(proyecto: number, sesion: string, c: { working?: boolean; unread?: boolean }) {
  const n = apply(estado, proyecto, sesion, c);
  if (JSON.stringify(n) === JSON.stringify(estado)) return;
  estado = n;
  window.dispatchEvent(new CustomEvent(PROJECT_ACTIVITY));
}
export const current = () => estado;
