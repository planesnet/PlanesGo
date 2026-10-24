// Reparto de una petición en sesiones paralelas (hijas): cada parte independiente de la petición la hace una sesión
// propia en su propia copia de trabajo del repositorio (git worktree), y al acabar se unen sus ramas a la de la
// sesión madre. Aquí, la lógica pura (propuesta, cuenta atrás, cola de hijas, mensaje de cada una, resumen);
// Chat.tsx la usa y el backend (internal/servidor/reparto.go) hace lo de git.
import type { Mensaje } from './api';

export const MAX_HIJAS = 4;

// Lo que contesta el servidor a /api/reparto/propone.
export interface Propuesta {
  partes?: string[]; intro?: string; outro?: string; rama_madre?: string; madre_sucia?: boolean; max: number; espera_s: number; motivo?: string;
}

export type EstadoHija = 'espera' | 'trabajando' | 'hecha' | 'parada' | 'error';
export type EstadoUnion = 'sin' | 'unida' | 'sin_cambios' | 'conflicto' | 'pendiente' | 'error';

export interface Hija {
  n: number;            // número de la parte, desde 1
  idx: number;          // número de la hija entre todas las de su madre (da nombre a su rama y su sesión: no se repite entre repartos)
  parte: string;        // el texto de la parte
  sesion: string;       // id de su sesión de pgocode
  estado: EstadoHija;
  union: EstadoUnion;
  carpeta?: string;     // su copia de trabajo
  rama?: string;        // su rama (pgo/<madre>-<n>)
  motivo?: string;      // por qué está pendiente o ha fallado
  ficheros?: string[];  // los que chocan
  aviso?: string;       // la copia o la rama se conservan
  quitada?: boolean;    // su copia ya se ha quitado
  resolving?: boolean;  // en conflicto: se le ha pedido al agente de la madre que una la rama y resuelva el choque
}

export interface Reparto {
  id: string;
  madre: string;        // id de la sesión madre
  proyecto: number;
  carpeta: string;      // la copia de trabajo de la madre
  ramaMadre: string;
  intro: string; outro: string;
  hijas: Hija[];
  creado: number;
  resumido?: boolean;   // el resumen final ya está en el chat de la madre
}

// Solo se pregunta al servidor por peticiones que parecen listar tres o más cosas, en una sesión que edita.
const LISTA = /^\s*(?:\d{1,2}[.)]|[-*•])\s+\S/;
export function candidata(texto: string, permisos: string | undefined): boolean {
  if (permisos !== 'editar' && permisos !== 'auto') return false;
  const t = texto.trim();
  if (!t || t.startsWith('/') || t.split(/\s+/).length < 9) return false;
  return t.split('\n').filter(l => LISTA.test(l)).length >= 3;
}

// Segundos que faltan para repartir solo (nunca negativo).
export const restante = (hasta: number, ahora: number) => Math.max(0, Math.ceil((hasta - ahora) / 1000));

const recorta = (t: string, n: number) => { const s = t.replace(/\s+/g, ' ').trim(); return s.length > n ? s.slice(0, n - 1).trimEnd() + '…' : s; };

// Nombre de la sesión de una hija.
export const nombreHija = (n: number, parte: string) => `Parte ${n}: ${recorta(parte, 50)}`;

// El mensaje de la hija: su parte, con el encargo común si lo había y una línea con las demás (cortas) para que no
// toque lo de ellas. Corto: no cuesta tokens extra a cada mensaje.
export function mensajeHija(r: { intro: string; outro: string; partes: string[] }, n: number): string {
  const otras = r.partes.map((p, i) => [i + 1, p] as const).filter(([i]) => i !== n).map(([i, p]) => `${i}) ${recorta(p, 60)}`);
  const l: string[] = [];
  if (r.intro) l.push(recorta(r.intro, 400));
  l.push(r.partes[n - 1]);
  if (r.outro) l.push(recorta(r.outro, 400));
  l.push(`(Es una de ${r.partes.length} partes independientes de esta petición. Las otras, que hacen otras sesiones en sus subcarpetas: ${otras.join('; ')}. Haz solo la tuya y no toques lo de las demás.)`);
  return l.join('\n\n');
}

// base: cuántas hijas ha tenido ya la madre en repartos anteriores (la rama «pgo/<madre>-<idx>» no se puede repetir).
export function nuevo(id: string, madre: { id: string; proyecto: number; carpeta: string }, p: Propuesta, ahora: number, idHija: (idx: number) => string, base = 0): Reparto {
  const partes = p.partes || [];
  return {
    id, madre: madre.id, proyecto: madre.proyecto, carpeta: madre.carpeta, ramaMadre: p.rama_madre || '', intro: p.intro || '', outro: p.outro || '', creado: ahora,
    hijas: partes.map((parte, i) => ({ n: i + 1, idx: base + i + 1, parte, sesion: idHija(base + i + 1), estado: 'espera' as const, union: 'sin' as const })),
  };
}

