// Error reports (Propiedades → Errores): the failures of pgocode itself become GitHub issues of PlanesGo, where the
// automatic PSF sessions can resolve them (internal/bugreport).

export interface KnownFailure { title: string; issue?: number; url?: string; count: number; first: string; last: string; last_sent?: string }

export interface ErrorReportsView {
  enabled: boolean;
  chosen: boolean;      // the person chose; otherwise, the default
  repo: string;
  access: boolean;      // the gh account of this computer can open issues in repo
  public: boolean;      // anyone can read the issues of repo
  dev: boolean;         // development build (off by default)
  max_per_day: number;
  recent: KnownFailure[] | null;
}

// statusText: why it is on or off, in one line.
export function statusText(v: ErrorReportsView): string {
  if (v.enabled && !v.access) return `Activado, pero la cuenta de gh de este equipo no puede abrir issues en ${v.repo}: no se enviará nada.`;
  if (v.enabled && v.public) return `Activado. Ojo: ${v.repo} es público y cualquiera puede leer estas issues.`;
  if (v.enabled) return v.chosen ? 'Activado.' : `Activado (por defecto: tu cuenta de gh puede abrir issues en ${v.repo}).`;
  if (v.chosen) return 'Desactivado.';
  if (v.dev) return 'Desactivado por defecto en las versiones de desarrollo.';
  if (v.access && v.public) return `Desactivado por defecto: ${v.repo} es público y cualquiera podría leer los fallos (pueden llevar nombres de proyectos, clientes o datos).`;
  return v.access ? 'Desactivado.' : `Desactivado: la cuenta de gh de este equipo no puede abrir issues en ${v.repo}.`;
}

// failureText: one failure seen on this computer, for the list.
export function failureText(k: KnownFailure): string {
  const times = k.count === 1 ? '1 vez' : `${k.count} veces`;
  const when = new Date(k.last).toLocaleString('es-ES', { dateStyle: 'short', timeStyle: 'short' });
  return `${times} · última ${when}` + (k.issue ? '' : ' · sin issue todavía');
}
