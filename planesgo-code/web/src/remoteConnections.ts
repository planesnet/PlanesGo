// Remote desktop connections remembered in this browser (RDP and AnyDesk), without passwords: the last one first.
// Also how an AnyDesk ID is shown and checked before sending it (the backend validates it again).

const storageKey = (kind: string, login: string) => `pgo:${kind}:${login.toLowerCase()}`;

export function readSaved<T>(kind: string, login: string): T[] {
  try {
    const l = JSON.parse(localStorage.getItem(storageKey(kind, login)) || '[]');
    return Array.isArray(l) ? l : [];
  } catch { return []; }
}

export function writeSaved<T>(kind: string, login: string, list: T[]) {
  try { localStorage.setItem(storageKey(kind, login), JSON.stringify(list)); } catch { /* no storage */ }
}

// remember: the item first and without repeating it (same), at most max.
export const remember = <T,>(list: T[], item: T, same: (a: T, b: T) => boolean, max = 20): T[] =>
  [item, ...list.filter(x => !same(x, item))].slice(0, max);

// AnyDesk IDs as AnyDesk shows them (123 456 789); aliases as they are.
export const formatPeer = (peer: string) => /^\d+$/.test(peer) ? peer.replace(/\B(?=(\d{3})+$)/g, ' ') : peer;

// normalizePeer: what the backend accepts (digits, with spaces between groups, or an alias like equipo@ad); '' if
// it is not valid.
export function normalizePeer(s: string): string {
  let t = s.trim();
  const id = t.split('/')[0];
  if (t.includes('/') && /^[\d ]+$/.test(id)) t = id; // «123 456 789/dominio/usuario», as AnyDesk shows it: the ID
  const digits = t.replace(/ /g, '');
  if (/^\d{6,12}$/.test(digits)) return digits;
  const alias = t.toLowerCase();
  return /^[a-z0-9][a-z0-9._-]{0,62}@[a-z0-9][a-z0-9_-]{0,31}$/.test(alias) ? alias : '';
}
