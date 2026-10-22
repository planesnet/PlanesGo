// Pruebas de la gestión automática del contexto: npm test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { COMPACT_AGENT_OWN, autoContextOn, contextPolicy, lastContext } from './contextPolicy.ts';
import type { Mensaje } from './api.ts';

test('contextPolicy: poda en OpenCode, relevo en Claude Code (caché caducada) y Antigravity, reserva en todos', () => {
  assert.deepEqual(contextPolicy('opencode', {}), { prune: true, relay: 'off', compactAt: 0 });
  assert.deepEqual(contextPolicy('claude-code', {}), { prune: false, relay: 'idle', compactAt: 0 });
  assert.deepEqual(contextPolicy('antigravity', {}), { prune: false, relay: 'on', compactAt: 0 });
  assert.deepEqual(contextPolicy('otro', {}), { prune: false, relay: 'off', compactAt: 0 });
});

test('contextPolicy: desactivada, ni poda ni relevo, y la compactación del agente', () => {
  for (const a of ['opencode', 'claude-code', 'antigravity']) {
    assert.deepEqual(contextPolicy(a, { autoContext: false }), { prune: false, relay: 'off', compactAt: COMPACT_AGENT_OWN });
  }
  assert.equal(autoContextOn({}), true);
  assert.equal(autoContextOn({ autoContext: false }), false);
});

test('lastContext: el contexto del último turno del agente que lo tiene', () => {
  const m = [
    { rol: 'user', texto: 'a' }, { rol: 'assistant', texto: 'b', uso: { entrada: 1, salida: 1, contexto: 30000 } },
    { rol: 'user', texto: 'c' }, { rol: 'assistant', texto: 'd', uso: { entrada: 1, salida: 1 } }, { rol: 'user', texto: 'e' },
  ] as Mensaje[];
  assert.equal(lastContext(m), 30000);
  assert.equal(lastContext([]), 0);
});
