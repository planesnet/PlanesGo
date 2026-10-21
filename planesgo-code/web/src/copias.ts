// Copias de trabajo (git worktree) del proyecto: las que crea pgocode para las ramas de las sesiones, las de los
// subagentes (.claude/worktrees) y las del usuario. Solo se quitan si no se pierde nada (lo comprueba el backend).
import type { Sesion } from './sesiones';

export interface Copia {
  carpeta: string; principal: string; rama: string; worktree: boolean; defecto: string; fusionada: boolean;
  cambios: number; stash: number; sin_subir: number;
  origen: 'pgocode' | 'subagente' | 'usuario'; sesion?: string; rama_nueva?: boolean;
  se_puede_quitar: boolean; pendiente?: string; error?: string;
}

export const ORIGENES: Record<Copia['origen'], string> = { pgocode: 'pgocode', subagente: 'subagente', usuario: 'usuario' };

// Estado resumido: con cambios (algo se perdería), fusionada (ya está en la rama por defecto) o limpia.
export function estado(c: Copia): 'cambios' | 'fusionada' | 'limpia' {
  if (c.error || !c.se_puede_quitar) return 'cambios';
  return c.fusionada ? 'fusionada' : 'limpia';
}

export const NOMBRE_ESTADO = { cambios: 'con cambios', fusionada: 'fusionada', limpia: 'limpia' } as const;

// La copia que creó pgocode para una sesión (la de su carpeta o la de su rama en otra carpeta), si ninguna otra
// sesión la usa (misma carpeta o misma rama): esas se conservan.
export function copiaDeSesion(copias: Copia[], s: Sesion, otras: Sesion[]): Copia | null {
  const c = copias.find(x => x.origen === 'pgocode' && (x.carpeta === s.carpeta || (x.sesion === s.id && x.rama === s.rama)));
  if (!c) return null;
  return otras.some(o => o.id !== s.id && (o.carpeta === c.carpeta || (c.rama !== '' && o.rama === c.rama))) ? null : c;
}

// Lo que se hará al quitar la copia (para la confirmación).
export function explicaQuitar(c: Copia, borrarRama: boolean): string {
  return `Se quitará la copia de trabajo:\n${c.carpeta}` + (c.rama ? `\n(rama «${c.rama}»)` : '') +
    '\n\nNo tiene cambios sin guardar ni commits sin subir: se quita con «git worktree remove» (sin forzar).' +
    (borrarRama && c.fusionada ? `\nLa rama ya está fusionada en «${c.defecto}»: también se borrará la rama local («git branch -d»).`
      : c.rama ? '\nLa rama se conserva.' : '');
}
