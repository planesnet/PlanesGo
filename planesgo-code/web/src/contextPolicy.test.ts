// Pruebas de la gestión automática del contexto: npm test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { COMPACT_AGENT_OWN, autoContextOn, contextPolicy, lastContext, relayAsBackup } from './contextPolicy.ts';
import type { Mensaje } from './api.ts';

test('contextPolicy: poda en OpenCode y Claude Code (relevo de reserva), relevo por vigencia en Antigravity', () => {
  assert.deepEqual(contextPolicy('opencode', {}), { prune: true, relay: 'off', vig: 'agent', compactAt: 0 });
  assert.deepEqual(contextPolicy('claude-code', {}), { prune: true, relay: 'idle', vig: 'agent', compactAt: 0 });
  assert.deepEqual(contextPolicy('antigravity', {}), { prune: false, relay: 'vig', vig: 'chat', compactAt: 0 });
  assert.deepEqual(contextPolicy('otro', {}), { prune: false, relay: 'off', vig: 'none', compactAt: 0 });
});

test('relayAsBackup: con el historial podable, sin relevo; si no se puede podar (o no se sabe), el de reserva', () => {
  const c = contextPolicy('claude-code', {});
  assert.equal(relayAsBackup(c, true), 'off');
  assert.equal(relayAsBackup(c, false), 'idle');
  assert.equal(relayAsBackup(c, undefined), 'idle');
  assert.equal(relayAsBackup(contextPolicy('antigravity', {}), true), 'vig');
});

test('contextPolicy: desactivada, ni poda ni relevo, y la compactación del agente', () => {
  for (const a of ['opencode', 'claude-code', 'antigravity']) {
    assert.deepEqual(contextPolicy(a, { autoContext: false }), { prune: false, relay: 'off', vig: 'none', compactAt: COMPACT_AGENT_OWN });
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
