// Fotos de la conversación: las capturas que devuelven las herramientas (guardadas por pgocode como evidencias) y
// las imágenes que el agente pone en sus respuestas (Markdown «![…](url)»). Para la tira del chat (de la más
// reciente a la más antigua) y el visor. Funciones puras: node --test src/fotos.test.ts
import type { Mensaje } from './api';

export interface Foto { src: string; titulo?: string }

// visible: lo que el navegador puede cargar (no rutas locales del equipo ni relativas a otra web).
export const visible = (src: string) => /^(https?:\/\/|data:image\/)/i.test(src.trim());

// imagenesMarkdown: las imágenes de un texto Markdown, en orden.
export function imagenesMarkdown(md: string): Foto[] {
  const l: Foto[] = [];
  for (const m of md.matchAll(/!\[([^\]]*)\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)/g)) {
    if (visible(m[2])) l.push({ src: m[2], titulo: m[1] || undefined });
  }
  return l;
}

// fotosDe: las de toda la conversación, de la más reciente a la más antigua y sin repetir (cuenta la última vez
// que aparece). url: la de una captura guardada (su ruta en la carpeta de la sesión).
export function fotosDe(mensajes: Mensaje[], url: (ruta: string) => string): Foto[] {
  const orden: Foto[] = [];
  for (const m of mensajes) {
    if (m.rol !== 'assistant') continue;
    const partes = m.partes?.length ? m.partes : [{ tipo: 'texto' as const, texto: m.texto }];
    for (const p of partes) {
      if (p.tipo === 'texto') orden.push(...imagenesMarkdown(p.texto));
      else if (p.tipo === 'herramienta') for (const r of p.h.capturas || []) orden.push({ src: url(r), titulo: p.h.nombre + (p.h.resumen ? ' · ' + p.h.resumen : '') });
    }
  }
  const vistas = new Set<string>();
  const l: Foto[] = [];
  for (let i = orden.length - 1; i >= 0; i--) {
    if (vistas.has(orden[i].src)) continue;
    vistas.add(orden[i].src);
    l.push(orden[i]);
  }
  return l;
}
