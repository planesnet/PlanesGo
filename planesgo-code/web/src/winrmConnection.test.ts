// Pruebas de la conexión WinRM de una sesión: npm test (node --test, sin dependencias).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { EMPTY_WINRM, winrmFormError, winrmStatusText, winrmTransport } from './winrmConnection.ts';
import { connectionStatus } from './sessionConnection.ts';

test('winrmFormError asks for what is missing', () => {
  assert.match(winrmFormError(EMPTY_WINRM, 'x'), /equipo/);
  assert.match(winrmFormError({ ...EMPTY_WINRM, host: 'http://sage:5985' }, 'x'), /solo el nombre/);
  assert.match(winrmFormError({ ...EMPTY_WINRM, host: 'sage x' }, 'x'), /solo el nombre/);
  assert.match(winrmFormError({ ...EMPTY_WINRM, host: 'sage' }, 'x'), /usuario/);
  assert.match(winrmFormError({ ...EMPTY_WINRM, host: 'sage', user: 'MAZANA\\luis' }, ''), /contraseña/);
  assert.equal(winrmFormError({ ...EMPTY_WINRM, host: '192.168.1.10:5986', user: 'luis' }, 'x'), '');
});

test('winrmTransport says how it travels', () => {
  assert.equal(winrmTransport({ https: false, trust_certificate: true }), 'HTTP con cifrado NTLM');
  assert.equal(winrmTransport({ https: true, trust_certificate: false }), 'HTTPS');
  assert.match(winrmTransport({ https: true, trust_certificate: true }), /sin comprobar/);
});

test('winrmStatusText sums up the connection', () => {
  assert.equal(winrmStatusText({ connected: false }), 'Sin conexión.');
  const v = { connected: true, config: { ...EMPTY_WINRM, host: 'sage', user: 'MAZANA\\luis' }, orders: 2 };
  assert.equal(winrmStatusText(v), 'MAZANA\\luis@sage (HTTP con cifrado NTLM) · solo lectura · 2 órdenes');
});

test('connectionStatus is shared', () => {
  assert.equal(connectionStatus('a@b', true, undefined, 1), 'a@b · con cambios · 1 orden');
});
