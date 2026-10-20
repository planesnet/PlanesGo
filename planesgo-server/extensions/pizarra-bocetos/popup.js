// Paleta de herramientas: la pizarra la abre background.js (la captura y la pestaña nueva siguen
// aunque la paleta se cierre); el enlace a PlanesGo se abre en una pestaña nueva.
document.getElementById('open-board').addEventListener('click', async () => {
  await chrome.runtime.sendMessage({ accion: 'abrir-pizarra' });
  window.close();
});
document.getElementById('open-planesgo').addEventListener('click', (e) => {
  e.preventDefault();
  chrome.tabs.create({ url: e.currentTarget.href });
  window.close();
});
