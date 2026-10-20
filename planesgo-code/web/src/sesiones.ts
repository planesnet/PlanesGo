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
  sesionAgente?: string; // sesión propia del agente (p. ej. claude --resume)
  permisos?: 'lectura' | 'editar' | 'auto'; // qué puede hacer un agente (Claude Code) en la sesión
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

export function nueva(proyecto: number, carpeta: string, rama: string, agente: string, nombre?: string): Sesion {
  const ahora = Date.now();
  return { id: ahora.toString(36) + Math.random().toString(36).slice(2, 6), nombre: nombre || (rama ? 'Sesión en ' + rama : 'Sesión'),
    proyecto, carpeta, rama, agente, creada: ahora, ultima: ahora };
}

export function conversacion(id: string): Mensaje[] {
  try { return JSON.parse(localStorage.getItem(claveConv(id)) || '[]') as Mensaje[]; } catch { return []; }
}

export function guardaConversacion(id: string, m: Mensaje[]) {
  try { localStorage.setItem(claveConv(id), JSON.stringify(m)); } catch { /* lleno: se queda en memoria */ }
}
