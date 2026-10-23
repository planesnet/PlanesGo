import { test } from 'node:test';
import assert from 'node:assert/strict';
import { estimateMemoryTokens, memoryState } from './projectMemory.ts';

test('estimateMemoryTokens: sin el título ni los comentarios', () => {
  assert.equal(estimateMemoryTokens('# Memoria del proyecto\n<!-- largo comentario que no cuenta -->\n- abcdefg'), 3);
  assert.equal(estimateMemoryTokens(''), 0);
});

test('memoryState: tope, cambios y vacía', () => {
  const view = { texto: '- El puerto es 3005', tokens: 6, max: 10, ruta: '/x/.pgocode/memoria.md' };
  assert.deepEqual(memoryState(view, view.texto), { tokens: 6, over: false, dirty: false, line: '~6 de 10 tokens' });
  const largo = memoryState(view, '- ' + 'x'.repeat(60));
  assert.equal(largo.over, true);
  assert.equal(largo.dirty, true);
  assert.match(largo.line, /pasa del tope.*sin guardar/);
  assert.equal(memoryState(view, '  ').line, 'Vacía: el agente no recibe nada.');
  assert.equal(memoryState(null, 'x').dirty, false);
});
