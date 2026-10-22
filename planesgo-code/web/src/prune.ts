// Poda automática del historial del agente (internal/llm/prune.go): al empezar cada turno, pgocode retira del
// historial del agente lo que ya no importa y se lo dice a la interfaz con el evento «poda». Aquí, lo que ve el
// usuario: los intercambios con algo fuera del contexto del agente, atenuados y con el motivo, y en el turno la línea
// «✂ pgocode ha retirado 48k tokens del historial». Funciones puras: node --test src/prune.test.ts
import type { Mensaje } from './api';

// PruneMark: un intercambio con algo fuera del contexto del agente. L2: sigue, pero sin lo superado (lecturas,
// imágenes, salidas…); L3: sin relación con lo último (sus herramientas, fuera; los textos se quedan).
export interface PruneMark { nivel: 'L2' | 'L3'; motivo: string; similitud?: number }

// PruneReport: el evento «poda» del chat.
export interface PruneReport {
  retirados: number;   // tokens que salen del historial en este turno
  repuestos?: number;  // piezas que vuelven (un intercambio que vuelve a ser relevante)
  intercambios: { punto: string; nivel: 'L2' | 'L3'; motivo: string; similitud?: number }[];
}

// applyPruneReport: el estado de cada respuesta anterior según el informe (por su punto). Las que no salen en él
// están enteras en el contexto (también las que han vuelto). Su mensaje del usuario lleva la misma marca. Los dos
// últimos mensajes (el del usuario y la respuesta en curso) no se tocan.
export function applyPruneReport(ms: Mensaje[], r: PruneReport): Mensaje[] {
  const por = new Map(r.intercambios.map(x => [x.punto, x]));
  const fin = ms.length - 2;
  const out = ms.map(m => m);
  for (let i = 0; i < fin; i++) {
    const m = ms[i];
    if (m.rol !== 'assistant' || !m.punto) continue;
    const x = por.get(m.punto.id);
    const marca: PruneMark | undefined = x ? { nivel: x.nivel, motivo: x.motivo, ...(x.similitud !== undefined ? { similitud: x.similitud } : {}) } : undefined;
    out[i] = withMark(m, marca);
    // El mensaje del usuario de ese intercambio (el anterior que sea del usuario).
    for (let j = i - 1; j >= 0; j--) {
      if (ms[j].rol === 'user') { out[j] = withMark(ms[j], marca); break; }
      if (ms[j].punto) break;
    }
  }
  return out;
}

function withMark(m: Mensaje, marca?: PruneMark): Mensaje {
  if (!marca) {
    if (!m.prune) return m;
    const { prune: _, ...r } = m;
    return r;
  }
  return { ...m, prune: marca };
}

// prunedLine: la línea del turno («✂ pgocode ha retirado 48k tokens del historial»).
export function prunedLine(p: { tokens: number; repuestos?: number }): string {
  const k = p.tokens >= 1000 ? `${Math.round(p.tokens / 1000)}k` : String(p.tokens);
  let t = p.tokens > 0 ? `✂ pgocode ha retirado ${k} tokens del historial` : '';
  if (p.repuestos) t += (t ? '; ' : '↺ pgocode ') + (p.repuestos === 1 ? 'ha repuesto 1 pieza que vuelve a ser relevante' : `ha repuesto ${p.repuestos} piezas que vuelven a ser relevantes`);
  return t;
}

// pruneClass: la clase CSS de un mensaje con marca (L3, atenuado; L2, un poco).
export const pruneClass = (m: Mensaje) => (m.prune ? (m.prune.nivel === 'L3' ? ' pruned-out' : ' pruned-rules') : '');
