// Pruebas de googleReader.js sin Electron (node --test electron/): una ventana falsa que imita los eventos de
// webContents y las descargas de la sesión.
'use strict';
const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const http = require('node:http');
const { EventEmitter } = require('node:events');
const g = require('./googleReader');

test('allowedUrl: solo Google por https (y el origen de pruebas)', () => {
  assert.ok(g.allowedUrl('https://docs.google.com/document/d/x/export?format=md'));
  assert.ok(g.allowedUrl('https://doc-0s-docs.googleusercontent.com/x'));
  assert.ok(!g.allowedUrl('http://docs.google.com/x'));
  assert.ok(!g.allowedUrl('https://docs.google.com.evil.com/x'));
  assert.ok(!g.allowedUrl('https://evilgoogle.com/x'));
  assert.ok(!g.allowedUrl('file:///etc/passwd'));
  assert.ok(g.allowedUrl('http://127.0.0.1:8181/document/d/x', 'http://127.0.0.1:8181'));
  assert.ok(!g.allowedUrl('http://127.0.0.1:8182/x', 'http://127.0.0.1:8181'));
});

test('isLoginUrl y resolvedUrl', () => {
  assert.ok(g.isLoginUrl('https://accounts.google.com/v3/signin/identifier?continue=x'));
  assert.ok(g.isLoginUrl('https://www.google.com/intl/es/gmail/about/'));
  assert.ok(g.isLoginUrl('https://workspace.google.com/products/drive/'));
  assert.ok(g.isLoginUrl('http://127.0.0.1:8181/ServiceLogin?continue=x'));
  assert.ok(!g.isLoginUrl('https://docs.google.com/document/d/x/edit'));
  assert.ok(!g.resolvedUrl('https://drive.google.com/open?id=x'));
  assert.ok(g.resolvedUrl('https://docs.google.com/document/d/x/edit'));
});

test('partitionOrder: proyecto, la recordada y las demás por uso', () => {
  const ex = [{ name: 'navegador-3', mtime: 1 }, { name: 'navegador-9', mtime: 5 }, { name: 'navegador-7', mtime: 3 }, { name: 'navegador-1', mtime: 0 }];
  assert.deepStrictEqual(g.partitionOrder(7, 'persist:navegador-3', ex),
    ['persist:navegador-7', 'persist:navegador-3', 'persist:navegador-9', 'persist:navegador-1']);
  assert.deepStrictEqual(g.partitionOrder(0, '', []), ['persist:navegador-google']);
});

test('existingPartitions: solo las del panel Web', () => {
  const d = fs.mkdtempSync(path.join(os.tmpdir(), 'pgo-part-'));
  fs.mkdirSync(path.join(d, 'Partitions', 'navegador-5'), { recursive: true });
  fs.mkdirSync(path.join(d, 'Partitions', 'otra'), { recursive: true });
  assert.deepStrictEqual(g.existingPartitions(d).map(p => p.name), ['navegador-5']);
  assert.deepStrictEqual(g.existingPartitions(path.join(d, 'nada')), []);
  fs.rmSync(d, { recursive: true });
});

test('cutText y readHead', () => {
  assert.deepStrictEqual(g.cutText('ñandú', 3), { text: 'ñan', truncated: true, total: 5 });
  assert.deepStrictEqual(g.cutText('abc', 5), { text: 'abc', truncated: false });
  const f = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'pgo-head-')), 'x');
  fs.writeFileSync(f, '﻿año');
  assert.deepStrictEqual(g.readHead(f, 100), { text: 'año', partial: false });
  assert.deepStrictEqual(g.readHead(f, 5), { text: 'a', partial: true }); // la ñ cortada no sale
  fs.rmSync(path.dirname(f), { recursive: true });
});

// Una ventana falsa: loadURL decide qué pasa según la URL (y la sesión).
function fakeElectron(behave) {
  const sessions = new Map();
  const created = [];
  let next = 1;
  class FakeWindow {
    constructor(opts) {
      const partition = opts.webPreferences.partition;
      if (!sessions.has(partition)) sessions.set(partition, new EventEmitter());
      const wc = new EventEmitter();
      Object.assign(wc, {
        id: next++, session: sessions.get(partition), url: '', title: '',
        setAudioMuted() {}, setWindowOpenHandler(h) { wc.openHandler = h; },
        getURL: () => wc.url, getTitle: () => wc.title,
        executeJavaScript: async () => wc.page,
        loadURL: async u => { setImmediate(() => behave(u, wc, partition)); },
      });
      this.webContents = wc;
      this.destroyed = false;
      created.push({ partition, opts, win: this });
    }
    destroy() { this.destroyed = true; }
  }
  return { BrowserWindow: FakeWindow, created };
}

const navigate = (wc, u, code, page, title = '') => {
  const ev = { preventDefault() { ev.prevented = true; } };
  wc.emit('will-navigate', ev, u);
  if (ev.prevented) return;
  wc.url = u; wc.page = page; wc.title = title;
  wc.emit('did-navigate', {}, u, code);
  wc.emit('did-finish-load');
};

// Una descarga como las de Electron: la sesión avisa (will-download), se guarda donde se diga y acaba.
const download = (wc, name, content) => {
  const item = new EventEmitter();
  Object.assign(item, { getTotalBytes: () => content.length, getFilename: () => name, getMimeType: () => 'text/markdown',
    cancel() {}, setSavePath(p) { item.path = p; } });
  wc.session.emit('will-download', {}, item, wc);
  wc.emit('did-fail-load', {}, -3, 'ERR_ABORTED', '', true);
  fs.writeFileSync(item.path, content);
  item.emit('done', {}, 'completed');
  return item;
};

