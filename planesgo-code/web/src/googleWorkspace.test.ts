import { test } from 'node:test';
import assert from 'node:assert/strict';
import { googleStatusText, scopeNames, sourceText, type GoogleStatus } from './googleWorkspace.ts';

const base: GoogleStatus = { conectado: false, gcloud: true, orden: 'gcloud auth application-default login …', orden_alternativa: 'gcloud auth login --enable-gdrive-access' };

test('estado de Google Workspace en una línea', () => {
  assert.equal(googleStatusText(null), '—');
  assert.equal(googleStatusText({ ...base, gcloud: false }), 'Falta la herramienta gcloud de Google en este equipo');
  assert.equal(googleStatusText({ ...base, conectado: true, email: 'luis@planesnet.com', solo_lectura: true }), 'Conectado como luis@planesnet.com · solo lectura de Drive');
  assert.match(googleStatusText({ ...base, conectado: true, email: 'luis@planesnet.com' }), /Drive completo \(pgocode solo lee\)/);
  assert.equal(googleStatusText({ ...base, email: 'luis@planesnet.com' }), 'luis@planesnet.com: sin acceso a Drive');
  assert.equal(googleStatusText(base), 'Sin acceso a Drive');
});

test('ámbitos y fuente en palabras', () => {
  assert.deepEqual(scopeNames(['openid', 'https://www.googleapis.com/auth/userinfo.email', 'https://www.googleapis.com/auth/drive.readonly', 'https://www.googleapis.com/auth/cloud-platform', 'https://www.googleapis.com/auth/otro']),
    ['identidad', 'correo', 'Drive (solo lectura)', 'Google Cloud', 'otro']);
  assert.deepEqual(scopeNames(undefined), []);
  assert.equal(sourceText('adc'), 'credenciales por defecto de gcloud (ADC)');
  assert.equal(sourceText(), '—');
});
