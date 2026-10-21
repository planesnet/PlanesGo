// Pruebas de lo que se guarda de las sesiones: npm test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { compacta } from './sesiones.ts';
import type { Mensaje } from './api.ts';

test('compacta: recorta las herramientas y el razonamiento de los mensajes antiguos, no los recientes', () => {
  const largo = 'x'.repeat(5000);
  const m = (i: number): Mensaje => ({ rol: 'assistant', texto: 'r' + i, partes: [
    { tipo: 'texto', texto: largo }, { tipo: 'pensamiento', texto: largo },
    { tipo: 'herramienta', h: { id: 'h' + i, nombre: 'Bash', entrada: largo, salida: largo, estado: 'ok' } }] });
  const l = [m(0), m(1), m(2)];
  const c = compacta(l, 1, 300);
  const h = (x: Mensaje) => (x.partes![2] as { h: { entrada?: string; salida?: string } }).h;
  assert.equal(h(c[0]).salida!.length, 301);
  assert.equal(h(c[0]).entrada!.length, 101);
  assert.equal((c[0].partes![1] as { texto: string }).texto.length, 301);
  assert.equal((c[0].partes![0] as { texto: string }).texto.length, 5000, 'el texto de la respuesta no se toca');
  assert.equal(c[2], l[2], 'el reciente queda igual');
  assert.equal(h(l[0]).salida!.length, 5000, 'no cambia el original');
});
