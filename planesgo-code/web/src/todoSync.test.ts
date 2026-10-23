// Pruebas de la lista de tareas sincronizada con lo que hace el agente: npm test.
// Los turnos de src/testdata/todo-*.json son reales (OpenCode con Gemini, de la BD de pgocode, y Claude Code de las
// muestras de internal/llm/testdata pasadas por el lector de Go), con la tarea que de verdad se hacía en cada acción
// («verdad», etiquetada a mano; -1: no está claro o aún no hay lista).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import * as S from './todoSync.ts';
import type { Elemento } from './tareas.ts';
import type { Herramienta, Mensaje } from './api.ts';

interface Turno { nota: string; eventos: ({ h: Herramienta } | { lista: Elemento[] } | { texto: string })[]; verdad: number[] }
const turno = (n: string): Turno => JSON.parse(readFileSync(new URL(`./testdata/todo-${n}.json`, import.meta.url), 'utf8'));

// replay: recorre el turno como lo ve el chat y, en cada acción, la tarea en curso según el agente (su última lista) y
// según pgocode (syncTodo con las acciones desde esa lista). Devuelve también la vista al acabar.
function replay(t: Turno) {
  let lista: Elemento[] | undefined, desde: Herramienta[] = [];
  const todas: Herramienta[] = [];
  const pasos: { agente: number; inferida: number; verdad: number }[] = [];
  let k = 0, texto = '';
  for (const e of t.eventos) {
    if ('lista' in e) { lista = e.lista; desde = []; continue; }
    if ('texto' in e) { texto = e.texto; continue; }
    texto = '';
    desde.push(e.h);
    todas.push(e.h);
    const verdad = t.verdad[k++];
    if (!lista) continue;
    const v = S.syncTodo(lista, desde);
    pasos.push({ agente: lista.findIndex(x => x.estado === 'curso'), inferida: v.ahora ? v.ahora.i : -1, verdad });
  }
  const fin = lista ? S.syncTodo(lista, desde, { acabado: true, textoFinal: texto, turno: todas }) : undefined;
  return { pasos, fin };
}

const aciertos = (ps: { agente: number; inferida: number; verdad: number }[]) => {
  const et = ps.filter(p => p.verdad >= 0);
  return { n: et.length, agente: et.filter(p => p.agente === p.verdad).length, inferida: et.filter(p => p.inferida === p.verdad).length };
};

const TURNOS = ['oc-vistas', 'oc-pantalla', 'oc-migracion', 'oc-sin-marcar', 'claude-lista', 'claude-lista-resume'];

test('medida: la tarea en curso inferida acierta al menos tanto como la del agente, y más en los turnos desfasados', () => {
  const tot = { n: 0, agente: 0, inferida: 0 };
  const filas: string[] = [];
  for (const n of TURNOS) {
    const a = aciertos(replay(turno(n)).pasos);
    tot.n += a.n; tot.agente += a.agente; tot.inferida += a.inferida;
    filas.push(`${n.padEnd(20)} ${String(a.n).padStart(3)} acciones · agente ${a.agente} · inferida ${a.inferida}`);
    assert.ok(a.inferida >= a.agente, `${n}: la inferencia no puede acertar menos que el agente (${a.inferida} < ${a.agente})`);
  }
  const pct = (x: number) => ((100 * x) / tot.n).toFixed(1) + ' %';
  console.log(filas.join('\n') + `\nTOTAL ${tot.n} acciones etiquetadas · agente ${tot.agente} (${pct(tot.agente)}) · inferida ${tot.inferida} (${pct(tot.inferida)})`);
  assert.ok(tot.inferida > tot.agente);
});

test('turnos bien llevados por el agente: la inferencia no cambia nada mientras trabaja', () => {
  for (const n of ['oc-vistas', 'oc-migracion', 'claude-lista']) {
    for (const p of replay(turno(n)).pasos) if (p.agente >= 0) assert.equal(p.inferida, p.agente, n);
  }
});

