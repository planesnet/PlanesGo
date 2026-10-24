// SQL Server connection of a work session (session properties → SQL Server; internal/servidor/sqlconn.go): the
// session's agents use it with «pgo sql» and pgocode's services from the backend. The password only goes to the
// backend, which keeps it in memory.

export interface SqlConfig { server: string; database: string; user: string; allow_changes: boolean; trust_certificate: boolean }

export interface SqlView {
  connected: boolean;
  config?: SqlConfig;
  databases?: string[]; // the ones the user can use (to change it while connected)
  server_version?: string;
  opened?: string;
  batches?: number;
}

export const EMPTY_SQL: SqlConfig = { server: '', database: '', user: '', allow_changes: false, trust_certificate: true };

// The last connection used in a project (without the password), to fill the form.
const key = (login: string, project: number) => `pgo:sql:${login.toLowerCase()}:${project}`;
export function readLastSql(login: string, project: number): SqlConfig {
  try { return { ...EMPTY_SQL, ...JSON.parse(localStorage.getItem(key(login, project)) || '{}') }; } catch { return { ...EMPTY_SQL }; }
}
export function saveLastSql(login: string, project: number, c: SqlConfig) {
  try { localStorage.setItem(key(login, project), JSON.stringify(c)); } catch { /* no storage */ }
}

// sqlFormError: what is missing before connecting ('' if nothing).
export function sqlFormError(c: SqlConfig, password: string): string {
  if (!c.server.trim()) return 'Falta el servidor (equipo, equipo:puerto o equipo\\instancia).';
  if (!c.user.trim()) return 'Falta el usuario (de SQL Server, o DOMINIO\\usuario para el de Windows).';
  if (!password) return 'Falta la contraseña.';
  return '';
}

// authKind: the authentication the user implies.
export const authKind = (user: string) => (user.includes('\\') ? 'Windows (NTLM)' : 'SQL Server');

// sqlStatusText: one line on the open connection.
export function sqlStatusText(v: SqlView): string {
  if (!v.connected || !v.config) return 'Sin conexión.';
  const c = v.config;
  const since = v.opened ? new Date(v.opened).toLocaleTimeString('es-ES', { hour: '2-digit', minute: '2-digit' }) : '';
  return `${c.user}@${c.server}${c.database ? '/' + c.database : ''} · ${c.allow_changes ? 'con cambios' : 'solo lectura'}` +
    (since ? ` · desde las ${since}` : '') + (v.batches ? ` · ${v.batches} ${v.batches === 1 ? 'orden' : 'órdenes'}` : '');
}
