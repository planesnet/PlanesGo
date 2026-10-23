// Pruebas del indicador del hilo de limpieza: npm test (node --test, sin dependencias ni tokens).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as C from './cleaner.ts';
import type { Mensaje } from './api.ts';

test('contextWindow y pct', () => {
  assert.equal(C.contextWindow('claude-code', 'haiku'), 200_000);
  assert.equal(C.contextWindow('claude-code', 'opus[1m]'), 1_000_000);
  assert.equal(C.contextWindow('antigravity', 'gemini-3.8-flash-low'), 1_048_576);
  assert.equal(C.pct(50_000, 200_000), 25);
  assert.equal(C.pct(1, 0), 0);
});

const conv: Mensaje[] = [
  { rol: 'user', texto: 'a' }, { rol: 'assistant', texto: 'b', uso: { entrada: 1, salida: 1, contexto: 100_000 } },
  { rol: 'user', texto: 'c' }, { rol: 'assistant', texto: 'd', uso: { entrada: 1, salida: 1, contexto: 124_000 } },
  { rol: 'user', texto: 'e' }, { rol: 'assistant', texto: 'f', pruned: { tokens: 42_000 }, uso: { entrada: 1, salida: 1, contexto: 82_000 } },
  { rol: 'user', texto: 'g', relevo: { de: 90_000, a: 3000, anterior: 'x', motivo: 'm' } }, { rol: 'assistant', texto: 'h', uso: { entrada: 1, salida: 1, contexto: 20_000 } },
  { rol: 'user', texto: 'i' }, { rol: 'assistant', texto: '' },
];

test('contextSeries: dientes de sierra: sube en cada turno y cae en cada limpieza', () => {
  const s = C.contextSeries(conv, 200_000);
  assert.deepEqual(s.map(p => p.tokens), [100_000, 124_000, 82_000, 82_000, 3000, 20_000]);
  assert.deepEqual(s.map(p => Boolean(p.drop)), [false, false, true, false, true, false]);
  assert.equal(s[2].pruned, 42_000);
  assert.equal(s[4].relay, true);
  assert.equal(C.pruneLabel(s, 2), '−42 k: 124 k → 82 k');
  assert.equal(C.pruneLabel(s, 4), 'relevo: 82 k → 3 k');
  assert.equal(C.pruneLabel(s, 1), '');
  assert.equal(C.contextSeries(conv, 200_000, 2).length, 1, 'los 2 últimos empiezan por una caída: se quita');
  assert.equal(C.contextSeries(conv, 200_000, 4)[0].drop, undefined, 'no empieza por una caída');
  const pts = C.sparkPoints(s, 100, 20);
  assert.equal(pts[0].x, 0);
  assert.equal(pts[2].x, pts[1].x, 'la caída es vertical');
  assert.ok(pts[2].y > pts[1].y, 'cae');
  assert.equal(pts[5].x, 100);
  assert.match(C.sparkPath(s, 100, 20), /^M0,[\d.]+ L/);
});

test('estado del recolector: línea, motor, piezas y lo que quedaría tras barrer', () => {
  assert.equal(C.statusLine({ fase: '', motor: 'ollama' }), 'al día');
  assert.equal(C.statusLine({ fase: 'marcando', pendientes: 3, motor: 'ollama' }), 'marcando 3 piezas');
  assert.equal(C.statusLine(null, true), 'marcando');
  assert.equal(C.statusLine({ fase: 'barriendo', motor: 'ollama' }), 'barriendo');
  assert.match(C.engineLine({ fase: '', motor: 'reglas' }), /solo reglas/);
  assert.match(C.engineLine({ fase: '', motor: 'ollama' }), /embeddinggemma/);
  assert.deepEqual(C.pieceCounts({ retirados: 0, intercambios: [], piezas: [{ id: 'a', vigencia: 4, fuera: true }, { id: 'b', vigencia: 1 }] }), { in: 1, out: 1 });
  assert.equal(C.projected(80_000, 30_000), 50_000);
  assert.equal(C.projected(10_000, 30_000), 0);
  assert.equal(C.ago(undefined), 'nunca');
  assert.equal(C.ago(1000, 1000 + 5 * 60000), 'hace 5 min');
});

const agy: Mensaje[] = [
  { rol: 'user', texto: 'viejo' }, { rol: 'assistant', texto: 'r0', uso: { entrada: 1, salida: 1 } },
  { rol: 'user', texto: 'Lee grande.py', relevo: { de: 1, a: 1, anterior: 'x', motivo: 'm' } },
  { rol: 'assistant', texto: '', fin: 5, uso: { entrada: 1, salida: 1 }, partes: [
    { tipo: 'herramienta', h: { id: 'paso-2', nombre: 'view_file', resumen: '/p/grande.py', salida: 'contenido', estado: 'ok' } },
    { tipo: 'texto', texto: 'Tiene 700 funciones.' }] },
  { rol: 'user', texto: 'Otra' }, { rol: 'assistant', texto: 'Vale', uso: { entrada: 1, salida: 1 }, partes: [
    { tipo: 'herramienta', h: { id: 'paso-2', nombre: 'run_command', resumen: 'ls', estado: 'error' } }, { tipo: 'texto', texto: 'Vale' }] },
  { rol: 'user', texto: 'En curso' }, { rol: 'assistant', texto: '' },
];

