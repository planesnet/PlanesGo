// Pruebas del enrutador de intenciones (router.ts): una tabla de mensajes reales o realistas con la ruta esperada.
// npm test (node --test, sin tokens).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as R from './router.ts';

const LIBRE: R.RouteInput = { text: '', isAgent: true, busy: false, canInject: true, alwaysTurn: false, hasConversation: true, permissions: 'editar', canFork: true };
const OCUPADO: R.RouteInput = { ...LIBRE, busy: true };
const AL_TURNO: R.RouteInput = { ...OCUPADO, alwaysTurn: true };

const LISTA = 'Haz estas tres cosas en el proyecto, cada una por su lado:\n1. Añade el modo oscuro a la página de ajustes\n2. Corrige la paginación del listado de facturas\n3. Traduce los textos del pie al inglés';

// [mensaje, situación, ruta, detalle o modificador esperado (opcional), lo que necesita del servidor (opcional)]
type Caso = [string, Partial<R.RouteInput>, R.Route, string?, ('classify' | 'split')?];
const CASOS: Caso[] = [
  // 0. Órdenes de pgocode
  ['/help', {}, 'command', 'help'],
  ['/ayuda', OCUPADO, 'command', 'help'],
  ['/plan añade un login con Google', {}, 'command', 'plan:nuevo'],
  ['/plan 2 quita el paso 3', {}, 'command', 'plan:revisar'],
  ['plan 3: usa SQLite en vez de ficheros', {}, 'command', 'plan:revisar'],
  ['ejecuta el plan 2', {}, 'command', 'plan:ejecutar'],
  ['descarta el plan 4', {}, 'command', 'plan:descartar'],
  ['reactiva el plan 4', {}, 'command', 'plan:reactivar'],
  ['borra el plan 4', {}, 'command', 'plan:borrar'],
  ['ejecuta el plan 2', OCUPADO, 'command', 'queue'],
  ['> ejecuta el plan 2', {}, 'command', 'plan:ejecutar'],
  // 1. Funciones directas (como en direct.test.ts)
  ['compila', {}, 'direct', 'build'],
  ['pasa los tests', {}, 'direct', 'test'],
  ['comprueba los tipos', {}, 'direct', 'typecheck'],
  ['lint', {}, 'direct', 'lint'],
  ['haz commit', {}, 'direct', 'commit'],
  ['vale, haz commit', {}, 'direct', 'commit'],
  ['¿qué cambios hay?', {}, 'direct', 'status'],
  ['enséñame el diff', {}, 'direct', 'diff'],
  ['¿qué ficheros has tocado?', {}, 'direct', 'files'],
  ['¿cuánto llevamos gastado?', {}, 'direct', 'cost'],
  ['¿En qué rama estamos?', {}, 'direct', 'where'],
  ['¿qué has hecho?', {}, 'direct', 'last_turn'],
  ['compila', OCUPADO, 'direct', 'queue'],
  ['¿qué cambios hay?', OCUPADO, 'direct', 'now'],
  ['run the tests', AL_TURNO, 'direct', 'queue'],
  // …pero no si el usuario lo manda al agente, con adjuntos, sin órdenes directas o con más palabras.
  ['>compila', {}, 'agent'],
  ['> haz commit', OCUPADO, 'agent', 'turn'],
  ['compila', { attachments: 1 }, 'agent', 'attachments'],
  ['compila', { directOn: false }, 'agent'],
  ['compila', { manual: 'conversacion' }, 'agent'],
  ['compila y arregla los errores', {}, 'agent'],
  ['haz commit y sube', {}, 'agent'],
  ['sube los cambios', {}, 'agent'],
  ['¿por qué falla el test?', {}, 'agent'],
  ['compila', { isAgent: false }, 'agent'],
  // 2. Preguntas aparte
  ['?', OCUPADO, 'aside', 'estado'],
  ['? cómo va', LIBRE, 'aside', 'estado'],
  ['¿cómo va?', OCUPADO, 'aside', 'estado'],
  ['¿por dónde vas?', OCUPADO, 'aside', 'estado'],
  ['¿qué hora es en Tokio?', OCUPADO, 'aside', 'suelta'],
  ['¿cuál es la capital de Australia?', OCUPADO, 'aside', 'suelta'],
  ['// ¿qué significa idempotente?', LIBRE, 'aside', 'suelta'],
  ['// ¿cómo funciona la caché que has puesto?', LIBRE, 'aside', 'contexto'],
  ['// ¿cómo funciona la caché?', { hasConversation: false }, 'aside', 'suelta'],
  ['// ¿quién inventó Go?', OCUPADO, 'aside', 'suelta'],
  ['// ¿por qué has usado un mapa ahí?', OCUPADO, 'aside', 'contexto'],
  ['¿qué hace esta función?', { ...OCUPADO, manual: 'contexto' }, 'aside', 'contexto'],
  ['algo', { ...OCUPADO, learned: { algo: 'suelta' } }, 'aside', 'suelta'],
  ['¿cómo va?', { ...OCUPADO, autoAside: false }, 'agent', 'queue'],
  ['¿cómo va?', LIBRE, 'agent'],
  // …las dudosas las decide el clasificador del servidor.
  ['¿por qué has usado un bucle aquí?', OCUPADO, 'agent', 'queue', 'classify'],
  ['¿hay alguna forma más rápida de hacerlo?', AL_TURNO, 'agent', 'turn', 'classify'],
  // …y lo que es para el agente, a la conversación (a la cola o al turno).
  ['cambia el color del botón a rojo', OCUPADO, 'agent', 'queue'],
  ['cambia el color del botón a rojo', AL_TURNO, 'agent', 'turn'],
  ['no uses esa librería', AL_TURNO, 'agent', 'turn'],
  ['sí, adelante', { ...OCUPADO, lastAgentText: '¿Quieres que lo haga así?' }, 'agent', 'queue'],
  ['¿y si lo hacemos con un mapa?', { ...OCUPADO, lastAgentText: '¿Lo hago con una lista o con un mapa?' }, 'agent', 'queue'],
  ['mira esta captura', { ...AL_TURNO, attachments: 1 }, 'agent', 'queue'],
  ['para ya', { ...OCUPADO, manual: 'turno', manualBy: 'alt' }, 'agent', 'turn'],
  ['para ya', { ...OCUPADO, manual: 'turno', canInject: false }, 'agent', 'queue'],
  // 3. Reparto en subtareas (lo confirma el servidor)
  [LISTA, {}, 'agent', undefined, 'split'],
  [LISTA, { permissions: 'lectura' }, 'agent'],
  [LISTA, { child: true }, 'agent'],
  [LISTA, { cutPending: true }, 'agent', 'cut'],
  [LISTA, { attachments: 2 }, 'agent'],
  [LISTA, OCUPADO, 'agent', 'queue'],
  ['> ' + LISTA, {}, 'agent'],
  [LISTA, { noSplit: true }, 'agent'],
  // 5. El agente, por defecto
  ['añade un test para el login', {}, 'agent'],
  ['refactoriza el módulo de facturas para usar el nuevo cliente', {}, 'agent'],
  ['/revisa-pr 123', {}, 'agent', 'skill-user'],
  ['hola', {}, 'agent'],
  ['¿qué hace esta función?', {}, 'agent'],
];

