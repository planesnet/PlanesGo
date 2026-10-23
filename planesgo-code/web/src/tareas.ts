// Tareas de los agentes (subagentes, órdenes en segundo plano, lista de tareas): el mismo concepto para todos.
// Funciones puras para poder probarlas: node --test src/tareas.test.ts

export interface Elemento { texto: string; estado: 'pendiente' | 'curso' | 'hecho' }
export interface Tarea {
  id: string; tipo?: 'subagente' | 'segundo_plano' | 'lista'; herramienta?: string; padre?: string;
  titulo?: string; subtipo?: string; estado?: 'curso' | 'ok' | 'error' | 'parada'; segundo_plano?: boolean;
  progreso?: string; salida?: string; detalle?: string; sesion_hija?: string; elementos?: Elemento[];
  inicio?: number; fin?: number; // cuándo la vio empezar y acabar pgocode (ms; para el tiempo en Actividad)
}

// fusiona: una actualización de una tarea (por id) sobre la lista; lo que llega vacío conserva lo anterior.
export function fusiona(l: Tarea[] | undefined, t: Tarea): Tarea[] {
  const lista = l || [];
  const i = lista.findIndex(x => x.id === t.id);
  if (i < 0) return [...lista, t];
  const v = lista[i];
  const n: Tarea = { ...v };
  for (const [k, x] of Object.entries(t) as [keyof Tarea, unknown][]) {
    if (x !== undefined && x !== '' && x !== false) (n as unknown as Record<string, unknown>)[k] = x;
  }
  return [...lista.slice(0, i), n, ...lista.slice(i + 1)];
}

// vivas: las que siguen en marcha (explican por qué el agente sigue ocupado).
export const vivas = (l: Tarea[] | undefined) => (l || []).filter(t => t.tipo !== 'lista' && t.estado === 'curso');

// alAcabar: al terminar el turno, lo que quedó «en curso» ya no lo está (el agente terminó o se paró).
export const alAcabar = (l: Tarea[] | undefined, parado: boolean, ahora?: number): Tarea[] | undefined =>
  l?.map(t => (t.estado === 'curso' && t.tipo !== 'lista' ? { ...t, estado: parado ? 'parada' : 'ok', ...(ahora && !t.fin ? { fin: ahora } : {}) } : t));

// conTiempos: la actualización de una tarea con cuándo empezó (la primera vez que llega) y cuándo acabó (al dejar de
// estar en curso), para fusionarla después: el agente no lo dice y en Actividad se ve cuánto lleva o tardó.
export function conTiempos(l: Tarea[] | undefined, t: Tarea, ahora: number): Tarea {
  if (t.tipo === 'lista') return t;
  const v = (l || []).find(x => x.id === t.id);
  const r: Tarea = { ...t };
  if (!v?.inicio) r.inicio = ahora;
  const estado = t.estado || v?.estado;
  if (estado && estado !== 'curso' && !v?.fin) r.fin = ahora;
  return r;
}

// deHerramienta: la tarea que creó una herramienta (para pintarla como tarea y no como herramienta suelta).
export const deHerramienta = (l: Tarea[] | undefined, id: string) => (l || []).find(t => t.herramienta === id && t.tipo !== 'lista');

export const NOMBRES: Record<string, string> = { subagente: 'Subagente', segundo_plano: 'En segundo plano', lista: 'Lista de tareas' };
