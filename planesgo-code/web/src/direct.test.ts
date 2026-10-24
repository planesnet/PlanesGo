// Pruebas de las órdenes directas (lo que pgocode hace sin el agente): npm test (node --test, sin tokens).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as D from './direct.ts';
import type { Mensaje } from './api.ts';

const SI: [string, D.DirectKind][] = [
  ['compila', 'build'], ['Compila.', 'build'], ['compílalo', 'build'], ['build', 'build'], ['haz build', 'build'], ['haz el build', 'build'],
  ['compila el proyecto', 'build'], ['compila, por favor', 'build'], ['oye, compila', 'build'], ['run the build', 'build'], ['rebuild', 'build'],
  ['compila otra vez', 'build'],
  ['pasa los tests', 'test'], ['ejecuta las pruebas', 'test'], ['test', 'test'], ['tests', 'test'], ['lanza los tests', 'test'],
  ['corre las pruebas unitarias', 'test'], ['run tests', 'test'], ['run the tests', 'test'], ['Pasa los tests de nuevo', 'test'], ['pasa todos los tests', 'test'],
  ['typecheck', 'typecheck'], ['comprueba los tipos', 'typecheck'], ['check types', 'typecheck'], ['pasa el tsc', 'typecheck'],
  ['lint', 'lint'], ['pasa el linter', 'lint'], ['run the linter', 'lint'],
  ['haz commit', 'commit'], ['commitea', 'commit'], ['haz un commit', 'commit'], ['Haz commit de todo', 'commit'], ['commit', 'commit'],
  ['commit the changes', 'commit'], ['make a commit', 'commit'], ['commitea los cambios', 'commit'], ['vale, haz commit', 'commit'],
  ['¿qué cambios hay?', 'status'], ['git status', 'status'], ['¿hay cambios sin commitear?', 'status'], ['qué hay sin commitear', 'status'],
  ['any uncommitted changes?', 'status'], ['estado de git', 'status'],
  ['diff', 'diff'], ['git diff', 'diff'], ['enséñame el diff', 'diff'], ['show me the diff', 'diff'],
  ['¿qué ficheros has tocado?', 'files'], ['¿Qué archivos has cambiado?', 'files'], ['which files did you change?', 'files'], ['ficheros modificados', 'files'],
  ['¿cuánto llevamos gastado?', 'cost'], ['¿cuántos tokens llevamos?', 'cost'], ['coste de la sesión', 'cost'], ['how much have we spent so far?', 'cost'],
  ['¿En qué rama estamos?', 'where'], ['rama actual', 'where'], ['¿en qué carpeta estás?', 'where'], ['which branch are we on?', 'where'],
  ['¿qué has hecho?', 'last_turn'], ['¿qué hiciste en el turno anterior?', 'last_turn'], ['resumen del último turno', 'last_turn'], ['what did you do?', 'last_turn'],
];

const NO = [
  // Más que la orden: al agente.
  'compila y arregla los errores', 'haz commit y sube', 'haz commit y push', 'pasa los tests del módulo de facturas y dime por qué falla',
  'compila en modo release', 'haz los tests', 'haz tests para el login', 'escribe los tests', 'añade un test', 'arregla el build',
  'el build falla', 'los tests fallan, ¿por qué?', '¿por qué falla el test?', 'commit con el mensaje «arreglo»', 'haz commit solo del README',
  'compila tú y mira los warnings', 'no compiles todavía', 'no hagas commit', '¿has compilado?', '¿has hecho commit?', 'revisa los tipos de la API',
  // Lo que no es mecánico: al agente.
  'sube', 'push', 'git push', 'publica', 'despliega', 'deploy', 'haz push', 'sube los cambios', 'haz un release', 'git reset --hard',
  'borra la rama', 'merge', 'haz merge con master',
  // Preguntas que no son de estado o son ambiguas.
  '¿qué has hecho con el login?', '¿qué ficheros hay en el proyecto?', '¿cuánto falta?', '¿dónde estamos?', 'estado', '¿cuánto llevamos?',
  'prueba', 'pruebas', '¿qué cambios harías?', 'cambios', 'what changed in the API?',
  // Prefijos y formas que no se interceptan.
  '>compila', '> haz commit', '?', '? compila', '//compila', '/compila', 'compila\ny después los tests', '',
  'compila ' + 'muy '.repeat(30),
];

