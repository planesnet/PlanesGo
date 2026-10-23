// Pruebas de googleReader.js sin Electron (node --test electron/): una ventana falsa que imita downloadURL y las
// descargas de la sesión (sin páginas: la ventana no tiene loadURL ni executeJavaScript).
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

test('isLoginUrl', () => {
  assert.ok(g.isLoginUrl('https://accounts.google.com/v3/signin/identifier?continue=x'));
  assert.ok(g.isLoginUrl('https://www.google.com/intl/es/gmail/about/'));
  assert.ok(g.isLoginUrl('https://workspace.google.com/products/drive/'));
  assert.ok(g.isLoginUrl('http://127.0.0.1:8181/ServiceLogin?continue=x'));
  assert.ok(!g.isLoginUrl('https://docs.google.com/document/d/x/export?format=md'));
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

// Una ventana falsa: solo downloadURL (y el aviso de navegación, que se bloquea); behave decide la descarga.
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
        id: next++, session: sessions.get(partition),
        setAudioMuted() {}, setWindowOpenHandler(h) { wc.openHandler = h; },
        downloadURL: u => { wc.downloaded = u; setImmediate(() => behave(u, wc, partition)); },
      });
      this.webContents = wc;
      this.destroyed = false;
      created.push({ partition, opts, win: this });
    }
    destroy() { this.destroyed = true; }
  }
  return { BrowserWindow: FakeWindow, created };
}

// Una descarga como las de Electron: la sesión avisa (will-download), se guarda donde se diga y acaba.
const download = (wc, { chain, name = 'f', mime = 'text/plain', disposition = 'attachment', content = '', total, state = 'completed', onlyUpdated }) => {
  const item = new EventEmitter();
  Object.assign(item, { getTotalBytes: () => total ?? Buffer.byteLength(content), getReceivedBytes: () => Buffer.byteLength(content),
    getFilename: () => name, getMimeType: () => mime, getContentDisposition: () => disposition, getURLChain: () => chain,
    getURL: () => chain[0], cancel() { item.cancelled = true; }, setSavePath(p) { item.path = p; } });
  wc.session.emit('will-download', {}, item, wc);
  if (item.path) fs.writeFileSync(item.path, content);
  // Las que fallan (4xx) en Electron de verdad: solo «updated» con «interrupted», sin «done».
  if (onlyUpdated) return item.emit('updated', {}, 'interrupted');
  item.emit('done', {}, item.cancelled ? 'cancelled' : state);
  return item;
};

test('run: descarga el enlace (sin páginas), sin sesión en el proyecto pero sí en otra', async () => {
  const userData = fs.mkdtempSync(path.join(os.tmpdir(), 'pgo-ud-'));
  fs.mkdirSync(path.join(userData, 'Partitions', 'navegador-2'), { recursive: true });
  let item;
  const fake = fakeElectron((u, wc, partition) => {
    if (partition === 'persist:navegador-7') {
      return download(wc, { chain: [u, 'https://accounts.google.com/ServiceLogin?continue=x'], mime: 'text/html', disposition: '', content: '<html>login</html>' });
    }
    item = download(wc, { chain: [u, 'https://doc-0k-docstext.googleusercontent.com/export/x'], name: 'Informe.md', mime: 'text/x-markdown',
      disposition: 'attachment; filename="Informe.md"', content: '# Informe\n\nHola ñandú' });
  });
  const r = g.createReader({ BrowserWindow: fake.BrowserWindow, userData, tmpDir: userData });
  const url = 'https://docs.google.com/document/d/x/export?format=md';
  const res = await r.run({ id: 'j1', url, project: 7 });
  assert.strictEqual(res.id, 'j1');
  assert.strictEqual(res.download, true);
  assert.strictEqual(res.fileName, 'Informe.md');
  assert.strictEqual(res.disposition, 'attachment; filename="Informe.md"');
  assert.strictEqual(Buffer.from(res.data, 'base64').toString('utf8'), '# Informe\n\nHola ñandú');
  assert.strictEqual(res.text, undefined);
  assert.deepStrictEqual(fake.created.map(c => c.partition), ['persist:navegador-7', 'persist:navegador-2']);
  assert.ok(fake.created.every(c => c.win.destroyed && c.opts.show === false && c.opts.webPreferences.sandbox && c.opts.webPreferences.javascript === false));
  assert.ok(fake.created.every(c => c.win.webContents.downloaded === url), 'solo descarga el enlace');
  assert.ok(!fs.existsSync(item.path), 'el fichero temporal se borra');
  // Nada de navegar ni de ventanas nuevas.
  const wc = fake.created[1].win.webContents;
  const ev = { preventDefault() { ev.prevented = true; } };
  wc.emit('will-navigate', ev, 'https://docs.google.com/document/d/x/edit');
  assert.ok(ev.prevented);
  assert.deepStrictEqual(wc.openHandler(), { action: 'deny' });
  // La sesión que tenía Google se recuerda: la siguiente vez, justo después de la del proyecto.
  fake.created.length = 0;
  await r.run({ id: 'j2', url, project: 9 });
  assert.deepStrictEqual(fake.created.map(c => c.partition), ['persist:navegador-9']);
  r.cleanup();
  assert.deepStrictEqual(fs.readdirSync(userData).filter(n => n.startsWith('pgo-google-')), []);
  fs.rmSync(userData, { recursive: true });
});

