// Tests of the agent desktop of a session: npm test (node --test, no dependencies).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { desktopFormError, desktopStatusText, EMPTY_DESKTOP, parseSize } from './agentDesktop.ts';

test('parseSize reads WxH', () => {
  assert.deepEqual(parseSize('1600x900'), [1600, 900]);
  assert.deepEqual(parseSize(' 1920 × 1080 '), [1920, 1080]);
  assert.equal(parseSize('grande'), null);
  assert.equal(parseSize('1600x'), null);
});

test('desktopFormError asks for what is missing', () => {
  assert.match(desktopFormError(EMPTY_DESKTOP, 'x'), /equipo/);
  assert.match(desktopFormError({ ...EMPTY_DESKTOP, host: 'sage' }, 'x'), /usuario/);
  assert.match(desktopFormError({ ...EMPTY_DESKTOP, host: 'sage', user: 'D\\u' }, ''), /contraseña/);
  assert.match(desktopFormError({ ...EMPTY_DESKTOP, host: 'sage', user: 'u', size: '5000x900' }, 'x'), /tamaño/);
  assert.equal(desktopFormError({ ...EMPTY_DESKTOP, host: 'sage', user: 'u' }, 'x'), '');
});

test('desktopStatusText sums up the desktop', () => {
  assert.equal(desktopStatusText({ supported: true, exchange: '/x', running: false }), 'Sin iniciar.');
  const status = { state: 'connected' as const, host: 'sage', user: 'D\\u', keyboard: 'unicode', width: 1600, height: 900, recording: true, exchange: '/x', started: '' };
  assert.equal(desktopStatusText({ supported: true, exchange: '/x', running: true, status, actions: 2 }), 'D\\u@sage · conectado · 1600x900 · grabando · 2 órdenes');
  assert.match(desktopStatusText({ supported: true, exchange: '/x', running: false, status: { ...status, state: 'ended', recording: false } }), /terminado · 1600x900$/);
});
