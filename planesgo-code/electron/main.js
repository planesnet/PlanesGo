// Planes Go Code escritorio (el cliente): usa el servidor pgocode de este equipo (lo arranca si no está en marcha; sigue
// funcionando aunque se cierre la ventana) y muestra su interfaz. Se empareja con él una vez y entra con su token.
// El panel de navegador usa <webview>, que puede cargar cualquier web (un iframe no).
const { app, BrowserWindow, Menu, clipboard, dialog, ipcMain, safeStorage, session, shell } = require('electron');
const os = require('node:os');
const { spawn } = require('node:child_process');
const path = require('node:path');
const fs = require('node:fs');
const http = require('node:http');
const googleReader = require('./googleReader');

const ADDR = process.env.PGO_ADDR || '127.0.0.1:7801';
const URL_ARNES = 'http://' + ADDR;

// Linux: sin ibus. Si ibus no conecta, GTK le pasa igualmente las pulsaciones y las descarta («Events queue
// growing too big»): no se podía escribir nada. El método de entrada sencillo de GTK sí admite tildes, ñ y
// teclas muertas. (Para que GTK lo lea, el lanzador también lo pone en el entorno: Makefile «escritorio».)
// Nombre interno «pgocode»: en Linux la ventana se anuncia con el nombre de la app (app_id en Wayland,
// WM_CLASS en X11) y el escritorio la enlaza con pgocode.desktop y su icono. El título visible sigue siendo
// «Planes Go Code». Va antes de que la app arranque.
app.setName('pgocode');
if (process.platform === 'linux') {
  app.setDesktopName('pgocode.desktop');
  process.env.GTK_IM_MODULE = 'gtk-im-context-simple';
  process.env.XMODIFIERS = '@im=none';
  process.env.QT_IM_MODULE = '';
  app.commandLine.appendSwitch('ozone-platform-hint', 'auto');
}
// Agente de usuario sin «Electron/…» ni «pgocode/…» (el de Chrome, que es lo que es): Google no deja iniciar sesión
// en navegadores que se anuncian como integrados, y el panel Web es donde el usuario abre su sesión de Google (con
// la que pgocode lee Drive, Docs y Gmail: googleReader.js).
app.userAgentFallback = app.userAgentFallback.replace(/\s(?:pgocode|Electron)\/\S+/g, '');
// Datos del servidor de este equipo (los del backend: -datos / PGO_DATOS): ahí deja el secreto de emparejar.
const DATOS_SERVIDOR = process.env.PGO_DATOS || path.join(os.homedir(), '.local', 'share', 'pgocode');
// Token de este cliente: el servidor solo atiende a los clientes emparejados (cookie de esta ventana), así que Planes
// Go Code no se puede usar desde un navegador normal salvo que el servidor se arranque con -navegador.
let tokenCliente = '';

// Una sola instancia: «pgocode» o «pgocode --rdp» con la app ya abierta se lo pasan a la que está en marcha, que abre
// su ventana (el escritorio remoto, con la misma sesión, o la principal). El backend y la sesión son los mismos.
const quiereRdp = argv => argv.includes('--rdp');
// app.exit, not app.quit: quit waits for «ready», and by then the startup below would already be under way (a second
// backend on the same port and its «ya está abierto» error box).
const unica = app.requestSingleInstanceLock();
if (!unica) app.exit(0);
let principal = null;
let rdp = null;

