// Pruebas del reparto en sesiones paralelas: npm test (node --test, sin dependencias).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as R from './reparto.ts';
import type { Mensaje } from './api.ts';

const prop = (partes: string[], x: Partial<R.Propuesta> = {}): R.Propuesta => ({ partes, max: 4, espera_s: 8, rama_madre: 'main', ...x });
const reparto = (n: number) => R.nuevo('r1', { id: 'm', proyecto: 1, carpeta: '/c' }, prop(Array.from({ length: n }, (_, i) => `hacer la cosa ${i + 1}`)), 1000, i => 'h' + i);

test('numeración de las hijas: no se repite entre repartos de una misma madre', () => {
  const r1 = reparto(3);
  assert.deepEqual(r1.hijas.map(h => [h.n, h.idx, h.sesion]), [[1, 1, 'h1'], [2, 2, 'h2'], [3, 3, 'h3']]);
  assert.equal(R.hijasDe([r1], 'm'), 3);
  assert.equal(R.hijasDe([r1], 'otra'), 0);
  const r2 = R.nuevo('r2', { id: 'm', proyecto: 1, carpeta: '/c' }, prop(['a', 'b', 'c']), 2000, i => 'h' + i, R.hijasDe([r1], 'm'));
  assert.deepEqual(r2.hijas.map(h => [h.n, h.idx, h.sesion]), [[1, 4, 'h4'], [2, 5, 'h5'], [3, 6, 'h6']]);
});

test('candidata: lista de tres o más, sesión que edita, texto largo', () => {
  const t = 'Necesito esto en la aplicación:\n1. Corregir el redondeo\n2. Añadir el campo referencia\n3. Traducir el informe';
  assert.equal(R.candidata(t, 'editar'), true);
  assert.equal(R.candidata(t, 'auto'), true);
  assert.equal(R.candidata(t, 'lectura'), false);
  assert.equal(R.candidata(t, undefined), false);
  assert.equal(R.candidata('/plan\n- a\n- b\n- c', 'editar'), false);
  assert.equal(R.candidata('Haz esto:\n- a\n- b', 'editar'), false);
  assert.equal(R.candidata('- uno\n- dos\n- tres', 'editar'), false, 'muy corta');
});

test('restante: cuenta atrás hacia 0', () => {
  assert.equal(R.restante(8000, 0), 8);
  assert.equal(R.restante(8000, 1), 8);
  assert.equal(R.restante(8000, 7001), 1);
  assert.equal(R.restante(8000, 8000), 0);
  assert.equal(R.restante(8000, 9000), 0);
});

test('mensajeHija: su parte, el encargo común y una línea con las demás', () => {
  const m = R.mensajeHija({ intro: 'En el módulo de facturas:', outro: 'Ejecuta las pruebas.', partes: ['Corrige el IVA', 'Añade el campo ' + 'x'.repeat(100), 'Traduce'] }, 2);
  assert.match(m, /^En el módulo de facturas:\n\nAñade el campo x+\n\nEjecuta las pruebas\./);
  assert.match(m, /Es una de 3 partes independientes/);
  assert.match(m, /1\) Corrige el IVA; 3\) Traduce/);
  assert.doesNotMatch(m, /2\) Añade/, 'no repite la suya en la lista de las otras');
  assert.ok(m.length < 600, 'corto');
  assert.doesNotMatch(R.mensajeHija({ intro: '', outro: '', partes: ['a', 'b', 'c'] }, 1), /^\n/);
});

test('cola: como mucho cuatro a la vez, el resto espera', () => {
  let r = reparto(6);
  const primeras = R.aLanzar(r);
  assert.deepEqual(primeras.map(h => h.n), [1, 2, 3, 4]);
  for (const h of primeras) r = R.pon(r, h.n, { estado: 'trabajando' });
  assert.equal(R.activas(r), 4);
  assert.deepEqual(R.aLanzar(r), []);
  r = R.pon(r, 2, { estado: 'hecha' });
  assert.deepEqual(R.aLanzar(r).map(h => h.n), [5]);
  r = R.pon(r, 5, { estado: 'trabajando' });
  r = R.pon(r, 1, { estado: 'parada' });
  assert.deepEqual(R.aLanzar(r).map(h => h.n), [6]);
  assert.equal(R.acabado(r), false);
  assert.equal(R.aLanzar(reparto(3)).length, 3);
});