test('run: exportación descargada, sin sesión en el proyecto pero sí en otra', async () => {
  const userData = fs.mkdtempSync(path.join(os.tmpdir(), 'pgo-ud-'));
  fs.mkdirSync(path.join(userData, 'Partitions', 'navegador-2'), { recursive: true });
  let item;
  const fake = fakeElectron((u, wc, partition) => {
    if (partition === 'persist:navegador-7') return navigate(wc, 'https://accounts.google.com/ServiceLogin?continue=' + encodeURIComponent(u), 200, {});
    item = download(wc, 'Informe.md', '# Informe\n\nHola ñandú');
  });
  const r = g.createReader({ BrowserWindow: fake.BrowserWindow, userData, tmpDir: userData });
  const res = await r.run({ id: 'j1', url: 'https://docs.google.com/document/d/x/export?format=md', mode: 'text', project: 7, maxChars: 12 });
  assert.strictEqual(res.id, 'j1');
  assert.strictEqual(res.download, true);
  assert.strictEqual(res.fileName, 'Informe.md');
  assert.strictEqual(res.text, '# Informe\n\nH');
  assert.strictEqual(res.truncated, true);
  assert.deepStrictEqual(fake.created.map(c => c.partition), ['persist:navegador-7', 'persist:navegador-2']);
  assert.ok(fake.created.every(c => c.win.destroyed && c.opts.show === false && c.opts.webPreferences.sandbox));
  assert.ok(!fs.existsSync(item.path), 'el fichero temporal se borra');
  // La sesión que tenía Google se recuerda: la siguiente vez, justo después de la del proyecto.
  fake.created.length = 0;
  await r.run({ id: 'j2', url: 'https://docs.google.com/document/d/x/export?format=md', mode: 'text', project: 9, maxChars: 100 });
  assert.deepStrictEqual(fake.created.map(c => c.partition), ['persist:navegador-9']);
  r.cleanup();
  assert.deepStrictEqual(fs.readdirSync(userData).filter(n => n.startsWith('pgo-google-')), []);
  fs.rmSync(userData, { recursive: true });
});

test('run: sin sesión de Google en ninguna, página leída, fuera de Google y resolve', async () => {
  const userData = fs.mkdtempSync(path.join(os.tmpdir(), 'pgo-ud-'));
  const fake = fakeElectron((u, wc) => {
    if (u.includes('/export')) return navigate(wc, 'https://accounts.google.com/v3/signin', 200, {});
    if (u.includes('/open?id=')) {
      wc.emit('did-redirect-navigation', {}, 'https://docs.google.com/spreadsheets/d/x/edit', false, true);
      return;
    }
    if (u.includes('/fuera')) return navigate(wc, 'https://example.com/', 200, {});
    navigate(wc, u, 200, { text: 'Acta\nPunto 1', contentType: 'text/html' }, 'Acta - Documentos de Google');
  });
  const r = g.createReader({ BrowserWindow: fake.BrowserWindow, userData, tmpDir: userData });
  let res = await r.run({ id: 'a', url: 'https://docs.google.com/document/d/x/export?format=md', mode: 'text', project: 4, maxChars: 100 });
  assert.strictEqual(res.login, true);
  res = await r.run({ id: 'b', url: 'https://docs.google.com/document/d/x/mobilebasic', mode: 'text', project: 4, maxChars: 100 });
  assert.deepStrictEqual([res.status, res.text, res.title, res.contentType], [200, 'Acta\nPunto 1', 'Acta - Documentos de Google', 'text/html']);
  res = await r.run({ id: 'c', url: 'https://drive.google.com/open?id=x', mode: 'resolve', project: 4, maxChars: 100 });
  assert.strictEqual(res.url, 'https://docs.google.com/spreadsheets/d/x/edit');
  res = await r.run({ id: 'd', url: 'https://docs.google.com/fuera', mode: 'text', project: 4, maxChars: 100 });
  assert.match(res.error, /fuera de Google/);
  res = await r.run({ id: 'e', url: 'https://example.com/', mode: 'text', project: 4, maxChars: 100 });
  assert.match(res.error, /solo se leen páginas de Google/);
  fs.rmSync(userData, { recursive: true });
});

test('startPolling: recoge con el secreto, lee y entrega', async () => {
  const seen = [];
  let delivered;
  const got = new Promise(ok => { delivered = ok; });
  let sent = false;
  const srv = http.createServer((req, res) => {
    seen.push([req.method, req.url, req.headers.cookie, req.headers['x-pgo']]);
    if (req.method === 'GET') {
      if (sent) { res.statusCode = 204; return setTimeout(() => res.end(), 50); }
      sent = true;
      res.setHeader('Content-Type', 'application/json');
      return res.end(JSON.stringify({ id: 'j', url: 'https://docs.google.com/x', mode: 'text', maxChars: 10 }));
    }
    let b = '';
    req.on('data', c => { b += c; });
    req.on('end', () => { res.end('{}'); delivered(JSON.parse(b)); });
  });
  await new Promise(ok => srv.listen(0, '127.0.0.1', ok));
  const stop = g.startPolling({ base: 'http://127.0.0.1:' + srv.address().port, token: 'secreto',
    reader: { run: async j => ({ id: j.id, text: 'leído' }) } });
  assert.deepStrictEqual(await got, { id: 'j', text: 'leído' });
  stop();
  assert.deepStrictEqual(seen[0], ['GET', '/api/escritorio/google/trabajo', 'pgo_escritorio=secreto', '1']);
  assert.deepStrictEqual(seen[1], ['POST', '/api/escritorio/google/resultado', 'pgo_escritorio=secreto', '1']);
  await new Promise(ok => setTimeout(ok, 120));
  srv.close();
  srv.closeAllConnections();
});
