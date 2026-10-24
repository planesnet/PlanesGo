import { test } from 'node:test';
import assert from 'node:assert/strict';
import { clientDate, stopQuestion, versionsText } from './clients.ts';

test('clientDate: unknown dates as «—»', () => {
  assert.equal(clientDate(''), '—');
  assert.equal(clientDate('0001-01-01T00:00:00Z'), '—');
  assert.match(clientDate('2026-10-24T10:00:00Z'), /24\/10\/26/);
});

test('stopQuestion says what is cut', () => {
  assert.match(stopQuestion('un agente está trabajando'), /^Ahora mismo un agente está trabajando: se cortará\./);
  assert.ok(stopQuestion().startsWith('Al parar el servidor'));
});

test('versiones del servidor y de la app', () => {
  assert.equal(versionsText('0.7.0', { app: '0.7.2', electron: '45.0.0' }),
    'Servidor 0.7.0 · esta app 0.7.2 (Electron 45.0.0). Tienen que compartir la versión mayor (0.7 de 0.7.0); las menores pueden ser distintas.');
  assert.match(versionsText('dev', { app: '', electron: '45.0.0' }), /^Servidor desarrollo · esta app desarrollo/);
  assert.match(versionsText('0.7.0', null), /en el navegador/);
});
