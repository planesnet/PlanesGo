// The client apps paired with the server (Propiedades → Servidor) and stopping it.
// node --test src/clients.test.ts

export interface PairedClient { id: string; name: string; created: string; last_seen: string }

// clientDate: a date of a client as the person reads it («—» if it is not known).
export function clientDate(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return '—';
  return d.toLocaleString('es-ES', { dateStyle: 'short', timeStyle: 'short' });
}

// stopQuestion: what stopping the server means, to confirm it (working: what is running now, as the server says it).
export function stopQuestion(working?: string): string {
  const now = working ? `Ahora mismo ${working}: se cortará. ` : '';
  return `${now}Al parar el servidor se paran los agentes, las sesiones automáticas y los programas en marcha, se cierra el tiempo que se está imputando y se cierra esta app. La próxima vez que abras Planes Go Code, el servidor vuelve a arrancar. ¿Parar el servidor?`;
}
