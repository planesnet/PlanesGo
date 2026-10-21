// Sesiones de trabajo de un proyecto: cada una con su rama, su carpeta y su agente por defecto.
// De momento se guardan en la app (por usuario y proyecto); más adelante, en Odoo.
import type { Mensaje } from './api';
import type { Avisos } from './costes';
import type { TipoHosting } from './ajustes';

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
  tipo?: Repo['tipo'];   // hosting que se deduce de su carpeta (respaldo)
  hosting?: TipoHosting; // alojamiento elegido para la sesión (obligatorio)
  esfuerzo?: string;     // nivel de esfuerzo del agente (vacío: el suyo por defecto)
  tareaId?: number;      // tarea de Odoo a la que se imputa (sin elegir: «pgocode - Sesión IA»)
  tarea?: string;
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

export const titulo = (s: Sesion) => s.nombre || 'Sesión nueva';

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
