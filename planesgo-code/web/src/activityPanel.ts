// Textos del panel Actividad (Actividad.tsx): los tokens de un subagente y el estado de cada subtarea en paralelo
// (la lista en vivo está en liveActivity.ts). Funciones puras: node --test src/activityPanel.test.ts
import type { Sesion } from './sesiones';
import { fraseUnion, NOMBRE_HIJA, type Reparto } from './reparto.ts';

// tokensOfProgress: los tokens del último progreso de un subagente («… · 12345 tokens»), si se saben.
export function tokensOfProgress(progreso?: string): number | undefined {
  const m = /(\d+) tokens/.exec(progreso || '');
  return m ? Number(m[1]) : undefined;
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
