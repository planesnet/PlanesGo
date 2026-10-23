import { test } from 'node:test';
import assert from 'node:assert/strict';
import { localAIStatusText, localAITestText } from './localAI.ts';

test('estado de la IA local en una línea', () => {
  assert.equal(localAIStatusText({ url: 'x', api: 'openai', listo: true }, 'embeddinggemma'), 'Conectado · compatible con OpenAI · embeddinggemma');
  assert.equal(localAIStatusText({ url: 'x', listo: false, error: 'timeout', reserva: true }), 'No responde (timeout): se usa el Ollama de este equipo');
  assert.match(localAIStatusText({ url: 'x', listo: false }), /solo con reglas/);
});

test('prueba de cada modelo en una línea', () => {
  assert.equal(localAITestText({ tipo: 'embeddings', modelo: 'embeddinggemma', ok: true, ms: 45, detalle: 'vector de 768 dimensiones' }), '✓ embeddinggemma · vector de 768 dimensiones · 45 ms');
  assert.equal(localAITestText({ tipo: 'texto', modelo: 'gemma3:4b', ok: true, ms: 1500, detalle: 'listo' }), '✓ gemma3:4b · «listo» · 1,5 s');
  assert.equal(localAITestText({ tipo: 'texto', ok: false, ms: 3, error: 'la IA local no tiene modelo de texto' }), '✗ sin modelo · la IA local no tiene modelo de texto');
});
