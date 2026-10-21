// Pruebas de las tareas de los agentes: npm test (node --test, sin dependencias).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as T from './tareas.ts';

test('fusiona: alta, actualización parcial y conserva lo que no llega', () => {
  let l = T.fusiona(undefined, { id: 'a', tipo: 'subagente', titulo: 'Leer', estado: 'curso', herramienta: 'h1' });
  l = T.fusiona(l, { id: 'a', progreso: 'Bash · 1 herramientas', estado: 'curso' });
  l = T.fusiona(l, { id: 'a', estado: 'ok', salida: 'hecho' });
  l = T.fusiona(l, { id: 'b', tipo: 'segundo_plano', segundo_plano: true, estado: 'curso' });
  assert.equal(l.length, 2);
  assert.deepEqual(l[0], { id: 'a', tipo: 'subagente', titulo: 'Leer', estado: 'ok', herramienta: 'h1', progreso: 'Bash · 1 herramientas', salida: 'hecho' });
});

test('fusiona: la lista se sustituye entera', () => {
  let l = T.fusiona(undefined, { id: 'lista', tipo: 'lista', elementos: [{ texto: 'uno', estado: 'pendiente' }] });
  l = T.fusiona(l, { id: 'lista', elementos: [{ texto: 'uno', estado: 'hecho' }, { texto: 'dos', estado: 'pendiente' }] });
  assert.deepEqual(l[0].elementos, [{ texto: 'uno', estado: 'hecho' }, { texto: 'dos', estado: 'pendiente' }]);
});

test('vivas y fin del turno', () => {
  const l: T.Tarea[] = [{ id: 'a', tipo: 'subagente', estado: 'curso' }, { id: 'b', tipo: 'segundo_plano', estado: 'ok' }, { id: 'lista', tipo: 'lista', estado: 'curso' }];
  assert.deepEqual(T.vivas(l).map(t => t.id), ['a']);
  assert.equal(T.alAcabar(l, true)![0].estado, 'parada');
  assert.equal(T.alAcabar(l, false)![0].estado, 'ok');
  assert.equal(T.alAcabar(l, false)![2].estado, 'curso', 'la lista no cambia');
  assert.equal(T.alAcabar(undefined, false), undefined);
});

test('la tarea de una herramienta', () => {
  const l: T.Tarea[] = [{ id: 'lista', tipo: 'lista', herramienta: 'x' }, { id: 'a', tipo: 'subagente', herramienta: 'h1' }];
  assert.equal(T.deHerramienta(l, 'h1')?.id, 'a');
  assert.equal(T.deHerramienta(l, 'x'), undefined);
});
