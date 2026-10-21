// Pruebas de las preguntas aparte: npm test (node --test, sin dependencias ni tokens).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as A from './aparte.ts';
import type { Mensaje } from './api.ts';

const ocupado = { ocupado: true, ultimo: 'Voy a crear el servidor HTTP.' };
const destino = (t: string, s: A.Situacion = ocupado) => A.decide(t, s);

test('con el agente trabajando: preguntas de estado, en local', () => {
  for (const t of ['actualiza el estado de la refactorizacón', '¿Cómo va?', 'cómo vas', '¿por dónde vas?', '¿qué estás haciendo?',
    'dame el estado', '¿cuánto falta?', 'estado', '¿ya has terminado?', '¿sigues ahí?', 'y cómo va eso', 'resume el progreso'])
    assert.equal(destino(t).destino, 'estado', t);
});

test('órdenes, respuestas y avisos al agente: a la conversación, sin dudar', () => {
  for (const t of ['haz también los tests', 'añade un Makefile', 'no toques el servidor', 'cambia el puerto a 8080', 'para ya',
    'sí, adelante', 'vale', 'ok', 'ten en cuenta que hay Windows', 'mejor usa Go 1.24', 'el puerto de Odoo es 8069',
    'Recuerda que no hay que hacer commit', 'también el README', 'gracias!']) {
    const d = destino(t);
    assert.equal(d.destino, 'conversacion', t);
    assert.ok(!d.duda, t + ': ' + d.motivo);
  }
});

test('preguntas: las claramente ajenas, sueltas; el resto, al clasificador', () => {
  assert.equal(destino('¿Qué significa idempotente?').destino, 'suelta');
  assert.equal(destino('¿quién inventó el lenguaje Go?').destino, 'suelta');
  for (const t of ['¿por qué has elegido esa estructura de directorios?', '¿qué puerto usa postgres por defecto?', '¿hay tests en el repo?']) {
    const d = destino(t);
    assert.equal(d.destino, 'conversacion', t);
    assert.ok(d.duda, t);
  }
  // Una pregunta ajena que nombra algo del trabajo: no es suelta.
  assert.notEqual(destino('¿qué significa `ctx` en servidor.go?').destino, 'suelta');
});

test('si el agente preguntó algo, lo que se escribe es su respuesta', () => {
  const d = destino('¿la de producción o la de pruebas?', { ocupado: true, ultimo: '¿Qué base de datos uso?' });
  assert.equal(d.destino, 'conversacion');
  assert.ok(!d.duda);
});

test('agente libre, adjuntos, órdenes de planes o decisión desactivada: a la conversación', () => {
  assert.equal(destino('¿cómo va?', { ocupado: false }).destino, 'conversacion');
  assert.ok(!destino('¿qué significa idempotente?', { ocupado: false }).duda, 'libre: sin clasificador');
  assert.equal(destino('¿cómo va?', { ...ocupado, adjuntos: 1 }).destino, 'conversacion');
  assert.equal(destino('ejecuta el plan 2', { ...ocupado, ordenPlan: true }).destino, 'conversacion');
  assert.equal(destino('¿cómo va?', { ...ocupado, activo: false }).destino, 'conversacion');
});

test('prefijos: «?» estado y «//» aparte, siempre (también con el agente libre)', () => {
  assert.deepEqual(A.decide('?', { ocupado: false }), { destino: 'estado', texto: '', forzado: true, motivo: 'prefijo «?»' });
  assert.equal(A.decide('// ¿qué hora es en Tokio?', { ocupado: false }).destino, 'suelta');
  const d = A.decide('// ¿por qué has cambiado servidor.go?', { ocupado: false, ultimo: 'Listo.', hayConversacion: true });
  assert.equal(d.destino, 'contexto');
  // Aunque el agente aún no haya dicho nada en el turno: si hay conversación, con contexto.
  assert.equal(A.decide('// ¿cuál es la palabra clave?', { ocupado: true, ultimo: '', hayConversacion: true }).destino, 'contexto');
  assert.equal(A.decide('// ¿cuál es la palabra clave?', { ocupado: false }).destino, 'suelta', 'sin conversación no hay contexto');
  assert.equal(d.texto, '¿por qué has cambiado servidor.go?');
  assert.equal(A.decide('// ¿qué hora es?', { ...ocupado, activo: false }).destino, 'suelta', 'los prefijos valen aunque esté desactivado');
});

test('prefijo «!»: a la conversación, aunque parezca una pregunta de estado o esté desactivado', () => {
  assert.deepEqual(A.decide('! ¿cómo va?', ocupado), { destino: 'conversacion', texto: '¿cómo va?', forzado: true, motivo: 'prefijo «!»' });
  assert.equal(A.decide('! el puerto es 8069', { ...ocupado, activo: false }).texto, 'el puerto es 8069');
  assert.equal(A.decide('¡Perfecto!', ocupado).destino, 'conversacion', '«¡» no es el prefijo');
  assert.ok(!A.decide('!importante', ocupado).forzado, 'el prefijo va seguido de un espacio');
});

