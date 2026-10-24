// SQL Server connection of a work session (session properties → SQL Server; internal/servidor/sqlconn.go): the
// session's agents use it with «pgo sql» and pgocode's services from the backend. The password only goes to the
// backend, which keeps it in memory.
import { connectionStatus, readLast, saveLast } from './sessionConnection.ts';

export interface SqlConfig { server: string; database: string; user: string; allow_changes: boolean; trust_certificate: boolean }

export interface SqlView {
  name?: string;     // this connection («principal» the main one)
  names?: string[];  // the session's open connections
  connected: boolean;
  config?: SqlConfig;
  databases?: string[]; // the ones the user can use (to change it while connected)
  server_version?: string;
  opened?: string;
  batches?: number;
}

export const EMPTY_SQL: SqlConfig = { server: '', database: '', user: '', allow_changes: false, trust_certificate: true };

// The last connection used in a project with that name (without the password), to fill the form.
const lastKey = (name: string) => (name === SQL_MAIN ? 'sql' : 'sql:' + name);
export const readLastSql = (login: string, project: number, name = SQL_MAIN) => readLast(lastKey(name), login, project, EMPTY_SQL);
export const saveLastSql = (login: string, project: number, c: SqlConfig, name = SQL_MAIN) => saveLast(lastKey(name), login, project, c);

// A session can have several connections, each with its name ('pruebas', 'contenedor'…); the main one is «principal»
// (the one «pgo sql» uses without --conexion).
export const SQL_MAIN = 'principal';
export const sqlNameError = (name: string) =>
  /^[a-z0-9][a-z0-9-]{0,31}$/.test(name) ? '' : 'El nombre: letras minúsculas sin acentos, cifras y guiones (hasta 32), p. ej. «pruebas».';
// sqlNames: the connections to choose from: the open ones, the main one and the one being defined.
export const sqlNames = (open: string[] | undefined, current: string) => [...new Set([SQL_MAIN, ...(open || []), current])].sort((a, b) => (a === SQL_MAIN ? -1 : b === SQL_MAIN ? 1 : a.localeCompare(b)));

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
  return connectionStatus(`${c.user}@${c.server}${c.database ? '/' + c.database : ''}`, c.allow_changes, v.opened, v.batches);
}