test('OpenCode con la lista desfasada: ▷ en la que hace y la marcada, probablemente hecha', () => {
  const t = turno('oc-pantalla');
  // Hasta la acción 45 (lee TaskModal y ProjectModal, de la 4.ª tarea, con la 3.ª en curso).
  let lista: Elemento[] = [], desde: Herramienta[] = [], k = 0;
  for (const e of t.eventos) {
    if ('lista' in e) { lista = e.lista; desde = []; } else if ('h' in e) { desde.push(e.h); if (k++ === 45) break; }
  }
  const v = S.syncTodo(lista, desde);
  assert.equal(v.ahora?.i, 3);
  assert.equal(v.ahora?.vista, 'inferida');
  assert.match(v.ahora!.motivo!, /ProjectModal|TaskModal/);
  assert.equal(v.items[2].vista, 'probable'); // la que el agente deja «en curso» (ya editó ProjectsListView y ProjectReportsView)
  assert.deepEqual(v.pendientes.map(x => x.i), [4]);
  assert.deepEqual(v.hechas.map(x => x.i), [0, 1, 2]);
  assert.equal(v.done, 2);
  assert.equal(v.probables, 1);
  // Al acabar (la última acción es tsc): la 4.ª y la 5.ª, probablemente hechas (hubo acciones que las cubren).
  const fin = replay(t).fin!;
  assert.deepEqual(fin.items.map(x => x.vista), ['hecha', 'hecha', 'probable', 'probable', 'probable']);
  assert.ok(fin.finished);
});

test('OpenCode sin actualizar la lista en 34 acciones: no inventa (solo lee), y al acabar lo dice', () => {
  const r = replay(turno('oc-sin-marcar'));
  assert.ok(r.pasos.every(p => p.inferida === 0));
  assert.deepEqual(r.fin!.items.map(x => x.vista), ['sin-hacer', 'sin-hacer', 'sin-hacer', 'sin-hacer']);
  assert.match(r.fin!.items[0].motivo!, /dejó en curso/);
});

test('al acabar: lo que el agente dice que ha terminado, probablemente hecho; lo que no se tocó, sin hacer', () => {
  // oc-vistas se corta tras editar index.css (la 4.ª, en curso) sin decir nada: la 5.ª (verificar) no se tocó.
  const vistas = replay(turno('oc-vistas')).fin!;
  assert.deepEqual(vistas.items.map(x => x.vista), ['hecha', 'hecha', 'hecha', 'probable', 'sin-hacer']);
  // El mismo final, si el agente dijera que ha terminado: también la 5.ª.
  const t = turno('oc-vistas');
  const ultima = t.eventos.filter(e => 'lista' in e).pop() as { lista: Elemento[] };
  const acciones = [(t.eventos[t.eventos.length - 1] as { h: Herramienta }).h];
  assert.equal(S.syncTodo(ultima.lista, acciones, { acabado: true, textoFinal: 'Se ha completado y verificado toda la implementación pendiente.' }).items[4].vista, 'probable');
  // Claude: «Borrar b.txt» se añadió y no se tocó.
  const fin = replay(turno('claude-lista-resume')).fin!;
  assert.equal(fin.items[4].vista, 'sin-hacer');
  assert.equal(fin.done, 4);
  assert.ok(!fin.finished);
  // Una lista actualizada tarde (al final, con la 1.ª «en curso» y la 2.ª pendiente) después de hacer las dos.
  const tarde: Elemento[] = [{ texto: 'Editar a.ts', estado: 'curso' }, { texto: 'Editar b.ts', estado: 'pendiente' }, { texto: 'Editar c.ts', estado: 'pendiente' }];
  const e = (f: string) => ({ nombre: 'edit', resumen: f, entrada: JSON.stringify({ filePath: f }) });
  assert.deepEqual(S.syncTodo(tarde, [], { acabado: true, turno: [e('/p/a.ts'), e('/p/b.ts')] }).items.map(x => x.vista), ['probable', 'probable', 'sin-hacer']);
  // Parado por el usuario: nada se da por hecho.
  const l: Elemento[] = [{ texto: 'Editar a.ts', estado: 'curso' }];
  const h = { nombre: 'edit', resumen: '/p/a.ts', entrada: '{"filePath":"/p/a.ts"}' };
  assert.equal(S.syncTodo(l, [h], { acabado: true, parado: true }).items[0].vista, 'pendiente');
  assert.equal(S.syncTodo(l, [h], { acabado: true }).items[0].vista, 'probable');
});

