// What the connections of a work session share (SQL Server, WinRM…; internal/servidor/sessionconn.go): the last
// form used in a project (without the password) and the line that sums up an open connection.

// readLast: the last connection of that kind used by the user in a project, over the empty one.
export function readLast<T extends object>(kind: string, login: string, project: number, empty: T): T {
  try { return { ...empty, ...JSON.parse(localStorage.getItem(lastKey(kind, login, project)) || '{}') }; } catch { return { ...empty }; }
}

export function saveLast(kind: string, login: string, project: number, value: object) {
  try { localStorage.setItem(lastKey(kind, login, project), JSON.stringify(value)); } catch { /* no storage */ }
}

const lastKey = (kind: string, login: string, project: number) => `pgo:${kind}:${login.toLowerCase()}:${project}`;

// connectionStatus: «<who> · solo lectura · desde las 10:05 · 3 órdenes».
export function connectionStatus(who: string, allowChanges: boolean, opened?: string, orders?: number): string {
  const since = opened ? new Date(opened).toLocaleTimeString('es-ES', { hour: '2-digit', minute: '2-digit' }) : '';
  return `${who} · ${allowChanges ? 'con cambios' : 'solo lectura'}` +
    (since ? ` · desde las ${since}` : '') + (orders ? ` · ${orders} ${orders === 1 ? 'orden' : 'órdenes'}` : '');
}
