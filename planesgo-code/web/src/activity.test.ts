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
