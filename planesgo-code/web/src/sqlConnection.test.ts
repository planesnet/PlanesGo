// Pruebas de la conexión con SQL Server de una sesión: npm test (node --test, sin dependencias).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { authKind, EMPTY_SQL, sqlFormError, sqlStatusText } from './sqlConnection.ts';
import * as S from './sqlConnection.ts';

test('sqlFormError asks for what is missing', () => {
  assert.match(sqlFormError(EMPTY_SQL, 'x'), /servidor/);
  assert.match(sqlFormError({ ...EMPTY_SQL, server: 'sage' }, 'x'), /usuario/);
  assert.match(sqlFormError({ ...EMPTY_SQL, server: 'sage', user: 'sa' }, ''), /contraseña/);
  assert.equal(sqlFormError({ ...EMPTY_SQL, server: 'sage', user: 'sa' }, 'x'), '');
});

test('authKind: DOMAIN\\user is Windows', () => {
  assert.equal(authKind('MAZANA\\luis'), 'Windows (NTLM)');
  assert.equal(authKind('sa'), 'SQL Server');
});

test('sqlStatusText sums up the connection', () => {
  assert.equal(sqlStatusText({ connected: false }), 'Sin conexión.');
  const v = { connected: true, config: { ...EMPTY_SQL, server: 'sage', database: 'Sage', user: 'sa' }, batches: 3 };
  assert.equal(sqlStatusText(v), 'sa@sage/Sage · solo lectura · 3 órdenes');
  assert.match(sqlStatusText({ ...v, config: { ...v.config, allow_changes: true }, batches: 1 }), /con cambios · 1 orden$/);
});

test('varias conexiones con nombre', () => {
  assert.equal(S.sqlNameError('pruebas'), '');
  assert.equal(S.sqlNameError('contenedor-2'), '');
  assert.notEqual(S.sqlNameError('Pruebas'), '');
  assert.notEqual(S.sqlNameError('año'), '');
  assert.notEqual(S.sqlNameError(''), '');
  assert.deepEqual(S.sqlNames(['pruebas', 'contenedor'], 'nueva'), ['principal', 'contenedor', 'nueva', 'pruebas']);
  assert.deepEqual(S.sqlNames(undefined, 'principal'), ['principal']);
});
