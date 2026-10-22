// Pruebas de «Continuar desde aquí»: npm test (node --test, sin dependencias ni tokens).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as A from './aparte.ts';
import type { Mensaje } from './api.ts';

const editar = (fichero: string) => ({ tipo: 'herramienta' as const, h: { id: fichero, nombre: 'Edit', resumen: '/repo/src/' + fichero, estado: 'ok' as const } });
const conv: Mensaje[] = [
  { rol: 'user', texto: 'recuerda la palabra AZUL' },
  { rol: 'assistant', texto: 'OK', partes: [editar('a.go'), { tipo: 'texto', texto: 'OK, AZUL' }], punto: { sesion: 's1', id: 'u1' } },
  { rol: 'user', texto: 'ahora recuerda VERDE' },
  { rol: 'assistant', texto: 'OK', punto: { sesion: 's1', id: 'u2' } },
  { rol: 'user', texto: 'ahora ROJO' },
  { rol: 'assistant', texto: 'OK', punto: { sesion: 's1', id: 'u3' } },
];

test('solo se continúa desde respuestas acabadas con algo detrás en el contexto', () => {
  assert.equal(A.canContinueFrom(conv, 1), true);
  assert.equal(A.canContinueFrom(conv, 3), true);
  assert.equal(A.canContinueFrom(conv, 0), false, 'un mensaje del usuario');
  assert.equal(A.canContinueFrom(conv, 5), false, 'la última respuesta: no hay nada que olvidar');
  assert.equal(A.canContinueFrom(conv, 1, 1), false, 'ya es el punto elegido');
  const cortada = A.markOutside(conv, 3);
  assert.equal(A.canContinueFrom(cortada, 3), false, 'lo de después ya está fuera del contexto');
  assert.equal(A.canContinueFrom(cortada, 5), false, 'fuera del contexto');
  assert.equal(A.canContinueFrom([...conv.slice(0, 2), { rol: 'user', texto: 'x' }, { rol: 'assistant', texto: '⚠', error: true }], 1), true);
});

test('lo posterior al punto se marca fuera del contexto, sin borrarlo', () => {
  const m = A.markOutside(conv, 1);
  assert.equal(m.length, conv.length);
  assert.deepEqual(m.map(x => Boolean(x.fuera)), [false, false, true, true, true, true]);
  assert.equal(A.isOutside(conv, 2, 1), true);
  assert.equal(A.isOutside(conv, 1, 1), false);
  assert.equal(A.isOutside(m, 4), true);
  assert.equal(A.isOutside(conv, 4), false);
});

test('la petición del corte: la sesión y el mensaje del punto, y el traspaso de reserva', () => {
  const c = A.cutRequest(conv, 1);
  assert.equal(c.sesion, 's1');
  assert.equal(c.punto, 'u1');
  assert.match(c.traspaso, /AZUL/);
  assert.doesNotMatch(c.traspaso, /VERDE|ROJO/);
  // Sin punto (respuesta de antes, o agente sin cortes): solo el traspaso.
  const sin = A.cutRequest(conv.map(m => ({ ...m, punto: undefined })), 3);
  assert.equal(sin.punto, undefined);
  assert.match(sin.traspaso, /VERDE/);
});

test('el traspaso: lo pedido, lo respondido, los ficheros y el estado, sin lo de después', () => {
  const t = A.handoff(conv, 3);
  assert.match(t, /^\[Traspaso de pgocode\]/);
  assert.match(t, /Usuario: recuerda la palabra AZUL/);
  assert.match(t, /Agente: OK, AZUL/);
  assert.match(t, /Ficheros cambiados: src\/a\.go/);
  assert.match(t, /Ficheros tocados en la conversación \(1\)/);
  assert.match(t, /Usuario: ahora recuerda VERDE/);
  assert.doesNotMatch(t, /ROJO/);
  assert.match(t, /Nuevo mensaje del usuario:\]$/);
});

test('el traspaso no pasa de unos 3k tokens: la primera petición y lo más reciente', () => {
  const largo: Mensaje[] = [{ rol: 'user', texto: 'PRIMERA petición' }];
  for (let i = 0; i < 60; i++) largo.push({ rol: 'assistant', texto: `respuesta ${i} ` + 'x'.repeat(900) }, { rol: 'user', texto: `pregunta ${i + 1}` });
  largo.push({ rol: 'assistant', texto: 'ÚLTIMA respuesta' });
  const t = A.handoff(largo, largo.length - 1);
  assert.ok(t.length <= A.MAX_HANDOFF, `largo ${t.length}`);
  assert.match(t, /PRIMERA petición/);
  assert.match(t, /ÚLTIMA respuesta/);
  assert.match(t, /mensajes intermedios omitidos/);
  // Un único mensaje enorme también se recorta.
  const enorme = A.handoff([{ rol: 'user', texto: 'y'.repeat(50000) }, { rol: 'assistant', texto: 'z'.repeat(50000) }], 1, 2000);
  assert.ok(enorme.length <= 2000, `largo ${enorme.length}`);
});

test('los mensajes ya fuera del contexto, con error o notas de pgocode no van en el traspaso', () => {
  const ms: Mensaje[] = [
    { rol: 'user', texto: 'uno' }, { rol: 'assistant', texto: 'r1' },
    { rol: 'user', texto: 'descartado', fuera: true }, { rol: 'assistant', texto: 'r-descartada', fuera: true },
    { rol: 'user', texto: 'falla' }, { rol: 'assistant', texto: '⚠ error', error: true },
    { rol: 'user', texto: 'descarta el plan 2' }, { rol: 'assistant', texto: 'Plan descartado', nota: true },
    { rol: 'user', texto: 'dos' }, { rol: 'assistant', texto: 'r2' },
  ];
  const t = A.handoff(ms, 9);
  assert.doesNotMatch(t, /Usuario: descartado|r-descartada|⚠|plan 2|Plan descartado/);
  assert.match(t, /r1[\s\S]*dos[\s\S]*r2/);
});
