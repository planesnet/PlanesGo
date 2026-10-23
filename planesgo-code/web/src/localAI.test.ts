import { test } from 'node:test';
import assert from 'node:assert/strict';
import { localAIStatusText } from './localAI.ts';

test('estado de la IA local en una línea', () => {
  assert.equal(localAIStatusText({ url: 'x', api: 'openai', listo: true }, 'embeddinggemma'), 'Conectado · compatible con OpenAI · embeddinggemma');
  assert.equal(localAIStatusText({ url: 'x', listo: false, error: 'timeout', reserva: true }), 'No responde (timeout): se usa el Ollama de este equipo');
  assert.match(localAIStatusText({ url: 'x', listo: false }), /solo con reglas/);
});
