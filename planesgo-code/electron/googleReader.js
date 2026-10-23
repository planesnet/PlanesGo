// Lectura de Google a través del navegador integrado, como la haría el usuario: el backend pide una página (un
// documento exportado, una búsqueda de Drive, la bandeja de Gmail…) y aquí se carga en una ventana oculta con la
// MISMA sesión de navegador que el panel Web del proyecto; se devuelve el texto de la página o el del fichero que la
// página descarga (las exportaciones de Docs, Sheets y Slides).
//
// Límites: no se leen, copian ni reutilizan cookies, tokens ni cabeceras; no se hace ninguna petición por cuenta
// propia (nada de net.request, session.fetch ni ses.cookies): solo se navega y se lee el texto de la página con
// guiones fijos de solo lectura (nunca hacen clic ni escriben en un formulario). Solo se cargan páginas de Google.
'use strict';
const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const http = require('node:http');

const MAX_DOWNLOAD = 20 * 1024 * 1024; // una exportación más grande no se descarga
const MAX_TRIES = 4; // sesiones de navegador que se prueban si la primera no tiene sesión de Google
const PARTITION_PREFIX = 'navegador-'; // las del panel Web: persist:navegador-<proyecto>

// allowedUrl: solo páginas de Google (https), o el origen de pruebas (PGO_GOOGLE_PRUEBA).
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

