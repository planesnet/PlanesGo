// Pruebas de los textos del panel Actividad: npm test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as A from './activityPanel.ts';
import type { Sesion } from './sesiones.ts';
import type { Reparto } from './reparto.ts';

const ses = (x: Partial<Sesion>): Sesion => ({ id: 's', nombre: '', proyecto: 1, carpeta: '/c', rama: 'main', agente: 'claude-code', creada: 0, ultima: 0, ...x });

test('childStatus: según el reparto guardado, no solo la rama', () => {
  const madre = ses({ id: 'm', rama: 'main' });
  const h = (n: number) => ses({ id: 'h' + n, madre: 'm', parte: n, rama: 'pgo/m-' + n });
  const r: Reparto = { id: 'r', madre: 'm', proyecto: 1, carpeta: '/c', ramaMadre: 'main', intro: '', outro: '', creado: 0, hijas: [
    { n: 1, idx: 1, parte: 'a', sesion: 'h1', estado: 'hecha', union: 'conflicto', rama: 'pgo/m-1', carpeta: '/w1', ficheros: ['x.go'] },
    { n: 2, idx: 2, parte: 'b', sesion: 'h2', estado: 'parada', union: 'sin_cambios', quitada: true },
    { n: 3, idx: 3, parte: 'c', sesion: 'h3', estado: 'trabajando', union: 'sin', rama: 'pgo/m-3' },
    { n: 4, idx: 4, parte: 'd', sesion: 'h4', estado: 'hecha', union: 'unida' },
  ] };
  const c1 = A.childStatus(h(1), madre, [r]);
  assert.equal(c1.kind, 'aviso');
  assert.match(c1.text, /NO unida: choca en x\.go/);
  assert.doesNotMatch(c1.text, /se unirá/);
  assert.equal(A.childStatus(h(2), madre, [r]).text, 'parada, sin cambios: nada que unir');
  assert.match(A.childStatus(h(3), madre, [r]).text, /trabajando .*se unirá a main/);
  assert.equal(A.childStatus(h(4), madre, [r]).kind, 'ok');
  // Sin reparto guardado: por la rama.
  assert.match(A.childStatus(ses({ id: 'z', rama: 'main' }), madre, []).text, /^unida/);
});
