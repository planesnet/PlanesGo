// PSFCode escritorio: arranca el backend Go (o usa uno que ya esté en marcha) y lo muestra en una ventana.
// El panel de navegador usa <webview>, que puede cargar cualquier web (un iframe no).
const { app, BrowserWindow, ipcMain, shell } = require('electron');
const { spawn } = require('node:child_process');
const path = require('node:path');
const fs = require('node:fs');
const http = require('node:http');

const ADDR = process.env.PSF_ADDR || '127.0.0.1:7801';
const URL_ARNES = 'http://' + ADDR;
let backend = null;

function binario() {
  const nombre = process.platform === 'win32' ? 'psfcode.exe' : 'psfcode';
  const candidatos = [
    process.env.PSF_BACKEND,
    path.join(process.resourcesPath || '', nombre),        // app empaquetada
    path.join(__dirname, '..', 'dist', nombre),            // desarrollo (make)
  ].filter(Boolean);
  return candidatos.find(p => fs.existsSync(p));
}

function vivo() {
  return new Promise(ok => {
    const r = http.get(URL_ARNES + '/healthz', res => { res.resume(); ok(res.statusCode === 200); });
    r.on('error', () => ok(false));
    r.setTimeout(1000, () => { r.destroy(); ok(false); });
  });
}

async function arrancaBackend() {
  if (await vivo()) return; // ya hay uno en marcha: se reutiliza
  const bin = binario();
  if (!bin) throw new Error('No encuentro el backend psfcode (compílalo con «make» o indica PSF_BACKEND)');
  backend = spawn(bin, ['-addr', ADDR], { stdio: 'inherit', env: process.env });
  backend.on('exit', c => { backend = null; if (c) console.error('psfcode terminó con código', c); });
  for (let i = 0; i < 100; i++) {
    if (await vivo()) return;
    await new Promise(r => setTimeout(r, 100));
  }
  throw new Error('El backend psfcode no responde en ' + URL_ARNES);
}

const esWeb = u => { try { return ['http:', 'https:'].includes(new URL(u).protocol); } catch { return false; } };

function ventana() {
  const w = new BrowserWindow({
    width: 1440, height: 900, minWidth: 900, minHeight: 600, title: 'PSFCode', autoHideMenuBar: true,
    webPreferences: { preload: path.join(__dirname, 'preload.js'), contextIsolation: true, nodeIntegration: false, sandbox: true, webviewTag: true },
  });
  // Los enlaces que abren ventana nueva van al navegador del sistema.
  w.webContents.setWindowOpenHandler(({ url }) => { if (esWeb(url)) shell.openExternal(url); return { action: 'deny' }; });
  // La ventana principal solo navega dentro del arnés.
  w.webContents.on('will-navigate', (e, url) => { if (!url.startsWith(URL_ARNES)) { e.preventDefault(); if (esWeb(url)) shell.openExternal(url); } });
  w.loadURL(URL_ARNES);
}

app.on('web-contents-created', (_, wc) => {
  // Endurecer los <webview>: sin preload, sin Node, solo http/https.
  wc.on('will-attach-webview', (e, prefs, params) => {
    delete prefs.preload;
    prefs.nodeIntegration = false;
    prefs.contextIsolation = true;
    if (!esWeb(params.src)) e.preventDefault();
  });
  wc.setWindowOpenHandler(({ url }) => { if (esWeb(url)) shell.openExternal(url); return { action: 'deny' }; });
});

ipcMain.handle('abrir-fuera', (_, url) => { if (esWeb(url)) return shell.openExternal(url); throw new Error('solo http o https'); });

app.whenReady().then(async () => {
  try {
    await arrancaBackend();
    ventana();
  } catch (e) {
    console.error(e.message);
    app.quit();
  }
});

app.on('window-all-closed', () => app.quit());
app.on('quit', () => { if (backend) backend.kill('SIGTERM'); });
