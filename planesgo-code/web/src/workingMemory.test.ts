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
  const base: W.RelayInput = { agent: 'claude-code', mode: 'idle', context: 90_000, idleMinutes: 0, hasAgentSession: true, memoryChars: 4000 };
  // Claude Code: con la caché caliente, nunca (aunque el contexto sea grande).
  assert.equal(W.shouldRelay(base), null);
  assert.equal(W.shouldRelay({ ...base, context: 300_000 }), null);
  // Con la caché caducada, en cuanto el historial pasa claramente de lo fijo del agente.
  assert.ok(W.shouldRelay({ ...base, context: 30_000, idleMinutes: 10 }));
  assert.equal(W.shouldRelay({ ...base, context: 20_000, idleMinutes: 10 }), null, 'Claude: poco más que lo fijo');
  // Antigravity: también con la caché caliente, al pasar del umbral.
  const ag: W.RelayInput = { ...base, agent: 'antigravity', mode: 'on' };
  assert.ok(W.shouldRelay(ag));
  assert.equal(W.shouldRelay({ ...ag, context: W.RELAY_THRESHOLD - 1 }), null);
  assert.ok(W.shouldRelay({ ...ag, context: 22_000, idleMinutes: 10 }));
  assert.ok(W.shouldRelay({ ...ag, context: 25_000, threshold: 20_000 }), 'umbral propio (banco)');
  // Nunca sin modo (OpenCode, o la gestión desactivada), con adjuntos, planes, sin sesión o si la memoria no ahorra.
  assert.equal(W.shouldRelay({ ...ag, mode: 'off', idleMinutes: 10 }), null);
  assert.equal(W.shouldRelay({ ...ag, attachments: 1 }), null);
  assert.equal(W.shouldRelay({ ...ag, planOrder: true }), null);
  assert.equal(W.shouldRelay({ ...ag, hasAgentSession: false }), null);
  assert.equal(W.shouldRelay({ ...ag, memoryChars: 200_000 }), null);
  // Por vigencia (Antigravity): cuando lo que está fuera de vigencia pesa bastante, no por el tamaño.
  const vg: W.RelayInput = { ...base, agent: 'antigravity', mode: 'vig', context: 90_000 };
  assert.equal(W.shouldRelay(vg), null, 'grande pero vigente');
  assert.match(W.shouldRelay({ ...vg, outTokens: 20_000 })!, /fuera de vigencia/);
  assert.equal(W.shouldRelay({ ...vg, outTokens: W.PRUNE_MIN_TOKENS - 1 }), null);
  assert.ok(W.shouldRelay({ ...vg, outTokens: 3000, outMin: 2000 }), 'mínimo forzado');
  assert.ok(W.shouldRelay({ ...vg, context: 30_000, idleMinutes: 10 }), 'y con la caché caducada');
});

test('memoria por vigencia: sin los intercambios ni las herramientas fuera de vigencia', () => {
  const ms: Mensaje[] = [
    { rol: 'user', texto: 'Crea el servidor', vig: { vigencia: 0 } },
    { rol: 'assistant', texto: '', vig: { vigencia: 0 }, partes: [{ tipo: 'herramienta', h: { id: 'a', nombre: 'Read', resumen: 'viejo.go', estado: 'ok', vig: { vigencia: 4, superada: 'lectura superada' } } },
      { tipo: 'herramienta', h: { id: 'b', nombre: 'Write', resumen: 'main.go', estado: 'ok', vig: { vigencia: 0.5 } } }, { tipo: 'texto', texto: 'Hecho.' }] },
    { rol: 'user', texto: 'Recuerda la palabra AZUL', vig: { vigencia: 3.6 } },
    { rol: 'assistant', texto: 'Vale, AZUL.', vig: { vigencia: 3.6 } },
    { rol: 'user', texto: 'Sigue', vig: { vigencia: 0 } },
    { rol: 'assistant', texto: 'Sigo.', vig: { vigencia: 0 } },
  ];
  const v = W.vigentOnly(ms);
  assert.equal(v.length, 4);
  const mem = W.buildWorkingMemory(v);
  assert.ok(!mem.includes('AZUL') && !mem.includes('viejo.go') && mem.includes('main.go'), mem);
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