test('acabado y seUne', () => {
  let r = reparto(3);
  for (const h of r.hijas) r = R.pon(r, h.n, { estado: 'hecha' });
  assert.equal(R.acabado(r), true);
  assert.equal(R.seUne(r.hijas[0]), true);
  r = R.pon(r, 1, { union: 'unida', quitada: true });
  assert.equal(R.seUne(r.hijas[0]), false);
  r = R.pon(r, 2, { union: 'conflicto' });
  assert.equal(R.seUne(r.hijas[1]), false, 'un conflicto no se reintenta a ciegas');
  r = R.pon(r, 3, { union: 'pendiente' });
  assert.equal(R.seUne(r.hijas[2]), true);
  assert.equal(R.seUne({ ...r.hijas[2], estado: 'trabajando' }), false);
  assert.equal(R.seUne({ ...r.hijas[2], estado: 'error' }), false);
  assert.equal(R.seUne({ ...r.hijas[2], estado: 'parada', union: 'sin' }), true, 'una parada se puede unir a mano');
});

test('alCargar: las que trabajaban sin turno quedan paradas', () => {
  let r = reparto(3);
  r = R.pon(r, 1, { estado: 'trabajando' });
  r = R.pon(r, 2, { estado: 'trabajando' });
  r = R.pon(r, 3, { estado: 'hecha' });
  const [x] = R.alCargar([r], s => s === 'h2');
  assert.deepEqual(x.hijas.map(h => h.estado), ['parada', 'trabajando', 'hecha']);
});

test('estadoAlAcabar', () => {
  const a = (x: Partial<Mensaje>): Mensaje => ({ rol: 'assistant', texto: 'ok', ...x });
  assert.equal(R.estadoAlAcabar(false, a({})), 'hecha');
  assert.equal(R.estadoAlAcabar(true, a({})), 'parada');
  assert.equal(R.estadoAlAcabar(false, a({ error: true })), 'error');
  assert.equal(R.estadoAlAcabar(false, undefined), 'error');
  assert.equal(R.estadoAlAcabar(false, { rol: 'user', texto: 'x' }), 'error');
});

test('ultimaRespuesta: la última del agente que dice algo', () => {
  const ms: Mensaje[] = [{ rol: 'user', texto: 'x' }, { rol: 'assistant', texto: ' Hecho. ' }, { rol: 'assistant', texto: '', partes: [] }];
  assert.equal(R.ultimaRespuesta(ms), 'Hecho.');
  assert.equal(R.ultimaRespuesta([]), '');
});

