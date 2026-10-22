import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as U from './unanswered.ts';
import type { Mensaje } from './api.ts';

const foto = { nombre: 'image.png', ruta: '/x/image.png', tamano: 10 };

test('el turno cortado antes de responder: su mensaje va con el siguiente (caso real de INTRADEMA)', () => {
  const ms: Mensaje[] = [
    { rol: 'user', texto: 'quitar las columnas correspondientes a las etapas de la lista de proyectos.' },
    { rol: 'assistant', texto: '' },
  ];
  assert.deepEqual(U.withUnanswered(ms, 'ahora'), { text: 'quitar las columnas correspondientes a las etapas de la lista de proyectos.\n\nahora', attachments: [] });
});

test('varios cortados seguidos, con sus adjuntos; lo respondido antes no', () => {
  const ms: Mensaje[] = [
    { rol: 'user', texto: 'hola' }, { rol: 'assistant', texto: 'Hola, ¿qué hago?', uso: { entrada: 1, salida: 1 } },
    { rol: 'user', texto: 'mira esto', adjuntos: [foto] }, { rol: 'assistant', texto: '' },
    { rol: 'user', texto: 'y esto' }, { rol: 'assistant', texto: '' },
  ];
  assert.deepEqual(U.unanswered(ms).map(m => m.texto), ['mira esto', 'y esto']);
  assert.deepEqual(U.withUnanswered(ms, 'arréglalo'), { text: 'mira esto\n\ny esto\n\narréglalo', attachments: [foto] });
});

test('si el agente llegó a hacer algo (partes, texto o consumo), sí le llegó', () => {
  for (const a of [{ rol: 'assistant' as const, texto: 'Voy', }, { rol: 'assistant' as const, texto: '', partes: [{ tipo: 'texto' as const, texto: 'x' }] },
    { rol: 'assistant' as const, texto: '', uso: { entrada: 5, salida: 0 } }]) {
    assert.deepEqual(U.unanswered([{ rol: 'user', texto: 'haz X' }, a]), []);
  }
  assert.deepEqual(U.withUnanswered([], 'hola'), { text: 'hola', attachments: [] });
});