test('agentClaimsDone: «he terminado» sí; «falta…» o «no he podido» no', () => {
  assert.ok(S.agentClaimsDone('He completado las tres tareas.'));
  assert.ok(S.agentClaimsDone('Se ha completado y verificado toda la implementación pendiente'.replace('pendiente', '')));
  assert.ok(!S.agentClaimsDone('He completado la primera; falta la segunda.'));
  assert.ok(!S.agentClaimsDone('No he podido compilar.'));
  assert.ok(!S.agentClaimsDone('Voy a editar el fichero.'));
});

test('presentación: ahora, pendientes, hechas y canceladas aparte', () => {
  const v = S.syncTodo([
    { texto: 'Uno', estado: 'hecho' }, { texto: 'Dos', estado: 'curso' }, { texto: 'Tres', estado: 'pendiente' }, { texto: 'Cuatro', estado: 'cancelada' },
  ], undefined);
  assert.equal(v.ahora?.texto, 'Dos');
  assert.deepEqual(v.pendientes.map(x => x.texto), ['Tres']);
  assert.deepEqual(v.hechas.map(x => x.texto), ['Uno']);
  assert.deepEqual(v.canceladas.map(x => x.texto), ['Cuatro']);
  assert.equal(v.total, 3);
  assert.ok(!v.inferred);
  // Sin ninguna en curso: la siguiente.
  assert.equal(S.syncTodo([{ texto: 'A', estado: 'pendiente' }], undefined).siguiente?.texto, 'A');
});

test('score: fichero, ruta y orden claros; palabras sueltas, solo indicio', () => {
  const a = (nombre: string, entrada: object) => S.actionClues({ nombre, resumen: '', entrada: JSON.stringify(entrada) })!;
  const t = S.taskClues('Añadir ViewNotificationPanel en ViewList.tsx (internal/odoo/)');
  assert.ok(S.score(t, a('edit', { filePath: '/p/src/components/ViewList.tsx' })) >= 3);
  assert.ok(S.score(t, a('write', { filePath: '/p/internal/odoo/cliente.go' })) >= 3);
  assert.equal(S.score(S.taskClues('Verificar compilación TypeScript'), a('bash', { command: 'npx tsc --noEmit' })), 3);
  assert.ok(S.score(S.taskClues('Revisar las vistas de proyecto'), a('edit', { filePath: '/p/ProjectManagementView.tsx' })) < 3);
  assert.equal(S.actionClues({ nombre: 'TodoWrite', resumen: '', entrada: '{}' }), undefined);
});

test('turnTodo y sessionTodo: la lista principal (no la de un subagente) y las acciones desde su actualización', () => {
  const h = (id: string, f: string, padre?: string): Herramienta => ({ id, nombre: 'edit', resumen: f, entrada: JSON.stringify({ filePath: f }), estado: 'ok', padre });
  const m: Mensaje = {
    rol: 'assistant', texto: '', partes: [
      { tipo: 'herramienta', h: h('1', '/p/a.ts') }, { tipo: 'herramienta', h: h('2', '/p/b.ts') }, { tipo: 'herramienta', h: h('3', '/p/b.ts', 'sub') },
    ],
    tareas: [
      { id: 'lista', tipo: 'lista', tras: 1, elementos: [{ texto: 'Cambiar a.ts', estado: 'curso' }, { texto: 'Cambiar b.ts', estado: 'pendiente' }] },
      { id: 'lista:sub', tipo: 'lista', padre: 'sub', elementos: [{ texto: 'Del subagente', estado: 'curso' }] },
    ],
  };
  const v = S.turnTodo(m, true)!;
  assert.equal(v.ahora?.texto, 'Cambiar b.ts');
  assert.equal(v.ahora?.vista, 'inferida');
  // En Actividad, con un turno siguiente sin lista: sus acciones también cuentan.
  const s = S.sessionTodo([m, { rol: 'user', texto: 'sigue' }, { rol: 'assistant', texto: 'Cambiado b.ts.', partes: [] }], false)!;
  // (a.ts se editó antes de la última actualización: al acabar también cuenta.)
  assert.deepEqual(s.items.map(x => x.vista), ['probable', 'probable']);
  // Sin «tras» (mensajes guardados antes de esto): solo la presentación.
  const viejo = { ...m, tareas: [{ ...m.tareas![0], tras: undefined }] };
  assert.equal(S.turnTodo(viejo, false)!.items[0].vista, 'curso');
});
