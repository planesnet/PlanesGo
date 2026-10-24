// Pruebas de las copias de trabajo: npm test (node --test, sin dependencias).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as C from './copias.ts';
import type { Sesion } from './sesiones.ts';

const copia = (x: Partial<C.Copia>): C.Copia => ({ carpeta: '/c', principal: '/r', rama: 'dev', worktree: true, defecto: 'main', fusionada: false,
  cambios: 0, stash: 0, sin_subir: 0, origen: 'pgocode', se_puede_quitar: true, ...x });
const sesion = (x: Partial<Sesion>): Sesion => ({ id: 's', nombre: '', proyecto: 1, carpeta: '/r', rama: 'dev', agente: 'claude-code', creada: 0, ultima: 0, ...x });

test('estado de una copia', () => {
  assert.equal(C.estado(copia({})), 'limpia');
  assert.equal(C.estado(copia({ fusionada: true })), 'fusionada');
  assert.equal(C.estado(copia({ fusionada: true, se_puede_quitar: false, cambios: 2 })), 'cambios');
  assert.equal(C.estado(copia({ error: 'x' })), 'cambios');
});

test('copiaDeSesion: solo las de pgocode y si nadie más las usa', () => {
  const s = sesion({ id: 'a', carpeta: '/c' });
  assert.equal(C.copiaDeSesion([copia({ origen: 'subagente' })], s, []), null);
  assert.equal(C.copiaDeSesion([copia({})], s, [])?.carpeta, '/c');
  assert.equal(C.copiaDeSesion([copia({})], s, [sesion({ id: 'b', carpeta: '/c', rama: 'otra' })]), null);
  // Copia de la rama en otra carpeta (red de seguridad del chat): por su sesión.
  const t = sesion({ id: 'a', carpeta: '/r', rama: 'dev' });
  assert.equal(C.copiaDeSesion([copia({ sesion: 'a' })], t, [])?.carpeta, '/c');
  assert.equal(C.copiaDeSesion([copia({ sesion: 'a' })], t, [sesion({ id: 'b', carpeta: '/r', rama: 'dev' })]), null);
});

test('explicaQuitar', () => {
  assert.match(C.explicaQuitar(copia({ fusionada: true }), true), /git branch -d/);
  assert.match(C.explicaQuitar(copia({}), true), /La rama se conserva/);
});

test('explicaQuitar: dice qué sesiones usan la copia y que se archivan; copia sin commits', () => {
  const s = (nombre: string) => ({ id: nombre, nombre, proyecto: 1, carpeta: '/c', rama: 'dev', agente: 'claude-code', creada: 0, ultima: 0 });
  assert.match(C.explicaQuitar(copia({}), true, [s('Una')]), /La usa esta sesión, que se quedaría sin carpeta:\n· Una\nSe archivará/);
  assert.match(C.explicaQuitar(copia({}), true, [s('A'), s('B')]), /La usan 2 sesiones.*\n· A\n· B\nSe archivarán/);
  assert.doesNotMatch(C.explicaQuitar(copia({}), true), /archiv/);
  assert.equal(C.estado(copia({ fusionada: true, sin_commits: true })), 'sin_cambios');
  assert.equal(C.copyUsers(copia({}), [s('A'), { ...s('B'), carpeta: '/otra' }, { ...s('C'), archivada: true }]).map(x => x.id).join(), 'A');
});

test('menú ⋮ de una subcarpeta', () => {
  const c = (x: Partial<C.Copia> = {}): C.Copia => ({ carpeta: '/r/x', principal: '/r', rama: 'feature', worktree: true, defecto: 'master', fusionada: false,
    cambios: 0, stash: 0, sin_subir: 0, origen: 'usuario', se_puede_quitar: true, ...x });
  assert.deepEqual(C.subfolderActions(c()), ['sesion', 'abrir', 'cambios', 'pr', 'quitar']);
  assert.deepEqual(C.subfolderActions(c({ sin_subir: 2, se_puede_quitar: false })), ['sesion', 'abrir', 'cambios', 'subir', 'pr', 'descartar']);
  assert.deepEqual(C.subfolderActions(c({ rama: 'master', sin_subir: 1 })), ['sesion', 'abrir', 'cambios', 'quitar']); // never push the default branch
  assert.deepEqual(C.subfolderActions(c({ huerfana: true })), ['olvidar']);
  assert.equal(C.estado(c({ huerfana: true })), 'huerfana');
  const t = C.explicaDescartar(c({ cambios: 3, sin_subir: 1, stash: 1, se_puede_quitar: false }));
  assert.match(t, /SE PIERDEN 3 ficheros/);
  assert.match(t, /1 commit sin subir se conserva/);
  assert.match(t, /stash.*se conservan/);
});
