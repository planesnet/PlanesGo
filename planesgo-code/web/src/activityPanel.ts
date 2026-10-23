// Textos del panel Actividad (Actividad.tsx): contadores, la línea de cada tarea y el estado de cada subtarea en
// paralelo. Funciones puras: node --test src/activityPanel.test.ts
import type { Tarea } from './tareas';
import type { Sesion } from './sesiones';
import { fraseUnion, NOMBRE_HIJA, type Reparto } from './reparto.ts';

const cut = (t: string, n: number) => { const x = t.replace(/\s+/g, ' ').trim(); return x.length > n ? x.slice(0, n - 1).trimEnd() + '…' : x; };

// countLabel: «2 en marcha · 3 terminados» (lo que haya); «0» si no hay nada. «terminadas» para las femeninas.
export function countLabel(running: number, done: number, feminine = false): string {
  if (!done) return String(running);
  return `${running} en marcha · ${done} ${feminine ? 'terminadas' : 'terminados'}`;
}

// tokensOfProgress: los tokens del último progreso de un subagente («… · 12345 tokens»), si se saben.
export function tokensOfProgress(progreso?: string): number | undefined {
  const m = /(\d+) tokens/.exec(progreso || '');
  return m ? Number(m[1]) : undefined;
}

const tokens = (n: number) => n >= 1000 ? Math.round(n / 1000) + ' k tokens' : n + ' tokens';

// taskLine: la línea de debajo del título de una tarea. En marcha, su progreso; terminada, lo que devolvió (recortado)
// y sus tokens si se saben; la ruta del fichero de salida solo si no hay nada mejor. title: el texto completo.
export function taskLine(t: Tarea): { text: string; title: string } {
  if (t.estado === 'curso') return { text: t.progreso || '', title: t.progreso || '' };
  const n = t.tipo === 'subagente' ? tokensOfProgress(t.progreso) : undefined;
  const salida = (t.salida || '').trim();
  if (salida) return { text: cut(salida, 160) + (n ? ' · ' + tokens(n) : ''), title: salida + (t.detalle ? '\n\nSalida completa: ' + t.detalle : '') };
  if (n) return { text: tokens(n), title: t.detalle || '' };
  return { text: t.detalle || '', title: t.detalle || '' };
}

// childStatus: cómo va una subtarea en paralelo (sesión hija), con lo que guarda el reparto (estado de la hija y de
// su unión). Sin reparto guardado, por su rama: si ya es la de la madre, está unida.
export function childStatus(h: Sesion, madre: Sesion | null | undefined, repartos: Reparto[]): { text: string; kind: 'ok' | 'curso' | 'aviso' } {
  const rama = madre?.rama || 'tu rama';
  const x = repartos.find(r => r.madre === h.madre)?.hijas.find(y => y.sesion === h.id)
    ?? repartos.flatMap(r => r.hijas).find(y => y.sesion === h.id);
  if (!x) {
    return h.rama === madre?.rama ? { text: `unida: sus cambios ya están en ${rama}`, kind: 'ok' }
      : { text: `${h.rama || 'sin rama'} → se unirá a ${rama}`, kind: 'curso' };
  }
  if (x.estado === 'espera' || x.estado === 'trabajando') return { text: `${NOMBRE_HIJA[x.estado]} · ${x.rama || 'su rama'} → se unirá a ${rama}`, kind: 'curso' };
  if (x.union === 'unida') return { text: `unida: sus cambios ya están en ${rama}`, kind: 'ok' };
  if (x.union === 'sin_cambios') return { text: `${NOMBRE_HIJA[x.estado]}, sin cambios: nada que unir`, kind: 'ok' };
  return { text: `${NOMBRE_HIJA[x.estado]} · ${fraseUnion(x)}`, kind: 'aviso' };
}
