// Pruebas de la barra de prueba del chat: npm test (node --test, sin dependencias).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as L from './lanzador.ts';

test('tamaños: el ordenador no se emula; horizontal intercambia ancho y alto', () => {
  assert.equal(L.tamano('ordenador', 'horizontal'), null);
  assert.deepEqual(L.tamano('movil', 'vertical'), { ancho: 390, alto: 844 });
  assert.deepEqual(L.tamano('movil', 'horizontal'), { ancho: 844, alto: 390 });
  assert.deepEqual(L.tamano('tablet', 'horizontal'), { ancho: 1180, alto: 820 });
});

test('escala: cabe entera en el hueco y nunca se amplía', () => {
  assert.equal(L.escalaMarco({ ancho: 390, alto: 844 }, { ancho: 2000, alto: 2000 }), 1);
  const f = L.escalaMarco({ ancho: 820, alto: 1180 }, { ancho: 600, alto: 900 });
  assert.ok(820 * f <= 600 - 16 + 1e-9 && 1180 * f <= 900 - 16 + 1e-9);
  assert.equal(L.escalaMarco({ ancho: 390, alto: 844 }, { ancho: 0, alto: 0 }), 1, 'sin hueco medido: sin escalar');
});

test('agente de usuario móvil solo en tablet y móvil', () => {
  assert.match(L.agenteUsuario('movil')!, /Mobile/);
  assert.doesNotMatch(L.agenteUsuario('tablet')!, /Mobile/);
  assert.equal(L.agenteUsuario('ordenador'), undefined);
});

test('direcciones: lo local va por http; un puerto solo vale', () => {
  assert.equal(L.normalizaUrl('localhost:8888'), 'http://localhost:8888/');
  assert.equal(L.normalizaUrl(':5173'), 'http://localhost:5173/');
  assert.equal(L.normalizaUrl('8069'), 'http://localhost:8069/');
  assert.equal(L.normalizaUrl('127.0.0.1:3000/app'), 'http://127.0.0.1:3000/app');
  assert.equal(L.normalizaUrl('miapp.test'), 'http://miapp.test/');
  assert.equal(L.normalizaUrl('ejemplo.com'), 'https://ejemplo.com/');
  assert.equal(L.normalizaUrl('https://x.com/a'), 'https://x.com/a');
  assert.equal(L.normalizaUrl('javascript:alert(1)'), '');
  assert.equal(L.normalizaUrl('  '), '');
});

test('sugerencias: en marcha primero, sin repetir ni vacías', () => {
  assert.deepEqual(L.sugerencias(['http://localhost:8888', undefined], ['localhost:8888', 'localhost:8069'], ':5173'),
    ['http://localhost:8888/', 'http://localhost:8069/', 'http://localhost:5173/']);
});

test('lo guardado: tolera basura y conserva lo válido', () => {
  assert.deepEqual(L.leePrueba(null), L.PRUEBA_DEFECTO);
  assert.deepEqual(L.leePrueba('no es json'), L.PRUEBA_DEFECTO);
  const p = L.leePrueba(JSON.stringify({ destino: 'sistema', url: 'http://localhost:8888/', dispositivo: 'movil', orientacion: 'vertical', programa: 3 }));
  assert.deepEqual(p, { destino: 'sistema', url: 'http://localhost:8888/', dispositivo: 'movil', orientacion: 'vertical' });
  assert.equal(L.leePrueba(JSON.stringify({ destino: 'raro', dispositivo: 'tele' })).destino, 'interno');
});

test('orientación por defecto del dispositivo', () => {
  assert.equal(L.orientacionDe('ordenador'), 'horizontal');
  assert.equal(L.orientacionDe('movil'), 'vertical');
});
