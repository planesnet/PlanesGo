// Mensajes que no llegaron al agente: los del usuario al final de la conversación cuyo turno se cortó antes de que
// el agente respondiera nada (respuesta vacía: sin texto, partes ni consumo). Al enviar el siguiente, van con él;
// si no, el agente no los ve nunca (si el turno se cortó antes de crear su sesión, el siguiente empieza otra; y la
// reparación de OpenCode quita la pregunta sin respuesta). Funciones puras: node --test src/unanswered.test.ts
import type { Adjunto, Mensaje } from './api';

const empty = (m: Mensaje) => m.rol === 'assistant' && !m.nota && !(m.partes?.length) && !m.texto.trim() && !m.uso;

export function unanswered(ms: Mensaje[]): Mensaje[] {
  const l: Mensaje[] = [];
  let i = ms.length - 1;
  while (i >= 1 && empty(ms[i]) && ms[i - 1].rol === 'user' && !ms[i - 1].nota) {
    l.unshift(ms[i - 1]);
    i -= 2;
  }
  return l;
}

// withUnanswered: el texto y los adjuntos para el agente con los mensajes que no le llegaron delante.
export function withUnanswered(ms: Mensaje[], text: string): { text: string; attachments: Adjunto[] } {
  const p = unanswered(ms);
  return { text: [...p.map(m => m.texto.trim()), text].filter(Boolean).join('\n\n'), attachments: p.flatMap(m => m.adjuntos || []) };
}
