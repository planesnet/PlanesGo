// Pizarra de bocetos: el icono abre la paleta (popup.html). Al pulsar «Pizarra» se captura la
// pestaña visible (si es una página web; activeTab da permiso solo para esa pestaña, concedido al
// abrir la paleta) y se abre la pizarra en una pestaña nueva, con la captura disponible como fondo.
async function abrirPizarra() {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  let shot = null;
  try {
    if (tab && /^https?:|^file:/.test(tab.url || '')) {
      shot = await chrome.tabs.captureVisibleTab(tab.windowId, { format: 'png' });
    }
  } catch (e) {
    console.debug('[pizarra/background.js] captureVisibleTab no disponible:', e && e.message);
  }
  await chrome.storage.session.set({ captura: shot, capturaTitulo: (tab && tab.title) || '' });
  await chrome.tabs.create({ url: chrome.runtime.getURL('board.html') + (shot ? '#captura' : '') });
}

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (msg && msg.accion === 'abrir-pizarra') { abrirPizarra().finally(() => sendResponse({ ok: true })); return true; }
});
