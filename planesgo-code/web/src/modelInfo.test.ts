// Pruebas del texto de los modelos: npm test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { modelLabel } from './modelInfo.ts';

test('modelos: ventana y precio de los conocidos; los demás, tal cual', () => {
  assert.equal(modelLabel('google-vertex/gemini-3.8-flash'), 'google-vertex/gemini-3.8-flash · 1 M · 0,75/3,75 $');
  assert.equal(modelLabel('vertex-deepseek/deepseek-ai/deepseek-v3.2-maas'), 'vertex-deepseek/deepseek-ai/deepseek-v3.2-maas · 164 k');
  assert.equal(modelLabel('haiku'), 'haiku');
});
