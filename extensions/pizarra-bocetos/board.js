(() => {
  // Coordenadas lógicas del papel = la pantalla del móvil (390 × 844 a escala ×2)
  const DIMS = { v: [780, 1560], h: [1560, 780], c: [1560, 878] };
  let capture = null; // imagen de la pestaña desde la que se abrió la pizarra (fondo "Captura")
  const GRID = 20;
  let W = 1560, H = 780;
  const paper = document.getElementById('paper');
  const bg = document.getElementById('bg'), draw = document.getElementById('draw');
  const bctx = bg.getContext('2d'), ctx = draw.getContext('2d');
  const state = { tool: 'pen', color: '#142030', size: 4, orient: 'h', snap: true, shapes: [], redo: [] };
  // Todo cambio guarda antes una instantánea: deshacer/rehacer cubren dibujar, mover y eliminar.
  const snapshot = () => JSON.stringify(state.shapes);
  function mutate(fn) { history.push(snapshot()); if (history.length > 200) history.shift(); state.redo = []; fn(); saveDraft(); render(); }
  let current = null, textBox = null, sel = -1, drag = null;
  const history = [];

  // Borrador local (comodidad: no se pierde el dibujo al recargar)
  const DRAFT = 'pizarra-bocetos-borrador';
  try { const d = JSON.parse(localStorage.getItem(DRAFT) || 'null'); if (d && Array.isArray(d.shapes)) { state.shapes = d.shapes; state.orient = d.orient === 'v' ? 'v' : 'h'; if (d.snap === false) state.snap = false; } } catch (e) {}
  const saveDraft = () => { try { localStorage.setItem(DRAFT, JSON.stringify({ shapes: state.shapes, orient: state.orient, snap: state.snap })); } catch (e) {} };

  function resize() {
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    const r = paper.getBoundingClientRect();
    for (const c of [bg, draw]) { c.width = Math.round(r.width * dpr); c.height = Math.round(r.height * dpr); }
    const s = (r.width * dpr) / W;
    bctx.setTransform(s, 0, 0, s, 0, 0); ctx.setTransform(s, 0, 0, s, 0, 0);
    renderBg(bctx); render();
  }

  // Fondo: pantalla del móvil con rejilla ligera (líneas cada 20, más marcadas cada 100) y la
  // cabecera de la app como referencia. Va en su propio lienzo: el borrador no lo toca.
  function renderBg(c) {
    if (state.orient === 'c' && capture) {
      c.save(); c.fillStyle = '#ffffff'; c.fillRect(0, 0, W, H); c.drawImage(capture, 0, 0, W, H);
      if (state.snap) { c.globalAlpha = 0.18; c.strokeStyle = '#64748b'; c.lineWidth = 1;
        for (let x = 0; x <= W; x += GRID * 5) { c.beginPath(); c.moveTo(x + 0.5, 0); c.lineTo(x + 0.5, H); c.stroke(); }
        for (let y = 0; y <= H; y += GRID * 5) { c.beginPath(); c.moveTo(0, y + 0.5); c.lineTo(W, y + 0.5); c.stroke(); } }
      c.restore(); return;
    }
    c.save(); c.fillStyle = '#ffffff'; c.fillRect(0, 0, W, H);
    for (let x = 0; x <= W; x += GRID) { c.strokeStyle = x % (GRID * 5) ? '#eef2f7' : '#dde5ee'; c.lineWidth = 1; c.beginPath(); c.moveTo(x + 0.5, 0); c.lineTo(x + 0.5, H); c.stroke(); }
    for (let y = 0; y <= H; y += GRID) { c.strokeStyle = y % (GRID * 5) ? '#eef2f7' : '#dde5ee'; c.lineWidth = 1; c.beginPath(); c.moveTo(0, y + 0.5); c.lineTo(W, y + 0.5); c.stroke(); }
    // Marco del dispositivo y cabecera de la app
    c.strokeStyle = '#64748b'; c.lineWidth = 4; c.beginPath(); c.roundRect(2, 2, W - 4, H - 4, 36); c.stroke();
    c.strokeStyle = '#cbd5e1'; c.lineWidth = 2; c.beginPath(); c.moveTo(2, 100); c.lineTo(W - 2, 100); c.stroke();
    c.fillStyle = '#b6c2d1'; c.font = '600 26px system-ui, sans-serif'; c.textBaseline = 'middle';
    c.fillText('AutoPyme Logistics', 40, 52);
    c.restore();
  }
  function pathShape(c, s) {
    c.save(); c.strokeStyle = s.color; c.fillStyle = s.color; c.lineWidth = s.size; c.lineCap = 'round'; c.lineJoin = 'round';
    if (s.type === 'pen') {
      c.beginPath(); s.pts.forEach((p, i) => i ? c.lineTo(p[0], p[1]) : c.moveTo(p[0], p[1]));
      if (s.pts.length === 1) c.lineTo(s.pts[0][0] + 0.1, s.pts[0][1]);
      c.stroke();
    } else if (s.type === 'rect') {
      c.beginPath(); c.roundRect(Math.min(s.x1, s.x2), Math.min(s.y1, s.y2), Math.abs(s.x2 - s.x1), Math.abs(s.y2 - s.y1), 6); c.stroke();
    } else if (s.type === 'line' || s.type === 'arrow') {
      c.beginPath(); c.moveTo(s.x1, s.y1); c.lineTo(s.x2, s.y2); c.stroke();
      if (s.type === 'arrow') {
        const a = Math.atan2(s.y2 - s.y1, s.x2 - s.x1), l = 12 + s.size * 2.2;
        c.beginPath(); c.moveTo(s.x2, s.y2); c.lineTo(s.x2 - l * Math.cos(a - 0.45), s.y2 - l * Math.sin(a - 0.45));
        c.moveTo(s.x2, s.y2); c.lineTo(s.x2 - l * Math.cos(a + 0.45), s.y2 - l * Math.sin(a + 0.45)); c.stroke();
      }
    } else if (s.type === 'text') {
      const k = s.k || 1;
      c.font = `${Math.round((18 + s.size * 2) * k)}px 'Segoe Print', 'Bradley Hand', 'Comic Sans MS', cursive`; c.textBaseline = 'top';
      String(s.text).split('\n').forEach((ln, i) => c.fillText(ln, s.x, s.y + i * (22 + s.size * 2.4) * k));
    }
    c.restore();
  }
  function render() {
    ctx.clearRect(0, 0, W, H);
    for (const s of state.shapes) pathShape(ctx, s);
    if (current) pathShape(ctx, current);
    if (sel >= 0 && state.shapes[sel]) {
      const [x1, y1, x2, y2] = bbox(state.shapes[sel]);
      ctx.save(); ctx.strokeStyle = '#0284c7'; ctx.lineWidth = 2; ctx.setLineDash([8, 6]);
      ctx.strokeRect(x1 - PAD, y1 - PAD, x2 - x1 + 2 * PAD, y2 - y1 + 2 * PAD); ctx.setLineDash([]);
      ctx.fillStyle = '#ffffff';
      for (const [, hx, hy] of handles(state.shapes[sel])) { ctx.fillRect(hx - HS / 2, hy - HS / 2, HS, HS); ctx.strokeRect(hx - HS / 2, hy - HS / 2, HS, HS); }
      ctx.restore();
    }
    document.getElementById('undo').disabled = !history.length;
    document.getElementById('redo').disabled = !state.redo.length;
    document.getElementById('delete-sel').disabled = !(sel >= 0 && state.shapes[sel]);
  }

  const pos = (e) => { const r = draw.getBoundingClientRect(); return [(e.clientX - r.left) * W / r.width, (e.clientY - r.top) * H / r.height]; };
  const snapv = (v) => state.snap ? Math.round(v / GRID) * GRID : v;
  const snapPos = (e) => pos(e).map(snapv);
  const commit = (s) => mutate(() => state.shapes.push(s));

  // Caja que ocupa cada objeto (selección y movimiento)
  function bbox(s) {
    if (s.type === 'pen') { const xs = s.pts.map(p => p[0]), ys = s.pts.map(p => p[1]); return [Math.min(...xs), Math.min(...ys), Math.max(...xs), Math.max(...ys)]; }
    if (s.type === 'text') { const lines = String(s.text).split('\n'), k = s.k || 1; return [s.x, s.y, s.x + Math.max(...lines.map(l => l.length)) * (10 + s.size) * k, s.y + lines.length * (22 + s.size * 2.4) * k]; }
    return [Math.min(s.x1, s.x2), Math.min(s.y1, s.y2), Math.max(s.x1, s.x2), Math.max(s.y1, s.y2)];
  }
  function moveShape(s, dx, dy) {
    if (s.type === 'pen') s.pts = s.pts.map(p => [p[0] + dx, p[1] + dy]);
    else if (s.type === 'text') { s.x += dx; s.y += dy; }
    else { s.x1 += dx; s.y1 += dy; s.x2 += dx; s.y2 += dy; }
  }
  // Redimensionar: tiradores en las esquinas de la caja de selección. Se escala el objeto de la
  // caja original a la nueva (el texto, de forma proporcional según la altura).
  const PAD = 8, HS = 14, MIN = 10;
  const handles = (s) => { const [x1, y1, x2, y2] = bbox(s); return [['nw', x1 - PAD, y1 - PAD], ['ne', x2 + PAD, y1 - PAD], ['sw', x1 - PAD, y2 + PAD], ['se', x2 + PAD, y2 + PAD]]; };
  function pickHandle(x, y) {
    if (!(state.tool === 'select' && sel >= 0 && state.shapes[sel])) return null;
    for (const [h, hx, hy] of handles(state.shapes[sel])) if (Math.abs(x - hx) <= HS && Math.abs(y - hy) <= HS) return h;
    return null;
  }
  function scaleShape(o, b, n) {
    const bw = b[2] - b[0], bh = b[3] - b[1];
    const fx = (v) => bw ? n[0] + (v - b[0]) * (n[2] - n[0]) / bw : v;
    const fy = (v) => bh ? n[1] + (v - b[1]) * (n[3] - n[1]) / bh : v;
    const s = JSON.parse(JSON.stringify(o));
    if (s.type === 'pen') s.pts = o.pts.map(p => [Math.round(fx(p[0]) * 10) / 10, Math.round(fy(p[1]) * 10) / 10]);
    else if (s.type === 'text') { s.k = Math.max(0.3, Math.min(8, (o.k || 1) * (bh ? (n[3] - n[1]) / bh : 1))); s.x = n[0]; s.y = n[1]; }
    else { s.x1 = fx(o.x1); s.y1 = fy(o.y1); s.x2 = fx(o.x2); s.y2 = fy(o.y2); }
    return s;
  }
  const handleCursor = (h) => h ? (h === 'nw' || h === 'se' ? 'nwse-resize' : 'nesw-resize') : 'default';

  function pick(x, y) { for (let i = state.shapes.length - 1; i >= 0; i--) if (hit(state.shapes[i], x, y)) return i; return -1; }
  function deleteSelected() { if (!(sel >= 0 && state.shapes[sel])) return; const i = sel; sel = -1; mutate(() => state.shapes.splice(i, 1)); }

  function hit(s, x, y) {
    const tol = Math.max(10, s.size + 6);
    const segDist = (ax, ay, bx, by) => { const dx = bx - ax, dy = by - ay, l = dx * dx + dy * dy; let t = l ? ((x - ax) * dx + (y - ay) * dy) / l : 0; t = Math.max(0, Math.min(1, t)); return Math.hypot(x - (ax + t * dx), y - (ay + t * dy)); };
    if (s.type === 'pen') { for (let i = 1; i < s.pts.length; i++) if (segDist(...s.pts[i - 1], ...s.pts[i]) < tol) return true; return s.pts.length === 1 && Math.hypot(x - s.pts[0][0], y - s.pts[0][1]) < tol; }
    if (s.type === 'line' || s.type === 'arrow') return segDist(s.x1, s.y1, s.x2, s.y2) < tol;
    if (s.type === 'rect') { const x1 = Math.min(s.x1, s.x2), x2 = Math.max(s.x1, s.x2), y1 = Math.min(s.y1, s.y2), y2 = Math.max(s.y1, s.y2);
      return [segDist(x1, y1, x2, y1), segDist(x2, y1, x2, y2), segDist(x1, y2, x2, y2), segDist(x1, y1, x1, y2)].some(d => d < tol); }
    if (s.type === 'text') { const lines = String(s.text).split('\n'); const k = s.k || 1; const w = Math.max(...lines.map(l => l.length)) * (10 + s.size) * k; const h = lines.length * (22 + s.size * 2.4) * k + 10; return x >= s.x - 6 && x <= s.x + w && y >= s.y - 6 && y <= s.y + h; }
    return false;
  }
  function eraseAt(x, y) {
    const i = pick(x, y); if (i >= 0) { sel = -1; mutate(() => state.shapes.splice(i, 1)); }
  }

  function openText(x, y) {
    closeText(true);
    const r = draw.getBoundingClientRect();
    const ta = document.createElement('textarea');
    ta.className = 'text-input'; ta.id = 'canvas-text'; ta.rows = 1; ta.setAttribute('aria-label', 'Texto del boceto');
    ta.style.left = (x * r.width / W) + 'px'; ta.style.top = (y * r.height / H) + 'px';
    ta.style.color = state.color; ta.style.fontSize = Math.max(14, (18 + state.size * 2) * r.width / W) + 'px';
    paper.appendChild(ta); textBox = { el: ta, x, y };
    setTimeout(() => ta.focus(), 0);
    ta.addEventListener('keydown', (e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); closeText(true); } else if (e.key === 'Escape') { closeText(false); } });
    ta.addEventListener('blur', () => closeText(true));
  }
  function closeText(keep) {
    if (!textBox) return; const { el, x, y } = textBox; textBox = null;
    const v = el.value.trim(); el.remove();
    if (keep && v) commit({ type: 'text', text: v, x, y, color: state.color, size: state.size });
  }

  draw.addEventListener('pointerdown', (e) => {
    if (e.button !== 0 && e.pointerType === 'mouse') return;
    const [x, y] = (state.tool === 'pen' || state.tool === 'eraser') ? pos(e) : snapPos(e);
    if (state.tool === 'text') { e.preventDefault(); openText(x, y); return; }
    draw.setPointerCapture(e.pointerId);
    if (state.tool === 'select') {
      const [rx, ry] = pos(e); const h = pickHandle(rx, ry);
      if (h) { drag = { resize: h, orig: JSON.parse(JSON.stringify(state.shapes[sel])), box: bbox(state.shapes[sel]), before: snapshot(), moved: false }; render(); return; }
      sel = pick(rx, ry);
      drag = sel >= 0 ? { x: snapv(rx), y: snapv(ry), before: snapshot(), moved: false } : null;
      render(); return;
    }
    if (state.tool === 'eraser') { current = { type: 'erasing' }; eraseAt(x, y); return; }
    current = state.tool === 'pen' ? { type: 'pen', pts: [[x, y]], color: state.color, size: state.size }
                                   : { type: state.tool, x1: x, y1: y, x2: x, y2: y, color: state.color, size: state.size };
    render();
  });
  draw.addEventListener('pointermove', (e) => {
    if (drag && drag.resize && sel >= 0) {
      const [rx, ry] = pos(e), h = drag.resize; let [x1, y1, x2, y2] = drag.box;
      const nx = snapv(rx + (h.includes('w') ? PAD : -PAD)), ny = snapv(ry + (h.includes('n') ? PAD : -PAD));
      if (h.includes('w')) x1 = Math.min(nx, x2 - MIN); else x2 = Math.max(nx, x1 + MIN);
      if (h.includes('n')) y1 = Math.min(ny, y2 - MIN); else y2 = Math.max(ny, y1 + MIN);
      state.shapes[sel] = scaleShape(drag.orig, drag.box, [x1, y1, x2, y2]); drag.moved = true; render();
      return;
    }
    if (drag && sel >= 0) {
      const [mx, my] = snapPos(e); const dx = mx - drag.x, dy = my - drag.y;
      if (dx || dy) { moveShape(state.shapes[sel], dx, dy); drag.x = mx; drag.y = my; drag.moved = true; render(); }
      return;
    }
    if (!current) { if (state.tool === 'select') draw.style.cursor = handleCursor(pickHandle(...pos(e))); return; }
    const [x, y] = (current.type === 'pen' || current.type === 'erasing') ? pos(e) : snapPos(e);
    if (current.type === 'erasing') { eraseAt(x, y); return; }
    if (current.type === 'pen') current.pts.push([Math.round(x * 10) / 10, Math.round(y * 10) / 10]);
    else { current.x2 = x; current.y2 = y; }
    render();
  });
  const end = () => {
    if (drag) { const d = drag; drag = null; if (d.moved) { history.push(d.before); state.redo = []; saveDraft(); render(); } return; }
    if (!current) return; const s = current; current = null;
    if (s.type === 'erasing') return;
    if (s.type !== 'pen' && Math.hypot(s.x2 - s.x1, s.y2 - s.y1) < 4) { render(); return; }
    commit(s); };
  draw.addEventListener('pointerup', end); draw.addEventListener('pointercancel', end);

  // Herramientas
  const press = (sel, btn) => document.querySelectorAll(sel).forEach(b => b.setAttribute('aria-pressed', String(b === btn)));
  document.querySelectorAll('[data-tool]').forEach(b => b.addEventListener('click', () => { closeText(true); state.tool = b.dataset.tool; press('[data-tool]', b); if (state.tool !== 'select') sel = -1; render(); draw.style.cursor = { text: 'text', eraser: 'cell', select: 'default' }[state.tool] || 'crosshair'; }));
  document.querySelectorAll('[data-color]').forEach(b => b.addEventListener('click', () => { state.color = b.dataset.color; press('[data-color]', b); }));
  document.querySelectorAll('[data-size]').forEach(b => b.addEventListener('click', () => { state.size = +b.dataset.size; press('[data-size]', b); }));
  const paperEl = document.getElementById('paper');
  function setOrient(o) {
    if (o === 'c' && !capture) o = 'h';
    closeText(true); sel = -1; state.orient = o; [W, H] = DIMS[o];
    paperEl.style.aspectRatio = W + ' / ' + H;
    paperEl.classList.toggle('portrait', o === 'v');
    document.querySelectorAll('[data-orient]').forEach(b => b.setAttribute('aria-pressed', String(b.dataset.orient === o)));
    saveDraft(); requestAnimationFrame(resize);
  }
  document.querySelectorAll('[data-orient]').forEach(b => b.addEventListener('click', () => { if (state.orient !== b.dataset.orient) setOrient(b.dataset.orient); }));
  const snapBtn = document.getElementById('snap');
  const setSnap = (v) => { state.snap = v; snapBtn.setAttribute('aria-pressed', String(v)); saveDraft(); };
  snapBtn.addEventListener('click', () => setSnap(!state.snap));
  const undo = () => { if (!history.length) return; state.redo.push(snapshot()); state.shapes = JSON.parse(history.pop()); sel = -1; saveDraft(); render(); };
  const redo = () => { if (!state.redo.length) return; history.push(snapshot()); state.shapes = JSON.parse(state.redo.pop()); sel = -1; saveDraft(); render(); };
  document.getElementById('delete-sel').addEventListener('click', deleteSelected);
  document.getElementById('undo').addEventListener('click', undo);
  document.getElementById('redo').addEventListener('click', redo);
  document.addEventListener('keydown', (e) => {
    if (e.target.closest && e.target.closest('input, textarea, select')) return;
    const k = e.key.toLowerCase();
    if ((e.ctrlKey || e.metaKey) && k === 'z') { e.preventDefault(); e.shiftKey ? redo() : undo(); return; }
    if ((e.ctrlKey || e.metaKey) && k === 'y') { e.preventDefault(); redo(); return; }
    if (!e.ctrlKey && !e.metaKey && k === 'g') { setSnap(!state.snap); return; }
    if (e.key === 'Delete' || e.key === 'Backspace') { if (sel >= 0) { e.preventDefault(); deleteSelected(); } return; }
    if (e.key === 'Escape') { sel = -1; render(); return; }
    const map = { v: 'select', p: 'pen', r: 'rect', l: 'line', a: 'arrow', t: 'text', e: 'eraser' };
    if (!e.ctrlKey && !e.metaKey && map[k]) document.querySelector(`[data-tool="${map[k]}"]`).click();
  });

  // Borrar todo con confirmación en la propia página
  const clearWrap = document.getElementById('clear-wrap');
  function showClear() {
    clearWrap.innerHTML = '<span class="confirm">¿Borrar el dibujo? <button class="btn danger" id="clear-yes" type="button">Borrar</button><button class="btn ghost" id="clear-no" type="button">Cancelar</button></span>';
    document.getElementById('clear-yes').onclick = () => { sel = -1; mutate(() => { state.shapes = []; }); resetClear(); };
    document.getElementById('clear-no').onclick = resetClear;
  }
  function resetClear() { clearWrap.innerHTML = '<button class="tbtn" id="clear" title="Borrar todo">Borrar todo</button>'; document.getElementById('clear').onclick = showClear; }
  document.getElementById('clear').onclick = showClear;

  // Exportar: PNG con fondo y dibujo
  function exportPng() {
    const c = document.createElement('canvas'); c.width = W; c.height = H;
    const x = c.getContext('2d'); renderBg(x); state.shapes.forEach(s => pathShape(x, s));
    return new Promise(res => c.toBlob(b => res(b), 'image/png'));
  }

  const statusEl = document.getElementById('status');
  let statusTimer = 0;
  const setStatus = (msg, cls) => {
    statusEl.textContent = msg; statusEl.className = 'toast show' + (cls ? ' ' + cls : '');
    clearTimeout(statusTimer); statusTimer = setTimeout(() => { statusEl.className = 'toast'; }, 2600);
  };
  let fileName = 'boceto';

  document.getElementById('copy-btn').addEventListener('click', async () => {
    closeText(true);
    if (!state.shapes.length && state.orient !== 'c') { setStatus('Dibuja algo antes de copiarlo.', 'err'); return; }
    try {
      const blob = await exportPng();
      await navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })]);
      setStatus('Imagen copiada. Pégala con Ctrl+V.', 'ok');
    } catch (err) {
      setStatus('No se ha podido copiar la imagen; descárgala.', 'err');
    }
  });
  document.getElementById('download-btn').addEventListener('click', async () => {
    closeText(true);
    const blob = await exportPng();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    const name = fileName.toLowerCase().replace(/[^a-z0-9áéíóúñü]+/gi, '-').replace(/^-|-$/g, '').slice(0, 80);
    a.href = url; a.download = (name || 'boceto') + '.png'; document.body.appendChild(a); a.click(); a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 2000);
    setStatus('Imagen descargada.', 'ok');
  });

  // Captura de la pestaña de origen (la guarda background.js al pulsar el icono)
  async function loadCapture() {
    try {
      if (!(window.chrome && chrome.storage && chrome.storage.session)) return;
      const { captura, capturaTitulo } = await chrome.storage.session.get(['captura', 'capturaTitulo']);
      if (!captura) return;
      const img = new Image();
      await new Promise((ok, ko) => { img.onload = ok; img.onerror = ko; img.src = captura; });
      capture = img;
      DIMS.c = [1560, Math.round(1560 * img.naturalHeight / img.naturalWidth)];
      document.querySelector('[data-orient="c"]').hidden = false;
      if (capturaTitulo) fileName = capturaTitulo;
      if (location.hash === '#captura') setOrient('c');
    } catch (e) {
      console.debug('[pizarra/board.js] sin captura disponible:', e && e.message);
    }
  }

  setSnap(state.snap); setOrient(state.orient === 'c' ? 'h' : state.orient); loadCapture();
  window.addEventListener('resize', resize);
  if (document.fonts && document.fonts.ready) document.fonts.ready.then(resize);
  resize();
})();