test('run: sin sesión en ninguna, fuera de Google, interrumpida y demasiado grande', async () => {
  const userData = fs.mkdtempSync(path.join(os.tmpdir(), 'pgo-ud-'));
  const fake = fakeElectron((u, wc) => {
    if (u.includes('/login')) return download(wc, { chain: [u, 'https://accounts.google.com/v3/signin'], mime: 'text/html', disposition: '' });
    if (u.includes('/fuera')) return download(wc, { chain: [u, 'https://example.com/x'], content: 'x' });
    if (u.includes('/roto')) return download(wc, { chain: [u], state: 'interrupted' });
    if (u.includes('/400')) return download(wc, { chain: [u], mime: 'text/html', onlyUpdated: true });
    if (u.includes('/grande')) return download(wc, { chain: [u], total: g.MAX_DOWNLOAD + 1 });
  });
  const r = g.createReader({ BrowserWindow: fake.BrowserWindow, userData, tmpDir: userData });
  let res = await r.run({ id: 'a', url: 'https://docs.google.com/login', project: 4 });
  assert.strictEqual(res.login, true);
  assert.strictEqual(res.data, undefined);
  res = await r.run({ id: 'b', url: 'https://docs.google.com/fuera', project: 4 });
  assert.match(res.error, /fuera de Google/);
  assert.strictEqual(res.data, undefined);
  res = await r.run({ id: 'c', url: 'https://docs.google.com/roto', project: 4 });
  assert.match(res.error, /Google no ha dado el fichero/);
  res = await r.run({ id: 'c2', url: 'https://docs.google.com/400', project: 4, timeoutMs: 2000 });
  assert.match(res.error, /Google no ha dado el fichero/);
  res = await r.run({ id: 'd', url: 'https://docs.google.com/grande', project: 4 });
  assert.match(res.error, /demasiado grande/);
  res = await r.run({ id: 'e', url: 'https://example.com/', project: 4 });
  assert.match(res.error, /solo se descargan enlaces de Google/);
  res = await r.run({ id: 'f', url: 'https://docs.google.com/nada', project: 4, timeoutMs: 50 });
  assert.match(res.error, /tiempo agotado/);
  r.cleanup();
  fs.rmSync(userData, { recursive: true });
});

test('sin guiones de lectura de páginas', () => {
  const src = fs.readFileSync(path.join(__dirname, 'googleReader.js'), 'utf8').replace(/\/\/.*$/gm, ''); // sin comentarios
  for (const bad of ['executeJavaScript', '.loadURL(', 'innerText', 'querySelector', 'cookies', 'net.request', 'session.fetch']) {
    assert.ok(!src.includes(bad), bad);
  }
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
      return res.end(JSON.stringify({ id: 'j', url: 'https://docs.google.com/x' }));
    }
    let b = '';
    req.on('data', c => { b += c; });
    req.on('end', () => { res.end('{}'); delivered(JSON.parse(b)); });
  });
  await new Promise(ok => srv.listen(0, '127.0.0.1', ok));
  const stop = g.startPolling({ base: 'http://127.0.0.1:' + srv.address().port, token: 'secreto',
    reader: { run: async j => ({ id: j.id, download: true, data: 'aG9sYQ==' }) } });
  assert.deepStrictEqual(await got, { id: 'j', download: true, data: 'aG9sYQ==' });
  stop();
  assert.deepStrictEqual(seen[0], ['GET', '/api/escritorio/google/trabajo', 'pgo_escritorio=secreto', '1']);
  assert.deepStrictEqual(seen[1], ['POST', '/api/escritorio/google/resultado', 'pgo_escritorio=secreto', '1']);
  await new Promise(ok => setTimeout(ok, 120));
  srv.close();
  srv.closeAllConnections();
});
