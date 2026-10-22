// Pruebas de la poda en la interfaz: npm test (node --test, sin dependencias ni tokens).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as P from './prune.ts';
import type { Mensaje } from './api.ts';

const conv: Mensaje[] = [
  { rol: 'user', texto: 'lee app.go' },
  { rol: 'assistant', texto: 'leído', punto: { sesion: 's', id: 'm1' } },
  { rol: 'user', texto: 'cambia el color' },
  { rol: 'assistant', texto: 'hecho', punto: { sesion: 's', id: 'm2' } },
  { rol: 'user', texto: 'sube la versión' },
  { rol: 'assistant', texto: '' },
];

test('el informe marca cada intercambio por su punto, con su mensaje del usuario', () => {
  const m = P.applyPruneReport(conv, { retirados: 20000, intercambios: [
    { punto: 'm1', nivel: 'L2', motivo: 'retirado del contexto: lecturas superadas' },
    { punto: 'm2', nivel: 'L3', motivo: 'fuera del contexto: sin relación con lo último · 0,15', similitud: 0.15 },
  ] });
  assert.equal(m[1].prune?.nivel, 'L2');
  assert.equal(m[0].prune?.nivel, 'L2');
  assert.equal(m[3].prune?.similitud, 0.15);
  assert.equal(m[2].prune?.nivel, 'L3');
  assert.equal(m[4].prune, undefined);
  assert.equal(P.pruneClass(m[3]), ' pruned-out');
  assert.equal(P.pruneClass(m[1]), ' pruned-rules');
  assert.equal(P.pruneClass(m[4]), '');
});

test('lo que ya no sale en el informe vuelve al contexto (sin marca)', () => {
  const m = P.applyPruneReport(conv, { retirados: 20000, intercambios: [{ punto: 'm2', nivel: 'L3', motivo: 'x' }] });
  const n = P.applyPruneReport(m, { retirados: 0, repuestos: 2, intercambios: [] });
  assert.equal(n[3].prune, undefined);
  assert.equal(n[2].prune, undefined);
  assert.equal('prune' in n[3], false);
  assert.equal(n[1], conv[1], 'lo que no cambia es el mismo objeto');
});

test('la línea del turno', () => {
  assert.equal(P.prunedLine({ tokens: 48_400 }), '✂ pgocode ha retirado 48k tokens del historial');
  assert.equal(P.prunedLine({ tokens: 20_000, repuestos: 1 }), '✂ pgocode ha retirado 20k tokens del historial; ha repuesto 1 pieza que vuelve a ser relevante');
  assert.equal(P.prunedLine({ tokens: 0, repuestos: 3 }), '↺ pgocode ha repuesto 3 piezas que vuelven a ser relevantes');
  assert.equal(P.prunedLine({ tokens: 0 }), '');
});

test('la poda está activada por defecto', () => {
  assert.equal(P.autoPruneOn({}), true);
  assert.equal(P.autoPruneOn({ autoPrune: false }), false);
});