test('tabla de mensajes y su ruta (≥60)', () => {
  assert.ok(CASOS.length >= 60, `solo ${CASOS.length} casos`);
  for (const [text, s, ruta, extra, srv] of CASOS) {
    const d = R.route({ ...LIBRE, ...s, text });
    const que = `«${text.slice(0, 40)}» ${JSON.stringify(s).slice(0, 80)}`;
    assert.equal(d.route, ruta, que + ' → ' + JSON.stringify(d));
    assert.equal(d.needsServer, srv, que + ' (servidor)');
    if (extra) assert.ok(d.detail === extra || d.modifiers.includes(extra as R.Modifier), `${que}: falta ${extra} en ${JSON.stringify(d)}`);
    assert.ok(d.reason, que + ' sin motivo');
    assert.ok(R.tagText(d, Boolean(s.busy)).startsWith('→'), que);
  }
});

test('quién lo hace y quién lo decide', () => {
  const r = (text: string, s: Partial<R.RouteInput> = {}) => R.route({ ...LIBRE, ...s, text });
  assert.deepEqual([r('compila').executor, r('compila').engine], ['pgocode', 'rules']);
  assert.equal(r('descarta el plan 2').executor, 'pgocode');
  assert.equal(r('/plan algo').executor, 'agent');
  assert.equal(r('?', OCUPADO).executor, 'pgocode');
  assert.equal(r('// ¿qué es esto?', OCUPADO).executor, 'agent-copy');
  assert.equal(r('// ¿qué es esto?', { ...OCUPADO, canFork: false }).executor, 'cheap-model');
  assert.equal(r('¿cuál es la capital de Francia?', OCUPADO).executor, 'local-ai');
  assert.equal(r('// algo', OCUPADO).engine, 'user');
  assert.equal(r('algo', { ...OCUPADO, learned: { algo: 'contexto' } }).engine, 'learned');
  assert.equal(r('> hola').engine, 'user');
  assert.equal(r('> hola').text, 'hola');
  assert.equal(r('// ¿y esto?', OCUPADO).text, '¿y esto?');
});

