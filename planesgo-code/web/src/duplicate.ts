// «Duplicar sin historial»: una sesión nueva con los mismos ajustes que otra (proyecto, rama, carpeta, alojamiento,
// agente, modelo, esfuerzo, permisos, MCP, tarea de Odoo, carpetas autorizadas, avisos…) pero sin su conversación:
// el agente empieza de cero. Lo que va por sesión en el almacén (conversación, cola, preguntas aparte, gasto) cuelga
// del id, así que con un id nuevo empieza vacío.
import type { Sesion } from './sesiones';

// Campos que NO se copian: la identidad, el nombre (se calcula), la conversación con el agente y el plan en
// revisión y el «Continuar desde aquí» pendiente (son de la conversación), las fechas y si está archivada. Todo lo demás (también los
// campos que se añadan) se copia.
const NOT_COPIED = ['id', 'nombre', 'sesionAgente', 'plan', 'corte', 'creada', 'ultima', 'archivada'] as const satisfies readonly (keyof Sesion)[];

const COPY_SUFFIX = / · copia \d+$/;

// copyName: «<base> · copia N» con el primer N libre entre los nombres que ya hay. Si el original ya es una copia,
// se parte de su nombre base (no se encadenan sufijos).
export function copyName(name: string, existing: readonly string[]): string {
  const base = (name.trim() || 'Sesión nueva').replace(COPY_SUFFIX, '');
  const taken = new Set(existing);
  let n = 1;
  while (taken.has(`${base} · copia ${n}`)) n++;
  return `${base} · copia ${n}`;
}

// duplicateSession: la copia de «s», con nombre libre entre las sesiones «others» de su proyecto.
export function duplicateSession(s: Sesion, others: readonly Sesion[], now = Date.now()): Sesion {
  const copy: Partial<Sesion> = structuredClone(s);
  for (const k of NOT_COPIED) delete copy[k];
  return {
    ...(copy as Omit<Sesion, (typeof NOT_COPIED)[number]>),
    id: now.toString(36) + Math.random().toString(36).slice(2, 6),
    nombre: copyName(s.nombre, others.map(o => o.nombre)),
    creada: now,
    ultima: now,
  };
}
