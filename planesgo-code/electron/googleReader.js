// Lectura de Google a través del navegador integrado, SOLO POR DESCARGA, como «Archivo → Descargar»: el backend pide
// un enlace de descarga (la exportación de un documento, la descarga de un fichero de Drive, el mensaje original de
// Gmail) y aquí se descarga en una ventana oculta con la MISMA sesión de navegador que el panel Web del proyecto; se
// devuelve el fichero descargado (el backend lo convierte en texto). Nunca se carga ni se lee una página: la ventana
// oculta no navega (solo descarga con webContents.downloadURL) y no hay guiones que lean el DOM. Que Google pide
// iniciar sesión se sabe por las URL de la descarga (lleva a accounts.google.com).
//
// Límites: no se leen, copian ni reutilizan cookies, tokens ni cabeceras; no se hace ninguna petición por cuenta
// propia (nada de net.request, session.fetch ni ses.cookies): solo se descarga el enlace, como lo haría el usuario.
// Solo enlaces de Google por https; nada de clics, ventanas nuevas ni sonido; los ficheros temporales se borran.
'use strict';
const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const http = require('node:http');

const MAX_DOWNLOAD = 20 * 1024 * 1024; // un fichero más grande no se descarga (gbrowser.MaxDownload)
const MAX_TRIES = 4; // sesiones de navegador que se prueban si la primera no tiene sesión de Google
const PARTITION_PREFIX = 'navegador-'; // las del panel Web: persist:navegador-<proyecto>

// allowedUrl: solo enlaces de Google (https), o el origen de pruebas (PGO_GOOGLE_PRUEBA).
function allowedUrl(u, testOrigin) {
  let x;
  try { x = new URL(u); } catch { return false; }
  if (testOrigin && x.origin === testOrigin) return true;
  return x.protocol === 'https:' && /(^|\.)(google\.com|googleusercontent\.com)$/.test(x.hostname);
}

// isLoginUrl: Google pide iniciar sesión (o enseña la portada de un producto, lo que hace sin sesión).
function isLoginUrl(u) {
  let x;
  try { x = new URL(u); } catch { return false; }
  return x.hostname === 'accounts.google.com' || x.hostname === 'workspace.google.com' ||
    (x.hostname === 'www.google.com' && /^\/(intl\/[^/]+\/)?(gmail|drive|docs|sheets)\/about/.test(x.pathname)) ||
    /\/ServiceLogin\b/.test(x.pathname);
}

// partitionOrder: en qué sesiones de navegador se prueba. Primero la del proyecto (la de su panel Web), luego la
// última que tenía sesión de Google y luego las demás del panel Web, de la más reciente a la más antigua.
function partitionOrder(project, remembered, existing) {
  const out = [];
  const add = p => { if (p && !out.includes(p)) out.push(p); };
  if (project) add('persist:' + PARTITION_PREFIX + project);
  add(remembered);
  [...existing].sort((a, b) => b.mtime - a.mtime).forEach(e => add('persist:' + e.name));
  if (!out.length) add('persist:' + PARTITION_PREFIX + 'google');
  return out.slice(0, MAX_TRIES);
}

// existingPartitions: las sesiones del panel Web que hay en disco (carpetas Partitions/navegador-*).
function existingPartitions(userData) {
  const dir = path.join(userData, 'Partitions');
  try {
    return fs.readdirSync(dir, { withFileTypes: true })
      .filter(d => d.isDirectory() && d.name.startsWith(PARTITION_PREFIX))
      .map(d => ({ name: d.name, mtime: fs.statSync(path.join(dir, d.name)).mtimeMs }));
  } catch { return []; }
}

