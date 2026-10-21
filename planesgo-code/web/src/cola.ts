// Cola de mensajes del chat (como en Claude Code): lo que se escribe mientras el agente trabaja espera su turno.
// Una cola por sesión. Si un turno se para o falla, la cola se detiene hasta que se envíe algo a mano o se pida
// seguir. Funciones puras (sin React) para poder probarlas: node --test src/cola.test.ts

export interface EnCola<A = unknown> { texto: string; adjuntos: A[] }
export interface Cola<A = unknown> { mensajes: EnCola<A>[]; parada: boolean }

export const vacia = <A>(): Cola<A> => ({ mensajes: [], parada: false });

// encola: añade al final; un texto vacío no se encola.
export function encola<A>(c: Cola<A>, texto: string, adjuntos: A[] = []): Cola<A> {
  const t = texto.trim();
  if (!t) return c;
  return { ...c, mensajes: [...c.mensajes, { texto: t, adjuntos }] };
}

export function quita<A>(c: Cola<A>, i: number): Cola<A> {
  return { ...c, mensajes: c.mensajes.filter((_, j) => j !== i) };
}

// siguiente: con el agente libre y la cola en marcha, el mensaje que toca enviar y la cola sin él.
export function siguiente<A>(c: Cola<A>, ocupado: boolean): { enviar?: EnCola<A>; cola: Cola<A> } {
  if (ocupado || c.parada || c.mensajes.length === 0) return { cola: c };
  const [primero, ...resto] = c.mensajes;
  return { enviar: primero, cola: { ...c, mensajes: resto } };
}

// Al empezar un turno (a mano o desde la cola) la cola vuelve a estar en marcha; si falla o se para, se detiene.
export const empieza = <A>(c: Cola<A>): Cola<A> => (c.parada ? { ...c, parada: false } : c);
export const falla = <A>(c: Cola<A>): Cola<A> => (c.parada ? c : { ...c, parada: true });

// devuelve: al parar o al editar, los mensajes vuelven al cuadro de texto (antes de lo ya escrito al parar;
// después, al editar uno solo) con sus adjuntos.
export function devuelve<A>(mensajes: EnCola<A>[], texto: string, adjuntos: A[], delante = true): { texto: string; adjuntos: A[] } {
  const ts = mensajes.map(m => m.texto);
  const as = mensajes.flatMap(m => m.adjuntos);
  return delante
    ? { texto: [...ts, texto].filter(Boolean).join('\n\n'), adjuntos: [...as, ...adjuntos] }
    : { texto: [texto, ...ts].filter(Boolean).join('\n\n'), adjuntos: [...adjuntos, ...as] };
}