test('detecta solo las órdenes enteras', () => {
  for (const [t, k] of SI) assert.equal(D.detect(t), k, t);
  for (const t of NO) assert.equal(D.detect(t), null, t);
  assert.ok(SI.length + NO.length >= 40);
});

test('mecánicas y respondidas en la interfaz', () => {
  assert.ok(D.isMechanical('build') && D.isMechanical('commit') && !D.isMechanical('status') && !D.isMechanical('cost'));
  assert.ok(D.answeredHere('cost') && D.answeredHere('last_turn') && !D.answeredHere('files'));
  assert.match(D.tagText('test', true), /al acabar el turno/);
  assert.match(D.tagText('status', true), /sin el agente/);
});

test('nota de un resultado: bien, mal y commit', () => {
  const bien = D.resultNote({ accion: 'build', ok: true, orden: 'make build', origen: 'Makefile', segundos: 2.5, salida: 'ok' });
  assert.match(bien.texto, /✓ \*\*Compilar\*\*: `make build` ha ido bien · 2,5 s/);
  assert.equal(bien.directo.ok, true);
  const mal = D.resultNote({ accion: 'test', ok: false, codigo: 1, orden: 'go test ./...', segundos: 0.2, salida: 'x\nFAIL', errores: '--- FAIL: TestX' });
  assert.match(mal.texto, /✗ \*\*Pruebas\*\*: `go test \.\/\.\.\.` ha fallado \(código 1\)/);
  assert.equal(mal.directo.errores, '--- FAIL: TestX');
  const c = D.resultNote({ accion: 'commit', ok: true, hash: 'abc123', rama: 'main', mensaje: 'Añade X', via: 'local/gemma3:4b', ficheros: ['a', 'b'] });
  assert.match(c.texto, /Commit\*\* `abc123` en `main`: «Añade X»/);
  assert.match(c.texto, /IA local \(gemma3:4b\)/);
  assert.match(c.texto, /sin push/);
  assert.match(D.resultNote({ accion: 'commit', ok: true }).texto, /no hay cambios/);
  assert.match(D.resultNote({ accion: 'status', ok: true, rama: 'main', ficheros: ['a.go'] }).texto, /Cambios sin commitear\*\* en `main` \(1\)/);
  assert.match(D.resultNote({ accion: 'status', ok: true, rama: 'main', ficheros: [] }).texto, /Sin cambios/);
  assert.match(D.resultNote({ accion: 'where', ok: true, rama: 'dev', carpeta: '/x/y', copia: true }).texto, /`\/x\/y` \(subcarpeta de la rama/);
});

test('mensaje para el agente: la orden y solo los errores', () => {
  const m = D.agentMessage({ accion: 'test', ok: false, orden: 'npm test', errores: 'not ok 1 - suma\n  expected 2', salida: 'mucho ruido' });
  assert.match(m, /^He ejecutado `npm test`/);
  assert.match(m, /not ok 1 - suma\n  expected 2/);
  assert.ok(!m.includes('mucho ruido'));
  assert.match(m, /vuelve a ejecutar `npm test`/);
  assert.ok(D.agentMessage({ accion: 'build', ok: false, orden: 'make', errores: 'e'.repeat(10000) }).length < 4500);
  assert.match(D.agentMessage({ accion: 'commit', ok: false, errores: 'hook falló' }), /el commit \(git add -A y git commit\)/);
});

const herr = (nombre: string, resumen: string, estado: 'hecho' | 'error' = 'hecho') => ({ tipo: 'herramienta' as const, h: { id: nombre + resumen, nombre, resumen, estado } });
const conv: Mensaje[] = [
  { rol: 'user', texto: 'arregla el login' },
  { rol: 'assistant', texto: 'Hecho', uso: { entrada: 30000, salida: 2000, coste: 0.5, contexto: 28000 }, partes: [
    herr('Read', '/p/web/src/login.ts'), herr('Edit', '/p/web/src/login.ts'), herr('Bash', 'npm test', 'error'), { tipo: 'texto', texto: 'He arreglado el login.' }] },
  { rol: 'user', texto: 'compila' },
  { rol: 'assistant', texto: '✓ Compilar', nota: true, directo: { accion: 'build', ok: true } },
];

test('respuestas con lo que ya sabe pgocode', () => {
  const f = D.filesAnswer(conv, ['web/src/login.ts', 'README.md']);
  assert.match(f, /tocado el agente en esta conversación \(1\)/);
  assert.match(f, /`src\/login\.ts`/);
  assert.match(f, /sin commitear según git \(2\)/);
  assert.match(D.filesAnswer([], []), /no ha editado/);
  const c = D.costAnswer(conv, { entrada: 100, salida: 50, coste: 0.01 });
  assert.match(c, /0,50 \$ · 32 k tokens/);
  assert.match(c, /1 turno del agente/);
  assert.match(c, /Preguntas aparte/);
  assert.match(c, /28 k tokens/);
  assert.match(c, /1 orden sin el agente/);
  const t = D.lastTurnAnswer(conv);
  assert.match(t, /3 acciones \(1 con error\)/);
  assert.match(t, /1\. Read `\/p\/web\/src\/login\.ts`/);
  assert.match(t, /3\. Bash `npm test` ✗/);
  assert.match(t, /He arreglado el login/);
  assert.match(D.lastTurnAnswer([]), /aún no ha hecho/);
});

test('plural en la nota del commit y del diff', () => {
  assert.match(D.resultNote({ accion: 'commit', ok: true, hash: 'abc', rama: 'main', mensaje: 'x', ficheros: ['a'] }).texto, /\n\n1 fichero · /);
  assert.match(D.resultNote({ accion: 'commit', ok: true, hash: 'abc', rama: 'main', mensaje: 'x', ficheros: ['a', 'b'] }).texto, /2 ficheros · /);
  assert.match(D.resultNote({ accion: 'diff', ok: true, ficheros: ['a'], salida: 'x' }).texto, /\(1 fichero\)/);
});

test('lo que hizo pgocode desde el último turno del agente, en una línea', () => {
  const c = D.resultNote({ accion: 'commit', ok: true, hash: 'abc123', rama: 'main', mensaje: 'Añade el IVA', ficheros: ['a'] });
  const t = D.resultNote({ accion: 'test', ok: true, orden: 'make test', segundos: 1 });
  const mal = D.resultNote({ accion: 'build', ok: false, orden: 'make build', segundos: 1, errores: 'x' });
  const nota = (r: { texto: string; directo: D.DirectNote }): Mensaje => ({ rol: 'assistant', texto: r.texto, nota: true, directo: r.directo });
  const ms: Mensaje[] = [
    { rol: 'user', texto: 'haz algo' }, nota(t), { rol: 'assistant', texto: 'hecho' },
    { rol: 'user', texto: 'haz commit' }, nota(c), { rol: 'user', texto: 'pasa los tests' }, nota(t),
    { rol: 'user', texto: '¿qué cambios hay?' }, { rol: 'assistant', texto: 'Sin cambios', nota: true, directo: { accion: 'status', ok: true } },
  ];
  assert.equal(D.sinceLastTurn(ms), '(pgocode, desde tu último turno: commit abc123 "Añade el IVA"; pruebas ✓ (`make test`).)');
  // Nada desde el último turno: nada.
  assert.equal(D.sinceLastTurn([...ms, { rol: 'user', texto: 'x' }, { rol: 'assistant', texto: 'y' }]), '');
  // Lo que ya va en el mensaje («Pásaselo al agente») no se repite.
  const conFallo = [...ms, nota(mal)];
  assert.match(D.sinceLastTurn(conFallo), /compilar ✗/);
  assert.doesNotMatch(D.sinceLastTurn(conFallo, D.agentMessage(mal.directo)), /compilar/);
});
