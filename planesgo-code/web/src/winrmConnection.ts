// WinRM (remote PowerShell) connection of a work session (session properties → PowerShell remoto;
// internal/servidor/winrmconn.go): the session's agents use it with «pgo ps» and pgocode's services from the backend.
// The password only goes to the backend, which keeps it in memory.
import { connectionStatus, readLast, saveLast } from './sessionConnection.ts';

export interface WinRMConfig { host: string; user: string; https: boolean; trust_certificate: boolean; allow_changes: boolean }

export interface WinRMView {
  connected: boolean;
  config?: WinRMConfig;
  server?: string; // computer and user, as the server says
  opened?: string;
  orders?: number;
}

export interface WinRMResult { stdout: string; stderr: string; exit_code: number; truncated?: boolean; read_only: boolean; ms: number }

export const EMPTY_WINRM: WinRMConfig = { host: '', user: '', https: false, trust_certificate: false, allow_changes: false };

export const readLastWinRM = (login: string, project: number) => readLast('winrm', login, project, EMPTY_WINRM);
export const saveLastWinRM = (login: string, project: number, c: WinRMConfig) => saveLast('winrm', login, project, c);

// winrmFormError: what is missing or wrong before connecting ('' if nothing). The backend checks it again.
export function winrmFormError(c: WinRMConfig, password: string): string {
  const host = c.host.trim();
  if (!host) return 'Falta el equipo (nombre, IP o equipo:puerto).';
  if (/^[a-z]+:\/\//i.test(host) || /[\s;/\\]/.test(host)) return 'El equipo es solo el nombre o la IP, con :puerto si no es el habitual (5985 HTTP, 5986 HTTPS).';
  if (!c.user.trim()) return 'Falta el usuario (local, o DOMINIO\\usuario).';
  if (!password) return 'Falta la contraseña.';
  return '';
}

// winrmTransport: how it travels, in words.
export const winrmTransport = (c: Pick<WinRMConfig, 'https' | 'trust_certificate'>) =>
  c.https ? (c.trust_certificate ? 'HTTPS (certificado sin comprobar)' : 'HTTPS') : 'HTTP con cifrado NTLM';

// winrmStatusText: one line on the open connection.
export function winrmStatusText(v: WinRMView): string {
  if (!v.connected || !v.config) return 'Sin conexión.';
  const c = v.config;
  return connectionStatus(`${c.user}@${c.host} (${winrmTransport(c)})`, c.allow_changes, v.opened, v.orders);
}
