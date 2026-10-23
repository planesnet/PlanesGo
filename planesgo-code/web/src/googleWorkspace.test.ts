import { test } from 'node:test';
import assert from 'node:assert/strict';
import { clientFileProblem, clientText, googleActions, googleStatusText, scopeNames, type GoogleStatus } from './googleWorkspace.ts';

const base: GoogleStatus = { conectado: false, cliente: true, cliente_origen: 'fichero', cliente_proyecto: 'planes-dev' };
const own: GoogleStatus = { ...base, conectado: true, fuente: 'oauth', email: 'luis@planesnet.com', solo_lectura: true };

test('estado de Google Workspace en una línea', () => {
  assert.equal(googleStatusText(null), '—');
  assert.equal(googleStatusText({ ...base, cliente: false }), 'Falta registrar la aplicación de Google: carga el fichero del cliente');
  assert.equal(googleStatusText(base), 'Sin conectar');
  assert.equal(googleStatusText({ ...base, en_curso: true }), 'Esperando a que aceptes en la página de Google…');
  assert.equal(googleStatusText(own), 'Conectado como luis@planesnet.com · solo lectura de Drive');
  assert.equal(googleStatusText({ ...base, conectado: true, fuente: 'gcloud', email: 'luis@planesnet.com' }),
    'Con las credenciales de Google de este equipo (luis@planesnet.com) · Drive completo (pgocode solo lee)');
  assert.doesNotMatch(googleStatusText({ ...base, conectado: true, fuente: 'adc' }), /gcloud/);
});

test('botones de la sección', () => {
  assert.deepEqual(googleActions(null), { connect: false, disconnect: false, loadClient: false });
  assert.deepEqual(googleActions({ ...base, cliente: false }), { connect: false, disconnect: false, loadClient: true });
  assert.deepEqual(googleActions(base), { connect: true, disconnect: false, loadClient: true });
  assert.deepEqual(googleActions({ ...base, en_curso: true }), { connect: false, disconnect: false, loadClient: true });
  assert.deepEqual(googleActions(own), { connect: false, disconnect: true, loadClient: true });
  // Con el respaldo de gcloud se puede conectar la cuenta propia; nada que desconectar.
  assert.deepEqual(googleActions({ ...base, conectado: true, fuente: 'adc' }), { connect: true, disconnect: false, loadClient: true });
  // El cliente lo da el servidor: no hay fichero que cambiar.
  assert.equal(googleActions({ ...base, cliente_origen: 'servidor' }).loadClient, false);
});

test('ámbitos, cliente y fichero', () => {
  assert.deepEqual(scopeNames(['openid', 'https://www.googleapis.com/auth/userinfo.email', 'https://www.googleapis.com/auth/drive.readonly', 'https://www.googleapis.com/auth/cloud-platform', 'https://www.googleapis.com/auth/otro']),
    ['identidad', 'correo', 'Drive (solo lectura)', 'Google Cloud', 'otro']);
  assert.deepEqual(scopeNames(undefined), []);
  assert.equal(clientText(base), 'Registrada (proyecto planes-dev) · fichero cargado en este equipo');
  assert.equal(clientText({ ...base, cliente_origen: 'servidor', cliente_proyecto: undefined }), 'Registrada · la da el servidor de PlanesGo');
  assert.equal(clientText({ ...base, cliente: false }), '—');
  assert.equal(clientFileProblem('client_secret_123.apps.googleusercontent.com.json', 400), '');
  assert.match(clientFileProblem('foto.png', 400), /JSON/);
  assert.match(clientFileProblem('x.json', 100_000), /demasiado grande/);
});
