// Sesiones de trabajo de un proyecto: cada una con su rama, su carpeta y su agente por defecto.
// De momento se guardan en la app (por usuario y proyecto); más adelante, en Odoo.
import type { Mensaje } from './api';
import type { Avisos } from './costes';
import type { TipoHosting } from './ajustes';
import type { PsfState } from './psf';

export interface Sesion {
  id: string;
  nombre: string;
  proyecto: number;
  carpeta: string;
  rama: string;
  agente: string;       // id del agente (claude-code, antigravity, opencode)
  modelo?: string;
  sesionAgente?: string; // sesión propia del agente (p. ej. claude --resume)
  permisos?: 'lectura' | 'editar' | 'auto' | 'plan'; // qué puede hacer el agente; «plan»: redacta un plan sin tocar nada
  modoPrevio?: 'lectura' | 'editar' | 'auto'; // el modo de antes de pasar a «plan» (con el que se ejecuta el plan)
  plan?: string;         // ruta del plan en revisión de la sesión (modo plan)
  autorizadas?: string[]; // carpetas de fuera del proyecto a las que el agente puede acceder (si no, solo la suya)
  avisos?: Avisos;        // avisos y límites de coste de la sesión (lo que falte, los del agente)
  mcp?: string[];         // servidores MCP de Claude Code además de «pgo» (por defecto ninguno: ahorran tokens y procesos)
  decideAparte?: boolean; // pgocode decide qué va a la conversación y qué aparte (por defecto, sí)
  directo?: boolean;      // pgocode hace él las órdenes mecánicas (compilar, pruebas, commit…) y responde lo que ya sabe, sin el agente (por defecto, sí; direct.ts)
  tipo?: Repo['tipo'];   // hosting que se deduce de su carpeta (respaldo)
  hosting?: TipoHosting; // alojamiento elegido para la sesión (obligatorio)
  esfuerzo?: string;     // nivel de esfuerzo del agente (vacío: el suyo por defecto)
  autoContext?: boolean; // gestión automática del contexto del agente: poda, relevo, compactación (por defecto, sí; contextPolicy.ts)
  tareaId?: number;      // tarea de Odoo a la que se imputa (sin elegir: «pgocode - Sesión IA»)
  tarea?: string;
  madre?: string;        // sesión madre de la que es hija (reparto en sesiones paralelas: reparto.ts)
  parte?: number;        // número de la parte que hace (desde 1)
  archivada?: boolean;   // archivada (p. ej. se quitó su copia de trabajo): fuera de la lista principal, con su conversación
  merged?: boolean;      // hija terminada y unida a la madre (su copia ya no existe): va plegada bajo la madre
  automatic?: boolean;  // automatic PSF session (psf.ts): works on its own, without anyone opening it
  psf?: PsfState;        // its state: issue, branch, PR, phase, iteration and verdicts
  corte?: number;        // «Continuar desde aquí» pendiente: el índice del mensaje desde el que sigue el agente (se aplica al enviar)
  creada: number;
  ultima: number;
}

export interface Repo { tipo: 'local' | 'odoo.sh' | 'autopyme.com'; repositorio?: string; remoto?: string; carpeta: string; rama?: string; ramas: string[] }

const clave = (login: string, proyecto: number) => `pgo:sesiones:${login.toLowerCase()}:${proyecto}`;
const claveConv = (id: string) => 'pgo:conv:' + id;

// sessionOrder: orden estable de la lista, las más nuevas primero (por creación). Antes era por la última actividad,
// que se apuntaba al acabar cada turno: la sesión saltaba arriba al recibir su título y se elegía la que no era.
export const sessionOrder = (a: Sesion, b: Sesion) => (b.creada || 0) - (a.creada || 0) || a.id.localeCompare(b.id);

export function lista(login: string, proyecto: number): Sesion[] {
  try { return (JSON.parse(localStorage.getItem(clave(login, proyecto)) || '[]') as Sesion[]).sort(sessionOrder); } catch { return []; }
}

function guardaLista(login: string, proyecto: number, l: Sesion[]) {
  try { localStorage.setItem(clave(login, proyecto), JSON.stringify(l)); } catch { /* sin almacenamiento */ }
}

export function guarda(login: string, s: Sesion): Sesion[] {
  const l = [s, ...lista(login, s.proyecto).filter(x => x.id !== s.id)];
  guardaLista(login, s.proyecto, l);
  return l.sort(sessionOrder);
}

export function borra(login: string, s: Sesion): Sesion[] {
  const l = lista(login, s.proyecto).filter(x => x.id !== s.id);
  guardaLista(login, s.proyecto, l);
  try { for (const k of [claveConv(s.id), 'pgo:aparte-gasto:' + s.id]) localStorage.removeItem(k); } catch { /* sin almacenamiento */ }
  return l;
}

// Una sesión nueva no tiene nombre: lo toma del primer mensaje, como en Claude Code.
export function nueva(proyecto: number, carpeta: string, rama: string, agente: string, nombre = ''): Sesion {
  const ahora = Date.now();
  return { id: ahora.toString(36) + Math.random().toString(36).slice(2, 6), nombre, proyecto, carpeta, rama, agente, creada: ahora, ultima: ahora };
}

