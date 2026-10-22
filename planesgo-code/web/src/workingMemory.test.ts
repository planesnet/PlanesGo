// Pruebas de la memoria de trabajo y del relevo: npm test (node --test, sin dependencias ni tokens).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as W from './workingMemory.ts';
import type { Mensaje, Herramienta } from './api.ts';

const h = (nombre: string, resumen: string, estado: Herramienta['estado'] = 'ok'): Mensaje['partes'] extends (infer P)[] | undefined ? P : never =>
  ({ tipo: 'herramienta', h: { id: Math.random().toString(36), nombre, resumen, estado } });

const conv: Mensaje[] = [
  { rol: 'user', texto: 'Crea un servidor HTTP en Go con un endpoint /salud. No uses frameworks externos.' },
  { rol: 'assistant', texto: '', partes: [
    { tipo: 'texto', texto: 'Voy a crearlo.' },
    h('Write', 'cmd/servidor/main.go'), h('Bash', 'go build ./...', 'error'), h('Edit', 'cmd/servidor/main.go'),
    h('Bash', 'go build ./...'), h('Bash', 'ls -la'), h('Read', 'go.mod'),
    { tipo: 'texto', texto: 'Hecho: servidor en cmd/servidor/main.go. He decidido usar net/http de la biblioteca estándar.' },
  ], tareas: [{ id: 'l', tipo: 'lista', elementos: [{ texto: 'Crear main.go', estado: 'hecho' }, { texto: 'Añadir pruebas', estado: 'pendiente' }] }] },
  { rol: 'user', texto: 'Añade pruebas y ten en cuenta que el puerto es el 8080.' },
  { rol: 'assistant', texto: '', partes: [h('Write', 'cmd/servidor/main_test.go'), h('Bash', 'go test ./...'), { tipo: 'texto', texto: 'Pruebas añadidas y en verde.' }] },
];

test('memoria de trabajo: objetivo, peticiones, ficheros, órdenes, tareas, decisiones y última respuesta', () => {
  const w = W.collectWorkingMemory(conv);
  assert.match(w.goal!, /servidor HTTP/);
  assert.deepEqual(w.requests, ['Añade pruebas y ten en cuenta que el puerto es el 8080.']);
  assert.deepEqual(w.files, ['cmd/servidor/main.go', 'cmd/servidor/main_test.go']);
  assert.deepEqual(w.read, ['go.mod']);
  assert.equal(w.turns[0].reply, 'Hecho: servidor en cmd/servidor/main.go. He decidido usar net/http de la biblioteca estándar.');
  // La orden repetida vale por su último resultado; «ls» no es relevante.
  assert.deepEqual(w.commands, [{ cmd: 'go build ./...', ok: true }, { cmd: 'go test ./...', ok: true }]);
  assert.deepEqual(w.tasks.map(t => t.state), ['hecho', 'pendiente']);
  assert.ok(w.decisions.some(d => d.startsWith('Usuario: No uses frameworks')), w.decisions.join(' | '));
  assert.ok(w.decisions.some(d => /He decidido usar net\/http/.test(d)), w.decisions.join(' | '));
  assert.equal(w.lastAnswer, 'Pruebas añadidas y en verde.');
  const t = W.buildWorkingMemory(conv);
  for (const s of ['## Objetivo', '## Conversación', 'Agente: Hecho: servidor', '## Ficheros consultados', '## Lista de tareas', '- [ ] Añadir pruebas', 'cmd/servidor/main_test.go', 'ok: `go test ./...`', '## Última respuesta']) assert.ok(t.includes(s), s);
});

test('memoria de trabajo: vacía sin conversación y nunca pasa del máximo', () => {
  assert.equal(W.buildWorkingMemory([]), '');
  const largo: Mensaje[] = [{ rol: 'user', texto: 'x'.repeat(5000) }];
  for (let i = 0; i < 50; i++) {
    largo.push({ rol: 'user', texto: 'Petición ' + i + ' ' + 'y'.repeat(800) });
    largo.push({ rol: 'assistant', texto: '', partes: [h('Edit', `src/fichero_largo_numero_${i}_${'z'.repeat(60)}.ts`), { tipo: 'texto', texto: 'r'.repeat(6000) }] });
  }
  const t = W.buildWorkingMemory(largo);
  assert.ok(t.length <= W.MEMORY_MAX, String(t.length));
  assert.ok(t.includes('## Última respuesta'));
});

test('mensaje del relevo: la memoria delante, o detrás si empieza por una orden', () => {
  assert.match(W.relayMessage('M', 'hola'), /^\[Memoria de trabajo[\s\S]*M[\s\S]*hola$/);
  assert.match(W.relayMessage('M', '/skill haz algo'), /^\/skill haz algo\n\n\[Memoria/);
});

test('cuándo hay relevo', () => {
  const base: W.RelayInput = { agent: 'claude-code', context: 90_000, idleMinutes: 0, hasAgentSession: true, memoryChars: 4000 };
  assert.ok(W.shouldRelay(base));
  assert.equal(W.shouldRelay({ ...base, context: 70_000 }), null);
  assert.ok(W.shouldRelay({ ...base, agent: 'opencode', context: 70_000 }));
  // Sin caché (parada) y con al menos la mitad del umbral.
  assert.ok(W.shouldRelay({ ...base, context: 45_000, idleMinutes: 10 }));
  assert.equal(W.shouldRelay({ ...base, context: 30_000, idleMinutes: 10 }), null);
  // Nunca con adjuntos, planes, sin sesión, con «nunca» o si la memoria no ahorra.
  assert.equal(W.shouldRelay({ ...base, attachments: 1 }), null);
  assert.equal(W.shouldRelay({ ...base, planOrder: true }), null);
  assert.equal(W.shouldRelay({ ...base, hasAgentSession: false }), null);
  assert.equal(W.shouldRelay({ ...base, setting: 'nunca' }), null);
  assert.equal(W.shouldRelay({ ...base, memoryChars: 200_000 }), null);
  // Umbral propio.
  assert.ok(W.shouldRelay({ ...base, context: 25_000, setting: 20_000 }));
  assert.equal(W.relayThreshold('antigravity', 'auto'), 60_000);
});

test('memoria de trabajo: sin lo que quedó fuera del contexto con «Continuar desde aquí»', () => {
  const cortada: Mensaje[] = [
    ...conv.slice(0, 2),
    { ...conv[2], texto: 'Recuerda la palabra clave VERDE.', fuera: true },
    { ...conv[3], fuera: true },
    { rol: 'user', texto: '¿Qué palabras clave recuerdas?' },
    { rol: 'assistant', texto: 'Ninguna.' },
  ];
  const w = W.collectWorkingMemory(cortada);
  assert.deepEqual(w.requests, ['¿Qué palabras clave recuerdas?']);
  assert.deepEqual(w.files, ['cmd/servidor/main.go']);
  assert.ok(!W.buildWorkingMemory(cortada).includes('VERDE'));
});