test('etiqueta y cambio de destino', () => {
  assert.equal(A.etiqueta(A.decide('¿cómo va?', ocupado)), '→ Estado (sin tokens)');
  assert.equal(A.etiqueta(A.decide('haz los tests', ocupado)), '→ Conversación');
  assert.match(A.etiqueta(A.decide('¿hay tests en el repo?', ocupado)), /lo decide pgocode/);
  assert.equal(A.etiqueta({ destino: 'suelta', texto: 'x', motivo: '' }), '→ Aparte · suelta');
  assert.deepEqual(['conversacion', 'contexto', 'suelta', 'estado'].map(d => A.siguiente(d as A.Destino)), ['contexto', 'suelta', 'estado', 'conversacion']);
});

test('aprende de las correcciones (por la firma del mensaje)', () => {
  assert.equal(A.firma('¿Oye, qué tal va el despliegue en Odoo?'), 'despliegue odoo');
  let a: A.Aprendidas = {};
  a = A.aprende(a, '¿Qué versión de Odoo usa el cliente?', 'conversacion');
  assert.equal(destino('¿qué versión de Odoo tiene?', { ...ocupado, aprendidas: a }).destino, 'conversacion');
  assert.ok(!destino('¿qué versión de Odoo tiene?', { ...ocupado, aprendidas: a }).duda);
  a = A.aprende(a, '¿qué significa idempotente?', 'contexto');
  assert.equal(destino('¿Qué significa idempotente aquí?', { ...ocupado, aprendidas: a }).destino, 'contexto');
  for (let i = 0; i < 250; i++) a = A.aprende(a, `palabra${i} otra cosa`, 'suelta');
  assert.equal(Object.keys(a).length, 200);
});

const turno: Mensaje = {
  rol: 'assistant', texto: 'Empiezo.Sigo con el servidor.',
  uso: { entrada: 120000, salida: 3000, coste: 0.42 },
  partes: [
    { tipo: 'texto', texto: 'Empiezo.' },
    { tipo: 'herramienta', h: { id: '1', nombre: 'Write', resumen: '/home/x/intradema/internal/config/config.go', estado: 'ok' } },
    { tipo: 'herramienta', h: { id: '2', nombre: 'Write', resumen: '/home/x/intradema/internal/odoo/cliente.go', estado: 'ok' } },
    { tipo: 'herramienta', h: { id: '3', nombre: 'Bash', resumen: 'go build ./...', estado: 'error' } },
    { tipo: 'texto', texto: 'Sigo con el servidor.' },
    { tipo: 'herramienta', h: { id: '4', nombre: 'Write', resumen: '/home/x/intradema/internal/servidor/servidor.go', estado: 'curso' } },
  ],
  tareas: [{ id: 'l', tipo: 'lista', elementos: [
    { texto: 'Inicializar módulo Go', estado: 'hecho' }, { texto: 'Servidor HTTP', estado: 'hecho' },
    { texto: 'Arranque CLI/LAN y servicio de Windows', estado: 'curso' }, { texto: 'Rediseñar interfaz', estado: 'pendiente' }] },
  { id: 's', tipo: 'subagente', titulo: 'Revisar estilos', estado: 'curso', progreso: 'Read estilos.css' }],
};

test('estado del turno: tareas, ficheros, órdenes, consumo y lo último que ha dicho', () => {
  const e = A.resumenEstado(turno, { trabajando: true, segundos: 754, enCola: 1 });
  assert.match(e, /Trabajando\*\* desde hace 12 min · 123k tokens · \$0\.42 · 1 en cola/);
  assert.match(e, /2 de 4 hechas/);
  assert.match(e, /Ahora: Arranque CLI\/LAN/);
  assert.match(e, /Pendientes: Rediseñar interfaz/);
  assert.match(e, /subagente «Revisar estilos» \(Read estilos\.css\)/);
  assert.match(e, /Ficheros cambiados \(3\):.*config\/config\.go.*servidor\/servidor\.go/);
  assert.match(e, /Última orden:\*\* `go build/);
  assert.match(e, /Ahora mismo:\*\* Write/);
  assert.match(e, /Sigo con el servidor/);
  assert.match(A.resumenEstado(undefined, { trabajando: true }), /acaba de empezar/);
  assert.match(A.resumenEstado(turno, { trabajando: false }), /Último turno/);
});

test('resumen de la conversación para el modelo barato (sin copia de la sesión)', () => {
  const ms: Mensaje[] = [{ rol: 'user', texto: 'Refactoriza intradema a Go' }, turno];
  const r = A.resumenContexto(ms);
  assert.match(r, /^Primera petición del usuario: Refactoriza intradema a Go/);
  assert.match(r, /Agente: Sigo con el servidor/);
  assert.match(r, /2 de 4 hechas/);
  assert.ok(A.resumenContexto([...ms, { rol: 'user', texto: 'x'.repeat(50000) }], 3000).length <= 3000);
});

test('incorporar: va al final del próximo mensaje', () => {
  assert.equal(A.incorporacion([]), '');
  const t = A.incorporacion([{ pregunta: '¿qué puerto?', respuesta: 'El 8069.' }]);
  assert.match(t, /Pregunta: ¿qué puerto\?\nRespuesta: El 8069\./);
  assert.ok(t.startsWith('\n\n[Preguntas que hice aparte'));
});
