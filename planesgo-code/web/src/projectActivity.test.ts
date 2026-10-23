import { test } from 'node:test';
import assert from 'node:assert/strict';
import { apply, busy, order } from './projectActivity.ts';

test('actividad por proyecto: trabajando o sin leer lo mantiene arriba', () => {
  let s = apply({}, 1, 'a', { working: true });
  assert.ok(busy(s, 1));
  s = apply(s, 1, 'a', { working: false, unread: true });
  assert.deepEqual(s[1], { working: [], unread: ['a'] });
  s = apply(s, 1, 'a', { unread: false });
  assert.equal(busy(s, 1), false);
  const l = [{ id: 3 }, { id: 1 }, { id: 2 }];
  assert.deepEqual(order(l, apply({}, 2, 'x', { working: true })).map(p => p.id), [2, 3, 1]);
});
