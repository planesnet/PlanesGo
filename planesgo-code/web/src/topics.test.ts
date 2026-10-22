// Pruebas de las ramas por tema: npm test (node --test, sin dependencias ni tokens).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as TP from './topics.ts';
import * as AP from './aparte.ts';
import { collectWorkingMemory } from './workingMemory.ts';
import { childOf, nueva, type Sesion } from './sesiones.ts';
import type { Mensaje, Herramienta } from './api.ts';

const h = (nombre: string, resumen: string, estado: Herramienta['estado'] = 'ok'): Mensaje['partes'] extends (infer P)[] | undefined ? P : never =>
  ({ tipo: 'herramienta', h: { id: Math.random().toString(36), nombre, resumen, estado } });

const conv: Mensaje[] = [
  { rol: 'user', texto: 'Rediseña la portada: cabecera más compacta y tarjetas en dos columnas.' },
  { rol: 'assistant', texto: '', partes: [h('Read', 'web/src/Portada.tsx'), h('Read', 'web/src/estilos.css'), h('Edit', 'web/src/estilos.css'),
    h('Bash', 'npm run build'), { tipo: 'texto', texto: 'Hecho: cabecera de 48 px y rejilla de dos columnas. '.repeat(40) }] },
  { rol: 'user', texto: 'Ahora las tarjetas con sombra suave y esquinas de 8 px.' },
  { rol: 'assistant', texto: '', partes: [h('Edit', 'web/src/Tarjeta.tsx'), { tipo: 'texto', texto: 'Listo.' }] },
];

test('shouldCheckTopic: solo con agente, contexto relevante y una petición de verdad', () => {
  const ok = { context: 45_000, text: 'Añadir las etiquetas a la lista de tareas', agent: true };
  assert.equal(TP.shouldCheckTopic(ok), true);
  assert.equal(TP.shouldCheckTopic({ ...ok, context: 12_000 }), false, 'conversación aún pequeña');
  assert.equal(TP.shouldCheckTopic({ ...ok, context: undefined }), false);
  assert.equal(TP.shouldCheckTopic({ ...ok, agent: false }), false);
  assert.equal(TP.shouldCheckTopic({ ...ok, attachments: 1 }), false);
  assert.equal(TP.shouldCheckTopic({ ...ok, planOrder: true }), false);
  assert.equal(TP.shouldCheckTopic({ ...ok, active: false }), false);
  assert.equal(TP.shouldCheckTopic({ ...ok, text: 'vale, sigue' }), false, 'demasiado corto');
  assert.equal(TP.shouldCheckTopic({ ...ok, text: '/revisa las etiquetas de la lista' }), false, 'orden o skill');
});

test('topicSummary: el tema en 2 o 3 líneas (objetivo, últimas peticiones, ficheros)', () => {
  const s = TP.topicSummary(collectWorkingMemory(conv));
  const l = s.split('\n');
  assert.ok(l.length >= 2 && l.length <= 3, s);
  assert.match(l[0], /^Objetivo: Rediseña la portada/);
  assert.match(s, /sombra suave/);
  assert.match(s, /Ficheros: .*Tarjeta\.tsx/);
  assert.doesNotMatch(s, /web\/src\//, 'solo el nombre de los ficheros');
  assert.ok(s.length < 800);
  assert.equal(TP.topicSummary(collectWorkingMemory([])), '');
});

test('childContext: objetivo y ficheros, sin la conversación; acotado', () => {
  const c = TP.childContext(collectWorkingMemory(conv));
  assert.match(c, /Rediseña la portada/);
  assert.match(c, /estilos\.css, web\/src\/Tarjeta\.tsx/);
  assert.match(c, /consultados allí \(1\): web\/src\/Portada\.tsx/);
  assert.doesNotMatch(c, /cabecera de 48 px|sombra suave|npm run build/, 'ni respuestas, ni peticiones, ni órdenes');
  const largo: Mensaje[] = [{ rol: 'user', texto: 'x'.repeat(5000) }, { rol: 'assistant', texto: '', partes: Array.from({ length: 400 }, (_, i) => h('Edit', `carpeta/muy/larga/fichero${i}.ts`)) }];
  assert.ok(TP.childContext(collectWorkingMemory(largo)).length <= TP.CHILD_CONTEXT_MAX);
  assert.equal(TP.childContext(collectWorkingMemory([])), '');
  const abs: Mensaje[] = [{ rol: 'user', texto: 'Objetivo' }, { rol: 'assistant', texto: '', partes: [h('Read', '/home/u/repo/src/a.ts'), h('Read', '/otra/b.ts')] }];
  assert.match(TP.childContext(collectWorkingMemory(abs), TP.CHILD_CONTEXT_MAX, '/home/u/repo/'), /\(2\): src\/a\.ts, \/otra\/b\.ts$/);
});

test('childMessage: el contexto delante del mensaje (detrás si es una orden «/…»)', () => {
  const m = TP.childMessage('Objetivo: X', 'Añade etiquetas');
  assert.ok(m.startsWith('[Contexto') && m.endsWith('Añade etiquetas'));
  assert.ok(TP.childMessage('Objetivo: X', '/skill haz algo').startsWith('/skill haz algo'));
  assert.equal(TP.childMessage('', 'hola'), 'hola');
});

test('childOf: mismos parámetros que la madre, sin historial y enlazada a ella', () => {
  const m: Sesion = { ...nueva(7, '/repo', 'main', 'claude-code', 'Diseño'), modelo: 'haiku', permisos: 'editar', esfuerzo: 'low', hosting: 'local' as Sesion['hosting'],
    mcp: ['x'], autorizadas: ['/otra'], tareaId: 3, tarea: 'T', renovarContexto: 'nunca', sesionAgente: 'abc', plan: 'planes/1.md' };
  const c = childOf(m, 'Añadir las etiquetas');
  assert.notEqual(c.id, m.id);
  assert.equal(c.madre, m.id);
  assert.equal(c.nombre, 'Añadir las etiquetas');
  for (const k of ['proyecto', 'carpeta', 'rama', 'agente', 'modelo', 'permisos', 'esfuerzo', 'hosting', 'mcp', 'autorizadas', 'tareaId', 'tarea', 'renovarContexto'] as const) assert.deepEqual(c[k], m[k], k);
  assert.equal(c.sesionAgente, undefined);
  assert.equal(c.plan, undefined);
});

test('destino «hija»: etiqueta, ciclo con la propuesta y nunca se aprende', () => {
  assert.equal(AP.etiqueta({ destino: 'hija', texto: '', motivo: '' }), '→ Parece otro tema: nueva sesión hija');
  assert.equal(AP.siguiente('hija', false, true), 'conversacion');
  assert.equal(AP.siguiente('estado', false, true), 'hija');
  assert.equal(AP.siguiente('estado', false, false), 'conversacion');
  assert.deepEqual(AP.aprende({}, 'Añadir etiquetas a la lista', 'hija'), {});
  assert.equal(AP.decide('Añadir etiquetas a la lista', { ocupado: true, aprendidas: { 'anadir etiquetas': 'hija' } }).destino, 'conversacion');
});
