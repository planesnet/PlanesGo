// Pruebas de los avisos y límites de coste: npm test (node --test, sin dependencias).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as C from './costes.ts';

const uso = (coste: number, entrada: number, salida = 0) => ({ rol: 'assistant' as const, texto: '', uso: { coste, entrada, salida } });

test('gasto: suma los consumos de las respuestas', () => {
  const g = C.gasto([{ rol: 'user', texto: 'x' }, uso(0.5, 1000, 10), uso(1.98, 1_026_899, 3866), { rol: 'assistant', texto: 'sin uso' }]);
  assert.equal(g.turnos, 2);
  assert.ok(Math.abs(g.coste - 2.48) < 1e-9);
  assert.equal(g.entrada + g.salida, 1_031_775);
});

test('efectivos: lo de la sesión y, si falta, lo del agente', () => {
  assert.deepEqual(C.efectivos('opencode', { avisoCoste: 0.2 }), { avisoCoste: 0.2, limiteCoste: 3, avisoTurno: 0.5 });
  assert.deepEqual(C.efectivos('otro', undefined), {});
  assert.equal(C.efectivos('opencode', { limiteCoste: undefined }).limiteCoste, 3, 'vacío no pisa el del agente');
});

test('nivel según los umbrales en $ y en tokens', () => {
  const a = { avisoCoste: 1, limiteCoste: 3, avisoTokens: 1_000_000 };
  assert.equal(C.nivel({ coste: 0.5, entrada: 10, salida: 0, turnos: 1 }, a), 'ok');
  assert.equal(C.nivel({ coste: 1.2, entrada: 10, salida: 0, turnos: 1 }, a), 'aviso');
  assert.equal(C.nivel({ coste: 0.1, entrada: 2_000_000, salida: 0, turnos: 1 }, a), 'aviso');
  assert.equal(C.nivel({ coste: 3, entrada: 10, salida: 0, turnos: 1 }, a), 'limite');
});

test('nuevos: solo al cruzar el umbral, y el turno caro', () => {
  const a = { avisoCoste: 1, limiteCoste: 3, avisoTurno: 0.5 };
  const g = (coste: number) => ({ coste, entrada: 0, salida: 0, turnos: 1 });
  assert.deepEqual(C.nuevos(g(0.2), g(0.4), a), []);
  assert.deepEqual(C.nuevos(g(0.8), g(1.1), a).map(x => x.nivel), ['aviso']);
  assert.deepEqual(C.nuevos(g(1.1), g(1.3), a), [], 'ya avisado: no se repite');
  const l = C.nuevos(g(1.5), g(3.2), a);
  assert.deepEqual(l.map(x => x.nivel), ['limite', 'aviso']);
  assert.match(l[0].texto, /límite de 3,00 \$ \(lleva 3,20 \$\)/);
  assert.match(l[1].texto, /turno ha costado 1,70 \$/);
});

test('formatos', () => {
  assert.equal(C.dinero(2.4806), '2,48 $');
  assert.equal(C.dinero(0.004), '<0,01 $');
  assert.equal(C.tokens(1_027_899), '1,0 M');
  assert.equal(C.tokens(45_300), '45 k');
});
