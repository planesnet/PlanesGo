import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as H from './inputHistory.ts';

const items = H.historyItems(['uno', ' dos ', 'dos', '', 'tres'], ['cola A', 'cola B']);

test('historyItems: primero la cola (la última delante), después los enviados, sin vacíos ni repetidos seguidos', () => {
  assert.deepEqual(items, [
    { source: 'queue', queueIndex: 1, text: 'cola B' }, { source: 'queue', queueIndex: 0, text: 'cola A' },
    { source: 'sent', text: 'tres' }, { source: 'sent', text: 'dos' }, { source: 'sent', text: 'uno' },
  ]);
});

test('moveHistory: ↑ recorre hacia atrás guardando el borrador; ↓ vuelve y devuelve el borrador', () => {
  let r = H.moveHistory(items, H.HISTORY_START, 'up', 'borrador')!;
  assert.equal(r.text, 'cola B');
  assert.deepEqual(r.item, { source: 'queue', queueIndex: 1, text: 'cola B' });
  for (const t of ['cola A', 'tres', 'dos', 'uno']) { r = H.moveHistory(items, r.state, 'up', r.text)!; assert.equal(r.text, t); }
  assert.equal(H.moveHistory(items, r.state, 'up', r.text), null, 'no hay más');
  r = H.moveHistory(items, r.state, 'down', r.text)!;
  assert.equal(r.text, 'dos');
  for (let i = 0; i < 3; i++) r = H.moveHistory(items, r.state, 'down', r.text)!;
  assert.equal(r.text, 'cola B');
  r = H.moveHistory(items, r.state, 'down', r.text)!;
  assert.equal(r.text, 'borrador');
  assert.equal(r.item, undefined);
  assert.deepEqual(r.state, H.HISTORY_START);
  assert.equal(H.moveHistory(items, r.state, 'down', 'borrador'), null, 'en el borrador, ↓ no hace nada');
  assert.equal(H.moveHistory([], H.HISTORY_START, 'up', ''), null, 'sin historial, ↑ no hace nada');
});

test('las flechas solo actúan en la primera (↑) o la última línea (↓)', () => {
  const v = 'línea 1\nlínea 2\nlínea 3';
  assert.ok(H.onFirstLine(v, 3));
  assert.ok(!H.onFirstLine(v, 10));
  assert.ok(H.onLastLine(v, v.length));
  assert.ok(!H.onLastLine(v, 10));
  assert.ok(H.onFirstLine('', 0) && H.onLastLine('', 0));
});

test('el aviso de la cola', () => {
  assert.match(H.queueNotice(0), /^Editando el mensaje 1 de la cola/);
});
