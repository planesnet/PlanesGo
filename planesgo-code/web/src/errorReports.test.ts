// Pruebas de los informes de errores: npm test (node --test, sin dependencias).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { failureText, statusText, type ErrorReportsView } from './errorReports.ts';

const view = (x: Partial<ErrorReportsView>): ErrorReportsView =>
  ({ enabled: false, chosen: false, repo: 'planesnet/PlanesGo', access: true, dev: false, max_per_day: 5, recent: [], ...x });

test('statusText says why it is on or off', () => {
  assert.match(statusText(view({ enabled: true })), /por defecto/);
  assert.equal(statusText(view({ enabled: true, chosen: true })), 'Activado.');
  assert.match(statusText(view({ enabled: true, chosen: true, access: false })), /no se enviará nada/);
  assert.match(statusText(view({ dev: true })), /desarrollo/);
  assert.match(statusText(view({ access: false })), /no puede abrir issues/);
  assert.equal(statusText(view({ chosen: true })), 'Desactivado.');
});

test('failureText counts and says when there is no issue yet', () => {
  const k = { title: 'Fallo de pgocode: x', count: 3, first: '2026-10-24T10:00:00Z', last: '2026-10-24T11:00:00Z' };
  assert.match(failureText(k), /^3 veces · última .* · sin issue todavía$/);
  assert.doesNotMatch(failureText({ ...k, count: 1, issue: 7 }), /sin issue/);
});