function binario() {
  const nombre = process.platform === 'win32' ? 'pgocode-backend.exe' : 'pgocode-backend';
  const candidatos = [
    process.env.PGO_BACKEND,
    path.join(__dirname, nombre),                          // app instalada (paquete): junto a main.js
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

// El servidor de este equipo: si ya está en marcha se usa; si no, se arranca por su cuenta (no es hijo de esta app:
// sigue con los agentes y las sesiones aunque se cierre la ventana).
async function servidor() {
  if (await vivo()) return;
  const bin = binario();
  if (!bin) throw new Error('No encuentro el servidor pgocode (compílalo con «make» o indica PGO_BACKEND)');
  const p = spawn(bin, ['-addr', ADDR], { detached: true, stdio: 'ignore', windowsHide: true,
    env: { ...process.env, PGO_ELECTRON: process.versions.electron, PGO_DESACOPLADO: '1' } }); // sin lo heredado (proceso.StartClean)
  p.unref();
  for (let i = 0; i < 100; i++) {
    if (await vivo()) return;
    await new Promise(r => setTimeout(r, 100));
  }
  throw new Error('El servidor pgocode no responde en ' + URL_ARNES);
}

// pide: una petición al servidor con el token de este cliente (JSON de vuelta; error con el mensaje del servidor).
function pide(metodo, ruta, cuerpo) {
  return new Promise((ok, mal) => {
    const datos = cuerpo ? JSON.stringify(cuerpo) : '';
    const r = http.request(URL_ARNES + ruta, { method: metodo, headers: { 'X-PGO': '1', 'Content-Type': 'application/json',
      Cookie: 'pgo_escritorio=' + tokenCliente, 'Content-Length': Buffer.byteLength(datos) } }, res => {
      let t = '';
      res.on('data', c => { t += c; });
      res.on('end', () => {
        let j = {};
        try { j = JSON.parse(t || '{}'); } catch { /* sin JSON */ }
        if (res.statusCode >= 200 && res.statusCode < 300) ok(j); else mal(Object.assign(new Error(j.error || 'HTTP ' + res.statusCode), { status: res.statusCode }));
      });
    });
    r.on('error', mal);
    r.setTimeout(10000, () => r.destroy(new Error('el servidor no responde')));
    r.end(datos);
  });
}

// Token del cliente, cifrado con el almacén del sistema (safeStorage) en los datos de esta app, solo para este usuario.
const ficheroToken = () => path.join(app.getPath('userData'), 'cliente.json');
function leeToken() {
  try {
    const j = JSON.parse(fs.readFileSync(ficheroToken(), 'utf8'));
    return j.cifrado ? safeStorage.decryptString(Buffer.from(j.cifrado, 'base64')) : (j.token || '');
  } catch { return ''; }
}
function guardaToken(t) {
  const j = safeStorage.isEncryptionAvailable() ? { cifrado: safeStorage.encryptString(t).toString('base64') } : { token: t };
  fs.writeFileSync(ficheroToken(), JSON.stringify(j), { mode: 0o600 });
}

// emparejado: este cliente con un token que el servidor reconoce; si no lo tiene (o lo han revocado), se empareja con
// el secreto que el servidor deja en su carpeta de datos (solo lo puede leer este usuario del sistema).
async function emparejado() {
  tokenCliente = leeToken();
  if (tokenCliente) {
    try { await pide('GET', '/api/clientes/yo'); return; } catch (e) { if (e.status !== 401 && e.status !== 403) throw e; }
  }
  let secreto = '';
  try { secreto = fs.readFileSync(path.join(DATOS_SERVIDOR, 'emparejar'), 'utf8').trim(); } catch { /* sin acceso */ }
  if (!secreto) throw new Error('No se puede emparejar con el servidor de este equipo: falta su secreto (' + path.join(DATOS_SERVIDOR, 'emparejar') + ')');
  tokenCliente = '';
  const r = await pide('POST', '/api/clientes/emparejar', { secret: secreto, name: os.hostname() + ' · ' + os.userInfo().username });
  tokenCliente = r.token;
  guardaToken(tokenCliente);
}

// vigilaServidor: si el servidor se actualiza (otra versión), esta app se vuelve a abrir con la nueva (sus ficheros
// también han cambiado); si se ha caído, se vuelve a arrancar.
function vigilaServidor(version) {
  setInterval(async () => {
    try {
      const v = await pide('GET', '/api/version');
      if (v.version && v.version !== version) { app.relaunch(); app.exit(0); }
    } catch {
      if (!(await vivo())) servidor().catch(e => console.error('[pgocode] ' + e.message));
    }
  }, 20000);
}

const esWeb = u => { try { return ['http:', 'https:'].includes(new URL(u).protocol); } catch { return false; } };
// Comparación por origen exacto (no por prefijo: «…:78010» o «…:7801@otra.com» no son el arnés).
const ORIGEN_ARNES = new URL(URL_ARNES).origin;
const esArnes = u => { try { return new URL(u).origin === ORIGEN_ARNES; } catch { return false; } };

// Permisos: todo denegado por defecto, salvo escribir en el portapapeles (el botón «Copiar» de la interfaz y de
// las webs que se prueban en el navegador interno, p. ej. copiar la dirección con token de una pantalla de Odoo;
// el navegador solo lo deja tras un clic). Leerlo, cámara, micrófono, ubicación, notificaciones…: no.
function endurece(ses) {
  const permitido = (wc, permiso, origen) => {
    const o = origen || (wc && wc.getURL()) || '';
    return permiso === 'clipboard-sanitized-write' && (esArnes(o) || esWeb(o));
  };
  ses.setPermissionRequestHandler((wc, permiso, cb, detalles) => cb(permitido(wc, permiso, detalles && detalles.requestingUrl)));
  ses.setPermissionCheckHandler((wc, permiso, origen) => permitido(wc, permiso, origen));
}
app.on('session-created', endurece);

// Menú contextual de la interfaz y de las webs del navegador interno.
function menuContextual(p, w) {
  const m = [];
  if (p.isEditable) m.push({ role: 'cut', label: 'Cortar', enabled: p.editFlags.canCut });
  if (p.selectionText || p.isEditable) m.push({ role: 'copy', label: 'Copiar', enabled: p.editFlags.canCopy });
  if (p.isEditable) m.push({ role: 'paste', label: 'Pegar', enabled: p.editFlags.canPaste });
  if (p.linkURL && esWeb(p.linkURL)) {
    if (m.length) m.push({ type: 'separator' });
    m.push({ label: 'Copiar el enlace', click: () => clipboard.writeText(p.linkURL) });
    m.push({ label: 'Abrir el enlace en el navegador', click: () => shell.openExternal(p.linkURL) });
  }
  if (m.length) m.push({ type: 'separator' });
  m.push({ role: 'selectAll', label: 'Seleccionar todo' });
  Menu.buildFromTemplate(m).popup(w ? { window: w } : {});
}

function ventana() {
  // La ventana se abre donde y como estaba la última vez (si esa posición sigue cayendo en alguna pantalla).
  const ficheroVentana = path.join(app.getPath('userData'), 'ventana.json');
  let previa = {};
  try { previa = JSON.parse(fs.readFileSync(ficheroVentana, 'utf8')); } catch { /* primera vez */ }
  const { screen } = require('electron');
  const visible = previa.x !== undefined && screen.getAllDisplays().some(d => {
    const a = d.workArea;
    return previa.x < a.x + a.width - 100 && previa.x + (previa.width || 0) > a.x + 100 && previa.y >= a.y - 10 && previa.y < a.y + a.height - 100;
  });
  const w = new BrowserWindow({
    width: previa.width || 1440, height: previa.height || 900, ...(visible ? { x: previa.x, y: previa.y } : {}),
    minWidth: 900, minHeight: 600, title: 'Planes Go Code', autoHideMenuBar: true,
    icon: path.join(__dirname, 'icono', 'pgocode.png'),
    webPreferences: { preload: path.join(__dirname, 'preload.js'), contextIsolation: true, nodeIntegration: false, sandbox: true, webviewTag: true },
  });
  // Los enlaces que abren ventana nueva van al navegador del sistema.
  w.webContents.setWindowOpenHandler(({ url }) => { if (esWeb(url)) shell.openExternal(url); return { action: 'deny' }; });
  // La ventana principal solo navega dentro del arnés.
  const soloArnes = (e, url) => { if (!esArnes(url)) { e.preventDefault(); if (esWeb(url)) shell.openExternal(url); } };
  w.webContents.on('will-navigate', soloArnes);
  w.webContents.on('will-redirect', soloArnes);
  principal = w;
  if (previa.maximizada) w.maximize();
  const guardaVentana = () => {
    try {
      const b = w.getNormalBounds();
      fs.writeFileSync(ficheroVentana, JSON.stringify({ ...b, maximizada: w.isMaximized() }));
    } catch { /* sin disco: la próxima vez, tamaño por defecto */ }
  };
  let espera;
  const guardaLuego = () => { clearTimeout(espera); espera = setTimeout(guardaVentana, 500); };
  w.on('resize', guardaLuego);
  w.on('move', guardaLuego);
  w.on('maximize', guardaVentana);
  w.on('unmaximize', guardaVentana);
  w.on('close', () => { guardaVentana(); console.error('[pgocode] ventana cerrada'); });
  // Las ventanas ocultas de lectura de Google no mantienen viva la app; la del escritorio remoto, sí.
  w.on('closed', () => { principal = null; if (!rdp) app.quit(); });
  // Menú contextual (clic derecho): copiar lo seleccionado, cortar y pegar en los campos, copiar enlaces.
  w.webContents.on('context-menu', (_, p) => menuContextual(p, w));
  w.webContents.on('did-fail-load', (_, c, d) => console.error('[pgocode] no carga la interfaz:', c, d));
  w.loadURL(URL_ARNES);
}

// Ventana del escritorio remoto (pgocode --rdp): la interfaz de pgocode en modo escritorio remoto, con la sesión de
// esta app. La ventana de Windows la abre el backend (FreeRDP).
function ventanaRdp() {
  if (rdp) { if (rdp.isMinimized()) rdp.restore(); rdp.focus(); return; }
  rdp = new BrowserWindow({
    width: 560, height: 680, minWidth: 460, minHeight: 520, title: 'Escritorio remoto · Planes Go Code', autoHideMenuBar: true,
    icon: path.join(__dirname, 'icono', 'pgocode.png'),
    webPreferences: { preload: path.join(__dirname, 'preload.js'), contextIsolation: true, nodeIntegration: false, sandbox: true },
  });
  rdp.webContents.setWindowOpenHandler(({ url }) => { if (esWeb(url)) shell.openExternal(url); return { action: 'deny' }; });
  const soloArnes = (e, url) => { if (!esArnes(url)) e.preventDefault(); };
  rdp.webContents.on('will-navigate', soloArnes);
  rdp.webContents.on('will-redirect', soloArnes);
  rdp.webContents.on('context-menu', (_, p) => menuContextual(p, rdp));
  rdp.on('closed', () => { rdp = null; if (!principal) app.quit(); });
  rdp.loadURL(URL_ARNES + '/?modo=rdp');
}

app.on('second-instance', (_, argv) => {
  if (quiereRdp(argv)) return ventanaRdp();
  if (!principal) return ventana();
  if (principal.isMinimized()) principal.restore();
  principal.focus();
});

app.on('web-contents-created', (_, wc) => {
  // Endurecer los <webview>: sin preload, sin Node, solo http/https.
  wc.on('will-attach-webview', (e, prefs, params) => {
    delete prefs.preload;
    prefs.nodeIntegration = false;
    prefs.contextIsolation = true;
    if (!esWeb(params.src)) e.preventDefault();
  });
  wc.setWindowOpenHandler(({ url }) => { if (esWeb(url)) shell.openExternal(url); return { action: 'deny' }; });
  // Las webs del navegador interno también tienen menú contextual (copiar, pegar, copiar enlaces).
  if (wc.getType() === 'webview') wc.on('context-menu', (_, p) => menuContextual(p, BrowserWindow.fromWebContents(wc.hostWebContents)));
  // Usar el navegador interno es trabajo de la persona (imputación de tiempo): sus teclas y su ratón no llegan a la
  // ventana de la interfaz, así que se le avisa (como mucho cada 5 s).
  if (wc.getType() === 'webview') {
    let ultimo = 0;
    const avisa = () => {
      const ahora = Date.now();
      if (ahora - ultimo < 5000 || !wc.hostWebContents || wc.hostWebContents.isDestroyed()) return;
      ultimo = ahora;
      wc.hostWebContents.send('pgo:actividad');
    };
    wc.on('before-input-event', avisa);
    wc.on('input-event', avisa); // ratón y rueda (Electron ≥ 31)
  }
});

ipcMain.handle('escritorio-remoto', () => { ventanaRdp(); });
ipcMain.handle('abrir-fuera', (_, url) => { if (esWeb(url)) return shell.openExternal(url); throw new Error('solo http o https'); });
// Diálogo de carpetas del sistema (carpeta de una sesión, directorio principal de un proyecto).
ipcMain.handle('elegir-carpeta', async (e, inicial) => {
  const ventana = BrowserWindow.fromWebContents(e.sender);
  const r = await dialog.showOpenDialog(ventana, {
    title: 'Elegir carpeta',
    defaultPath: typeof inicial === 'string' && inicial ? inicial : app.getPath('home'),
    properties: ['openDirectory', 'createDirectory'],
  });
  return r.canceled || !r.filePaths.length ? null : r.filePaths[0];
});

app.whenReady().then(async () => {
  if (!unica) return; // the running instance does it (second-instance)
  endurece(session.defaultSession);
  try {
    await servidor();
    await emparejado();
    // La cookie solo existe en la sesión de esta ventana (los <webview> usan otras particiones).
    await session.defaultSession.cookies.set({ url: URL_ARNES, name: 'pgo_escritorio', value: tokenCliente, httpOnly: true, sameSite: 'strict' });
    vigilaServidor((await pide('GET', '/api/version')).version);
    if (quiereRdp(process.argv)) ventanaRdp(); else ventana();
    // Descargas de Google que pide el backend (pgo drive leer, pgo gmail leer), con la sesión del panel Web.
    const reader = googleReader.createReader({ BrowserWindow, userData: app.getPath('userData'), tmpDir: app.getPath('temp'),
      testOrigin: process.env.PGO_GOOGLE_PRUEBA || '', log: m => console.error('[pgocode] ' + m) });
    const paraGoogle = googleReader.startPolling({ base: URL_ARNES, token: tokenCliente, reader, log: m => console.error('[pgocode] ' + m) });
    app.on('before-quit', () => { paraGoogle(); reader.cleanup(); });
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
