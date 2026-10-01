// Pizarra de bocetos: al pulsar el icono se captura la pestaña visible (si es una página web;
// activeTab da permiso solo para esa pestaña y solo en ese momento) y se abre la pizarra en una
// pestaña nueva, con la captura disponible como fondo.
chrome.action.onClicked.addListener(async (tab) => {
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
});
