// Pruebas de la caché del prompt de la sesión: npm test (node --test, sin dependencias).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as CS from './cacheStatus.ts';
import type { Mensaje } from './api.ts';

const T0 = Date.UTC(2026, 9, 23, 10, 0, 0);
const MIN = 60_000;

// conversation: turnos que acaban en los minutos dados (cada uno dura 30 s), con su contexto.
function conversation(ends: number[], contexts: number[], ttl?: number): Mensaje[] {
  const l: Mensaje[] = [];
  ends.forEach((e, i) => {
    l.push({ rol: 'user', texto: 'm' + i });
    l.push({ rol: 'assistant', texto: 'r' + i, fin: T0 + e * MIN, uso: { entrada: 1, salida: 1, segundos: 30, contexto: contexts[i], ...(ttl ? { cache_ttl: ttl } : {}) } });
  });
  return l;
}

test('turnGaps: del fin de un turno al principio del siguiente', () => {
  const g = CS.turnGaps(conversation([0, 2, 20], [50_000, 60_000, 58_000]));
  assert.equal(g.length, 2);
  assert.equal(Math.round(g[0].gap), 90);       // 2 min menos los 30 s del turno
  assert.equal(g[0].context, 50_000);
  assert.equal(g[0].growth, 10_000);
  assert.equal(g[1].growth, 0);                  // tras una poda, no crece
});

test('chooseClaudeTTL: con pocas pausas, 1 h', () => {
  assert.equal(CS.chooseClaudeTTL([]), '1h');
  assert.equal(CS.chooseClaudeTTL(conversation([0], [50_000]), T0 + MIN), '1h');
});

test('chooseClaudeTTL: la pausa de ahora también cuenta', () => {
  const rapid = conversation([0, 1, 2, 3], [27_000, 27_100, 27_200, 27_300]);
  assert.equal(CS.chooseClaudeTTL(rapid, T0 + 3.5 * MIN), '5m');
  assert.equal(CS.chooseClaudeTTL(rapid, T0 + 26 * MIN), '1h'); // 23 min parada: con 1 h no se habría perdido
});

test('chooseClaudeTTL: turnos seguidos (pausas de menos de 5 min): 5 min; con pausas de 5 a 60 min: 1 h', () => {
  const rapid = conversation([0, 1, 2, 3, 4, 5], [50_000, 60_000, 70_000, 80_000, 90_000, 100_000]);
  assert.equal(CS.chooseClaudeTTL(rapid, T0 + 5.5 * MIN), '5m');
  const slow = conversation([0, 15, 35, 50, 80], [50_000, 52_000, 54_000, 56_000, 58_000]);
  assert.equal(CS.chooseClaudeTTL(slow, T0 + 200 * MIN), '1h');
});

test('chooseClaudeTTL: no pasa de 5 min a 1 h con la caché caliente (la reescribiría entera)', () => {
  const slow = conversation([0, 15, 35, 50, 80], [50_000, 52_000, 54_000, 56_000, 58_000], 300);
  assert.equal(CS.chooseClaudeTTL(slow, T0 + 82 * MIN), '5m');
  assert.equal(CS.chooseClaudeTTL(slow, T0 + 90 * MIN), '1h');
});

test('ttlCost: la caché caducada se reescribe; la caliente se lee', () => {
  const g = [{ gap: 600, context: 100_000, growth: 0 }];
  assert.equal(CS.ttlCost('5m', g), 125_000);
  assert.equal(CS.ttlCost('1h', g), 10_000);
});

test('cacheState y cacheLine: caliente, caducada y sin datos', () => {
  const l = conversation([0], [120_000], 300);
  const warm = CS.cacheState(l, 'claude-code', T0 + 2 * MIN)!;
  assert.equal(warm.warm, true);
  assert.equal(CS.cacheLine(warm, 'claude-code', T0 + 2 * MIN), 'Caché caliente: caduca en 3 min (dura 5 min desde la última llamada).');
  const cold = CS.cacheState(l, 'claude-code', T0 + 17 * MIN)!;
  assert.equal(cold.warm, false);
  const line = CS.cacheLine(cold, 'claude-code', T0 + 17 * MIN);
  assert.equal(line, 'Caché caducada hace 12 min: el próximo mensaje escribirá de nuevo en caché 120 k tokens.');
  assert.ok(!line.includes('$'));
  assert.match(CS.cacheLine(CS.cacheState(l, 'opencode', T0 + 17 * MIN), 'opencode', T0 + 17 * MIN), /reenviará 120 k tokens sin caché/);
  // Sin TTL en el consumo: el de Claude Code con suscripción (1 h).
  assert.equal(CS.cacheState(conversation([0], [1000]), 'claude-code', T0)!.ttl, 3600);
  assert.equal(CS.cacheState(conversation([0], [1000]), 'antigravity', T0)!.ttl, 120);
  assert.equal(CS.cacheState(conversation([0], [1000]), 'otro', T0), null);
  assert.equal(CS.cacheLine(null, 'otro'), '');
});

test('duration y cacheMinutes', () => {
  assert.equal(CS.duration(20_000), 'menos de 1 min');
  assert.equal(CS.duration(65 * MIN), '1 h 5 min');
  assert.equal(CS.duration(60 * MIN), '1 h');
  assert.equal(CS.cacheMinutes(conversation([0], [1000], 3600), 'claude-code'), 60);
  assert.equal(CS.cacheMinutes([], 'opencode'), 5);
});

test('con la caché de 1 h, 20 min parada no es «sin caché» (ni aviso ni relevo)', async () => {
  const CO = await import('./costes.ts');
  const W = await import('./workingMemory.ts');
  assert.match(CO.avisoContexto(120_000, 20, 'claude-code')!, /sin caché/);
  assert.equal(CO.avisoContexto(120_000, 20, 'claude-code', 60), null);
  const base = { agent: 'claude-code', mode: 'vig' as const, context: 120_000, idleMinutes: 20, hasAgentSession: true, memoryChars: 4000 };
  assert.ok(W.shouldRelay(base));
  assert.equal(W.shouldRelay({ ...base, cacheMinutes: 60 }), null);
});
