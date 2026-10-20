// Cliente de la API del backend Go. Toda escritura lleva X-PGO (defensa CSRF) y la cookie de sesión.

export interface Usuario { nombre: string; login: string; odoo?: string; version?: string }
export interface Many2One { id: number; nombre: string }
export interface Proyecto { id: number; nombre: string; cliente: Many2One; responsable: Many2One; carpetas: string[] }
export interface Proveedor {
  id: string; nombre: string; tipo: 'api' | 'agente'; disponible: boolean; motivo?: string;
  modelos: string[] | null; por_defecto: string; necesita_carpeta: boolean;
}
export interface Mensaje { rol: 'user' | 'assistant'; texto: string; error?: boolean }

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

export interface EventoChat { texto?: string; sesion_agente?: string; error?: string; fin?: boolean }

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
  interface Window { psf?: { escritorio: true; abrirFuera: (url: string) => Promise<void> } }
}
export const enEscritorio = () => Boolean(window.psf?.escritorio);