// createReader: el lector (Electron se pasa como dependencia para poder probarlo sin Electron).
function createReader({ BrowserWindow, userData, testOrigin, tmpDir, log = () => {} }) {
  const active = new Map(); // webContents.id → trabajo en curso
  const hooked = new WeakSet();
  let remembered = '';
  let dlDir = '';
  const downloadsDir = () => {
    if (!dlDir) dlDir = fs.mkdtempSync(path.join(tmpDir || os.tmpdir(), 'pgo-google-'));
    return dlDir;
  };

  // Las descargas de las ventanas ocultas (solo las suyas: las del panel Web siguen igual) van a un fichero temporal
  // que se lee entero, se devuelve en base64 y se borra.
  function hookDownloads(ses) {
    if (hooked.has(ses)) return;
    hooked.add(ses);
    ses.on('will-download', (_e, item, wc) => {
      const a = wc && active.get(wc.id);
      if (!a) return;
      const tooBig = () => { item.cancel(); a.fail('el fichero es demasiado grande (más de 20 MB)'); };
      if (item.getTotalBytes() > MAX_DOWNLOAD) { tooBig(); return; }
      const file = path.join(downloadsDir(), crypto.randomBytes(8).toString('hex'));
      item.setSavePath(file);
      // Una descarga que falla (Google responde 4xx: sin acceso, no existe, no es de ese tipo) queda «interrupted» y
      // Electron no siempre avisa con «done»: se acaba aquí.
      let ended = false;
      const end = state => {
        if (ended) return;
        ended = true;
        try {
          const chain = typeof item.getURLChain === 'function' ? item.getURLChain() : [item.getURL()];
          const last = chain[chain.length - 1] || a.job.url;
          if (chain.some(isLoginUrl)) { a.finish({ login: true, url: last }); return; }
          if (!chain.every(u => allowedUrl(u, testOrigin))) { a.fail('la descarga lleva fuera de Google'); return; }
          if (state !== 'completed') { a.fail(state === 'interrupted' ? 'Google no ha dado el fichero' : 'descarga ' + ({ cancelled: 'cancelada' }[state] || state)); return; }
          const data = fs.readFileSync(file);
          if (data.length > MAX_DOWNLOAD) { a.fail('el fichero es demasiado grande (más de 20 MB)'); return; }
          a.finish({ download: true, url: last, fileName: item.getFilename(), contentType: item.getMimeType(),
            disposition: typeof item.getContentDisposition === 'function' ? item.getContentDisposition() : '',
            data: data.toString('base64') });
        } catch (e) { a.fail(e.message); } finally { try { fs.rmSync(file, { force: true }); } catch { /* ya no está */ } }
      };
      item.on('updated', (_e2, state) => {
        if (state === 'interrupted') { end('interrupted'); item.cancel(); } else if (item.getReceivedBytes() > MAX_DOWNLOAD) tooBig();
      });
      item.once('done', (_e2, state) => end(state));
    });
  }

  // fetchIn: descarga el enlace en una ventana oculta con esa sesión (sin cargar ninguna página).
  function fetchIn(job, partition) {
    return new Promise(resolve => {
      const win = new BrowserWindow({ show: false, width: 800, height: 600,
        webPreferences: { partition, sandbox: true, contextIsolation: true, nodeIntegration: false, javascript: false, backgroundThrottling: false, spellcheck: false } });
      const wc = win.webContents;
      let finished = false;
      const a = { job };
      const done = r => {
        if (finished) return;
        finished = true;
        clearTimeout(timer);
        active.delete(wc.id);
        try { win.destroy(); } catch { /* ya cerrada */ }
        resolve({ url: job.url, ...r });
      };
      a.finish = done;
      a.fail = e => done({ error: e });
      active.set(wc.id, a);
      const timer = setTimeout(() => done({ error: 'tiempo agotado' }), job.timeoutMs || 45000);
      wc.setAudioMuted(true);
      wc.setWindowOpenHandler(() => ({ action: 'deny' })); // nada de ventanas nuevas
      wc.on('will-navigate', e => e.preventDefault()); // ni páginas: solo la descarga
      hookDownloads(wc.session);
      wc.downloadURL(job.url);
    });
  }

  // run: un trabajo del backend. Si en la sesión del proyecto no hay sesión de Google, se prueban las demás del
  // panel Web (la primera con sesión se recuerda).
  async function run(job) {
    if (!allowedUrl(job.url, testOrigin)) return { id: job.id, url: job.url, error: 'solo se descargan enlaces de Google' };
    let last = null;
    for (const p of partitionOrder(job.project, remembered, existingPartitions(userData))) {
      last = await fetchIn(job, p);
      if (!last.login) {
        if (!last.error) remembered = p;
        break;
      }
      log('google: sin sesión de Google en ' + p);
    }
    return { ...last, id: job.id };
  }

  // cleanup: fuera la carpeta temporal de las descargas (al cerrar la app).
  const cleanup = () => { if (dlDir) try { fs.rmSync(dlDir, { recursive: true, force: true }); } catch { /* da igual */ } };

  return { run, cleanup };
}

// startPolling: pide trabajo al backend (con el secreto de la app de escritorio), descarga y entrega el resultado.
function startPolling({ base, token, reader, log = () => {} }) {
  let stopped = false;
  const request = (method, p, body) => new Promise((ok, ko) => {
    const u = new URL(p, base);
    const data = body ? Buffer.from(JSON.stringify(body)) : null;
    const r = http.request(u, { method, headers: { Cookie: 'pgo_escritorio=' + token, 'X-PGO': '1',
      ...(data ? { 'Content-Type': 'application/json', 'Content-Length': data.length } : {}) } }, res => {
      const chunks = [];
      res.on('data', c => chunks.push(c));
      res.on('end', () => ok({ status: res.statusCode, body: Buffer.concat(chunks).toString('utf8') }));
    });
    r.on('error', ko);
    r.setTimeout(40000, () => r.destroy(new Error('sin respuesta')));
    if (data) r.write(data);
    r.end();
  });
  (async () => {
    while (!stopped) {
      try {
        const r = await request('GET', '/api/escritorio/google/trabajo');
        if (r.status === 204) continue;
        if (r.status !== 200) throw new Error('backend ' + r.status);
        const job = JSON.parse(r.body);
        let res;
        try { res = await reader.run(job); } catch (e) { res = { id: job.id, error: e.message }; }
        await request('POST', '/api/escritorio/google/resultado', res);
      } catch (e) {
        if (stopped) break;
        log('google: ' + e.message);
        await new Promise(r => setTimeout(r, 2000));
      }
    }
  })();
  return () => { stopped = true; };
}

module.exports = { allowedUrl, isLoginUrl, partitionOrder, existingPartitions, createReader, startPolling, MAX_DOWNLOAD };
