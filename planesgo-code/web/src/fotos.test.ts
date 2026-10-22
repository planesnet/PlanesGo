import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as F from './fotos.ts';
import type { Mensaje } from './api.ts';

test('imágenes de un texto Markdown: solo las que el navegador puede cargar', () => {
  const md = 'Mira: ![Opción 1](http://localhost:3005/a.png?v=2) y ![](/design-options/b.png) ![x](data:image/png;base64,AAA) ![c](<https://e.com/c d.png>)';
  assert.deepEqual(F.imagenesMarkdown(md).map(f => f.src), ['http://localhost:3005/a.png?v=2', 'data:image/png;base64,AAA']);
  assert.equal(F.imagenesMarkdown(md)[0].titulo, 'Opción 1');
});

test('fotos de la conversación: de la más reciente a la más antigua, sin repetir', () => {
  const url = (r: string) => '/api/evidencia?ruta=' + r;
  const ms: Mensaje[] = [
    { rol: 'user', texto: '![no](http://x/u.png)' },
    { rol: 'assistant', texto: '', partes: [
      { tipo: 'herramienta', h: { id: '1', nombre: 'read', estado: 'ok', capturas: ['a.png'] } },
      { tipo: 'texto', texto: '![uno](http://x/1.png)' },
    ] },
    { rol: 'assistant', texto: '![dos](http://x/2.png) ![uno otra vez](http://x/1.png)' },
  ];
  assert.deepEqual(F.fotosDe(ms, url).map(f => f.src), ['http://x/1.png', 'http://x/2.png', '/api/evidencia?ruta=a.png']);
  assert.equal(F.fotosDe(ms, url)[2].titulo, 'read');
});
