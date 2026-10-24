// SQL Server connection of a work session (session properties → SQL Server; internal/servidor/sqlconn.go): the
// session's agents use it with «pgo sql» and pgocode's services from the backend. The password only goes to the
// backend, which keeps it in memory.
import { connectionStatus, readLast, saveLast } from './sessionConnection.ts';

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
export const readLastSql = (login: string, project: number) => readLast('sql', login, project, EMPTY_SQL);
export const saveLastSql = (login: string, project: number, c: SqlConfig) => saveLast('sql', login, project, c);

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
