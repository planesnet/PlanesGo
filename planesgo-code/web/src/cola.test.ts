// Pruebas de la cola de mensajes del chat: npm test (node --test, sin dependencias).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as C from './cola.ts';

const textos = (c: C.Cola<string>) => c.mensajes.map(m => m.texto);

test('encola en orden y no encola textos vacíos', () => {
  let c = C.vacia<string>();
  c = C.encola(c, '  uno  ', ['a.png']);
  c = C.encola(c, '   ');
  c = C.encola(c, 'dos');
  assert.deepEqual(textos(c), ['uno', 'dos']);
  assert.deepEqual(c.mensajes[0].adjuntos, ['a.png']);
});

test('con el agente ocupado no sale nada; libre, sale el primero', () => {
  const c = C.encola(C.encola(C.vacia<string>(), 'uno'), 'dos');
  assert.equal(C.siguiente(c, true).enviar, undefined);
  const r = C.siguiente(c, false);
  assert.equal(r.enviar?.texto, 'uno');
  assert.deepEqual(textos(r.cola), ['dos']);
});

test('cola vacía: no hay siguiente', () => {
  assert.equal(C.siguiente(C.vacia(), false).enviar, undefined);
});

test('un fallo detiene la cola y empezar un turno la reanuda', () => {
  let c = C.falla(C.encola(C.vacia<string>(), 'uno'));
  assert.equal(c.parada, true);
  assert.equal(C.siguiente(c, false).enviar, undefined, 'detenida no envía');
  assert.deepEqual(textos(c), ['uno'], 'detenida conserva los mensajes');
  c = C.empieza(c);
  assert.equal(C.siguiente(c, false).enviar?.texto, 'uno');
});

test('sin cambios devuelve el mismo objeto (React no vuelve a pintar)', () => {
  const c = C.vacia<string>();
  assert.equal(C.empieza(c), c);
  const p = C.falla(c);
  assert.equal(C.falla(p), p);
  assert.equal(C.encola(c, ''), c);
});

test('quitar un mensaje de en medio', () => {
  const c = C.encola(C.encola(C.encola(C.vacia<string>(), 'uno'), 'dos'), 'tres');
  assert.deepEqual(textos(C.quita(c, 1)), ['uno', 'tres']);
});

test('parar: la cola vuelve delante de lo escrito, con sus adjuntos', () => {
  const c = C.encola(C.encola(C.vacia<string>(), 'uno', ['a']), 'dos', ['b']);
  assert.deepEqual(C.devuelve(c.mensajes, 'borrador', ['x']), { texto: 'uno\n\ndos\n\nborrador', adjuntos: ['a', 'b', 'x'] });
  assert.deepEqual(C.devuelve(c.mensajes, '', []), { texto: 'uno\n\ndos', adjuntos: ['a', 'b'] });
});

test('editar: el mensaje va detrás de lo ya escrito', () => {
  assert.deepEqual(C.devuelve([{ texto: 'dos', adjuntos: ['b'] }], 'uno', ['a'], false), { texto: 'uno\n\ndos', adjuntos: ['a', 'b'] });
});

// Simulación del chat: un agente que tarda en responder y el usuario escribiendo mientras tanto.
class ChatSimulado {
  colas: Record<string, C.Cola<string>> = {};
  enCurso: string | null = null; // sesión con turno en marcha
  enviados: string[] = [];
  cuadro = '';
  cola(id: string) { return this.colas[id] || C.vacia<string>(); }
  envia(id: string, t: string) {
    if (this.enCurso) return;
    this.colas[id] = C.empieza(this.cola(id));
    this.enCurso = id;
    this.enviados.push(`${id}:${t}`);
  }
  escribe(id: string, t: string) {
    if (this.enCurso) this.colas[id] = C.encola(this.cola(id), t);
    else this.envia(id, t);
  }
  termina(id: string, fallo = false) {
    if (fallo) this.colas[id] = C.falla(this.cola(id));
    this.enCurso = null;
    this.efecto(id);
  }
  para(id: string) {
    this.cuadro = C.devuelve(this.cola(id).mensajes, this.cuadro, []).texto;
    this.colas[id] = C.falla({ ...this.cola(id), mensajes: [] });
    this.enCurso = null;
    this.efecto(id);
  }
  // El useEffect del chat: con el agente libre, sale el siguiente de la sesión abierta.
  efecto(id: string) {
    const r = C.siguiente(this.cola(id), Boolean(this.enCurso));
    if (!r.enviar) return;
    this.colas[id] = r.cola;
    this.envia(id, r.enviar.texto);
  }
}

test('flujo: los mensajes en cola se envían uno a uno al acabar cada turno', () => {
  const c = new ChatSimulado();
  c.escribe('s1', 'primero');
  c.escribe('s1', 'segundo');
  c.escribe('s1', 'tercero');
  assert.deepEqual(c.enviados, ['s1:primero']);
  c.termina('s1');
  assert.deepEqual(c.enviados, ['s1:primero', 's1:segundo']);
  c.termina('s1');
  c.termina('s1');
  assert.deepEqual(c.enviados, ['s1:primero', 's1:segundo', 's1:tercero']);
  assert.equal(c.enCurso, null);
});

test('flujo: un turno fallido detiene la cola; un envío a mano la reanuda', () => {
  const c = new ChatSimulado();
  c.escribe('s1', 'primero');
  c.escribe('s1', 'segundo');
  c.termina('s1', true);
  assert.deepEqual(c.enviados, ['s1:primero'], 'tras el fallo no se envía «segundo»');
  c.escribe('s1', 'corrige');
  assert.deepEqual(c.enviados, ['s1:primero', 's1:corrige']);
  c.termina('s1');
  assert.deepEqual(c.enviados, ['s1:primero', 's1:corrige', 's1:segundo']);
});

test('flujo: parar devuelve la cola al cuadro y no envía nada más', () => {
  const c = new ChatSimulado();
  c.escribe('s1', 'primero');
  c.escribe('s1', 'segundo');
  c.escribe('s1', 'tercero');
  c.cuadro = 'a medias';
  c.para('s1');
  assert.deepEqual(c.enviados, ['s1:primero']);
  assert.equal(c.cuadro, 'segundo\n\ntercero\n\na medias');
  assert.deepEqual(c.cola('s1').mensajes, []);
});

test('flujo: cada sesión tiene su propia cola', () => {
  const c = new ChatSimulado();
  c.escribe('s1', 'uno');
  c.escribe('s1', 'dos');
  c.escribe('s2', 'otra');
  assert.deepEqual(c.cola('s2').mensajes.map(m => m.texto), ['otra']);
  c.termina('s1');
  assert.deepEqual(c.enviados, ['s1:uno', 's1:dos'], 'la cola de s2 espera a que se abra s2');
  c.termina('s1');
  c.efecto('s2');
  assert.deepEqual(c.enviados, ['s1:uno', 's1:dos', 's2:otra']);
});
