// node --test src/externalAccess.test.ts
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { externalUntil, maskToken } from './externalAccess.ts';

test('acceso externo: hasta cuándo', () => {
  const now = new Date(2026, 9, 24, 10, 0);
  assert.match(externalUntil({ expires: new Date(2026, 9, 24, 18, 30).toISOString() }, now), /^hasta hoy a las 18:30$/);
  assert.match(externalUntil({ expires: new Date(2026, 9, 25, 9, 5).toISOString() }, now), /^hasta 25 oct.* a las 09:05$/);
  assert.equal(externalUntil({ expires: 'x' }, now), 'sin fecha de fin');
});

test('acceso externo: token oculto pero reconocible', () => {
  assert.equal(maskToken('0123456789abcdef'), '012345…cdef');
  assert.equal(maskToken('corto'), '••••');
});
