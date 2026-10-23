// Pruebas de la actividad en vivo (panel Actividad): npm test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as LA from './liveActivity.ts';
import * as T from './tareas.ts';
import { currentAction } from './activity.ts';
import type { Mensaje } from './api.ts';

const snap = (x: Partial<LA.LiveSnapshot>): LA.LiveSnapshot => ({ sesion: 's', tareas: [], trabajando: false, ...x });

test('elapsed: segundos, minutos y horas, como un monitor', () => {
  assert.equal(LA.elapsed(8_400), '8 s');
  assert.equal(LA.elapsed(125_000), '2:05');
  assert.equal(LA.elapsed(3_780_000), '1 h 03 min');
  assert.equal(LA.elapsed(-5), '0 s');
});

test('todoView: la tarea en curso, el progreso y la siguiente', () => {
  const lista: T.Tarea = { id: 'lista', tipo: 'lista', elementos: [
    { texto: 'Leer', estado: 'hecho' }, { texto: 'Editar', estado: 'curso' }, { texto: 'Probar', estado: 'pendiente' }] };
  const v = LA.todoView([{ id: 'x', tipo: 'subagente' }, lista])!;
  assert.equal(v.done, 1);
  assert.equal(v.total, 3);
  assert.equal(v.current?.texto, 'Editar');
  assert.equal(v.next?.texto, 'Probar');
  assert.equal(v.finished, false);
  assert.equal(LA.todoView([]), undefined);
  // La última lista manda (la de turnos anteriores no).
  const otra: T.Tarea = { id: 'lista', tipo: 'lista', elementos: [{ texto: 'A', estado: 'hecho' }] };
  assert.equal(LA.todoView([lista, otra])!.finished, true);
});

test('agentRow: trabajando, con lo que hace, el tiempo del turno, tokens y % de contexto', () => {
  const r = LA.agentRow(snap({ trabajando: true, accion: 'editando Actividad.tsx', inicio: 1000, tokensTurno: 46_500, contexto: 30_000, umbral: 150_000, enCola: 2 }), 1000 + 83_000);
  assert.equal(r.dot, 'curso');
  assert.equal(r.name, 'trabajando · 2 en cola');
  assert.equal(r.detail, 'editando Actividad.tsx');
  assert.equal(r.time, '1:23');
  assert.equal(r.tokens, '47 k · 20 %');
  assert.match(r.title, /20 % de 150 k/);
});

test('agentRow: libre (con cola, el punto de la cola) y sin instantánea', () => {
  assert.equal(LA.agentRow(snap({}), 0).name, 'libre');
  assert.equal(LA.agentRow(snap({}), 0).time, '');
  assert.equal(LA.agentRow(snap({ enCola: 1, colaParada: true }), 0).dot, 'cola');
  assert.equal(LA.agentRow(snap({ enCola: 1, colaParada: true }), 0).name, 'libre · 1 en cola (parada)');
  assert.equal(LA.agentRow(undefined, 0).dot, 'libre');
});

test('taskRow: subagente en marcha con su último paso, tiempo y tokens; terminado con lo que devolvió', () => {
  const t: T.Tarea = { id: 'a', tipo: 'subagente', subtipo: 'Explore', titulo: 'Revisar el panel', estado: 'curso', progreso: 'Reading activity.ts · Read · 1 herramientas · 14200 tokens', inicio: 1000 };
  const r = LA.taskRow(t, 13_000);
  assert.deepEqual([r.dot, r.kind, r.name, r.detail, r.time, r.tokens], ['curso', 'subagente', 'Revisar el panel', 'Reading activity.ts', '12 s', '14 k']);
  assert.match(r.title, /Explore/);
  const h = LA.taskRow({ ...t, estado: 'ok', salida: 'Todo bien', fin: 10_000 }, 60_000);
  assert.deepEqual([h.dot, h.detail, h.time], ['ok', 'Todo bien', '9 s']);
  const f = LA.taskRow({ id: 'b', tipo: 'segundo_plano', titulo: 'make test', estado: 'error' }, 0);
  assert.deepEqual([f.dot, f.kind, f.detail], ['error', 'segundo plano', 'con error']);
});

