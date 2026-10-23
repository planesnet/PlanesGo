import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as A from './ayuda.ts';

test('«/help» y «/ayuda» abren la ayuda; lo demás no', () => {
  for (const t of ['/help', '/ayuda', ' /HELP ', '/Ayuda\n']) assert.ok(A.esAyuda(t), t);
  for (const t of ['/help me', 'help', '/plan /help', '/helper']) assert.ok(!A.esAyuda(t), t);
});

test('la ayuda refleja las teclas de envío según el agente y la opción', () => {
  const claves = (o: { alTurno: boolean; inyecta: boolean }) => A.secciones(o).flatMap(s => s.entradas);
  const intro = (o: { alTurno: boolean; inyecta: boolean }) => claves(o).find(e => e.clave === 'Intro')!.que;
  assert.match(intro({ alTurno: true, inyecta: true }), /le llega ya/);
  assert.match(intro({ alTurno: false, inyecta: true }), /a la cola/);
  assert.match(intro({ alTurno: true, inyecta: false }), /a la cola/);
  assert.match(claves({ alTurno: true, inyecta: true }).find(e => e.clave.startsWith('Ctrl+Intro'))!.que, /Nueva línea/);
  assert.match(claves({ alTurno: true, inyecta: true }).find(e => e.clave === '>')!.que, /turno en curso/);
  assert.ok(claves({ alTurno: true, inyecta: true }).some(e => e.clave === 'Alt+Intro'));
  assert.ok(!claves({ alTurno: true, inyecta: false }).some(e => e.clave === 'Alt+Intro'));
  assert.ok(claves({ alTurno: true, inyecta: true }).some(e => e.clave.startsWith('/help')));
  assert.match(claves({ alTurno: false, inyecta: false }).find(e => e.clave === '↑ ↓')!.que, /cola/);
});

test('la ayuda explica las órdenes que hace pgocode sin el agente', () => {
  const e = A.secciones({ alTurno: true, inyecta: true }).find(s => s.titulo.startsWith('Sin el agente'))!.entradas;
  assert.ok(e.some(x => x.clave.includes('compila')) && e.some(x => x.clave.includes('haz commit')));
  assert.match(e.find(x => x.clave.includes('haz commit'))!.que, /Nunca push/);
  assert.match(e.find(x => x.clave.startsWith('Para que'))!.que, /«>»/);
});

test('la ayuda resume el enrutador y remite a su documentación', () => {
  const e = A.secciones({ alTurno: true, inyecta: true }).find(s => s.titulo.startsWith('Quién hace qué'))!.entradas;
  assert.match(e.find(x => x.clave === 'Orden')!.que, /funciones directas .* aparte .* reparto .* agente/);
  assert.ok(e.some(x => x.que.includes('docs/enrutador.md')));
});
