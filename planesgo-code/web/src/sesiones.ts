// Sesiones de trabajo de un proyecto: cada una con su rama, su carpeta y su agente por defecto.
// De momento se guardan en la app (por usuario y proyecto); más adelante, en Odoo.
import type { Mensaje } from './api';

export interface Sesion {
  id: string;
  nombre: string;
  proyecto: number;
  carpeta: string;
  rama: string;
  agente: string;       // id del proveedor (claude-code, opencode, gemini-cli, claude, openai, gemini…)
  modelo?: string;
  esfuerzo?: string;     // nivel de esfuerzo del agente (vacío: el suyo por defecto)
  sesionAgente?: string; // sesión propia del agente (p. ej. claude --resume)
  permisos?: 'lectura' | 'editar' | 'auto'; // qué puede hacer un agente (Claude Code) en la sesión
  tipo?: Repo['tipo'];   // hosting de su carpeta (para el icono de la lista)
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
  try { localStorage.removeItem(claveConv(s.id)); } catch { /* sin almacenamiento */ }
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

export function guardaConversacion(id: string, m: Mensaje[]) {
  try { localStorage.setItem(claveConv(id), JSON.stringify(m)); } catch { /* lleno: se queda en memoria */ }
}
