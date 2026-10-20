// Propiedades de cada proyecto (por usuario). De momento en la app; más adelante, en Odoo.
export type Detalle = 'conciso' | 'normal' | 'detallado';
export interface AjustesProyecto { detalle: Detalle }

export const AJUSTES_DEFECTO: AjustesProyecto = { detalle: 'normal' };

const clave = (login: string, proyecto: number) => `pgo:ajustes:${login.toLowerCase()}:${proyecto}`;

export function leeAjustes(login: string, proyecto: number): AjustesProyecto {
  try { return { ...AJUSTES_DEFECTO, ...JSON.parse(localStorage.getItem(clave(login, proyecto)) || '{}') }; } catch { return AJUSTES_DEFECTO; }
}

export function guardaAjustes(login: string, proyecto: number, a: AjustesProyecto) {
  try { localStorage.setItem(clave(login, proyecto), JSON.stringify(a)); } catch { /* sin almacenamiento */ }
}
