import type { Tarea } from './tareas';
import type { DirectNote } from './direct';
import type { Copia } from './copias';
import type { Propuesta } from './reparto';
import type { PruneReport, Vigencia } from './prune';
import type { RouteStamp } from './router';
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
export interface Herramienta { id: string; nombre: string; resumen?: string; entrada?: string; salida?: string; estado: 'curso' | 'ok' | 'error'; skill?: string; padre?: string; capturas?: string[]; tokens?: number; tamano?: number; vig?: Vigencia } // tokens: los de la conversación hasta esta acción; vig: su vigencia (prune.ts)
export interface Uso { entrada: number; salida: number; coste?: number; segundos?: number; contexto?: number; contexto_cache?: number; contexto_escrita?: number } // contexto: tokens de la última llamada; de ellos, contexto_cache leídos de caché y contexto_escrita escritos en ella
export type Parte = { tipo: 'texto'; texto: string } | { tipo: 'pensamiento'; texto: string } | { tipo: 'herramienta'; h: Herramienta };
export interface SkillPgo { nombre: string; puntuacion: number; modo: 'sugerida' | 'elegida' }
export interface Adjunto { nombre: string; ruta: string; tamano: number; tipo?: string }
export interface Mensaje { rol: 'user' | 'assistant'; texto: string; error?: boolean; partes?: Parte[]; uso?: Uso; skill?: SkillPgo; adjuntos?: Adjunto[]; tareas?: Tarea[]; plan?: { ruta: string; titulo: string; estado?: 'borrador' | 'ejecutado' | 'descartado' | 'borrado'; numero?: number }; nota?: boolean; reparto?: boolean; inyectados?: string[]; fin?: number; vivo?: Uso; relevo?: Relevo; punto?: { sesion: string; id: string }; fuera?: boolean; vig?: Vigencia; pruned?: { tokens: number; repuestos?: number }; directo?: DirectNote; ruta?: RouteStamp } // ruta: la decisión del enrutador para un mensaje del usuario (router.ts); directo: nota de una orden que ha hecho pgocode sin el agente (direct.ts); fin: cuándo acabó el turno (ms); vivo: lo consumido en el turno mientras trabaja; punto: el último mensaje del agente en su sesión (para «Continuar desde aquí»); fuera: el agente ya no lo tiene en su contexto; vig: la vigencia del intercambio en el contexto del agente (prune.ts); pruned: lo que retiró al empezar este turno
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

export const post = <T,>(ruta: string, cuerpo: unknown, signal?: AbortSignal) =>
  fetch(ruta, {
    method: 'POST', credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', 'X-PGO': '1' },
    body: JSON.stringify(cuerpo), signal,
  }).then(r => respuesta<T>(r));

export interface EventoChat { texto?: string; pensamiento?: string; herramienta?: Herramienta; tarea?: Tarea; uso?: Uso; vivo?: Uso; sesion_agente?: string; punto?: string; error?: string; fin?: boolean; poda?: PruneReport; via?: string; subagentes?: string; memoria?: { tokens: number; motivo: string } } // via: quién respondió una pregunta aparte (copia, modelo, local/<modelo>); subagentes: pgocode los ha sugerido (el porqué); memoria: el mensaje lleva la memoria del proyecto (sus tokens y por qué)

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

// Reparto en sesiones paralelas (reparto.ts): proponerlo, crear la copia de una hija y unir su rama a la de la madre.
export const proponeReparto = (proyecto: number, carpeta: string, rama: string, texto: string, permisos: string) =>
  post<Propuesta>('/api/reparto/propone', { proyecto, carpeta, rama, texto, permisos });
// COPIES_CHANGED: aviso de que han cambiado las copias de trabajo (Actividad → Copias de trabajo se recarga).
export const COPIES_CHANGED = 'pgo:copias';
const avisaCopias = <T,>(r: T) => { window.dispatchEvent(new Event(COPIES_CHANGED)); return r; };
export const preparaHija = (proyecto: number, sesion: string, carpeta: string, n: number, hija: string) =>
  post<{ carpeta: string; rama: string; rama_madre: string }>('/api/reparto/prepara', { proyecto, sesion, carpeta, n, hija }).then(avisaCopias);
export interface UnionHija { estado: 'unida' | 'sin_cambios' | 'conflicto' | 'pendiente' | 'error'; motivo?: string; ficheros?: string[]; commit?: boolean; quitada: boolean; rama_borrada: boolean; aviso?: string }
// parte: el texto de la parte (para el mensaje de sus commits: «pgocode: parte 2 — añade el cupón VERANO»).
export const uneHija = (proyecto: number, madre: string, ramaMadre: string, hija: string, rama: string, n: number, parte = '') =>
  post<UnionHija>('/api/reparto/une', { proyecto, madre, rama_madre: ramaMadre, hija, rama, n, parte }).then(avisaCopias);
