// Propiedades de la sesión → Programas: lo que se edita es un borrador; al pulsar «Guardar» se aplican los cambios
// (guardar los nuevos o cambiados y quitar los que ya no están). «Cancelar» no deja nada a medias.
import type { ConfigPrograma } from './api';

export interface ProgramDraft { original: ConfigPrograma[]; current: ConfigPrograma[] }

const same = (a: ConfigPrograma, b: ConfigPrograma) => a.nombre === b.nombre && a.orden === b.orden && (a.url || '') === (b.url || '');

// programChanges: qué hay que guardar y qué quitar para pasar de original a current (un cambio de nombre es quitar
// el viejo y guardar el nuevo).
export function programChanges(d: ProgramDraft): { save: ConfigPrograma[]; remove: string[] } {
  return {
    save: d.current.filter(c => !d.original.some(o => same(o, c))),
    remove: d.original.filter(o => !d.current.some(c => c.nombre === o.nombre)).map(o => o.nombre),
  };
}

// replaceProgram: el borrador con un programa añadido (previous vacío) o cambiado (por su nombre anterior).
export function replaceProgram(l: ConfigPrograma[], c: ConfigPrograma, previous?: string): ConfigPrograma[] {
  const rest = l.filter(x => x.nombre !== previous && x.nombre !== c.nombre);
  const i = previous ? l.findIndex(x => x.nombre === previous) : -1;
  return i < 0 ? [...rest, c] : [...rest.slice(0, i), c, ...rest.slice(i)];
}