test('resumen: qué hizo cada parte, tokens y coste de cada una y total, qué se unió y qué no', () => {
  let r = reparto(3);
  r = R.pon(r, 1, { estado: 'hecha', union: 'unida', quitada: true });
  r = R.pon(r, 2, { estado: 'hecha', union: 'conflicto', ficheros: ['a.py', 'b.py'], rama: 'pgo/m-2', carpeta: '/ramas/x' });
  r = R.pon(r, 3, { estado: 'parada', rama: 'pgo/m-3', carpeta: '/ramas/y' });
  const t = R.resumen(r, {
    h1: { respuesta: 'Corregido el IVA.\n\nY más', entrada: 1000, salida: 500, coste: 0.5 },
    h2: { respuesta: 'x'.repeat(1000), entrada: 2000, salida: 500, coste: 0.25 },
    h3: { respuesta: '', entrada: 0, salida: 0, coste: 0 },
  });
  assert.match(t, /Reparto en 3 sesiones paralelas/);
  assert.match(t, /- \*\*Parte 1\*\* \(hacer la cosa 1\) · hecha {2}\n {2}Respondió: «Corregido el IVA\. Y más» {2}\n {2}2 k tokens · 0,50 \$ {2}\n {2}Unión: unida \(subcarpeta quitada\)/);
  assert.match(t, /Unión: NO unida: choca en a\.py, b\.py\. Se conserva la rama «pgo\/m-2» \(subcarpeta en \/ramas\/x\)/);
  assert.match(t, /Parte 3\*\* .* · parada[\s\S]*Unión: parada: sin unir\. Se conserva la rama «pgo\/m-3»/);
  assert.match(t, /\*\*Total:\*\* 4 k tokens · 0,75 \$/);
  assert.match(t, /\*\*Unidas:\*\* 1 de 3 · sin unir: 2, 3/);
  assert.ok(!/x{401}/.test(t), 'las respuestas se recortan');
  assert.match(R.avisoConflicto(r.hijas[1]), /parte 2 choca .* a\.py, b\.py/);
});

test('frases de unión pendiente y con error', () => {
  const h = (x: Partial<R.Hija>): R.Hija => ({ n: 1, parte: 'p', sesion: 's', estado: 'hecha', union: 'sin', ...x });
  assert.match(R.fraseUnion(h({ union: 'pendiente', motivo: 'la madre tiene cambios' })), /pendiente de unir \(la madre tiene cambios\).*Unir cambios/);
  assert.match(R.fraseUnion(h({ union: 'error', motivo: 'boom', rama: 'pgo/x-1' })), /NO unida: boom/);
  assert.equal(R.fraseUnion(h({ union: 'sin_cambios', quitada: true })), 'sin cambios que unir (subcarpeta quitada)');
});

test('una parte cuya copia no se llegó a crear no habla de ramas', () => {
  const h = { n: 1, idx: 1, parte: 'x', sesion: 's', estado: 'error', union: 'sin', motivo: 'la carpeta tiene que estar dentro de /home/luis' } as unknown as R.Hija;
  assert.equal(R.fraseUnion(h), 'no empezó: la carpeta tiene que estar dentro de /home/luis');
  assert.doesNotMatch(R.resumen({ id: 'r', madre: 'm', hijas: [h] } as unknown as R.Reparto, {}), /undefined/);
});

test('resumen: los bloques de texto de la respuesta de una hija no se pegan', () => {
  const m: Mensaje = { rol: 'assistant', texto: 'Respeto su estilo.Sigo con el resto.', partes: [
    { tipo: 'texto', texto: 'Respeto su estilo.' }, { tipo: 'herramienta', h: { id: '1', nombre: 'Edit', estado: 'ok' } }, { tipo: 'texto', texto: 'Sigo con el resto.' }] };
  assert.equal(R.ultimaRespuesta([m]), 'Respeto su estilo.\n\nSigo con el resto.');
  const r = R.pon(reparto(3), 1, { estado: 'hecha', union: 'unida' });
  assert.match(R.resumen(r, { h1: { respuesta: R.ultimaRespuesta([m]), entrada: 1, salida: 1, coste: 0 } }), /estilo\. Sigo/);
  assert.equal(R.ultimaRespuesta([{ rol: 'assistant', texto: 'solo texto' }]), 'solo texto');
});

test('conflicto: resolver con el agente de la madre y volver a unir', () => {
  const r = R.pon(reparto(3), 2, { estado: 'hecha', union: 'conflicto', rama: 'pgo/m-2', carpeta: '/c-2', ficheros: ['a.go'] });
  const h = r.hijas[1];
  assert.ok(!R.seUne(h));
  const msg = R.mensajeResolver(h, 'main');
  assert.match(msg, /git merge pgo\/m-2/);
  assert.match(msg, /`a\.go`/);
  assert.match(msg, /hacer la cosa 2/);
  assert.ok(R.seUne({ ...h, resolving: true }));
});
