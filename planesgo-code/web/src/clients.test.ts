import { test } from 'node:test';
import assert from 'node:assert/strict';
import { clientDate, stopQuestion } from './clients.ts';

test('clientDate: unknown dates as «—»', () => {
  assert.equal(clientDate(''), '—');
  assert.equal(clientDate('0001-01-01T00:00:00Z'), '—');
  assert.match(clientDate('2026-10-24T10:00:00Z'), /24\/10\/26/);
});

test('stopQuestion says what is cut', () => {
  assert.match(stopQuestion('un agente está trabajando'), /^Ahora mismo un agente está trabajando: se cortará\./);
  assert.ok(stopQuestion().startsWith('Al parar el servidor'));
});
