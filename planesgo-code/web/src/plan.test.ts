// Pruebas del modo plan: npm test (node --test, sin dependencias).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as P from './plan.ts';

test('primera petición: pide un plan en Markdown sin tocar nada', () => {
  const t = P.peticionPlan('Añade la cola de mensajes');
  assert.match(t, /Modo plan/);
  assert.match(t, /sin modificar ningún fichero/);
  assert.match(t, /No lo ejecutes/);
  assert.ok(t.endsWith('Petición:\nAñade la cola de mensajes'));
});

test('revisión: incluye el plan actual (editado) y lo pedido', () => {
  const t = P.peticionPlan('Quita el paso 3', '# Plan\n1. uno\n2. dos (editado a mano)\n3. tres\n');
  assert.match(t, /2\. dos \(editado a mano\)/);
  assert.match(t, /plan completo revisado/);
  assert.ok(t.endsWith('Quita el paso 3'));
  assert.equal(P.peticionPlan('x', '   '), P.peticionPlan('x'), 'un plan vacío cuenta como sin plan');
});

test('el plan es el texto tras la última herramienta', () => {
  const m = {
    rol: 'assistant' as const, texto: 'Voy a mirar el código.# Plan\n1. paso',
    partes: [
      { tipo: 'texto' as const, texto: 'Voy a mirar el código.' },
      { tipo: 'herramienta' as const, h: { id: '1', nombre: 'Read', estado: 'ok' as const } },
      { tipo: 'texto' as const, texto: '# Plan\n' },
      { tipo: 'texto' as const, texto: '1. paso\n' },
    ],
  };
  assert.equal(P.textoDelPlan(m), '# Plan\n1. paso');
});

test('sin herramientas: todo el texto; con error o del usuario: nada', () => {
  assert.equal(P.textoDelPlan({ rol: 'assistant', texto: '# Plan\nA' }), '# Plan\nA');
  assert.equal(P.textoDelPlan({ rol: 'assistant', texto: '# Plan', partes: [{ tipo: 'texto', texto: '# Plan' }] }), '# Plan');
  assert.equal(P.textoDelPlan({ rol: 'assistant', texto: '⚠ fallo', error: true }), '');
  assert.equal(P.textoDelPlan({ rol: 'user', texto: 'hola' }), '');
  assert.equal(P.textoDelPlan(undefined), '');
});

test('acaba en una herramienta: no hay plan', () => {
  const m = { rol: 'assistant' as const, texto: 'mirando', partes: [
    { tipo: 'texto' as const, texto: 'mirando' },
    { tipo: 'herramienta' as const, h: { id: '1', nombre: 'Grep', estado: 'ok' as const } },
  ] };
  assert.equal(P.textoDelPlan(m), '');
});

test('título: primer encabezado, sin formato; si no, la petición', () => {
  assert.equal(P.tituloPlan('Intro\n## Plan: **cola** de `mensajes`\n# Otro', 'x'), 'Plan: cola de mensajes');
  assert.equal(P.tituloPlan('sin títulos', 'Añade el modo plan\ncon detalles'), 'Añade el modo plan');
  assert.equal(P.tituloPlan('', ''), 'Plan');
  assert.equal(P.tituloPlan('# ' + 'a'.repeat(100), '').length, 80);
});

test('ejecución: indica la ruta y lleva el plan', () => {
  const t = P.peticionEjecucion('/p/.pgocode/planes/x.md', '\n# Plan\n1. a\n');
  assert.match(t, /\/p\/\.pgocode\/planes\/x\.md/);
  assert.ok(t.endsWith('# Plan\n1. a'));
});

test('órdenes de planes en el chat', () => {
  assert.deepEqual(P.interpreta('/plan Añade un fichero saludo.txt'), { tipo: 'nuevo', peticion: 'Añade un fichero saludo.txt' });
  assert.deepEqual(P.interpreta('/plan 2 quita el paso 3'), { tipo: 'revisar', numero: 2, peticion: 'quita el paso 3' });
  assert.deepEqual(P.interpreta('Plan #12: usa Go 1.24\ny añade pruebas'), { tipo: 'revisar', numero: 12, peticion: 'usa Go 1.24\ny añade pruebas' });
  assert.deepEqual(P.interpreta('plan 3, más corto'), { tipo: 'revisar', numero: 3, peticion: 'más corto' });
  assert.deepEqual(P.interpreta('Ejecuta el plan 4'), { tipo: 'ejecutar', numero: 4 });
  assert.deepEqual(P.interpreta('ejecutar plan #4.'), { tipo: 'ejecutar', numero: 4 });
  assert.deepEqual(P.interpreta('descarta el plan 2'), { tipo: 'descartar', numero: 2 });
  assert.deepEqual(P.interpreta('reactiva el plan 2'), { tipo: 'reactivar', numero: 2 });
  assert.deepEqual(P.interpreta('Borra el plan 2'), { tipo: 'borrar', numero: 2 });
  // Mensajes normales: nada.
  for (const t of ['hola', '¿qué opinas del plan 2?', 'ejecuta las pruebas', 'el plan 2 me gusta', '/planes', 'plan']) assert.equal(P.interpreta(t), null, t);
});