test('pasos con el servidor: clasificador y reparto', () => {
  const dudosa = R.route({ ...OCUPADO, text: '¿por qué has usado un bucle aquí?' });
  const local = R.resolveClassify(dudosa, { tipo: 'contexto', modelo: 'local/gemma3:4b' }, OCUPADO);
  assert.deepEqual([local.route, local.engine, local.executor, local.needsServer], ['aside', 'local-ai', 'agent-copy', undefined]);
  assert.match(local.reason, /IA local \(gemma3:4b\)/);
  const pago = R.resolveClassify(dudosa, { tipo: 'conversacion', modelo: 'gemini-2.5-flash-lite' }, OCUPADO);
  assert.deepEqual([pago.route, pago.engine], ['agent', 'cheap-model']);
  const nada = R.resolveClassify(dudosa, null, OCUPADO);
  assert.deepEqual([nada.route, nada.needsServer], ['agent', undefined]);
  assert.match(nada.reason, /no ha respondido/);

  const lista = R.route({ ...LIBRE, text: LISTA });
  const si = R.resolveSplit(lista, { partes: ['a', 'b', 'c'], max: 4, espera_s: 8 });
  assert.deepEqual([si.route, si.executor, si.engine, si.detail], ['split', 'subsessions', 'embeddings', '3']);
  const no = R.resolveSplit(lista, { max: 4, espera_s: 8, motivo: 'la sesión no edita' });
  assert.deepEqual([no.route, no.needsServer], ['agent', undefined]);
  assert.match(no.reason, /la sesión no edita/);
  assert.match(R.resolveSplit(lista, null).reason, /no ha respondido/);
});

test('withTimeout: cae a null si tarda o falla', async () => {
  assert.equal(await R.withTimeout(new Promise(res => setTimeout(() => res(1), 50)), 5), null);
  assert.equal(await R.withTimeout(Promise.reject(new Error('x')), 50), null);
  assert.equal(await R.withTimeout(Promise.resolve(2), 50), 2);
});

test('la cola: lo que sale solo y lo encolado con «>»', () => {
  assert.ok(R.standalone('compila') && R.standalone('/plan x') && R.standalone('ejecuta el plan 2') && !R.standalone('hola'));
  assert.ok(!R.standalone('compila', false));
  assert.ok(!R.standalone('> compila'));
  assert.deepEqual(R.fromQueue(['> compila']), { text: 'compila', forced: true });
  assert.deepEqual(R.fromQueue(['hola', '> compila']), { text: 'hola\n\ncompila', forced: false });
  // Lo que sale de la cola con «>» va al agente aunque sea una orden directa.
  const q = R.fromQueue(['> compila']);
  assert.equal(R.route({ ...LIBRE, text: q.text, noDirect: q.forced }).route, 'agent');
});

test('etiqueta, distintivo, registro y resumen', () => {
  const d = R.route({ ...LIBRE, text: 'descarta el plan 2' });
  assert.equal(R.tagText(d, false), '→ pgocode: descartar el plan 2 (0 tokens)');
  assert.match(R.tagTitle(d), /lo hace pgocode .* decide: reglas/);
  assert.equal(R.tagText(R.route({ ...LIBRE, text: LISTA }), false), '→ Conversación o reparto en subtareas: lo decide pgocode al enviar');
  assert.equal(R.tagText(R.route({ ...OCUPADO, text: '¿cuál es la capital de Francia?' }), true), '→ Aparte · suelta (IA local)');
  assert.equal(R.tagText(R.route({ ...LIBRE, text: '/help' }), false), '→ Ayuda de pgocode (0 tokens)');
  assert.equal(R.tagText(R.route({ ...AL_TURNO, text: 'no uses esa librería' }), true), '→ Al turno en curso (el agente lo lee ya)');

  let s = R.stamp(R.route({ ...OCUPADO, text: 'cambia el color' }));
  assert.equal(R.badge(s), 'agente · cola');
  s = R.addModifier(R.addModifier(s, 'skill'), 'subagents')!;
  assert.equal(R.badge(s), 'agente · cola · skill elegida · subagentes sugeridos');
  assert.equal(R.addModifier(s, 'skill'), s);
  assert.equal(R.badge(R.stamp(R.route({ ...LIBRE, text: 'compila' }))), 'pgocode');
  assert.equal(R.badge({ route: 'aside', executor: 'local-ai', engine: 'rules', reason: 'x', modifiers: [] }), 'aparte · IA local');
  assert.equal(R.badge({ route: 'aside', executor: 'agent-copy', engine: 'rules', reason: 'x', modifiers: [] }), 'aparte · copia');
  assert.equal(R.badge({ route: 'split', executor: 'subsessions', engine: 'embeddings', reason: 'x', modifiers: [], detail: '3' }), 'reparto en 3');

  const p = R.logPayload('s1', s, 12.4);
  assert.deepEqual([p.sesion, p.destino, p.motor, p.ms], ['s1', 'agent', 'rules', 12]);

  let t: R.RouteTally = {};
  for (const x of ['compila', 'hola', 'adiós', 'descarta el plan 1']) t = R.tally(t, R.stamp(R.route({ ...LIBRE, text: x })));
  t = R.tally(t, { route: 'aside', executor: 'local-ai', engine: 'rules', reason: '', modifiers: [] });
  assert.equal(R.summary(t), 'Esta sesión: 2 al agente · 1 directa · 1 aparte (1 IA local) · 1 orden de pgocode. 3 de 5 sin tokens de pago.');
  assert.match(R.summary({}), /ningún mensaje/);
});
