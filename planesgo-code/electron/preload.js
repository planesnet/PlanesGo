// Puente mínimo entre la interfaz y Electron: saber que estamos en escritorio y abrir enlaces fuera.
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('psf', {
  escritorio: true,
  abrirFuera: url => ipcRenderer.invoke('abrir-fuera', String(url)),
});
