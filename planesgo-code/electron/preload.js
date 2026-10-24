// Puente mínimo entre la interfaz y Electron: saber que estamos en escritorio y abrir enlaces fuera.
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('psf', {
  escritorio: true,
  abrirFuera: url => ipcRenderer.invoke('abrir-fuera', String(url)),
  elegirCarpeta: inicial => ipcRenderer.invoke('elegir-carpeta', inicial ? String(inicial) : ''),
  // La ventana del escritorio remoto (RDP y AnyDesk), la misma que «pgocode --rdp».
  escritorioRemoto: () => ipcRenderer.invoke('escritorio-remoto'),
  // Actividad de la persona en el navegador interno (sus eventos no llegan a esta ventana).
  alActividad: cb => { const f = () => cb(); ipcRenderer.on('pgo:actividad', f); return () => ipcRenderer.removeListener('pgo:actividad', f); },
});
