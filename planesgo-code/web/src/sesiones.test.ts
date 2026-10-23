// Pruebas de lo que se guarda de las sesiones: npm test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { compacta, ordenaConHijas, sessionOrder, type Sesion } from './sesiones.ts';
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

test('ordenaConHijas: cada hija debajo de su madre, por orden de parte', () => {
  const s = (id: string, x: Partial<Sesion> = {}): Sesion => ({ id, nombre: id, proyecto: 1, carpeta: '/c', rama: 'main', agente: 'claude-code', creada: 0, ultima: 0, ...x });
  const l = [s('a'), s('h2', { madre: 'm', parte: 2 }), s('m'), s('h1', { madre: 'm', parte: 1 }), s('huerfana', { madre: 'borrada', parte: 1 }), s('z')];
  assert.deepEqual(ordenaConHijas(l).map(x => x.s.id + (x.hija ? '*' : '')), ['a', 'm', 'h1*', 'h2*', 'huerfana', 'z']);
  assert.deepEqual(ordenaConHijas([]), []);
});

test('sessionOrder: orden estable por creación; recibir título o acabar un turno (ultima) no la mueve', () => {
  const s = (id: string, creada: number, ultima: number): Sesion => ({ id, nombre: '', proyecto: 1, carpeta: '/c', rama: 'main', agente: 'claude-code', creada, ultima });
  const l = [s('vieja', 1, 99), s('nueva', 2, 3)].sort(sessionOrder);
  assert.deepEqual(l.map(x => x.id), ['nueva', 'vieja']);
});
