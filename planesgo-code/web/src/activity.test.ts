import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as A from './activity.ts';
import type { Mensaje } from './api.ts';

const h = (nombre: string, resumen: string, estado: 'curso' | 'ok' = 'curso') => ({ tipo: 'herramienta' as const, h: { id: nombre + resumen, nombre, resumen, estado } });

test('la herramienta en curso, con su fichero u orden', () => {
  const m: Mensaje = { rol: 'assistant', texto: '', partes: [h('Read', '/home/x/src/ProjectTable.tsx', 'ok'), h('Bash', 'npm run build')] };
  assert.equal(A.actividad(m), 'ejecutando npm run build');
  assert.equal(A.actividad({ ...m, partes: [h('read', '/a/b/inventario.py')] }), 'leyendo inventario.py');
  assert.equal(A.actividad({ ...m, partes: [h('edit_file', '/a/b/lineas.py')] }), 'editando lineas.py');
  assert.equal(A.actividad({ ...m, partes: [h('mcp__pgo__pgo_time', '')] }), 'usando pgo_time');
});

test('pensando, escribiendo y subagentes en marcha', () => {
  assert.equal(A.actividad({ rol: 'assistant', texto: '', partes: [{ tipo: 'pensamiento', texto: 'mm' }] }), 'pensando');
  assert.equal(A.actividad({ rol: 'assistant', texto: 'Voy', partes: [{ tipo: 'texto', texto: 'Voy' }] }), 'escribiendo la respuesta');
  const m: Mensaje = { rol: 'assistant', texto: '', partes: [{ tipo: 'texto', texto: 'Lanzo' }],
    tareas: [{ id: 't1', tipo: 'subagente', titulo: 'Revisar las pruebas de imputación', estado: 'curso' }] };
  assert.equal(A.actividad(m), 'escribiendo la respuesta · subagente «Revisar las pruebas de imputación» en marcha');
  assert.equal(A.actividad(undefined), '');
});

test('consumo por acción: la diferencia con la anterior; varias en la misma llamada, solo la primera', () => {
  const t = (id: string, tokens: number) => ({ tipo: 'herramienta', h: { id, nombre: 'Read', estado: 'ok' as const, tokens } });
  const r = A.consumoPorAccion([t('a', 120_000), { tipo: 'texto' }, t('b', 135_000), t('c', 135_000), t('d', 150_500)], 100_000);
  assert.deepEqual([...r], [['a', 20_000], ['b', 15_000], ['d', 15_500]]);
});

test('bolita de cada acción: hecha, en curso o el motivo del fallo', () => {
  assert.equal(A.toolStatusTitle({ estado: 'ok' }), 'Hecha');
  assert.equal(A.toolStatusTitle({ estado: 'curso' }), 'En curso');
  assert.equal(A.toolStatusTitle({ estado: 'error', salida: '\n  Error: oldString not found in content\nmás' }), 'Ha fallado: Error: oldString not found in content. El agente suele reintentarlo.');
  assert.match(A.toolStatusTitle({ estado: 'error' }), /no dio el motivo/);
});

test('silencio del agente: nada antes de un minuto; luego, cuánto', () => {
  assert.equal(A.silenceLabel(59_000), '');
  assert.equal(A.silenceLabel(NaN), '');
  assert.match(A.silenceLabel(3 * 60_000 + 5000), /hace 3 min/);
  // Con una orden en curso, la espera es de la orden, no del modelo.
  const m = { rol: 'assistant' as const, texto: '', partes: [{ tipo: 'herramienta' as const, h: { id: 't', nombre: 'Bash', estado: 'curso' as const, resumen: 'make lento' } }] };
  assert.equal(A.silenceLabel(4 * 60_000, m), ' · la orden sigue en marcha (4 min)');
  assert.match(A.silenceLabel(4 * 60_000, { ...m, partes: [] }), /el modelo está generando/);
});
