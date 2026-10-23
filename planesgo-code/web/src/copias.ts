// Copias de trabajo (git worktree) del proyecto: las que crea pgocode para las ramas de las sesiones, las de los
// subagentes (.claude/worktrees) y las del usuario. Solo se quitan si no se pierde nada (lo comprueba el backend).
import type { Sesion } from './sesiones';

export interface Copia {
  carpeta: string; principal: string; rama: string; worktree: boolean; defecto: string; fusionada: boolean;
  cambios: number; stash: number; sin_subir: number; sin_commits?: boolean; // sin_commits: rama recién creada, sin commits propios
  origen: 'pgocode' | 'subagente' | 'usuario'; sesion?: string; rama_nueva?: boolean;
  se_puede_quitar: boolean; pendiente?: string; error?: string;
}

export const ORIGENES: Record<Copia['origen'], string> = { pgocode: 'pgocode', subagente: 'subagente', usuario: 'usuario' };

// Estado resumido: con cambios (algo se perdería), sin cambios (rama recién creada: para git está «fusionada», pero
// no tiene nada propio), fusionada (ya está en la rama por defecto) o limpia.
export function estado(c: Copia): 'cambios' | 'sin_cambios' | 'fusionada' | 'limpia' {
  if (c.error || !c.se_puede_quitar) return 'cambios';
  if (c.sin_commits) return 'sin_cambios';
  return c.fusionada ? 'fusionada' : 'limpia';
}

export const NOMBRE_ESTADO = { cambios: 'con cambios', sin_cambios: 'sin cambios', fusionada: 'fusionada', limpia: 'limpia' } as const;

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
  return `Se quitará la copia de trabajo:\n${c.carpeta}` + (c.rama ? `\n(rama «${c.rama}»)` : '') +
    '\n\nNo tiene cambios sin guardar ni commits sin subir: se quita con «git worktree remove» (sin forzar).' +
    (borrarRama && c.sin_commits && c.fusionada ? '\nLa rama no tiene commits propios: también se borrará («git branch -d»).'
      : borrarRama && c.fusionada ? `\nLa rama ya está fusionada en «${c.defecto}»: también se borrará la rama local («git branch -d»).`
      : c.rama ? '\nLa rama se conserva.' : '') +
    (usuarios.length ? `\n\n${usuarios.length === 1 ? 'La usa esta sesión, que se quedaría sin carpeta' : `La usan ${usuarios.length} sesiones, que se quedarían sin carpeta`}:\n${nombres}\n` +
      `${usuarios.length === 1 ? 'Se archivará (su conversación se conserva; está' : 'Se archivarán (su conversación se conserva; están'} al final de la lista de sesiones, en «Archivadas»).` : '');
}
