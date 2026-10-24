// Agent desktop of a work session (session properties → Escritorio del agente; internal/servidor/agentdesktop.go):
// a Windows desktop over RDP on a virtual screen that the session's agents drive with «pgo escritorio». The password
// only goes to the backend, which hands it to FreeRDP.

export interface AgentDesktopConfig { host: string; user: string; keyboard: 'unicode' | 'keys'; size: string }

export interface AgentDesktopStatus {
  state: 'starting' | 'connected' | 'ended';
  error?: string;
  host: string;
  user: string;
  keyboard: string;
  width: number;
  height: number;
  recording: boolean;
  video?: string;
  exchange: string;
  started: string;
}

export interface AgentDesktopView {
  supported: boolean;
  missing?: string; // programs that are not installed
  exchange: string;
  running: boolean;
  status?: AgentDesktopStatus;
  actions?: number;
  held?: boolean;    // the person has the screen (Pantalla del agente): the agent waits
}

export const EMPTY_DESKTOP: AgentDesktopConfig = { host: '', user: '', keyboard: 'unicode', size: '1600x900' };

// The last desktop used in a project (without the password), to fill the form.
const key = (login: string, project: number) => `pgo:agent-desktop:${login.toLowerCase()}:${project}`;
export function readLastDesktop(login: string, project: number): AgentDesktopConfig {
  try { return { ...EMPTY_DESKTOP, ...JSON.parse(localStorage.getItem(key(login, project)) || '{}') }; } catch { return { ...EMPTY_DESKTOP }; }
}
export function saveLastDesktop(login: string, project: number, c: AgentDesktopConfig) {
  try { localStorage.setItem(key(login, project), JSON.stringify(c)); } catch { /* no storage */ }
}

// parseSize: «1600x900» → [1600, 900] (null if it is not a size).
export function parseSize(s: string): [number, number] | null {
  const m = /^\s*(\d{3,4})\s*[x×]\s*(\d{3,4})\s*$/i.exec(s);
  return m ? [Number(m[1]), Number(m[2])] : null;
}

// desktopFormError: what is missing before starting ('' if nothing). The backend checks it all again.
export function desktopFormError(c: AgentDesktopConfig, password: string): string {
  if (!c.host.trim()) return 'Falta el equipo (nombre o IP, y si hace falta :puerto).';
  if (!c.user.trim()) return 'Falta el usuario (DOMINIO\\usuario o usuario).';
  if (!password) return 'Falta la contraseña.';
  const size = parseSize(c.size);
  if (!size || size[0] < 320 || size[0] > 3840 || size[1] < 200 || size[1] > 2160) return 'El tamaño va de 320x200 a 3840x2160 (por ejemplo, 1600x900).';
  return '';
}

// desktopStatusText: one line on the session's desktop.
export function desktopStatusText(v: AgentDesktopView): string {
  const s = v.status;
  if (!s) return 'Sin iniciar.';
  const state = { starting: 'iniciando sesión…', connected: 'conectado', ended: 'terminado' }[s.state];
  const since = s.started ? new Date(s.started).toLocaleTimeString('es-ES', { hour: '2-digit', minute: '2-digit' }) : '';
  return `${s.user}@${s.host} · ${state} · ${s.width}x${s.height}` + (s.recording ? ' · grabando' : '') +
    (since && s.state !== 'ended' ? ` · desde las ${since}` : '') + (v.actions ? ` · ${v.actions} ${v.actions === 1 ? 'orden' : 'órdenes'}` : '');
}
