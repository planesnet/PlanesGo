// Pruebas de la compactación del historial del agente: npm test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { compactAtTokens, lastContext } from './compaction.ts';
import type { Mensaje } from './api.ts';

test('compactAtTokens: vacío es el valor por defecto; dentro de los límites, en tokens', () => {
  assert.equal(compactAtTokens(undefined), 0);
  assert.equal(compactAtTokens(0), 0);
  assert.equal(compactAtTokens(60), 60000);
  assert.equal(compactAtTokens(5), 20000);
  assert.equal(compactAtTokens(5000), 1000000);
});

test('lastContext: el contexto del último turno del agente que lo tiene', () => {
  const m = [
    { rol: 'user', texto: 'a' }, { rol: 'assistant', texto: 'b', uso: { entrada: 1, salida: 1, contexto: 30000 } },
    { rol: 'user', texto: 'c' }, { rol: 'assistant', texto: 'd', uso: { entrada: 1, salida: 1 } }, { rol: 'user', texto: 'e' },
  ] as Mensaje[];
  assert.equal(lastContext(m), 30000);
  assert.equal(lastContext([]), 0);
});
