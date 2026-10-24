import { test } from 'node:test';
import assert from 'node:assert/strict';
import { formatPeer, normalizePeer, remember } from './remoteConnections.ts';

test('remember: la última primero, sin repetir y con un máximo', () => {
  const same = (a: { id: string }, b: { id: string }) => a.id === b.id;
  assert.deepEqual(remember([{ id: 'a' }, { id: 'b' }], { id: 'b' }, same), [{ id: 'b' }, { id: 'a' }]);
  assert.deepEqual(remember([{ id: 'a' }, { id: 'b' }], { id: 'c' }, same, 2), [{ id: 'c' }, { id: 'a' }]);
});

test('formatPeer: el ID de AnyDesk en grupos de tres', () => {
  assert.equal(formatPeer('123456789'), '123 456 789');
  assert.equal(formatPeer('1234567890'), '1 234 567 890');
  assert.equal(formatPeer('equipo@ad'), 'equipo@ad');
});

test('normalizePeer: cifras o alias; lo demás no', () => {
  assert.equal(normalizePeer(' 123 456 789 '), '123456789');
  assert.equal(normalizePeer('Equipo-01@AD'), 'equipo-01@ad');
  for (const bad of ['', '-h', '12345', '123456789/dominio', '$(id)@ad', 'a b@ad', '123456789 --plain']) assert.equal(normalizePeer(bad), '', bad);
});