// ordenaConHijas: la lista con cada hija justo debajo de su madre (si la madre está en la lista), en el orden de
// sus partes; el resto, como venía. «hija»: cuelga de una madre de la lista. Las hijas ya unidas (merged) van
// plegadas bajo su madre salvo que esté en «abiertas» (o sean la sesión activa): la madre lleva cuántas tiene unidas
// («unidas») y si están plegadas.
export function ordenaConHijas(l: Sesion[], abiertas: ReadonlySet<string> = new Set(), activa?: string): { s: Sesion; hija: boolean; unidas?: number; plegadas?: boolean }[] {
  const ids = new Set(l.map(s => s.id));
  const hijasDe = (m: string) => l.filter(s => s.madre === m).sort((a, b) => (a.parte || 0) - (b.parte || 0));
  const out: { s: Sesion; hija: boolean; unidas?: number; plegadas?: boolean }[] = [];
  for (const s of l) {
    if (s.madre && ids.has(s.madre)) continue; // va con su madre
    const hs = hijasDe(s.id);
    const unidas = hs.filter(h => h.merged).length;
    const plegadas = unidas > 0 && !abiertas.has(s.id);
    out.push({ s, hija: false, ...(unidas ? { unidas, plegadas } : {}) });
    for (const h of hs) if (!(plegadas && h.merged && h.id !== activa)) out.push({ s: h, hija: true });
  }
  return out;
}

// ARCHIVE_SESSIONS: evento para archivar sesiones ({proyecto, ids}); lo atiende App.tsx (que guarda la lista).
export const ARCHIVE_SESSIONS = 'pgo:archiva-sesiones';

// OPEN_SESSION: event to open a session of the open project ({detail: id}); App.tsx handles it.
export const OPEN_SESSION = 'pgo:abre-sesion';

export const titulo = (s: Sesion) => s.nombre || 'Sesión nueva';

// identificador: lo que hace falta para encontrar una sesión en los registros (tiempos/*.jsonl y depuracion.log,
// por su id de pgocode) y su conversación en el agente (por su sesión propia).
export function identificador(s: Sesion): string {
  return [`Sesión de pgocode: ${s.id}`, `Agente: ${s.agente}${s.sesionAgente ? ` · sesión del agente: ${s.sesionAgente}` : ' · sin sesión del agente todavía'}`,
    `Proyecto: ${s.proyecto} · carpeta: ${s.carpeta}${s.rama ? ` · rama: ${s.rama}` : ''}`, `Nombre: ${titulo(s)}`].join('\n');
}

// Nombre a partir del primer mensaje: la primera línea, recortada.
export function nombreDe(texto: string): string {
  const l = texto.trim().split('\n')[0].replace(/\s+/g, ' ');
  return l.length > 60 ? l.slice(0, 57).trimEnd() + '…' : l;
}

export function conversacion(id: string): Mensaje[] {
  try { return JSON.parse(localStorage.getItem(claveConv(id)) || '[]') as Mensaje[]; } catch { return []; }
}

// guardaConversacion: lo de las herramientas de los mensajes antiguos se recorta (es lo que más ocupa y ya se ha
// leído); si aun así no cabe, se recorta más. Si no cabe de ninguna manera, se avisa (no se pierde en silencio).
export function guardaConversacion(id: string, m: Mensaje[]) {
  for (const [recientes, max] of [[10, 1500], [2, 300]] as const) {
    try { localStorage.setItem(claveConv(id), JSON.stringify(compacta(m, recientes, max))); return; } catch { /* lleno: más corto */ }
  }
  window.dispatchEvent(new CustomEvent('pgo:almacen-lleno', { detail: id }));
}

// compacta: los mensajes salvo los «recientes» últimos, con la entrada y la salida de sus herramientas y su
// razonamiento recortados a «max» caracteres.
export function compacta(m: Mensaje[], recientes: number, max: number): Mensaje[] {
  const corta = (t: string | undefined, n: number) => (t && t.length > n ? t.slice(0, n) + '…' : t);
  return m.map((x, i) => i >= m.length - recientes || !x.partes ? x : {
    ...x,
    partes: x.partes.map(p => p.tipo === 'herramienta' ? { ...p, h: { ...p.h, entrada: corta(p.h.entrada, max / 3), salida: corta(p.h.salida, max) } }
      : p.tipo === 'pensamiento' ? { ...p, texto: corta(p.texto, max)! } : p),
  });
}

// onlySession: al entrar en un proyecto sin pedir ninguna sesión, la única que tiene (sin contar las archivadas ni las
// hijas de un reparto); null si hay varias o ninguna, o si se pide crear una nueva.
export function onlySession(l: Sesion[], pedida: string): Sesion | null {
  if (pedida) return null;
  const vivas = l.filter(s => !s.archivada && !s.madre);
  return vivas.length === 1 ? vivas[0] : null;
}
