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

test('contextSeries: el % de cada turno, con las podas y los relevos marcados', () => {
  const s = C.contextSeries(conv, 200_000);
  assert.deepEqual(s.map(p => p.pct), [50, 62, 41, 10]);
  assert.equal(s[2].pruned, 42_000);
  assert.equal(s[3].relay, true);
  assert.equal(C.pruneLabel(s, 2), '−42 k, 62 % → 41 %');
  assert.equal(C.pruneLabel(s, 1), '');
  assert.equal(C.contextSeries(conv, 200_000, 2).length, 2);
  const pts = C.sparkPoints(s, 100, 20);
  assert.equal(pts[0].x, 0);
  assert.equal(pts[3].x, 100);
  assert.ok(pts[1].y < pts[0].y, 'más contexto, más arriba');
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
