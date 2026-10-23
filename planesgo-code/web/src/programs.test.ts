// Pruebas del borrador de programas: npm test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as P from './programs.ts';

test('programChanges: solo lo nuevo o cambiado se guarda; lo que falta se quita; sin cambios, nada', () => {
  const odoo = { nombre: 'odoo', orden: './odoo-bin' }, web = { nombre: 'web', orden: 'npm run dev', url: 'http://localhost:5173/' };
  assert.deepEqual(P.programChanges({ original: [odoo, web], current: [odoo, web] }), { save: [], remove: [] });
  const cur = P.replaceProgram([odoo, web], { nombre: 'odoo2', orden: './odoo-bin -c x' }, 'odoo');
  assert.deepEqual(cur.map(c => c.nombre), ['odoo2', 'web']);
  assert.deepEqual(P.programChanges({ original: [odoo, web], current: cur }), { save: [{ nombre: 'odoo2', orden: './odoo-bin -c x' }], remove: ['odoo'] });
  const nuevo = P.replaceProgram([odoo], { nombre: 'api', orden: 'go run .' });
  assert.deepEqual(P.programChanges({ original: [odoo], current: nuevo }).save.map(c => c.nombre), ['api']);
  assert.deepEqual(P.programChanges({ original: [odoo, web], current: [web] }), { save: [], remove: ['odoo'] });
});
