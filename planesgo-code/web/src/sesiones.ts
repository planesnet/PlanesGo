// Sesiones de trabajo de un proyecto: cada una con su rama, su carpeta y su agente por defecto.
// De momento se guardan en la app (por usuario y proyecto); más adelante, en Odoo.
import type { Mensaje } from './api';
import type { Avisos } from './costes';
import type { TipoHosting } from './ajustes';
import type { RelaySetting } from './workingMemory';

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
  renovarContexto?: RelaySetting; // renovar el contexto del agente con la memoria de trabajo (por defecto, automático)
  tipo?: Repo['tipo'];   // hosting que se deduce de su carpeta (respaldo)
  hosting?: TipoHosting; // alojamiento elegido para la sesión (obligatorio)
  esfuerzo?: string;     // nivel de esfuerzo del agente (vacío: el suyo por defecto)
  tareaId?: number;      // tarea de Odoo a la que se imputa (sin elegir: «pgocode - Sesión IA»)
  tarea?: string;
  madre?: string;        // sesión de la que sale (sesión hija: otro tema, con lo mínimo)
  creada: number;
  ultima: number;
}

export interface Repo { tipo: 'local' | 'odoo.sh' | 'autopyme.com'; repositorio?: string; remoto?: string; carpeta: string; rama?: string; ramas: string[] }

const clave = (login: string, proyecto: number) => `pgo:sesiones:${login.toLowerCase()}:${proyecto}`;
const claveConv = (id: string) => 'pgo:conv:' + id;

export function lista(login: string, proyecto: number): Sesion[] {
  try { return (JSON.parse(localStorage.getItem(clave(login, proyecto)) || '[]') as Sesion[]).sort((a, b) => b.ultima - a.ultima); } catch { return []; }
}

function guardaLista(login: string, proyecto: number, l: Sesion[]) {
  try { localStorage.setItem(clave(login, proyecto), JSON.stringify(l)); } catch { /* sin almacenamiento */ }
}

export function guarda(login: string, s: Sesion): Sesion[] {
  const l = [s, ...lista(login, s.proyecto).filter(x => x.id !== s.id)];
  guardaLista(login, s.proyecto, l);
  return l.sort((a, b) => b.ultima - a.ultima);
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

// childOf: una sesión hija, con los mismos parámetros que la madre pero sin su historial (ni la sesión del agente,
// ni su plan en revisión), enlazada a ella.
export function childOf(m: Sesion, nombre: string): Sesion {
  const { id: _i, sesionAgente: _a, plan: _p, creada: _c, ultima: _u, nombre: _n, madre: _m, ...params } = m;
  const n = nueva(m.proyecto, m.carpeta, m.rama, m.agente, nombre);
  return { ...params, ...n, madre: m.id };
}

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
