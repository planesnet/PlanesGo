// node --test src/agentScreen.test.ts
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { keyAction, screenPoint } from './agentScreen.ts';

const k = (key: string, m: Partial<{ ctrlKey: boolean; altKey: boolean; metaKey: boolean; shiftKey: boolean }> = {}) =>
  keyAction({ key, ctrlKey: false, altKey: false, metaKey: false, shiftKey: false, ...m });

test('pantalla del agente: dónde cae un clic', () => {
  assert.deepEqual(screenPoint(400, 225, 800, 450, 1600, 900), [800, 450]);
  assert.deepEqual(screenPoint(799, 449, 800, 450, 1600, 900), [1598, 898]);
  assert.deepEqual(screenPoint(900, -5, 800, 450, 1600, 900), [1599, 0]); // never outside the screen
});

test('pantalla del agente: teclas', () => {
  assert.deepEqual(k('ñ'), { text: 'ñ' });
  assert.deepEqual(k('€'), { text: '€' });
  assert.deepEqual(k('A', { shiftKey: true }), { text: 'A' });
  assert.deepEqual(k('Enter'), { key: 'Return' });
  assert.deepEqual(k('Tab', { shiftKey: true }), { key: 'shift+Tab' });
  assert.deepEqual(k('c', { ctrlKey: true }), { key: 'ctrl+c' });
  assert.deepEqual(k('F4', { altKey: true }), { key: 'alt+F4' });
  assert.deepEqual(k('ArrowDown'), { key: 'Down' });
  assert.deepEqual(k(' '), { text: ' ' });
  assert.equal(k('Shift'), null);
  assert.equal(k('Dead'), null);
});
