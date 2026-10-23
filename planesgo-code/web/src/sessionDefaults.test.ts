// Pruebas de los valores de una sesión nueva: npm test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as D from './sessionDefaults.ts';
import type { Sesion } from './sesiones.ts';

const ses = (x: Partial<Sesion>): Sesion => ({ id: 's', nombre: '', proyecto: 1, carpeta: '', rama: '', agente: 'claude-code', creada: 0, ultima: 0, ...x });

test('lastHosting: el de la última sesión creada del proyecto (sin contar hijas); si no, el del proyecto', () => {
  const l = [ses({ id: 'a', creada: 1, hosting: 'local' }), ses({ id: 'b', creada: 3, hosting: 'odoo.sh' }), ses({ id: 'h', creada: 5, hosting: 'gcloud', madre: 'b' }), ses({ id: 'c', creada: 4 })];
  assert.equal(D.lastHosting(l), 'odoo.sh');
  assert.equal(D.lastHosting([], 'sage'), 'sage');
  assert.equal(D.lastHosting([]), undefined);
});

test('newSessionDefaults: nace en «editar» aunque la anterior estuviera en «auto», con el alojamiento y el modelo recordados', () => {
  const n = D.newSessionDefaults(ses({ permisos: 'auto' }), { sessions: [ses({ hosting: 'local', creada: 1 })], model: 'haiku' });
  assert.equal(n.permisos, 'editar');
  assert.equal(n.hosting, 'local');
  assert.equal(n.modelo, 'haiku');
  // Lo que ya trae (p. ej. el alojamiento de la sesión abierta en la misma rama) se respeta.
  assert.equal(D.newSessionDefaults(ses({ hosting: 'sage' }), { sessions: [ses({ hosting: 'local' })] }).hosting, 'sage');
});
