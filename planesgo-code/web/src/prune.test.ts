// Pruebas de la poda y la vigencia en la interfaz: npm test (node --test, sin dependencias ni tokens).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as P from './prune.ts';
import type { Mensaje } from './api.ts';

const conv: Mensaje[] = [
  { rol: 'user', texto: 'lee app.go' },
  { rol: 'assistant', texto: 'leído', punto: { sesion: 's', id: 'm1' }, partes: [
    { tipo: 'herramienta', h: { id: 'call_1', nombre: 'read', estado: 'ok' } },
    { tipo: 'texto', texto: 'leído' },
  ] },
  { rol: 'user', texto: 'cambia el color' },
  { rol: 'assistant', texto: 'hecho', punto: { sesion: 's', id: 'm2' } },
  { rol: 'user', texto: 'sube la versión' },
  { rol: 'assistant', texto: '' },
];

const informe: P.PruneReport = { retirados: 20000, intercambios: [
  { punto: 'm1', vigencia: 1.2, hace: 4, relacion: 0.45, horas: 3 },
  { punto: 'm2', vigencia: 3.4, hace: 3, relacion: 0.21, horas: 2, fuera: true },
], piezas: [{ id: 'call_1', vigencia: 4.2, hace: 4, horas: 3, superada: 'lectura superada', fuera: true }] };

test('el informe da su vigencia a cada intercambio (por su punto, con su mensaje del usuario) y a cada herramienta', () => {
  const m = P.applyPruneReport(conv, informe);
  assert.equal(m[1].vig?.vigencia, 1.2);
  assert.equal(m[0].vig?.vigencia, 1.2);
  assert.equal(m[3].vig?.fuera, true);
  assert.equal(m[2].vig?.relacion, 0.21);
  assert.equal(m[4].vig, undefined, 'el turno en curso no se toca');
  assert.equal(m[5].vig, undefined);
  const h = m[1].partes?.[0];
  assert.ok(h && h.tipo === 'herramienta' && h.h.vig?.superada === 'lectura superada');
  assert.equal(m[1].partes?.[1], conv[1].partes?.[1], 'el texto es el mismo objeto');
});

test('un informe nuevo cambia la vigencia; lo que no cambia es el mismo objeto; lo que no sale se queda', () => {
  const m = P.applyPruneReport(conv, informe);
  const n = P.applyPruneReport(m, { retirados: 0, intercambios: [{ punto: 'm1', vigencia: 1.2, hace: 4, relacion: 0.45, horas: 3 }] });
  assert.equal(n[1], m[1]);
  assert.equal(n[3], m[3], 'm2 no sale: se queda como estaba');
  const o = P.applyPruneReport(m, { retirados: 0, intercambios: [{ punto: 'm2', vigencia: 0.5, hace: 3, relacion: 0.6 }] });
  assert.equal(o[3].vig?.fuera, undefined, 'vuelve al contexto');
  assert.equal(o[3].vig?.vigencia, 0.5);
});

test('la opacidad: 1 en 0, baja suave hasta 0,4 en 3 o más', () => {
  assert.equal(P.vigOpacity(undefined), 1);
  assert.equal(P.vigOpacity({ vigencia: 0 }), 1);
  assert.equal(P.vigOpacity({ vigencia: 1.5 }), 0.7);
  assert.equal(P.vigOpacity({ vigencia: 3 }), 0.4);
  assert.equal(P.vigOpacity({ vigencia: 7 }), 0.4);
  let prev = 1;
  for (let v = 0.25; v <= 3; v += 0.25) { const o = P.vigOpacity({ vigencia: v }); assert.ok(o < prev); prev = o; }
  // Dentro de un mensaje ya desvanecido, la pieza se ve con su opacidad (se multiplican).
  assert.equal(P.relativeOpacity(0.4, 0.8), 0.5);
  assert.equal(P.relativeOpacity(0.9, 0.8), 1);
});

test('el porqué al pasar el ratón', () => {
  assert.equal(P.vigTitle({ vigencia: 2.4, hace: 6, relacion: 0.21, horas: 3 }), 'vigencia 2,4: hace 6 intercambios · poca relación (0,21) · hace 3 h');
  assert.equal(P.vigTitle({ vigencia: 0 }), 'vigencia 0: reciente o instrucciones: vital');
  assert.equal(P.vigTitle({ vigencia: 3.6, hace: 1, relacion: 0.55, horas: 0.2, superada: 'lectura superada', fuera: true }),
    'vigencia 3,6: hace 1 intercambio · mucha relación (0,55) · hace 12 min · lectura superada · fuera del contexto del agente');
  assert.equal(P.vigTitle({ vigencia: 1, hace: 9, horas: 50, nota: 'salida recortada' }), 'vigencia 1,0: hace 9 intercambios · hace 2 días · salida recortada');
});

test('la línea del turno', () => {
  assert.equal(P.prunedLine({ tokens: 48_400 }), '✂ pgocode ha retirado 48k tokens del historial');
  assert.equal(P.prunedLine({ tokens: 20_000, repuestos: 1 }), '✂ pgocode ha retirado 20k tokens del historial; ha repuesto 1 pieza que vuelve a ser relevante');
  assert.equal(P.prunedLine({ tokens: 0, repuestos: 3 }), '↺ pgocode ha repuesto 3 piezas que vuelven a ser relevantes');
  assert.equal(P.prunedLine({ tokens: 0 }), '');
});

test('vigTitle: pasos y compactación del agente', () => {
  assert.equal(P.vigTitle({ vigencia: 3.2, pasos: 14, relacion: 0.3 }), 'vigencia 3,2: hace 14 pasos · poca relación (0,30)');
  assert.equal(P.vigTitle({ vigencia: 3, superada: 'resumido por la compactación del agente', fuera: true }),
    'vigencia 3,0: resumido por la compactación del agente · fuera del contexto del agente');
});