// resolvedUrl: en el modo «resolve», Drive ya ha llevado de /open?id=… a la página del fichero.
function resolvedUrl(u) {
  try { return new URL(u).pathname !== '/open'; } catch { return false; }
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

// cutText: como mucho max caracteres (y cuántos había, si se recorta).
function cutText(s, max) {
  s = String(s || '');
  const chars = Array.from(s);
  if (!max || chars.length <= max) return { text: s, truncated: false };
  return { text: chars.slice(0, max).join(''), truncated: true, total: chars.length };
}

// readHead: los primeros bytes de un fichero, en texto (sin el carácter cortado del final).
function readHead(file, maxBytes) {
  const fd = fs.openSync(file, 'r');
  try {
    const size = fs.fstatSync(fd).size;
    const buf = Buffer.alloc(Math.min(size, maxBytes));
    const n = fs.readSync(fd, buf, 0, buf.length, 0);
    let text = buf.subarray(0, n).toString('utf8');
    if (n < size) text = text.replace(/�+$/, '');
    if (text.charCodeAt(0) === 0xFEFF) text = text.slice(1);
    return { text, partial: n < size };
  } finally { fs.closeSync(fd); }
}

// Guiones de lectura (solo leen el DOM: ni clics, ni escritura). Cada uno devuelve {text, contentType, items}.
const WAIT = `const sleep = ms => new Promise(r => setTimeout(r, ms));
const until = async (get, limit) => { const end = Date.now() + limit; let last = -1, same = 0, r = get();
  while (Date.now() < end) { r = get(); if (r.length && r.length === last) { if (++same >= 3) break; } else same = 0;
    last = r.length; await sleep(400); } return r; };
const visible = el => !!(el.offsetWidth || el.offsetHeight || el.getClientRects().length);
const txt = el => (el && (el.innerText || el.textContent) || '').trim();`;

const SCRIPTS = {
  text: `(() => ({ text: document.body ? document.body.innerText : '', contentType: document.contentType }))()`,
  resolve: `(() => ({ contentType: document.contentType }))()`,
  // Las pestañas de una hoja de cálculo (vista htmlview): id (gid) y nombre.
  sheets: `(() => ({ contentType: document.contentType, items: Array.from(document.querySelectorAll('[id^="sheet-button-"]'))
    .map(li => ({ id: li.id.slice('sheet-button-'.length), name: (li.innerText || li.textContent || '').trim() }))
    .filter(x => /^\\d+$/.test(x.id) && x.name) }))()`,
  // Las filas de una búsqueda o carpeta de Drive: las que llevan data-id (el del fichero).
  drive: `(async () => { ${WAIT}
    const rows = () => { const seen = new Set(), out = [];
      for (const el of document.querySelectorAll('[data-id]')) {
        const id = el.getAttribute('data-id');
        if (!/^[A-Za-z0-9_-]{15,}$/.test(id) || seen.has(id) || !visible(el)) continue;
        const lines = txt(el).split('\\n').map(s => s.trim()).filter(Boolean).slice(0, 12);
        if (!lines.length) continue;
        seen.add(id);
        const labels = [];
        for (const x of [el, ...el.querySelectorAll('[aria-label],[title],img[alt],[data-tooltip]')].slice(0, 40))
          for (const a of ['aria-label', 'title', 'alt', 'data-tooltip']) {
            const v = x.getAttribute(a); if (v && labels.length < 12 && !labels.includes(v)) labels.push(v.slice(0, 200)); }
        const a = el.querySelector('a[href]');
        out.push({ id, lines, labels, href: a ? a.href : '' });
      }
      return out; };
    return { contentType: document.contentType, items: await until(rows, 12000) }; })()`,
  // Las filas de la bandeja o de una búsqueda de Gmail (solo las visibles).
  'gmail-list': `(async () => { ${WAIT}
    const rows = () => Array.from(document.querySelectorAll('tr.zA')).filter(visible).map(tr => {
      const t = tr.querySelector('[data-legacy-thread-id]');
      const from = Array.from(tr.querySelectorAll('.yX [email], .yW [email]')).map(e => e.getAttribute('name') || txt(e));
      const date = tr.querySelector('.xW [title]');
      return { id: t ? t.getAttribute('data-legacy-thread-id') : '', from: from.length ? from.join(', ') : txt(tr.querySelector('.yX, .yW')),
        name: txt(tr.querySelector('.bog')), snippet: txt(tr.querySelector('.y2')),
        date: date ? (txt(date) || date.getAttribute('title')) : txt(tr.querySelector('.xW')), unread: tr.classList.contains('zE') };
    });
    let r = [];
    const end = Date.now() + 15000;
    while (Date.now() < end) {
      r = await until(rows, 1500);
      if (r.length || Array.from(document.querySelectorAll('td.TC')).some(visible)) break;
    }
    return { contentType: document.contentType, items: r }; })()`,
  // Un hilo en la vista de impresión de Gmail: el texto de cada mensaje.
  'gmail-print': `(() => ({ contentType: document.contentType, text: document.body ? document.body.innerText : '',
    items: Array.from(document.querySelectorAll('table.message')).map(m => ({ text: (m.innerText || '').trim() })) }))()`,
};

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

  // Las descargas de las ventanas de lectura (solo las suyas: las del panel Web siguen igual) van a un fichero
  // temporal que se lee y se borra.
  function hookDownloads(ses) {
    if (hooked.has(ses)) return;
    hooked.add(ses);
    ses.on('will-download', (_e, item, wc) => {
      const a = wc && active.get(wc.id);
      if (!a) return;
      if (item.getTotalBytes() > MAX_DOWNLOAD) { item.cancel(); a.fail('el fichero es demasiado grande'); return; }
      a.downloading = true;
      const file = path.join(downloadsDir(), crypto.randomBytes(8).toString('hex'));
      item.setSavePath(file);
      item.once('done', (_e2, state) => {
        try {
          if (state !== 'completed') { a.fail('descarga ' + state); return; }
          const h = readHead(file, a.job.maxChars * 4 || MAX_DOWNLOAD);
          const c = cutText(h.text, a.job.maxChars);
          a.finish({ download: true, fileName: item.getFilename(), contentType: item.getMimeType(), status: 200,
            text: c.text, truncated: c.truncated || h.partial, total: h.partial ? 0 : c.total });
        } catch (e) { a.fail(e.message); } finally { try { fs.rmSync(file, { force: true }); } catch { /* ya no está */ } }
      });
    });
  }

  // loadIn: carga la página en una ventana oculta con esa sesión y la lee.
  function loadIn(job, partition) {
    return new Promise(resolve => {
      const win = new BrowserWindow({ show: false, width: 1280, height: 900,
        webPreferences: { partition, sandbox: true, contextIsolation: true, nodeIntegration: false, backgroundThrottling: false, spellcheck: false } });
      const wc = win.webContents;
      const res = { url: job.url, status: 0 };
      let finished = false;
      const a = { job, downloading: false };
      const done = r => {
        if (finished) return;
        finished = true;
        clearTimeout(timer);
        active.delete(wc.id);
        try { win.destroy(); } catch { /* ya cerrada */ }
        resolve({ ...res, ...r });
      };
      a.finish = done;
      a.fail = e => done({ error: e });
      active.set(wc.id, a);
      const timer = setTimeout(() => done({ error: 'tiempo agotado' }), job.timeoutMs || 45000);
      wc.setAudioMuted(true);
      wc.setWindowOpenHandler(() => ({ action: 'deny' })); // nada de ventanas nuevas
      hookDownloads(wc.session);
      const check = u => {
        res.url = u;
        if (isLoginUrl(u)) done({ login: true });
        else if (job.mode === 'resolve' && resolvedUrl(u)) done({});
      };
      // Solo páginas de Google: lo demás no se carga.
      const guard = (e, u) => { if (!allowedUrl(u, testOrigin)) { e.preventDefault(); done({ url: u, error: 'la página lleva fuera de Google' }); } };
      wc.on('will-navigate', guard);
      wc.on('will-redirect', guard);
      wc.on('did-redirect-navigation', (_e, u, _inPlace, isMain) => { if (isMain) check(u); });
      wc.on('did-navigate', (_e, u, code) => { res.status = code; check(u); });
      wc.on('did-fail-load', (_e, code, desc, _u, isMain) => {
        if (!isMain) return;
        // -3 (ERR_ABORTED): la página se ha convertido en una descarga (o la ha parado «resolve»).
        setTimeout(() => { if (!a.downloading) done({ error: desc || String(code) }); }, code === -3 ? 1500 : 0);
      });
      wc.on('did-finish-load', async () => {
        if (a.downloading || finished) return;
        if (isLoginUrl(wc.getURL())) { done({ login: true, url: wc.getURL() }); return; }
        try {
          const out = await wc.executeJavaScript(SCRIPTS[job.mode] || SCRIPTS.text, true) || {};
          const c = cutText(out.text, job.maxChars);
          done({ url: wc.getURL(), title: wc.getTitle(), contentType: out.contentType || '', items: out.items || [],
            text: c.text, truncated: c.truncated, total: c.total });
        } catch (e) { done({ error: e.message }); }
      });
      wc.loadURL(job.url).catch(() => { /* lo cuentan los eventos */ });
    });
  }

  // run: un trabajo del backend. Si en la sesión del proyecto no hay sesión de Google, se prueban las demás del
  // panel Web (la primera con sesión se recuerda).
  async function run(job) {
    if (!allowedUrl(job.url, testOrigin)) return { id: job.id, url: job.url, error: 'solo se leen páginas de Google' };
    let last = null;
    for (const p of partitionOrder(job.project, remembered, existingPartitions(userData))) {
      last = await loadIn(job, p);
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

// startPolling: pide trabajo al backend (con el secreto de la app de escritorio), lo hace y entrega el resultado.
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

module.exports = { allowedUrl, isLoginUrl, resolvedUrl, partitionOrder, existingPartitions, cutText, readHead, SCRIPTS, createReader, startPolling };
