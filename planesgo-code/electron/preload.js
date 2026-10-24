// Puente mínimo entre la interfaz y Electron: saber que estamos en escritorio y abrir enlaces fuera.
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('psf', {
  escritorio: true,
  abrirFuera: url => ipcRenderer.invoke('abrir-fuera', String(url)),
  elegirCarpeta: inicial => ipcRenderer.invoke('elegir-carpeta', inicial ? String(inicial) : ''),
  // La ventana del escritorio remoto (RDP y AnyDesk), la misma que «pgocode --rdp».
  escritorioRemoto: () => ipcRenderer.invoke('escritorio-remoto'),
  // Visores (RDP y AnyDesk) en este equipo, por el ayudante de escritorio: conectar o desconectar, qué hay instalado y
  // sus avisos (una sesión que termina, con su error).
  remoto: orden => ipcRenderer.invoke('remoto', orden),
  visores: () => ipcRenderer.invoke('visores'),
  alRemoto: cb => { const f = (_, ev) => cb(ev); ipcRenderer.on('pgo:remoto', f); return () => ipcRenderer.removeListener('pgo:remoto', f); },
  // Cerrar la app (tras parar el servidor: no lo vuelve a arrancar).
  salir: () => ipcRenderer.invoke('salir'),
  // La versión de esta app y la de su motor (Electron).
  versionApp: () => ipcRenderer.invoke('version-app'),
  // Abrir una carpeta de este equipo en el gestor de archivos.
  abrirRuta: ruta => ipcRenderer.invoke('abrir-ruta', String(ruta)),
  // Actividad de la persona en el navegador interno (sus eventos no llegan a esta ventana).
  alActividad: cb => { const f = () => cb(); ipcRenderer.on('pgo:actividad', f); return () => ipcRenderer.removeListener('pgo:actividad', f); },
});
