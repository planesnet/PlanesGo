(() => {
  // El papel es un tapete con el dispositivo dibujado encima (pantalla del móvil 390 × 844 a
  // escala ×2). Con un patrón que no cabe entero, sus otras partes salen como pantallas
  // adicionales en el tapete, a la derecha del dispositivo.
  const DEV = { v: [780, 1560], h: [1560, 780] };
  const M = 80, GAP = 200, HEADER = 100; // margen del tapete, hueco entre pantallas, cabecera de la app
  let capture = null, capH = 878; // imagen de la pestaña desde la que se abrió la pizarra (fondo "Captura")
  const GRID = 20;
  let W = 1720, H = 940, lay = { screens: [] };
  const paper = document.getElementById('paper');
  const bg = document.getElementById('bg'), draw = document.getElementById('draw');
  const bctx = bg.getContext('2d'), ctx = draw.getContext('2d');
  const state = { tool: 'pen', color: '#142030', size: 4, orient: 'h', snap: true, pattern: '', shapes: [], redo: [] };
  // Todo cambio guarda antes una instantánea: deshacer/rehacer cubren dibujar, mover y eliminar.
  const snapshot = () => JSON.stringify(state.shapes);
  function mutate(fn) { history.push(snapshot()); if (history.length > 200) history.shift(); state.redo = []; fn(); saveDraft(); render(); }
  let current = null, textBox = null, sel = -1, drag = null;
  const history = [];

  // Borrador local (comodidad: no se pierde el dibujo al recargar)
  const DRAFT = 'pizarra-bocetos-borrador';
  function saveDraft() { try { localStorage.setItem(DRAFT, JSON.stringify({ v: 2, shapes: state.shapes, orient: state.orient, snap: state.snap, pattern: state.pattern })); } catch (e) {} }

  function resize() {
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    const r = paper.getBoundingClientRect();
    for (const c of [bg, draw]) { c.width = Math.round(r.width * dpr); c.height = Math.round(r.height * dpr); }
    const s = (r.width * dpr) / W;
    bctx.setTransform(s, 0, 0, s, 0, 0); ctx.setTransform(s, 0, 0, s, 0, 0);
    renderBg(bctx); render();
  }

  // ── Patrones de diseño (patterns.js) ─────────────────────────────────────────────────
  // Cada patrón trae sus pantallas para compacto (vertical) y expandido (horizontal). La primera
  // va en el dispositivo; las demás, como pantallas en el tapete enlazadas con una flecha.
  const PATTERNS = {};
  for (const p of (window.PizarraPatrones && window.PizarraPatrones.list) || []) PATTERNS[p.id] = p;
  const WIDE_MIN = 1200; // ancho del dispositivo (unidades) a partir del cual se usa el modo expandido

  // Disposición del tapete según orientación y patrón
  function layout() {
    if (state.orient === 'c') return { W: 1560, H: capH, screens: [] };
    const [dw, dh] = DEV[state.orient], p = PATTERNS[state.pattern];
    const specs = p ? (dw >= WIDE_MIN ? p.wide : p.compact) : [{}];
    const screens = specs.map((spec, i) => ({ x: M + i * (dw + GAP), y: M, w: dw, h: dh, extra: i > 0, spec }));
    const n = screens.length;
    return { W: M * 2 + n * dw + (n - 1) * GAP, H: M * 2 + dh, screens };
  }

  function gridLines(c, x0, y0, w, h, minor, major) {
    c.lineWidth = 1;
    for (let x = x0; x <= x0 + w; x += GRID) { c.strokeStyle = x % (GRID * 5) ? minor : major; c.beginPath(); c.moveTo(x + 0.5, y0); c.lineTo(x + 0.5, y0 + h); c.stroke(); }
    for (let y = y0; y <= y0 + h; y += GRID) { c.strokeStyle = y % (GRID * 5) ? minor : major; c.beginPath(); c.moveTo(x0, y + 0.5); c.lineTo(x0 + w, y + 0.5); c.stroke(); }
  }
  function drawScreen(c, s) {
    const frame = () => { c.beginPath(); c.roundRect(s.x, s.y, s.w, s.h, 36); };
    c.save(); c.shadowColor = 'rgba(15,23,42,.14)'; c.shadowBlur = 30; c.shadowOffsetY = 10; c.fillStyle = '#ffffff'; frame(); c.fill(); c.restore();
    c.save(); frame(); c.clip();
    gridLines(c, s.x, s.y, s.w, s.h, '#eef2f7', '#dde5ee');
    // Barra superior de la app (salvo que la pantalla no la lleve) y contenido del patrón
    const spec = s.spec, full = { x: s.x, y: s.y, w: s.w, h: s.h };
    let r = full, anchor = null;
    if (spec.bar !== false) { window.PizarraPatrones ? window.PizarraPatrones.appBar(c, { x: s.x, y: s.y, w: s.w, h: HEADER }, spec.bar || {}) : null; r = { x: s.x, y: s.y + HEADER, w: s.w, h: s.h - HEADER }; }
    if (spec.draw) anchor = spec.draw(c, r, full) || null;
    c.restore();
    // Marco del dispositivo (discontinuo en las pantallas adicionales del tapete)
    c.save(); c.strokeStyle = s.extra ? '#94a3b8' : '#64748b'; c.lineWidth = 4; if (s.extra) c.setLineDash([14, 10]); frame(); c.stroke(); c.restore();
    return anchor;
  }
  // Flecha en el tapete desde el elemento que abre la pantalla siguiente
  function drawLink(c, from, to, label) {
    c.save(); c.strokeStyle = '#64748b'; c.fillStyle = '#64748b'; c.lineWidth = 3; c.setLineDash([10, 8]);
    c.beginPath(); c.moveTo(from[0] + 12, from[1]); c.lineTo(to[0] - 16, from[1]); c.stroke(); c.setLineDash([]);
    c.beginPath(); c.moveTo(to[0] - 4, from[1]); c.lineTo(to[0] - 22, from[1] - 11); c.lineTo(to[0] - 22, from[1] + 11); c.closePath(); c.fill();
    c.font = '600 22px system-ui, sans-serif'; c.textAlign = 'center'; c.textBaseline = 'bottom'; c.fillText(label, (from[0] + to[0]) / 2, from[1] - 12);
    c.restore();
  }

  // Fondo: tapete con rejilla, dispositivo(s) con la cabecera de la app y el patrón elegido.
  // Va en su propio lienzo: el borrador no lo toca.
  function renderBg(c) {
    if (state.orient === 'c' && capture) {
      c.save(); c.fillStyle = '#ffffff'; c.fillRect(0, 0, W, H); c.drawImage(capture, 0, 0, W, H);
      if (state.snap) { c.globalAlpha = 0.18; c.strokeStyle = '#64748b'; c.lineWidth = 1;
        for (let x = 0; x <= W; x += GRID * 5) { c.beginPath(); c.moveTo(x + 0.5, 0); c.lineTo(x + 0.5, H); c.stroke(); }
        for (let y = 0; y <= H; y += GRID * 5) { c.beginPath(); c.moveTo(0, y + 0.5); c.lineTo(W, y + 0.5); c.stroke(); } }
      c.restore(); return;
    }
    c.save(); c.fillStyle = '#e9eef4'; c.fillRect(0, 0, W, H);
    gridLines(c, 0, 0, W, H, '#e1e7ee', '#d6dee8');
    const anchors = lay.screens.map(s => drawScreen(c, s));
    lay.screens.forEach((s, i) => {
      if (!i) return; const prev = lay.screens[i - 1], a = anchors[i - 1] || [prev.x + prev.w, prev.y + prev.h / 2];
      drawLink(c, a, [s.x, a[1]], prev.spec.link || '');
    });
    c.restore();
  }
  const HAND = "'Segoe Print', 'Bradley Hand', 'Comic Sans MS', cursive";
  // Parte el texto en líneas que caben en maxW (para las cajas de texto)
  function wrapText(c, text, maxW) {
    const out = [];
    for (const para of String(text).split('\n')) {
      let line = '';
      for (const word of para.split(/\s+/)) { const t = line ? line + ' ' + word : word; if (c.measureText(t).width > maxW && line) { out.push(line); line = word; } else line = t; }
      out.push(line);
    }
    return out;
  }
  function pathShape(c, s, noText) {
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
    } else if (s.type === 'box') {
      // Caja de texto: fondo blanco, borde y texto ajustado al ancho de la caja
      const x1 = Math.min(s.x1, s.x2), y1 = Math.min(s.y1, s.y2), w = Math.abs(s.x2 - s.x1), h = Math.abs(s.y2 - s.y1);
      c.beginPath(); c.roundRect(x1, y1, w, h, 8); c.fillStyle = 'rgba(255,255,255,.92)'; c.fill();
      c.lineWidth = Math.max(2, s.size / 2); c.stroke();
      if (!noText && s.text) {
        c.clip(); c.fillStyle = s.color; c.font = `${18 + s.size * 2}px ${HAND}`; c.textBaseline = 'top';
        const lh = 22 + s.size * 2.4;
        wrapText(c, s.text, w - 28).forEach((ln, i) => c.fillText(ln, x1 + 14, y1 + 12 + i * lh));
      }
    } else if (s.type === 'text' && !noText) {
      const k = s.k || 1;
      c.font = `${Math.round((18 + s.size * 2) * k)}px 'Segoe Print', 'Bradley Hand', 'Comic Sans MS', cursive`; c.textBaseline = 'top';
      String(s.text).split('\n').forEach((ln, i) => c.fillText(ln, s.x, s.y + i * (22 + s.size * 2.4) * k));
    }
    c.restore();
  }
  function render() {
    ctx.clearRect(0, 0, W, H);
    state.shapes.forEach((s, i) => pathShape(ctx, s, textBox && textBox.index === i));
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
    if (s.type === 'box') { const x1 = Math.min(s.x1, s.x2), x2 = Math.max(s.x1, s.x2), y1 = Math.min(s.y1, s.y2), y2 = Math.max(s.y1, s.y2); return x >= x1 - tol && x <= x2 + tol && y >= y1 - tol && y <= y2 + tol; }
    if (s.type === 'rect') { const x1 = Math.min(s.x1, s.x2), x2 = Math.max(s.x1, s.x2), y1 = Math.min(s.y1, s.y2), y2 = Math.max(s.y1, s.y2);
      return [segDist(x1, y1, x2, y1), segDist(x2, y1, x2, y2), segDist(x1, y2, x2, y2), segDist(x1, y1, x1, y2)].some(d => d < tol); }
    if (s.type === 'text') { const lines = String(s.text).split('\n'); const k = s.k || 1; const w = Math.max(...lines.map(l => l.length)) * (10 + s.size) * k; const h = lines.length * (22 + s.size * 2.4) * k + 10; return x >= s.x - 6 && x <= s.x + w && y >= s.y - 6 && y <= s.y + h; }
    return false;
  }
  function eraseAt(x, y) {
    const i = pick(x, y); if (i >= 0) { sel = -1; mutate(() => state.shapes.splice(i, 1)); }
  }

  // Editor de texto sobre el lienzo: crea un texto nuevo en (x, y) o cambia el de un texto o una
  // caja ya dibujados (index). Intro termina, Mayús+Intro añade una línea, Esc cancela.
  const editable = (i) => i >= 0 && (state.shapes[i].type === 'text' || state.shapes[i].type === 'box');
  function openText(x, y, index = -1) {
    closeText(true);
    const s = index >= 0 ? state.shapes[index] : null, f = draw.getBoundingClientRect().width / W;
    const ta = document.createElement('textarea');
    ta.className = 'text-input'; ta.id = 'canvas-text'; ta.setAttribute('aria-label', 'Texto del boceto');
    ta.value = s ? s.text || '' : ''; ta.style.color = s ? s.color : state.color;
    if (s && s.type === 'box') {
      const [x1, y1, x2, y2] = bbox(s);
      ta.classList.add('in-box');
      Object.assign(ta.style, { left: x1 * f + 'px', top: y1 * f + 'px', width: (x2 - x1) * f + 'px', height: (y2 - y1) * f + 'px',
        fontSize: (18 + s.size * 2) * f + 'px', lineHeight: (22 + s.size * 2.4) * f + 'px', padding: `${12 * f}px ${14 * f}px` });
    } else {
      const sx = s ? s.x : x, sy = s ? s.y : y, size = s ? s.size : state.size;
      ta.style.left = sx * f + 'px'; ta.style.top = sy * f + 'px';
      ta.style.fontSize = Math.max(14, (18 + size * 2) * (s && s.k || 1) * f) + 'px';
      const rows = () => { ta.rows = Math.max(1, ta.value.split('\n').length); }; rows(); ta.addEventListener('input', rows);
    }
    paper.appendChild(ta); textBox = { el: ta, x, y, index };
    setTimeout(() => { ta.focus(); ta.select(); }, 0);
    ta.addEventListener('keydown', (e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); closeText(true); } else if (e.key === 'Escape') { closeText(false); } });
    ta.addEventListener('blur', () => closeText(true));
    render();
  }
  function closeText(keep) {
    if (!textBox) return; const { el, x, y, index } = textBox; textBox = null;
    const v = el.value.trim(); el.remove();
    if (index >= 0) {
      const s = state.shapes[index];
      if (keep && s && v !== (s.text || '')) {
        if (!v && s.type === 'text') { sel = -1; mutate(() => state.shapes.splice(index, 1)); }
        else mutate(() => { s.text = v; });
      } else render();
      return;
    }
    if (keep && v) commit({ type: 'text', text: v, x, y, color: state.color, size: state.size });
  }

  draw.addEventListener('pointerdown', (e) => {
    if (e.button !== 0 && e.pointerType === 'mouse') return;
    const [x, y] = (state.tool === 'pen' || state.tool === 'eraser') ? pos(e) : snapPos(e);
    if (state.tool === 'text') { e.preventDefault(); const i = pick(...pos(e)); editable(i) ? openText(0, 0, i) : openText(x, y); return; }
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
    if (s.type === 'box') {
      // Tamaño mínimo para poder escribir; al soltar se abre para escribir dentro
      if (Math.abs(s.x2 - s.x1) < 80) s.x2 = s.x1 + (s.x2 < s.x1 ? -1 : 1) * 200;
      if (Math.abs(s.y2 - s.y1) < 40) s.y2 = s.y1 + (s.y2 < s.y1 ? -1 : 1) * 60;
      s.text = ''; commit(s); openText(0, 0, state.shapes.length - 1); return;
    }
    commit(s); };
  // Doble clic sobre un texto o una caja: cambiar su texto
  draw.addEventListener('dblclick', (e) => { const i = pick(...pos(e)); if (editable(i)) { drag = null; openText(0, 0, i); } });
  draw.addEventListener('pointerup', end); draw.addEventListener('pointercancel', end);

  // Herramientas
  const press = (sel, btn) => document.querySelectorAll(sel).forEach(b => b.setAttribute('aria-pressed', String(b === btn)));
  document.querySelectorAll('[data-tool]').forEach(b => b.addEventListener('click', () => { closeText(true); state.tool = b.dataset.tool; press('[data-tool]', b); if (state.tool !== 'select') sel = -1; render(); draw.style.cursor = { text: 'text', eraser: 'cell', select: 'default' }[state.tool] || 'crosshair'; }));
  document.querySelectorAll('[data-color]').forEach(b => b.addEventListener('click', () => { state.color = b.dataset.color; press('[data-color]', b); }));
  document.querySelectorAll('[data-size]').forEach(b => b.addEventListener('click', () => { state.size = +b.dataset.size; press('[data-size]', b); }));
  const paperEl = document.getElementById('paper'), patternSel = document.getElementById('pattern');
  const groups = {};
  for (const p of Object.values(PATTERNS)) {
    if (!groups[p.cat]) { groups[p.cat] = document.createElement('optgroup'); groups[p.cat].label = p.cat; patternSel.appendChild(groups[p.cat]); }
    groups[p.cat].appendChild(new Option(p.name, p.id));
  }
  function applyLayout() {
    lay = layout(); W = lay.W; H = lay.H;
    paperEl.style.aspectRatio = W + ' / ' + H;
    paperEl.style.width = `min(100%, calc((100vh - 230px) * ${(W / H).toFixed(4)}))`;
    patternSel.value = state.pattern; patternSel.disabled = state.orient === 'c';
    saveDraft(); requestAnimationFrame(resize);
  }
  patternSel.addEventListener('change', () => { closeText(true); sel = -1; state.pattern = patternSel.value; applyLayout(); });
  function setOrient(o) {
    if (o === 'c' && !capture) o = 'h';
    closeText(true); sel = -1; state.orient = o;
    document.querySelectorAll('[data-orient]').forEach(b => b.setAttribute('aria-pressed', String(b.dataset.orient === o)));
    applyLayout();
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
    const map = { v: 'select', p: 'pen', r: 'rect', l: 'line', a: 'arrow', t: 'text', b: 'box', e: 'eraser' };
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
    if (!state.shapes.length && !state.pattern && state.orient !== 'c') { setStatus('Dibuja algo o carga un patrón antes de copiarlo.', 'err'); return; }
    try {
      // La promesa va dentro del ClipboardItem: write() se llama en el propio clic y no pierde el permiso
      await navigator.clipboard.write([new ClipboardItem({ 'image/png': exportPng() })]);
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
      capH = Math.round(1560 * img.naturalHeight / img.naturalWidth);
      document.querySelector('[data-orient="c"]').hidden = false;
      if (capturaTitulo) fileName = capturaTitulo;
      if (location.hash === '#captura') setOrient('c');
    } catch (e) {
      console.debug('[pizarra/board.js] sin captura disponible:', e && e.message);
    }
  }

  // Borrador guardado (va aquí: necesita PATTERNS y moveShape)
  try {
    const d = JSON.parse(localStorage.getItem(DRAFT) || 'null');
    if (d && Array.isArray(d.shapes)) {
      state.shapes = d.shapes; state.orient = d.orient === 'v' ? 'v' : 'h'; if (d.snap === false) state.snap = false;
      if (PATTERNS[d.pattern]) state.pattern = d.pattern;
      if (!d.v) state.shapes.forEach(sh => moveShape(sh, M, M)); // borradores anteriores al tapete
    }
  } catch (e) {}
  setSnap(state.snap); setOrient(state.orient === 'c' ? 'h' : state.orient); loadCapture();
  window.addEventListener('resize', resize);
  if (document.fonts && document.fonts.ready) document.fonts.ready.then(resize);
  resize();
})();
