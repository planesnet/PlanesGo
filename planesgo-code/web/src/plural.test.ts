import { test } from 'node:test';
import assert from 'node:assert/strict';
import { plural } from './plural.ts';

test('plural: singular con 1, plural con el resto', () => {
  assert.equal(plural(1, 'fichero'), '1 fichero');
  assert.equal(plural(0, 'fichero'), '0 ficheros');
  assert.equal(plural(3, 'acción', 'acciones'), '3 acciones');
  assert.equal(plural(1, 'acción', 'acciones'), '1 acción');
});
