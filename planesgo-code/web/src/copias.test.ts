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