export const activas = (r: Reparto) => r.hijas.filter(h => h.estado === 'trabajando').length;

// Las hijas que pueden empezar ya: las primeras en espera hasta completar el máximo a la vez.
export function aLanzar(r: Reparto, max = MAX_HIJAS): Hija[] {
  const libres = Math.max(0, max - activas(r));
  return r.hijas.filter(h => h.estado === 'espera').slice(0, libres);
}

export function pon(r: Reparto, n: number, cambios: Partial<Hija>): Reparto {
  return { ...r, hijas: r.hijas.map(h => (h.n === n ? { ...h, ...cambios } : h)) };
}

// Las hijas que ha tenido una madre en los repartos guardados (para numerar las nuevas a continuación).
export const hijasDe = (l: Reparto[], madre: string) => Math.max(0, ...l.filter(r => r.madre === madre).flatMap(r => r.hijas.map(h => h.idx || h.n)));

// Una hija que ha acabado bien y aún no se ha unido (o quedó pendiente): se puede unir.
// En conflicto, después de pedírselo al agente de la madre (al volver a unir, si ya lo ha unido él, sale «sin cambios»).
export const seUne = (h: Hija) => (h.estado === 'hecha' || h.estado === 'parada') && !h.quitada && (h.union === 'sin' || h.union === 'pendiente' || h.union === 'error' || (h.union === 'conflicto' && Boolean(h.resolving)));

// Todas las hijas han acabado (o se pararon) y las que acabaron bien tienen su unión resuelta o a la espera del usuario.
export const acabado = (r: Reparto) => r.hijas.every(h => h.estado === 'hecha' || h.estado === 'parada' || h.estado === 'error');

// Hay algo que el usuario puede hacer: unir a mano lo pendiente.
export const hayPendientes = (r: Reparto) => r.hijas.some(h => seUne(h) && h.estado === 'hecha' && h.union !== 'sin') || r.hijas.some(h => h.estado === 'parada' && seUne(h));

// Al abrir la app: las hijas que figuraban trabajando ya no tienen turno (se cerró la app): quedan paradas, con su
// rama y su copia para unir a mano.
export function alCargar(l: Reparto[], conTurno: (sesion: string) => boolean): Reparto[] {
  return l.map(r => ({ ...r, hijas: r.hijas.map(h => (h.estado === 'trabajando' || h.estado === 'espera') && !conTurno(h.sesion) ? { ...h, estado: 'parada' as const } : h) }));
}

// El estado de una hija según cómo acabó su turno: lo último que dijo (con error o no) y si se paró.
export function estadoAlAcabar(parada: boolean, ultimo: Mensaje | undefined): EstadoHija {
  if (parada) return 'parada';
  if (!ultimo || ultimo.rol !== 'assistant' || ultimo.error) return 'error';
  return 'hecha';
}

export const NOMBRE_UNION: Record<EstadoUnion, string> = {
  sin: 'sin unir', unida: 'unida', sin_cambios: 'sin cambios', conflicto: 'en conflicto', pendiente: 'pendiente de unir', error: 'no se ha podido unir',
};
export const NOMBRE_HIJA: Record<EstadoHija, string> = { espera: 'en cola', trabajando: 'trabajando', hecha: 'hecha', parada: 'parada', error: 'con error' };

export interface DatosHija { respuesta: string; entrada: number; salida: number; coste: number }

// La última respuesta del agente en una conversación, para el resumen (sin avisos de error). Sus bloques de texto
// (separados por herramientas) van con un salto: en el texto seguido del mensaje quedan pegados («…su estilo.Sigo…»).
export function ultimaRespuesta(ms: Mensaje[]): string {
  for (let i = ms.length - 1; i >= 0; i--) {
    const m = ms[i];
    if (m.rol !== 'assistant' || m.nota) continue;
    const bloques = (m.partes || []).flatMap(p => (p.tipo === 'texto' && p.texto.trim() ? [p.texto.trim()] : []));
    const t = bloques.length ? bloques.join('\n\n') : m.texto.trim();
    if (t) return t;
  }
  return '';
}

const tokens = (n: number) => n >= 1_000_000 ? (n / 1_000_000).toFixed(1).replace('.', ',') + ' M' : n >= 1000 ? Math.round(n / 1000) + ' k' : String(n);
const dinero = (x: number) => (x < 0.01 && x > 0 ? '<0,01' : x.toFixed(2).replace('.', ',')) + ' $';

