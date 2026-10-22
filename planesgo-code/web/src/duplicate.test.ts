// Pruebas de «Duplicar sin historial»: npm test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { copyName, duplicateSession } from './duplicate.ts';
import type { Sesion } from './sesiones.ts';

const sesion = (x: Partial<Sesion>): Sesion => ({ id: 's', nombre: 'Arreglar login', proyecto: 7, carpeta: '/r/dev', rama: 'dev', agente: 'claude-code', creada: 1, ultima: 2, ...x });

test('copyName: primer número libre', () => {
  assert.equal(copyName('Arreglar login', ['Arreglar login']), 'Arreglar login · copia 1');
  assert.equal(copyName('Arreglar login', ['Arreglar login', 'Arreglar login · copia 1']), 'Arreglar login · copia 2');
  assert.equal(copyName('A', ['A · copia 1', 'A · copia 3']), 'A · copia 2');
});

test('copyName: de una copia no encadena sufijos', () => {
  assert.equal(copyName('A · copia 1', ['A', 'A · copia 1']), 'A · copia 2');
  assert.equal(copyName('A · copia 2', ['A', 'A · copia 1', 'A · copia 2']), 'A · copia 3');
  assert.equal(copyName('A · copia 1', ['A · copia 1']), 'A · copia 2');
});

test('copyName: sin nombre', () => {
  assert.equal(copyName('', []), 'Sesión nueva · copia 1');
});

test('duplicateSession: copia los ajustes, no la conversación ni la identidad', () => {
  const s = sesion({
    modelo: 'haiku', sesionAgente: 'abc', permisos: 'plan', modoPrevio: 'editar', plan: '/r/plan.md', autorizadas: ['/otra'],
    avisos: { coste: 5 } as unknown as Sesion['avisos'], mcp: ['playwright'], decideAparte: false, tipo: 'local', hosting: 'odoo.sh' as Sesion['hosting'],
    esfuerzo: 'high', tareaId: 42, tarea: 'Tarea X', corte: 3, renovarContexto: 20000, compactAtK: 30, madre: 'm',
  });
  const c = duplicateSession(s, [s], 1000);
  assert.notEqual(c.id, s.id);
  assert.equal(c.nombre, 'Arreglar login · copia 1');
  assert.equal(c.creada, 1000);
  assert.equal(c.ultima, 1000);
  assert.equal(c.sesionAgente, undefined);
  assert.equal(c.plan, undefined);
  assert.ok(!('sesionAgente' in c) && !('plan' in c) && !('corte' in c), 'ni la sesión del agente, ni el plan, ni el corte pendiente');
  const { id: _a, nombre: _b, sesionAgente: _c, plan: _d, corte: _k, creada: _e, ultima: _f, ...ajustes } = s;
  const { id: _g, nombre: _h, creada: _i, ultima: _j, ...copiados } = c;
  assert.deepEqual(copiados, ajustes);
  // Copia profunda: cambiar la copia no toca el original.
  c.autorizadas!.push('/más');
  assert.deepEqual(s.autorizadas, ['/otra']);
});

test('duplicateSession: dos copias seguidas', () => {
  const s = sesion({});
  const c1 = duplicateSession(s, [s]);
  const c2 = duplicateSession(s, [s, c1]);
  const c3 = duplicateSession(c1, [s, c1, c2]);
  assert.deepEqual([c1.nombre, c2.nombre, c3.nombre], ['Arreglar login · copia 1', 'Arreglar login · copia 2', 'Arreglar login · copia 3']);
});
