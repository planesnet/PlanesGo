import { test } from 'node:test';
import assert from 'node:assert/strict';
import { clientFileProblem, clientText, googleActions, googleServices, googleStatusText, scopeNames, type GoogleStatus } from './googleWorkspace.ts';

const base: GoogleStatus = { conectado: false, cliente: true, cliente_origen: 'fichero', cliente_proyecto: 'planes-dev' };
const own: GoogleStatus = { ...base, conectado: true, drive: true, gmail: true, fuente: 'oauth', email: 'luis@planesnet.com', solo_lectura: true };
const driveOnly: GoogleStatus = { ...own, gmail: false };

test('estado de Google Workspace en una línea', () => {
  assert.equal(googleStatusText(null), '—');
  assert.equal(googleStatusText({ ...base, cliente: false }), 'Falta registrar la aplicación de Google: carga el fichero del cliente');
  assert.equal(googleStatusText(base), 'Sin conectar');
  assert.equal(googleStatusText({ ...base, en_curso: true }), 'Esperando a que aceptes en la página de Google…');
  assert.equal(googleStatusText(own), 'Conectado como luis@planesnet.com · solo lectura');
  assert.equal(googleStatusText({ ...own, drive: false, solo_lectura: false }), 'Conectado como luis@planesnet.com · solo lectura');
  assert.equal(googleStatusText({ ...base, conectado: true, drive: true, fuente: 'gcloud', email: 'luis@planesnet.com' }),
    'Con las credenciales de Google de este equipo (luis@planesnet.com) · Drive completo (pgocode solo lee)');
  assert.doesNotMatch(googleStatusText({ ...base, conectado: true, fuente: 'adc' }), /gcloud/);
});

test('botones de la sección', () => {
  assert.deepEqual(googleActions(null), { connect: false, reconnect: false, disconnect: false, loadClient: false });
  assert.deepEqual(googleActions({ ...base, cliente: false }), { connect: false, reconnect: false, disconnect: false, loadClient: true });
  assert.deepEqual(googleActions(base), { connect: true, reconnect: false, disconnect: false, loadClient: true });
  assert.deepEqual(googleActions({ ...base, en_curso: true }), { connect: false, reconnect: false, disconnect: false, loadClient: true });
  assert.deepEqual(googleActions(own), { connect: false, reconnect: false, disconnect: true, loadClient: true });
  // Con el respaldo de gcloud se puede conectar la cuenta propia; nada que desconectar.
  assert.deepEqual(googleActions({ ...base, conectado: true, fuente: 'adc' }), { connect: true, reconnect: false, disconnect: false, loadClient: true });
  // Conectado antes de Gmail (o desmarcado en Google): «Volver a conectar», sin dejar de poder desconectar.
  assert.deepEqual(googleActions(driveOnly), { connect: true, reconnect: true, disconnect: true, loadClient: true });
  assert.deepEqual(googleActions({ ...driveOnly, en_curso: true }), { connect: false, reconnect: true, disconnect: true, loadClient: true });
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

test('Drive y Gmail con su estado', () => {
  assert.deepEqual(googleServices(null), []);
  assert.deepEqual(googleServices(base), []);
  assert.deepEqual(googleServices(own).map(x => [x.name, x.ok]), [['Drive, Docs y Sheets', true], ['Gmail', true]]);
  const [drive, gmail] = googleServices(driveOnly);
  assert.ok(drive.ok && drive.text.startsWith('autorizado'));
  assert.ok(!gmail.ok && /Volver a conectar/.test(gmail.text));
  // Con las credenciales de gcloud del equipo: lo que hay que hacer es conectar la cuenta.
  assert.match(googleServices({ ...base, conectado: true, drive: true, gmail: false, fuente: 'adc' })[1].text, /conecta tu cuenta/);
  assert.deepEqual(scopeNames(['https://www.googleapis.com/auth/gmail.readonly']), ['Gmail (solo lectura)']);
});