// La unión de una hija, en una frase.
export function fraseUnion(h: Hija): string {
  // Sin rama: la copia no se llegó a crear (no hay nada que unir ni que conservar).
  if (!h.rama && !h.carpeta && h.union === 'sin') return h.estado === 'error' ? `no empezó: ${h.motivo || 'no se pudo crear su copia del repositorio'}` : 'sin copia: nada que unir';
  switch (h.union) {
    case 'unida': return 'unida' + (h.quitada ? ' (subcarpeta quitada)' : h.aviso ? ` (${h.aviso})` : '');
    case 'sin_cambios': return 'sin cambios que unir' + (h.quitada ? ' (subcarpeta quitada)' : '');
    case 'conflicto': return `NO unida: choca en ${(h.ficheros || []).join(', ') || 'algún fichero'}. Se conserva la rama «${h.rama}» (subcarpeta en ${h.carpeta})`;
    case 'pendiente': return `pendiente de unir (${h.motivo || 'la madre no está lista'}): botón «Unir cambios»`;
    case 'error': return `NO unida: ${h.motivo || 'error'}. Se conserva la rama «${h.rama}»`;
    default: return h.estado === 'parada' ? `parada: sin unir. Se conserva la rama «${h.rama}» (subcarpeta en ${h.carpeta})` : h.estado === 'error' ? `con error: sin unir. Se conserva la rama «${h.rama}»` : 'sin unir';
  }
}

// El resumen para el chat de la madre: qué hizo cada parte (su última respuesta, recortada), cuánto costó cada una y
// en total, y qué se unió y qué no.
export function resumen(r: Reparto, datos: Record<string, DatosHija>): string {
  const l: string[] = [`**Reparto en ${r.hijas.length} sesiones paralelas: resumen**`, ''];
  let ent = 0, sal = 0, coste = 0;
  for (const h of r.hijas) {
    const d = datos[h.sesion];
    // Una viñeta por parte; sus líneas con salto duro (dos espacios) para que no se junten.
    const x = [`- **Parte ${h.n}** (${recorta(h.parte, 80)}) · ${NOMBRE_HIJA[h.estado]}`];
    if (d?.respuesta) x.push('  Respondió: «' + recorta(d.respuesta, 400) + '»');
    if (d) {
      ent += d.entrada; sal += d.salida; coste += d.coste;
      x.push(`  ${tokens(d.entrada + d.salida)} tokens${d.coste ? ' · ' + dinero(d.coste) : ''}`);
    }
    x.push(`  Unión: ${fraseUnion(h)}`);
    l.push(x.join('  \n'));
  }
  l.push('', `**Total:** ${tokens(ent + sal)} tokens${coste ? ' · ' + dinero(coste) : ''}`);
  const unidas = r.hijas.filter(h => h.union === 'unida' || h.union === 'sin_cambios').length;
  l.push(`**Unidas:** ${unidas} de ${r.hijas.length}` + (unidas < r.hijas.length ? ` · sin unir: ${r.hijas.filter(h => h.union !== 'unida' && h.union !== 'sin_cambios').map(h => h.n).join(', ')}` : ''));
  return l.join('\n');
}

// La unión hecha a mano (botón «Unir cambios»), en una frase para el chat de la madre.
export const fraseManual = (h: Hija) => `Parte ${h.n}: ${fraseUnion(h)}.`;

// El mensaje para que el agente de la sesión madre una la rama de una parte en conflicto y resuelva el choque.
export function mensajeResolver(h: Hija, ramaMadre: string): string {
  const f = (h.ficheros || []).map(x => '`' + x + '`').join(', ');
  return `Une la rama \`${h.rama}\` (parte ${h.n} del reparto: ${recorta(h.parte, 120)}) a la rama actual${ramaMadre ? ` (\`${ramaMadre}\`)` : ''} con \`git merge ${h.rama}\` y resuelve los conflictos${f ? ` en ${f}` : ''} conservando lo de las dos partes. Después comprueba que compila y pasan las pruebas, y haz el commit de la unión.`;
}

// Aviso inmediato en el chat de la madre por un conflicto.
export function avisoConflicto(h: Hija): string {
  return `La parte ${h.n} choca con lo ya unido en: ${(h.ficheros || []).join(', ')}. No se ha unido: se conserva la rama «${h.rama}» en ${h.carpeta}. El resto sigue.`;
}

// Persistencia (todos los repartos de la app en una clave; los terminados más antiguos se descartan).
const CLAVE = 'pgo:repartos';
export function lee(): Reparto[] {
  try { const l = JSON.parse(localStorage.getItem(CLAVE) || '[]'); return Array.isArray(l) ? l as Reparto[] : []; } catch { return []; }
}
export function guarda(l: Reparto[]) {
  try { localStorage.setItem(CLAVE, JSON.stringify(l.sort((a, b) => b.creado - a.creado).slice(0, 30))); } catch { /* sin almacenamiento */ }
}
