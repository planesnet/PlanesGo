// Poda automática del historial del agente (internal/llm/prune.go) y vigencia (internal/llm/vigencia.go): cada
// intercambio y cada pieza (herramienta, imagen, adjunto) tiene una vigencia, un número ≥ 0: 0 es vital y 1, 2, 3…
// cada vez más viejo y menos importante (antigüedad, tiempo, poca relación con lo último, superada). Con 3 o más,
// fuera del contexto del agente. pgocode la manda con el evento «poda» al empezar cada turno y, en segundo plano,
// por /api/chat/vigencia. Aquí, lo que ve el usuario: cada cosa se desvanece según su vigencia, con el porqué al
// pasar el ratón, y en el turno la línea «✂ pgocode ha retirado 48k tokens del historial».
// Funciones puras: node --test src/prune.test.ts
import type { Herramienta, Mensaje, Parte } from './api';

// Vigencia: la de un intercambio o una pieza, con el porqué.
export interface Vigencia {
  vigencia: number;
  hace?: number;      // intercambios desde entonces
  relacion?: number;  // relación con lo último (similitud de embeddings)
  horas?: number;     // horas desde entonces
  superada?: string;  // «lectura superada», «orden repetida»…
  nota?: string;      // «salida recortada»
  fuera?: boolean;    // fuera del contexto del agente ahora
}

// PruneReport: el evento «poda» del chat (y la respuesta de /api/chat/vigencia).
export interface PruneReport {
  retirados: number;   // tokens que salen del historial en este turno
  repuestos?: number;  // piezas que vuelven (un intercambio que vuelve a ser relevante)
  intercambios: ({ punto: string } & Vigencia)[];
  piezas?: ({ id: string } & Vigencia)[];
}

// VIG_CUT: con esta vigencia o más, fuera del contexto del agente. VIG_MIN_OPACITY: lo más desvanecido (legible).
export const VIG_CUT = 3;
export const VIG_MIN_OPACITY = 0.4;

function vigOf(x: Vigencia): Vigencia {
  const v: Vigencia = { vigencia: x.vigencia };
  if (x.hace !== undefined) v.hace = x.hace;
  if (x.relacion !== undefined) v.relacion = x.relacion;
  if (x.horas !== undefined) v.horas = x.horas;
  if (x.superada) v.superada = x.superada;
  if (x.nota) v.nota = x.nota;
  if (x.fuera) v.fuera = true;
  return v;
}

const sameVig = (a?: Vigencia, b?: Vigencia) => JSON.stringify(a) === JSON.stringify(b);

// applyPruneReport: la vigencia de cada intercambio (en la respuesta del agente, por su punto, y en el mensaje del
// usuario que la pidió) y de cada herramienta (por su id). Lo que no sale en el informe se queda como estaba (p. ej.
// lo anterior a una compactación del agente). Lo que no cambia es el mismo objeto.
export function applyPruneReport(ms: Mensaje[], r: PruneReport): Mensaje[] {
  const por = new Map(r.intercambios.map(x => [x.punto, vigOf(x)]));
  const piezas = new Map((r.piezas || []).map(x => [x.id, vigOf(x)]));
  const out = ms.map(m => withPieces(m, piezas));
  for (let i = 0; i < ms.length; i++) {
    const m = ms[i];
    if (m.rol !== 'assistant' || !m.punto) continue;
    const v = por.get(m.punto.id);
    if (!v) continue;
    out[i] = withVig(out[i], v);
    // El mensaje del usuario de ese intercambio (el anterior que sea del usuario).
    for (let j = i - 1; j >= 0; j--) {
      if (ms[j].rol === 'user') { out[j] = withVig(out[j], v); break; }
      if (ms[j].punto) break;
    }
  }
  return out;
}

function withVig(m: Mensaje, v: Vigencia): Mensaje {
  return sameVig(m.vig, v) ? m : { ...m, vig: v };
}

function withPieces(m: Mensaje, piezas: Map<string, Vigencia>): Mensaje {
  if (!m.partes?.length || !piezas.size) return m;
  let cambia = false;
  const ps = m.partes.map((p): Parte => {
    if (p.tipo !== 'herramienta') return p;
    const v = piezas.get(p.h.id);
    if (!v || sameVig(p.h.vig, v)) return p;
    cambia = true;
    return { tipo: 'herramienta', h: { ...p.h, vig: v } as Herramienta };
  });
  return cambia ? { ...m, partes: ps } : m;
}

// vigOpacity: 1 con vigencia 0, bajando suave hasta VIG_MIN_OPACITY en VIG_CUT o más.
export function vigOpacity(v?: Vigencia): number {
  if (!v || !(v.vigencia > 0)) return 1;
  const t = Math.min(v.vigencia / VIG_CUT, 1);
  return Math.round((1 - (1 - VIG_MIN_OPACITY) * t) * 100) / 100;
}

// relativeOpacity: la de una pieza dentro de su mensaje ya desvanecido (las opacidades se multiplican).
export function relativeOpacity(piece: number, parent: number): number {
  return parent > 0 ? Math.min(1, Math.round((piece / parent) * 100) / 100) : piece;
}

const coma = (n: number, d: number) => n.toFixed(d).replace('.', ',');

function ago(h: number): string {
  if (h < 1) return `hace ${Math.max(1, Math.round(h * 60))} min`;
  if (h < 48) return `hace ${Math.round(h)} h`;
  return `hace ${Math.round(h / 24)} días`;
}

function relation(s: number): string {
  return `${s >= 0.5 ? 'mucha relación' : s >= 0.4 ? 'algo de relación' : 'poca relación'} (${coma(s, 2)})`;
}

// vigTitle: el porqué, al pasar el ratón («vigencia 2,4: hace 6 intercambios · poca relación (0,21) · hace 3 h»).
export function vigTitle(v: Vigencia): string {
  const l: string[] = [];
  if (v.hace) l.push(v.hace === 1 ? 'hace 1 intercambio' : `hace ${v.hace} intercambios`);
  if (v.relacion !== undefined) l.push(relation(v.relacion));
  if (v.horas) l.push(ago(v.horas));
  if (v.superada) l.push(v.superada);
  if (v.nota) l.push(v.nota);
  if (!l.length) l.push('reciente o instrucciones: vital');
  if (v.fuera) l.push('fuera del contexto del agente');
  return `vigencia ${v.vigencia > 0 ? coma(v.vigencia, 1) : '0'}: ${l.join(' · ')}`;
}

// prunedLine: la línea del turno («✂ pgocode ha retirado 48k tokens del historial»).
export function prunedLine(p: { tokens: number; repuestos?: number }): string {
  const k = p.tokens >= 1000 ? `${Math.round(p.tokens / 1000)}k` : String(p.tokens);
  let t = p.tokens > 0 ? `✂ pgocode ha retirado ${k} tokens del historial` : '';
  if (p.repuestos) t += (t ? '; ' : '↺ pgocode ') + (p.repuestos === 1 ? 'ha repuesto 1 pieza que vuelve a ser relevante' : `ha repuesto ${p.repuestos} piezas que vuelven a ser relevantes`);
  return t;
}
