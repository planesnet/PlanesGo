// Subcarpetas (git worktree) del proyecto: las que crea pgocode para las ramas de las sesiones, las de los
// subagentes (.claude/worktrees) y las del usuario. Solo se quitan si no se pierde nada (lo comprueba el backend);
// descartarlas (perdiendo lo pendiente) lo pide una persona con su menú ⋮.
import type { Sesion } from './sesiones';

export interface Copia {
  carpeta: string; principal: string; rama: string; worktree: boolean; defecto: string; fusionada: boolean;
  cambios: number; stash: number; sin_subir: number; sin_commits?: boolean; // sin_commits: rama recién creada, sin commits propios
  origen: 'pgocode' | 'subagente' | 'usuario'; sesion?: string; rama_nueva?: boolean;
  se_puede_quitar: boolean; pendiente?: string; error?: string;
  huerfana?: boolean; // git aún la conoce, pero su carpeta ya no existe (se borró a mano)
}

export const ORIGENES: Record<Copia['origen'], string> = { pgocode: 'pgocode', subagente: 'subagente', usuario: 'usuario' };

// Estado resumido: con cambios (algo se perdería), sin cambios (rama recién creada: para git está «fusionada», pero
// no tiene nada propio), fusionada (ya está en la rama por defecto) o limpia.
export function estado(c: Copia): 'huerfana' | 'cambios' | 'sin_cambios' | 'fusionada' | 'limpia' {
  if (c.huerfana) return 'huerfana';
  if (c.error || !c.se_puede_quitar) return 'cambios';
  if (c.sin_commits) return 'sin_cambios';
  return c.fusionada ? 'fusionada' : 'limpia';
}

export const NOMBRE_ESTADO = { huerfana: 'sin carpeta', cambios: 'con cambios', sin_cambios: 'sin cambios', fusionada: 'fusionada', limpia: 'limpia' } as const;

// copyUsers: las sesiones que trabajan en la carpeta de la copia (al quitarla se quedarían sin carpeta).
export function copyUsers(c: Copia, sesiones: Sesion[]): Sesion[] {
  return sesiones.filter(s => s.carpeta === c.carpeta && !s.archivada);
}

// La copia que creó pgocode para una sesión (la de su carpeta o la de su rama en otra carpeta), si ninguna otra
// sesión la usa (misma carpeta o misma rama): esas se conservan.
export function copiaDeSesion(copias: Copia[], s: Sesion, otras: Sesion[]): Copia | null {
  const c = copias.find(x => x.origen === 'pgocode' && (x.carpeta === s.carpeta || (x.sesion === s.id && x.rama === s.rama)));
  if (!c) return null;
  return otras.some(o => o.id !== s.id && (o.carpeta === c.carpeta || (c.rama !== '' && o.rama === c.rama))) ? null : c;
}

// Lo que se hará al quitar la copia (para la confirmación). usuarios: las sesiones que la usan (se archivan).
export function explicaQuitar(c: Copia, borrarRama: boolean, usuarios: Sesion[] = []): string {
  const nombres = usuarios.slice(0, 5).map(s => '· ' + (s.nombre || 'Sesión nueva')).join('\n') + (usuarios.length > 5 ? `\n· y ${usuarios.length - 5} más` : '');
  return `Se quitará la subcarpeta:\n${c.carpeta}` + (c.rama ? `\n(rama «${c.rama}»)` : '') +
    '\n\nNo tiene cambios sin guardar ni commits sin subir: se quita con «git worktree remove» (sin forzar).' +
    (borrarRama && c.sin_commits && c.fusionada ? '\nLa rama no tiene commits propios: también se borrará («git branch -d»).'
      : borrarRama && c.fusionada ? `\nLa rama ya está fusionada en «${c.defecto}»: también se borrará la rama local («git branch -d»).`
      : c.rama ? '\nLa rama se conserva.' : '') +
    (usuarios.length ? `\n\n${usuarios.length === 1 ? 'La usa esta sesión, que se quedaría sin carpeta' : `La usan ${usuarios.length} sesiones, que se quedarían sin carpeta`}:\n${nombres}\n` +
      `${usuarios.length === 1 ? 'Se archivará (su conversación se conserva; está' : 'Se archivarán (su conversación se conserva; están'} al final de la lista de sesiones, en «Archivadas»).` : '');
}

// ---- The ⋮ menu of a subfolder ----

// SubfolderAction: open a session there, open the folder, see its changes, push its branch, see its PR, remove it
// (nothing is lost), discard it (its uncommitted work is lost; the branch stays) or forget it (its folder is gone).
export type SubfolderAction = 'sesion' | 'abrir' | 'cambios' | 'subir' | 'pr' | 'quitar' | 'descartar' | 'olvidar';

// subfolderActions: what the menu offers for a subfolder, in its order. Never push or ask the PR of the default
// branch (pushing it can deploy); «descartar» only when «quitar» is not possible.
export function subfolderActions(c: Copia): SubfolderAction[] {
  if (c.huerfana) return ['olvidar'];
  if (c.error) return ['abrir'];
  const branch = Boolean(c.rama) && c.rama !== c.defecto;
  const l: SubfolderAction[] = ['sesion', 'abrir', 'cambios'];
  if (branch && c.sin_subir > 0) l.push('subir');
  if (branch) l.push('pr');
  l.push(c.se_puede_quitar ? 'quitar' : 'descartar');
  return l;
}

export const SUBFOLDER_ACTION: Record<SubfolderAction, { texto: string; icono: string; peligro?: boolean }> = {
  sesion: { texto: 'Abrir sesión aquí', icono: 'ir' },
  abrir: { texto: 'Abrir la carpeta', icono: 'carpeta' },
  cambios: { texto: 'Ver cambios', icono: 'rama' },
  subir: { texto: 'Subir la rama', icono: 'conectar' },
  pr: { texto: 'Ver su PR', icono: 'externo' },
  quitar: { texto: 'Quitar', icono: 'borrar', peligro: true },
  descartar: { texto: 'Descartar y quitar', icono: 'borrar', peligro: true },
  olvidar: { texto: 'Olvidar', icono: 'cerrar' },
};

// explicaDescartar: what discarding loses (for its confirmation): the uncommitted work; the commits stay in the
// branch, and stashes in the repository.
export function explicaDescartar(c: Copia, usuarios: Sesion[] = []): string {
  return `Se quitará la subcarpeta aunque tiene trabajo sin guardar:\n${c.carpeta}` + (c.rama ? `\n(rama «${c.rama}»)` : '') +
    (c.cambios ? `\n\nSE PIERDEN ${c.cambios} ${c.cambios === 1 ? 'fichero' : 'ficheros'} con cambios sin commit.` : '') +
    (c.sin_subir ? `\n${c.sin_subir} ${c.sin_subir === 1 ? 'commit sin subir se conserva' : 'commits sin subir se conservan'} en la rama.` : '') +
    (c.stash ? '\nLos cambios guardados con «git stash» se conservan en el repositorio.' : '') +
    (usuarios.length ? `\n\n${usuarios.length === 1 ? 'La sesión que la usa se archivará' : `Las ${usuarios.length} sesiones que la usan se archivarán`}.` : '');
}
