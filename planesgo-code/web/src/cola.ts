// Cola de mensajes del chat (como en Claude Code): lo que se escribe mientras el agente trabaja espera su turno.
// Una cola por sesión. Si un turno se para o falla, la cola se detiene hasta que se envíe algo a mano o se pida
// seguir. Funciones puras (sin React) para poder probarlas: node --test src/cola.test.ts

// origen: por qué se encoló (quién lo decidió y su motivo), para registrarlo así al salir; lo pone el enrutador.
export interface EnCola<A = unknown> { texto: string; adjuntos: A[]; origen?: QueueOrigin }
export interface QueueOrigin { engine: string; reason: string; forced?: boolean; generated?: boolean }
export interface Cola<A = unknown> { mensajes: EnCola<A>[]; parada: boolean }

export const vacia = <A>(): Cola<A> => ({ mensajes: [], parada: false });

// encola: añade al final; sin texto ni adjuntos no se encola (con adjuntos, aunque no haya texto).
export function encola<A>(c: Cola<A>, texto: string, adjuntos: A[] = [], origen?: QueueOrigin): Cola<A> {
  const t = texto.trim();
  if (!t && !adjuntos.length) return c;
  return { ...c, mensajes: [...c.mensajes, { texto: t, adjuntos, ...(origen ? { origen } : {}) }] };
}

// edita: el mensaje de la cola que se cargó con ↑ (inputHistory.ts), con su texto nuevo y sus adjuntos. Se busca
// en su sitio y, si la cola ha cambiado, por su texto original; null si ya no está (ya salió). Sin texto ni
// adjuntos, sale de la cola.
export function edita<A>(c: Cola<A>, i: number, original: string, texto: string): Cola<A> | null {
  const j = c.mensajes[i]?.texto === original ? i : c.mensajes.findIndex(m => m.texto === original);
  if (j < 0) return null;
  const t = texto.trim();
  if (!t && !c.mensajes[j].adjuntos.length) return quita(c, j);
  return { ...c, mensajes: c.mensajes.map((m, k) => (k === j ? { ...m, texto: t } : m)) };
}

export function quita<A>(c: Cola<A>, i: number): Cola<A> {
  return { ...c, mensajes: c.mensajes.filter((_, j) => j !== i) };
}

// siguiente: con el agente libre y la cola en marcha, lo que toca enviar y la cola sin ello. Los mensajes en cola
// salen juntos, en uno solo (una petición en vez de varias: el agente no relee la conversación por cada uno). Una
// orden («/plan …», «/skill …», «ejecuta el plan 2») sale sola: cambia lo que hace pgocode con el mensaje.
// partes: los textos que salen juntos, por separado (el enrutador mira si alguno se encoló con «>»); origenes, por
// qué se encoló cada uno.
export function siguiente<A>(c: Cola<A>, ocupado: boolean, esOrden: (t: string) => boolean = t => t.startsWith('/')): { enviar?: EnCola<A>; cola: Cola<A>; partes?: string[]; origenes?: (QueueOrigin | undefined)[] } {
  if (ocupado || c.parada || c.mensajes.length === 0) return { cola: c };
  let n = 1;
  if (!esOrden(c.mensajes[0].texto)) {
    while (n < c.mensajes.length && !esOrden(c.mensajes[n].texto)) n++;
  }
  const juntos = c.mensajes.slice(0, n);
  const enviar = { texto: juntos.map(m => m.texto).filter(Boolean).join('\n\n'), adjuntos: juntos.flatMap(m => m.adjuntos) };
  return { enviar, cola: { ...c, mensajes: c.mensajes.slice(n) }, partes: juntos.map(m => m.texto), origenes: juntos.map(m => m.origen) };
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
