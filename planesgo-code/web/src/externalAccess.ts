// External access to a work session (session properties → Acceso externo; internal/servidor/externalaccess.go).

// token and env: the access itself and the «export …» lines to give the external agent (kept with the session).
export interface ExternalAccess { id: string; label: string; login: string; file: string; token: string; env: string; created: string; expires: string }

// maskToken: the token, hidden but recognizable.
export const maskToken = (t: string) => (t.length > 12 ? t.slice(0, 6) + '…' + t.slice(-4) : '••••');

// externalUntil: one line on an access: until when it lasts.
export function externalUntil(a: Pick<ExternalAccess, 'expires'>, now = new Date()): string {
  const d = new Date(a.expires);
  if (isNaN(d.getTime())) return 'sin fecha de fin';
  const same = d.toDateString() === now.toDateString();
  const time = d.toLocaleTimeString('es-ES', { hour: '2-digit', minute: '2-digit' });
  return `hasta ${same ? 'hoy a las ' + time : d.toLocaleDateString('es-ES', { day: 'numeric', month: 'short' }) + ' a las ' + time}`;
}
