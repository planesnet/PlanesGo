// Avisos y límites de coste por sesión: ser consciente de lo que gasta cada conversación (en $ y en tokens) con
// avisos que no interrumpen (el indicador del pie del chat cambia de color). Funciones puras para poder probarlas: node --test src/costes.test.ts
import type { Mensaje } from './api';

export interface Avisos {
  avisoCoste?: number;    // $ acumulados en la sesión: aviso
  limiteCoste?: number;   // $ acumulados: límite (aviso en rojo)
  avisoTurno?: number;    // $ de un solo turno: aviso
  avisoTokens?: number;   // tokens acumulados (entrada + salida): aviso
  limiteTokens?: number;  // tokens acumulados: límite (aviso en rojo)
}

// Por defecto, según el plan de cada agente: Claude con suscripción (su coste es el equivalente de la API,
// orientativo), OpenCode con Vertex (pago real por uso), Antigravity (no informa del coste: por tokens).
export const DEFECTO: Record<string, Avisos> = {
  'claude-code': { avisoCoste: 5, limiteCoste: 20, avisoTurno: 2 },
  opencode: { avisoCoste: 1, limiteCoste: 3, avisoTurno: 0.5 },
  antigravity: { avisoTokens: 2_000_000, limiteTokens: 6_000_000 },
};
export const PLANES: Record<string, string> = {
  'claude-code': 'Suscripción de Claude: el coste es el equivalente de la API (orientativo).',
  opencode: 'Vertex AI: pago por uso, el coste es real.',
  antigravity: 'Antigravity no informa del coste: los avisos van por tokens.',
};

// efectivos: los de la sesión y, lo que no tenga, los del agente.
export const efectivos = (agente: string, propios?: Avisos): Avisos => ({ ...(DEFECTO[agente] || {}), ...limpia(propios) });
const limpia = (a?: Avisos): Avisos => Object.fromEntries(Object.entries(a || {}).filter(([, v]) => v !== undefined && v !== null && !(typeof v === 'number' && Number.isNaN(v)))) as Avisos;

export interface Gasto { coste: number; entrada: number; salida: number; turnos: number }

// gasto: lo acumulado en la conversación (la suma de los consumos de sus respuestas).
export function gasto(mensajes: Mensaje[]): Gasto {
  const g = { coste: 0, entrada: 0, salida: 0, turnos: 0 };
  for (const m of mensajes) {
    // Mientras trabaja el agente, sus tokens van por lo consumido hasta ahora (vivo); al acabar coincide con el uso.
    const u = m.vivo || m.uso;
    if (m.rol !== 'assistant' || !u) continue;
    g.coste += m.uso?.coste || 0;
    g.entrada += u.entrada || 0;
    g.salida += u.salida || 0;
    g.turnos++;
  }
  return g;
}

export type Nivel = 'ok' | 'aviso' | 'limite';

// nivel: dónde está el gasto respecto a los umbrales.
export function nivel(g: Gasto, a: Avisos): Nivel {
  const tokens = g.entrada + g.salida;
  if ((a.limiteCoste && g.coste >= a.limiteCoste) || (a.limiteTokens && tokens >= a.limiteTokens)) return 'limite';
  if ((a.avisoCoste && g.coste >= a.avisoCoste) || (a.avisoTokens && tokens >= a.avisoTokens)) return 'aviso';
  return 'ok';
}

// nuevos: los avisos que salta un turno (lo que se pasa por primera vez, y el coste del turno si es alto).
export function nuevos(antes: Gasto, despues: Gasto, a: Avisos): { nivel: Exclude<Nivel, 'ok'>; texto: string }[] {
  const l: { nivel: Exclude<Nivel, 'ok'>; texto: string }[] = [];
  const cruza = (umbral: number | undefined, x: number, y: number) => Boolean(umbral) && x < umbral! && y >= umbral!;
  const ta = antes.entrada + antes.salida, td = despues.entrada + despues.salida;
  if (cruza(a.limiteCoste, antes.coste, despues.coste)) l.push({ nivel: 'limite', texto: `La sesión ha llegado al límite de ${dinero(a.limiteCoste!)} (lleva ${dinero(despues.coste)}).` });
  else if (cruza(a.avisoCoste, antes.coste, despues.coste)) l.push({ nivel: 'aviso', texto: `La sesión ha pasado de ${dinero(a.avisoCoste!)} (lleva ${dinero(despues.coste)}).` });
  if (cruza(a.limiteTokens, ta, td)) l.push({ nivel: 'limite', texto: `La sesión ha llegado al límite de ${tokens(a.limiteTokens!)} tokens (lleva ${tokens(td)}).` });
  else if (cruza(a.avisoTokens, ta, td)) l.push({ nivel: 'aviso', texto: `La sesión ha pasado de ${tokens(a.avisoTokens!)} tokens (lleva ${tokens(td)}).` });
  const turno = despues.coste - antes.coste;
  if (a.avisoTurno && turno >= a.avisoTurno) l.push({ nivel: 'aviso', texto: `Este turno ha costado ${dinero(turno)} (más de ${dinero(a.avisoTurno)}).` });
  return l;
}

export const dinero = (x: number) => (x < 0.01 && x > 0 ? '<0,01' : x.toFixed(2).replace('.', ',')) + ' $';
export const tokens = (n: number) => n >= 1_000_000 ? (n / 1_000_000).toFixed(1).replace('.', ',') + ' M' : n >= 1000 ? Math.round(n / 1000) + ' k' : String(n);

// Precio por millón de tokens de entrada sin caché (para estimar lo que cuesta reenviar el historial); los que no
// están no se estiman (Claude con suscripción, Antigravity sin coste).
const ENTRADA_SIN_CACHE: Record<string, number> = { opencode: 2 };
// La caché implícita de los modelos dura unos minutos: pasado esto, el historial se reenvía a precio completo.
export const CACHE_MIN = 5;

// avisoContexto: antes de enviar, si la conversación es grande, cuánto se reenviará (y si irá sin caché porque lleva
// un rato parada). null: nada que avisar.
export function avisoContexto(contexto: number | undefined, minutosParada: number, agente: string): string | null {
  if (!contexto || contexto < 100_000) return null;
  const precio = ENTRADA_SIN_CACHE[agente];
  if (minutosParada >= CACHE_MIN) {
    const coste = precio ? ` (~${dinero(contexto * precio / 1e6)})` : '';
    return `Este mensaje reenviará ${tokens(contexto)} tokens de historial sin caché${coste}: la conversación lleva ${Math.round(minutosParada)} min parada. Puedes vaciar el contexto: el agente sigue en esta conversación y vuelve a leer lo que necesite.`;
  }
  if (contexto >= 150_000) return `Conversación grande: cada paso del agente reenvía ${tokens(contexto)} tokens. Puedes vaciar el contexto: el agente vuelve a leer lo que necesite.`;
  return null;
}
