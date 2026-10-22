import type { Tarea } from './tareas';
import type { Copia } from './copias';
// Cliente de la API del backend Go. Toda escritura lleva X-PGO (defensa CSRF) y la cookie de sesión.

export interface Usuario { nombre: string; login: string; odoo?: string; version?: string }
export interface Many2One { id: number; nombre: string }
export interface Proyecto { id: number; nombre: string; cliente: Many2One; responsable: Many2One; carpetas: string[] }
export interface Proveedor {
  id: string; nombre: string; tipo: 'api' | 'agente'; disponible: boolean; motivo?: string;
  modelos: string[] | null; por_defecto: string; necesita_carpeta: boolean;
  esfuerzos?: string[]; modos?: boolean; inyecta?: boolean; // inyecta: admite mensajes durante el turno
}
// Lo que hace el agente, igual para todos (Claude Code, Antigravity, OpenCode): texto, razonamiento y herramientas.
export interface Herramienta { id: string; nombre: string; resumen?: string; entrada?: string; salida?: string; estado: 'curso' | 'ok' | 'error'; skill?: string; padre?: string; capturas?: string[]; tokens?: number } // tokens: los de la conversación hasta esta acción
export interface Uso { entrada: number; salida: number; coste?: number; segundos?: number; contexto?: number } // contexto: tokens de la última llamada
export type Parte = { tipo: 'texto'; texto: string } | { tipo: 'pensamiento'; texto: string } | { tipo: 'herramienta'; h: Herramienta };
export interface SkillPgo { nombre: string; puntuacion: number; modo: 'sugerida' | 'elegida' }
export interface Adjunto { nombre: string; ruta: string; tamano: number; tipo?: string }
export interface Mensaje { rol: 'user' | 'assistant'; texto: string; error?: boolean; partes?: Parte[]; uso?: Uso; skill?: SkillPgo; adjuntos?: Adjunto[]; tareas?: Tarea[]; plan?: { ruta: string; titulo: string; estado?: 'borrador' | 'ejecutado' | 'descartado' | 'borrado'; numero?: number }; nota?: boolean; inyectados?: string[]; fin?: number; vivo?: Uso; relevo?: Relevo } // fin: cuándo acabó el turno (ms); vivo: lo consumido en el turno mientras trabaja
// Relevo: el mensaje fue a una sesión nueva del agente con la memoria de trabajo (de: tokens de contexto que tenía la
// sesión anterior; a: los de la memoria, estimados, y al acabar el turno los del contexto real de la sesión nueva);
// anterior: la sesión del agente de antes (para deshacerlo).
export interface Relevo { de: number; a: number; anterior: string; motivo: string }

// Sube un fichero adjunto a la carpeta de la sesión (para pasárselo al agente).
export async function subeAdjunto(proyecto: number, carpeta: string, sesion: string, f: File): Promise<Adjunto> {
  const d = new FormData();
  d.append('fichero', f, f.name || 'adjunto');
  const r = await fetch(`/api/adjunto?proyecto=${proyecto}&carpeta=${encodeURIComponent(carpeta)}&sesion=${encodeURIComponent(sesion)}`,
    { method: 'POST', credentials: 'same-origin', headers: { 'X-PGO': '1' }, body: d });
  return respuesta<Adjunto>(r);
}

export interface Candidato { nombre: string; ruta: string; puntuacion: number }
export interface Eleccion { candidatos: Candidato[]; elegida?: Candidato; umbral: number; ms: number; motor: string }

export class ErrorAPI extends Error {
  constructor(public estado: number, mensaje: string) { super(mensaje); }
}

async function respuesta<T>(r: Response): Promise<T> {
  if (r.ok) return r.json() as Promise<T>;
  let msg = r.statusText;
  try {
    const t = (await r.text()).trim();
    try { msg = JSON.parse(t).error || msg; } catch { if (t) msg = t; } // errores de texto (p. ej. del estudio)
  } catch { /* sin cuerpo */ }
  if (r.status === 401) window.dispatchEvent(new CustomEvent('pgo:sin-sesion', { detail: msg }));
  throw new ErrorAPI(r.status, msg);
}

export const get = <T,>(ruta: string) => fetch(ruta, { credentials: 'same-origin' }).then(r => respuesta<T>(r));

export const post = <T,>(ruta: string, cuerpo: unknown) =>
  fetch(ruta, {
    method: 'POST', credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', 'X-PGO': '1' },
    body: JSON.stringify(cuerpo),
  }).then(r => respuesta<T>(r));

export interface EventoChat { texto?: string; pensamiento?: string; herramienta?: Herramienta; tarea?: Tarea; uso?: Uso; vivo?: Uso; sesion_agente?: string; error?: string; fin?: boolean }

// Avisos del backend: cambios en los programas y URL que abrir en el navegador integrado.
export type Aviso = { tipo: 'procesos' } | { tipo: 'imputacion' } | { tipo: 'abrir'; url: string; proyecto: number; proceso?: string };
export function escuchaAvisos(cada: (a: Aviso) => void): () => void {
  const es = new EventSource('/api/eventos');
  es.onmessage = e => { try { cada(JSON.parse(e.data)); } catch { /* aviso incompleto */ } };
  return () => es.close();
}

export interface Proceso {
  nombre: string; proyecto: number; carpeta: string; orden: string; url?: string;
  estado: 'curso' | 'terminado' | 'error' | 'parado'; codigo: number; inicio: string; fin?: string; lanzado: 'usuario' | 'agente';
}
export interface ConfigPrograma { nombre: string; orden: string; url?: string }

// chat envía la conversación y va llamando a cada con los eventos SSE de la respuesta.
export async function chat(cuerpo: unknown, cada: (e: EventoChat) => void, señal: AbortSignal) {
  const r = await fetch('/api/chat', {
    method: 'POST', credentials: 'same-origin', signal: señal,
    headers: { 'Content-Type': 'application/json', 'X-PGO': '1' },
    body: JSON.stringify(cuerpo),
  });
  if (!r.ok || !r.body) await respuesta(r);
  const lector = r.body!.getReader();
  const dec = new TextDecoder();
  let buf = '';
  for (;;) {
    const { value, done } = await lector.read();
    if (done) break;
    buf += dec.decode(value, { stream: true });
    let i;
    while ((i = buf.indexOf('\n\n')) >= 0) {
      const bloque = buf.slice(0, i);
      buf = buf.slice(i + 2);
      for (const l of bloque.split('\n')) {
        if (l.startsWith('data: ')) {
          try { cada(JSON.parse(l.slice(6))); } catch { /* línea incompleta */ }
        }
      }
    }
  }
}

// Puente con Electron (preload): solo existe dentro de la app de escritorio.
declare global {
  interface Window { psf?: { escritorio: true; abrirFuera: (url: string) => Promise<void>; elegirCarpeta?: (inicial?: string) => Promise<string | null>; alActividad?: (cb: () => void) => () => void } }
}
export const enEscritorio = () => Boolean(window.psf?.escritorio);

// Copias de trabajo (git worktree) del proyecto y quitar una sin perder nada.
export const listaCopias = (proyecto: number) => get<{ copias: Copia[] }>(`/api/proyecto/copias?proyecto=${proyecto}`).then(r => r.copias);
export const quitaCopia = (proyecto: number, carpeta: string, borrarRama: boolean) =>
  post<{ ok: boolean; rama_borrada: boolean; aviso?: string }>('/api/proyecto/quitar-copia', { proyecto, carpeta, borrar_rama: borrarRama });