test('chatExchanges: desde el último relevo, con ids únicos por posición', () => {
  const xs = C.chatExchanges(agy);
  assert.equal(C.relayStart(agy), 2);
  assert.equal(xs.length, 2);
  assert.deepEqual(xs[0], { punto: 'm3', usuario: 'Lee grande.py', respuesta: 'Tiene 700 funciones.', en: 5,
    herramientas: [{ id: 'm3:paso-2', nombre: 'view_file', resumen: '/p/grande.py', salida: 'contenido' }] });
  assert.equal(xs[1].herramientas![0].id, 'm5:paso-2');
  assert.equal(xs[1].herramientas![0].error, true);
});

test('applyChatVigencia: cada intercambio y herramienta por su posición; markRelayedOut', () => {
  const r = { retirados: 0, intercambios: [{ punto: 'm3', vigencia: 3.4, superada: '' }, { punto: 'm5', vigencia: 0 }],
    piezas: [{ id: 'm3:paso-2', vigencia: 4.1, superada: 'lectura superada' }, { id: 'm5:paso-2', vigencia: 0 }] };
  const m = C.applyChatVigencia(agy, r);
  assert.equal(m[3].vig?.vigencia, 3.4);
  assert.equal(m[2].vig?.vigencia, 3.4);
  assert.equal(m[0], agy[0], 'lo de antes del relevo, igual');
  const h3 = m[3].partes![0];
  const h5 = m[5].partes![0];
  assert.ok(h3.tipo === 'herramienta' && h3.h.vig?.superada === 'lectura superada');
  assert.ok(h5.tipo === 'herramienta' && h5.h.vig?.vigencia === 0);
  assert.equal(C.applyChatVigencia(m, r)[3], m[3], 'sin cambios, el mismo objeto');
  const f = C.markRelayedOut(m);
  assert.equal(f[3].vig?.fuera, true);
  assert.equal(f[5].vig?.fuera, undefined);
});

test('chatExchanges: lo anterior a la última compactación del agente va marcado', () => {
  const ms: Mensaje[] = [
    { rol: 'user', texto: 'uno' }, { rol: 'assistant', texto: 'r1', uso: { entrada: 1, salida: 1 } },
    { rol: 'user', texto: 'dos' }, { rol: 'assistant', texto: '_(Antigravity ha compactado su historial.)_ r2', uso: { entrada: 1, salida: 1 } },
    { rol: 'user', texto: 'tres' }, { rol: 'assistant', texto: 'r3', uso: { entrada: 1, salida: 1 } },
  ];
  const xs = C.chatExchanges(ms);
  assert.equal(C.lastCompaction(ms), 3);
  assert.deepEqual(xs.map(x => Boolean(x.compactado)), [true, false, false]);
});

test('Barrer ahora: nota en el chat, marca en la evolución, aviso de la caché y qué hará', () => {
  const ms: Mensaje[] = [
    { rol: 'user', texto: 'a' }, { rol: 'assistant', texto: 'b', uso: { entrada: 1, salida: 1, contexto: 100_000 } },
  ];
  const n = C.sweepNote(23_000, 0, 5);
  assert.deepEqual(n, { rol: 'assistant', texto: '', nota: true, fin: 5, pruned: { tokens: 23_000 } });
  const s = C.contextSeries([...ms, n], 200_000);
  assert.equal(s.length, 2);
  assert.deepEqual(s[1], { tokens: 77_000, pct: 38.5, drop: true, pruned: 23_000 });
  assert.equal(C.pruneLabel(s, 1), '−23 k: 100 k → 77 k');
  const r = { retirados: 0, intercambios: [], pendiente: 30_000, cache_caliente: true, cache_resto: 40_000 };
  assert.equal(C.cacheWarning('claude-code', r), 'invalida la caché desde ahí: ~40 k tokens a precio de escritura');
  assert.equal(C.cacheWarning('opencode', r), 'invalida la caché desde ahí: ~40 k tokens a precio de entrada');
  assert.equal(C.cacheWarning('opencode', { ...r, cache_caliente: false }), '');
  assert.match(C.sweepHint('claude-code', { ...r, forzado_pendiente: true }), /próximo turno/);
  assert.equal(C.sweepHint('opencode', r), '');
  assert.match(C.sweepHint('opencode', { retirados: 0, intercambios: [] }), /nada fuera de vigencia/);
  assert.match(C.sweepHint('antigravity', null, true), /próximo envío/);
});

test('calibración: pesos, relecturas y ahorro en una línea', () => {
  const c = { pesos: { antiguedad: 0.12, pasos: 0.8, tiempo: 0.15, relacion: 20, pivote: 0.5 }, fecha: 1, piezas: 300, sesiones: 2,
    retiradas: 100, tokens: 180_000, tokens_paso: 3_000_000, de_mas: 2, retiradas_generales: 99, tokens_generales: 176_000, de_mas_generales: 4 };
  assert.equal(C.paramsLine(c), 'antigüedad 0,12 · pasos 0,8 · tiempo 0,15 · relación 20 (0,5)');
  assert.equal(C.overLine(c), '2 de 100 retiradas (2 %); con los generales, 4 de 99 (4 %)');
  assert.equal(C.savedLine(c), '180 k tokens retirados (3000 k tokens·paso); con los generales, 176 k');
});
