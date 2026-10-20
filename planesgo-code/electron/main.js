// Planes Go Code escritorio: arranca el backend Go (o usa uno que ya esté en marcha) y lo muestra en una ventana.
// El panel de navegador usa <webview>, que puede cargar cualquier web (un iframe no).
const { app, BrowserWindow, dialog, ipcMain, session, shell } = require('electron');
const crypto = require('node:crypto');
const { spawn } = require('node:child_process');
const path = require('node:path');
const fs = require('node:fs');
const http = require('node:http');

const ADDR = process.env.PGO_ADDR || '127.0.0.1:7801';
const URL_ARNES = 'http://' + ADDR;
let backend = null;

// Linux: sin ibus. Si ibus no conecta, GTK le pasa igualmente las pulsaciones y las descarta («Events queue
// growing too big»): no se podía escribir nada. El método de entrada sencillo de GTK sí admite tildes, ñ y
// teclas muertas. (Para que GTK lo lea, el lanzador también lo pone en el entorno: Makefile «escritorio».)
if (process.platform === 'linux') {
  process.env.GTK_IM_MODULE = 'gtk-im-context-simple';
  process.env.XMODIFIERS = '@im=none';
  process.env.QT_IM_MODULE = '';
  app.commandLine.appendSwitch('ozone-platform-hint', 'auto');
}
// Secreto de esta app: el backend solo atiende a quien lo traiga (cookie de esta ventana), así que Planes Go Code
// no se puede usar desde un navegador normal salvo que se arranque con -navegador.
const TOKEN_ESCRITORIO = crypto.randomBytes(32).toString('hex');

function binario() {
  const nombre = process.platform === 'win32' ? 'pgocode-backend.exe' : 'pgocode-backend';
  const candidatos = [
    process.env.PGO_BACKEND,
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
  // Un backend ya en marcha no conoce el secreto de esta ventana: no se puede reutilizar.
  if (await vivo()) throw new Error('Planes Go Code ya está abierto (o hay otro programa en ' + ADDR + '). Ciérralo antes de abrirlo de nuevo.');
  const bin = binario();
  if (!bin) throw new Error('No encuentro el backend pgocode (compílalo con «make» o indica PGO_BACKEND)');
  backend = spawn(bin, ['-addr', ADDR], { stdio: 'inherit', env: { ...process.env, PGO_TOKEN_ESCRITORIO: TOKEN_ESCRITORIO } });
  backend.on('exit', c => { backend = null; if (c) console.error('pgocode terminó con código', c); });
  for (let i = 0; i < 100; i++) {
    if (await vivo()) return;
    await new Promise(r => setTimeout(r, 100));
  }
  throw new Error('El backend pgocode no responde en ' + URL_ARNES);
}

const esWeb = u => { try { return ['http:', 'https:'].includes(new URL(u).protocol); } catch { return false; } };
// Comparación por origen exacto (no por prefijo: «…:78010» o «…:7801@otra.com» no son el arnés).
const ORIGEN_ARNES = new URL(URL_ARNES).origin;
const esArnes = u => { try { return new URL(u).origin === ORIGEN_ARNES; } catch { return false; } };

// Permisos: todo denegado por defecto. Solo la interfaz del arnés puede escribir en el portapapeles
// (botón «Copiar»); las webs del panel Navegador no reciben cámara, micrófono, ubicación, notificaciones…
function endurece(ses) {
  const permitido = (wc, permiso, origen) => permiso === 'clipboard-sanitized-write' && esArnes(origen || (wc && wc.getURL()) || '');
  ses.setPermissionRequestHandler((wc, permiso, cb, detalles) => cb(permitido(wc, permiso, detalles && detalles.requestingUrl)));
  ses.setPermissionCheckHandler((wc, permiso, origen) => permitido(wc, permiso, origen));
}
app.on('session-created', endurece);

function ventana() {
  const w = new BrowserWindow({
    width: 1440, height: 900, minWidth: 900, minHeight: 600, title: 'Planes Go Code', autoHideMenuBar: true,
    icon: path.join(__dirname, 'icono', 'pgocode.png'),
    webPreferences: { preload: path.join(__dirname, 'preload.js'), contextIsolation: true, nodeIntegration: false, sandbox: true, webviewTag: true },
  });
  // Los enlaces que abren ventana nueva van al navegador del sistema.
  w.webContents.setWindowOpenHandler(({ url }) => { if (esWeb(url)) shell.openExternal(url); return { action: 'deny' }; });
  // La ventana principal solo navega dentro del arnés.
  const soloArnes = (e, url) => { if (!esArnes(url)) { e.preventDefault(); if (esWeb(url)) shell.openExternal(url); } };
  w.webContents.on('will-navigate', soloArnes);
  w.webContents.on('will-redirect', soloArnes);
  w.on('close', () => console.error('[pgocode] ventana cerrada'));
  w.webContents.on('did-fail-load', (_, c, d) => console.error('[pgocode] no carga la interfaz:', c, d));
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
  endurece(session.defaultSession);
  try {
    await arrancaBackend();
    // La cookie solo existe en la sesión de esta ventana (los <webview> usan otras particiones).
    await session.defaultSession.cookies.set({ url: URL_ARNES, name: 'pgo_escritorio', value: TOKEN_ESCRITORIO, httpOnly: true, sameSite: 'strict' });
    ventana();
  } catch (e) {
    console.error(e.message);
    dialog.showErrorBox('Planes Go Code', e.message);
    app.quit();
  }
});

// Diagnóstico: por qué se cierra (caídas del proceso de la ventana o de la GPU, o cierre de la ventana).
app.on('render-process-gone', (_, wc, d) => console.error('[pgocode] proceso de la ventana caído:', d.reason, d.exitCode));
app.on('child-process-gone', (_, d) => console.error('[pgocode] proceso hijo caído:', d.type, d.reason, d.exitCode));
app.on('window-all-closed', () => { console.error('[pgocode] todas las ventanas cerradas'); app.quit(); });
app.on('quit', () => { if (backend) backend.kill('SIGTERM'); });