test('liveLists: en marcha y terminados por separado (los últimos primero), sin la lista de tareas', () => {
  const s = snap({ trabajando: true, tareas: [
    { id: 'l', tipo: 'lista', elementos: [] },
    { id: 's1', tipo: 'subagente', titulo: 'uno', estado: 'ok' },
    { id: 's2', tipo: 'subagente', titulo: 'dos', estado: 'ok' },
    { id: 's3', tipo: 'subagente', titulo: 'tres', estado: 'curso' },
    { id: 'b1', tipo: 'segundo_plano', titulo: 'servidor', estado: 'curso' },
  ], apartes: [{ id: 'a', pregunta: '¿qué rama?' }] });
  const l = LA.liveLists(s, 0);
  assert.deepEqual(l.subagents.map(r => r.name), ['tres']);
  assert.deepEqual(l.subagentsDone.map(r => r.name), ['dos', 'uno']);
  assert.deepEqual(l.background.map(r => r.name), ['servidor']);
  assert.equal(l.backgroundDone.length, 0);
  assert.equal(l.asides[0].name, '¿qué rama?');
  assert.equal(LA.liveLists(undefined, 0).subagents.length, 0);
});

test('needsPulse: solo si algo corre', () => {
  assert.equal(LA.needsPulse(undefined), false);
  assert.equal(LA.needsPulse(snap({})), false);
  assert.equal(LA.needsPulse(snap({ trabajando: true })), true);
  assert.equal(LA.needsPulse(snap({ tareas: [{ id: 'b', tipo: 'segundo_plano', estado: 'curso' }] })), true);
});

test('publishSnapshot: la última instantánea queda para el panel que se abre a mitad de turno', () => {
  LA.publishSnapshot(snap({ sesion: 'p1', trabajando: true, accion: 'pensando' }), 0);
  LA.publishSnapshot(snap({ sesion: 'p1', trabajando: true, accion: 'leyendo a.ts' }), 1);
  assert.equal(LA.lastSnapshot('p1')?.accion, 'leyendo a.ts');
  assert.equal(LA.lastSnapshot('otra'), undefined);
});

test('conTiempos: inicio la primera vez, fin al dejar de estar en curso; alAcabar cierra las que quedan', () => {
  let l: T.Tarea[] = [];
  l = T.fusiona(l, T.conTiempos(l, { id: 'a', tipo: 'subagente', estado: 'curso' }, 100));
  l = T.fusiona(l, T.conTiempos(l, { id: 'a', progreso: 'x', estado: 'curso' }, 200));
  assert.equal(l[0].inicio, 100);
  assert.equal(l[0].fin, undefined);
  l = T.fusiona(l, T.conTiempos(l, { id: 'a', estado: 'ok' }, 300));
  assert.equal(l[0].fin, 300);
  l = T.fusiona(l, T.conTiempos(l, { id: 'a', estado: 'ok', salida: 'hecho' }, 400));
  assert.equal(l[0].fin, 300);
  assert.equal(T.conTiempos(l, { id: 'lista', tipo: 'lista' }, 5).inicio, undefined);
  const c = T.alAcabar([{ id: 'b', tipo: 'segundo_plano', estado: 'curso', inicio: 1 }], false, 50)!;
  assert.deepEqual([c[0].estado, c[0].fin], ['ok', 50]);
});

test('currentAction: la acción en curso, pensando, escribiendo o esperando al modelo', () => {
  const h = (nombre: string, resumen: string, estado: 'curso' | 'ok' = 'curso') => ({ tipo: 'herramienta' as const, h: { id: nombre + resumen, nombre, resumen, estado } });
  const m = (partes: Mensaje['partes']): Mensaje => ({ rol: 'assistant', texto: '', partes });
  assert.equal(currentAction(m([h('Bash', 'make test')])), 'ejecutando make test');
  assert.equal(currentAction(m([h('Edit', '/a/b/Actividad.tsx')])), 'editando Actividad.tsx');
  assert.equal(currentAction(m([h('Agent', 'Revisar el panel')])), 'esperando al subagente «Revisar el panel»');
  assert.equal(currentAction(m([{ tipo: 'pensamiento', texto: 'x' }]), 1000), 'pensando');
  assert.equal(currentAction(m([{ tipo: 'pensamiento', texto: 'x' }]), 45_000), 'esperando al modelo');
  assert.equal(currentAction(m([{ tipo: 'texto', texto: 'x' }])), 'escribiendo la respuesta');
  assert.equal(currentAction(m([h('Read', '/a.ts', 'ok')])), 'esperando al modelo');
  assert.equal(currentAction(m([])), 'esperando al modelo');
  assert.equal(currentAction(m([]), 125_000), 'esperando al modelo · 2 min sin novedades');
  assert.equal(currentAction(m([h('Bash', 'sleep 300')]), 61_000), 'ejecutando sleep 300 · 1 min sin novedades');
});
