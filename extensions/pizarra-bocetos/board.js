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
  // Todo cambio guarda antes una instantánea (objetos, orientación y patrón): deshacer/rehacer
  // cubren dibujar, mover, agrupar, eliminar y cambiar de patrón u orientación.
  const snapshot = () => JSON.stringify({ shapes: state.shapes, orient: state.orient, pattern: state.pattern });
  function mutate(fn) { history.push(snapshot()); if (history.length > 200) history.shift(); state.redo = []; fn(); saveDraft(); render(); }
  // selected: objetos seleccionados (referencias a state.shapes). Un clic sobre un objeto de un
  // grupo selecciona el grupo entero.
  let current = null, textBox = null, selected = [], drag = null;
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
    // Sin patrón, la cabecera de la app como referencia; con patrón, la barra va en sus objetos
    if (!state.pattern && window.PizarraPatrones) window.PizarraPatrones.appBar(c, { x: s.x, y: s.y, w: s.w, h: HEADER }, {});
    c.restore();
    // Marco del dispositivo (discontinuo en las pantallas adicionales del tapete)
    c.save(); c.strokeStyle = s.extra ? '#94a3b8' : '#64748b'; c.lineWidth = 4; if (s.extra) c.setLineDash([14, 10]); frame(); c.stroke(); c.restore();
  }

  // Fondo: tapete con rejilla y dispositivo(s). El patrón no va aquí: se incrusta como objetos.
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
    lay.screens.forEach(s => drawScreen(c, s));
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
  // Trazo a mano (objetos de los patrones): líneas con una ligera curva y que se pasan un poco en
  // las esquinas. La semilla de cada objeto hace que el temblor sea siempre el mismo.
  function rng(seed) { let t = (Math.abs(seed | 0) % 2147483646) + 1; return () => { t = (t * 16807) % 2147483647; return (t - 1) / 2147483646; }; }
  function roughSeg(c, x1, y1, x2, y2, R, over) {
    const len = Math.hypot(x2 - x1, y2 - y1) || 1, ux = (x2 - x1) / len, uy = (y2 - y1) / len;
    const o1 = over * (0.3 + R() * 0.7), o2 = over * (0.3 + R() * 0.7), j = () => (R() - 0.5) * 1.6;
    const ax = x1 - ux * o1 + j(), ay = y1 - uy * o1 + j(), bx = x2 + ux * o2 + j(), by = y2 + uy * o2 + j();
    const bend = (R() - 0.5) * Math.min(7, len * 0.025);
    c.moveTo(ax, ay); c.quadraticCurveTo((ax + bx) / 2 - uy * bend, (ay + by) / 2 + ux * bend, bx, by);
  }
  function handShape(c, s) {
    const R = rng(s.seed);
    if (s.type === 'rect') {
      const x1 = Math.min(s.x1, s.x2), y1 = Math.min(s.y1, s.y2), x2 = Math.max(s.x1, s.x2), y2 = Math.max(s.y1, s.y2), r = s.r || 0;
      if (s.fill) { c.save(); c.fillStyle = s.fill; c.beginPath(); c.roundRect(x1, y1, x2 - x1, y2 - y1, Math.min(r, (x2 - x1) / 2, (y2 - y1) / 2)); c.fill(); c.restore(); }
      if (r >= 10) { // redondeado: dos pasadas algo desplazadas
        for (const a of [1, 0.45]) { const j = () => (R() - 0.5) * 3; c.globalAlpha = a; c.beginPath(); c.roundRect(x1 + j(), y1 + j(), x2 - x1 + j(), y2 - y1 + j(), Math.min(r, (x2 - x1) / 2, (y2 - y1) / 2)); c.stroke(); }
        c.globalAlpha = 1; return;
      }
      const over = Math.min(6, Math.min(x2 - x1, y2 - y1) * 0.08 + 1);
      c.beginPath(); roughSeg(c, x1, y1, x2, y1, R, over); roughSeg(c, x2, y1, x2, y2, R, over); roughSeg(c, x2, y2, x1, y2, R, over); roughSeg(c, x1, y2, x1, y1, R, over); c.stroke();
    } else {
      c.beginPath(); roughSeg(c, s.x1, s.y1, s.x2, s.y2, R, 0);
      if (s.type === 'arrow') {
        const a = Math.atan2(s.y2 - s.y1, s.x2 - s.x1), l = 12 + s.size * 2.2;
        roughSeg(c, s.x2, s.y2, s.x2 - l * Math.cos(a - 0.45), s.y2 - l * Math.sin(a - 0.45), R, 0);
        roughSeg(c, s.x2, s.y2, s.x2 - l * Math.cos(a + 0.45), s.y2 - l * Math.sin(a + 0.45), R, 0);
      }
      c.stroke();
    }
  }
  function pathShape(c, s, noText) {
    c.save(); c.strokeStyle = s.color; c.fillStyle = s.color; c.lineWidth = s.size; c.lineCap = 'round'; c.lineJoin = 'round';
    if (s.hand && (s.type === 'rect' || s.type === 'line' || s.type === 'arrow')) {
      handShape(c, s);
    } else if (s.type === 'pen') {
      c.beginPath(); s.pts.forEach((p, i) => i ? c.lineTo(p[0], p[1]) : c.moveTo(p[0], p[1]));
      if (s.pts.length === 1) c.lineTo(s.pts[0][0] + 0.1, s.pts[0][1]);
      c.stroke();
    } else if (s.type === 'rect') {
      c.beginPath(); c.roundRect(Math.min(s.x1, s.x2), Math.min(s.y1, s.y2), Math.abs(s.x2 - s.x1), Math.abs(s.y2 - s.y1), s.r || 6); c.stroke();
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
    selected = selected.filter(s => state.shapes.includes(s));
    if (selected.length) {
      const [x1, y1, x2, y2] = selBox();
      ctx.save(); ctx.strokeStyle = '#0284c7'; ctx.lineWidth = 2;
      // Con varios objetos sueltos, una marca fina en cada uno
      if (selected.length > 1 && !oneGroup()) { ctx.globalAlpha = 0.5; for (const o of selected) { const b = bbox(o); ctx.strokeRect(b[0] - 3, b[1] - 3, b[2] - b[0] + 6, b[3] - b[1] + 6); } ctx.globalAlpha = 1; }
      ctx.setLineDash(oneGroup() ? [14, 5, 3, 5] : [8, 6]);
      ctx.strokeRect(x1 - PAD, y1 - PAD, x2 - x1 + 2 * PAD, y2 - y1 + 2 * PAD); ctx.setLineDash([]);
      ctx.fillStyle = '#ffffff';
      for (const [, hx, hy] of handles()) { ctx.fillRect(hx - HS / 2, hy - HS / 2, HS, HS); ctx.strokeRect(hx - HS / 2, hy - HS / 2, HS, HS); }
      ctx.restore();
    }
    if (drag && drag.marquee) {
      const [ax, ay, bx, by] = drag.marquee;
      ctx.save(); ctx.fillStyle = 'rgba(2,132,199,.08)'; ctx.strokeStyle = '#0284c7'; ctx.lineWidth = 1.5; ctx.setLineDash([6, 4]);
      ctx.fillRect(Math.min(ax, bx), Math.min(ay, by), Math.abs(bx - ax), Math.abs(by - ay)); ctx.strokeRect(Math.min(ax, bx), Math.min(ay, by), Math.abs(bx - ax), Math.abs(by - ay)); ctx.restore();
    }
    document.getElementById('undo').disabled = !history.length;
    document.getElementById('redo').disabled = !state.redo.length;
    document.getElementById('delete-sel').disabled = !selected.length;
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
  // Caja de toda la selección
  function selBox() {
    const bs = selected.map(bbox);
    return [Math.min(...bs.map(b => b[0])), Math.min(...bs.map(b => b[1])), Math.max(...bs.map(b => b[2])), Math.max(...bs.map(b => b[3]))];
  }
  const oneGroup = () => selected.length > 1 && selected[0].g && selected.every(s => s.g === selected[0].g) && state.shapes.filter(s => s.g === selected[0].g).length === selected.length;
  const handles = () => { const [x1, y1, x2, y2] = selBox(); return [['nw', x1 - PAD, y1 - PAD], ['ne', x2 + PAD, y1 - PAD], ['sw', x1 - PAD, y2 + PAD], ['se', x2 + PAD, y2 + PAD]]; };
  function pickHandle(x, y) {
    if (!(state.tool === 'select' && selected.length)) return null;
    for (const [h, hx, hy] of handles()) if (Math.abs(x - hx) <= HS && Math.abs(y - hy) <= HS) return h;
    return null;
  }
  function scaleShape(o, b, n) {
    const bw = b[2] - b[0], bh = b[3] - b[1];
    const fx = (v) => bw ? n[0] + (v - b[0]) * (n[2] - n[0]) / bw : v;
    const fy = (v) => bh ? n[1] + (v - b[1]) * (n[3] - n[1]) / bh : v;
    const s = JSON.parse(JSON.stringify(o));
    if (s.type === 'pen') s.pts = o.pts.map(p => [Math.round(fx(p[0]) * 10) / 10, Math.round(fy(p[1]) * 10) / 10]);
    else if (s.type === 'text') { s.k = Math.max(0.3, Math.min(8, (o.k || 1) * (bh ? (n[3] - n[1]) / bh : 1))); s.x = fx(o.x); s.y = fy(o.y); }
    else { s.x1 = fx(o.x1); s.y1 = fy(o.y1); s.x2 = fx(o.x2); s.y2 = fy(o.y2); }
    return s;
  }
  const handleCursor = (h) => h ? (h === 'nw' || h === 'se' ? 'nwse-resize' : 'nesw-resize') : 'default';

  function pick(x, y) { for (let i = state.shapes.length - 1; i >= 0; i--) if (hit(state.shapes[i], x, y)) return i; return -1; }
  // Grupos: los objetos de un grupo comparten g. Los de un patrón llevan además pat mientras no
  // se toquen: así, al cambiar de orientación, se regeneran para la nueva forma.
  const members = (s) => s.g ? state.shapes.filter(o => o.g === s.g) : [s];
  function touch(objs) { for (const o of objs) for (const m of members(o)) delete m.pat; }
  const newGroupId = () => 'g' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
  function deleteSelected() {
    if (!selected.length) return; const del = selected; selected = [];
    mutate(() => { touch(del); state.shapes = state.shapes.filter(s => !del.includes(s)); });
  }
  const canGroup = () => selected.length > 1 && !oneGroup();
  const canUngroup = () => selected.some(s => s.g);
  function groupSelected() { if (!canGroup()) return; const g = newGroupId(), objs = selected; mutate(() => { touch(objs); objs.forEach(s => { s.g = g; }); }); }
  function ungroupSelected() { if (!canUngroup()) return; const objs = selected; mutate(() => { touch(objs); objs.forEach(s => { delete s.g; }); }); }
  function reorder(front) {
    if (!selected.length) return; const objs = state.shapes.filter(s => selected.includes(s));
    mutate(() => { const rest = state.shapes.filter(s => !objs.includes(s)); state.shapes = front ? [...rest, ...objs] : [...objs, ...rest]; });
  }

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
    const i = pick(x, y); if (i >= 0) { const s = state.shapes[i]; selected = selected.filter(o => o !== s); mutate(() => { touch([s]); state.shapes.splice(i, 1); }); }
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
        if (!v && s.type === 'text') { selected = []; mutate(() => { touch([s]); state.shapes.splice(index, 1); }); }
        else mutate(() => { touch([s]); s.text = v; });
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
      if (h) { drag = { resize: h, objs: selected.slice(), origs: selected.map(o => JSON.parse(JSON.stringify(o))), box: selBox(), before: snapshot(), moved: false }; render(); return; }
      const i = pick(rx, ry);
      if (i >= 0) {
        // Clic: el objeto (o su grupo); Mayús+clic: añadir o quitar de la selección
        const m = members(state.shapes[i]);
        if (e.shiftKey) selected = m.every(o => selected.includes(o)) ? selected.filter(o => !m.includes(o)) : [...selected, ...m.filter(o => !selected.includes(o))];
        else if (!selected.includes(state.shapes[i])) selected = m;
        drag = { x: snapv(rx), y: snapv(ry), before: snapshot(), moved: false };
      } else {
        // Arrastrar en vacío: selección por área
        drag = { marquee: [rx, ry, rx, ry], base: e.shiftKey ? selected.slice() : [] };
        if (!e.shiftKey) selected = [];
      }
      render(); return;
    }
    if (state.tool === 'eraser') { current = { type: 'erasing' }; eraseAt(x, y); return; }
    current = state.tool === 'pen' ? { type: 'pen', pts: [[x, y]], color: state.color, size: state.size }
                                   : { type: state.tool, x1: x, y1: y, x2: x, y2: y, color: state.color, size: state.size };
    render();
  });
  draw.addEventListener('pointermove', (e) => {
    if (drag && drag.marquee) { const [rx, ry] = pos(e); drag.marquee[2] = rx; drag.marquee[3] = ry; render(); return; }
    if (drag && drag.resize) {
      const [rx, ry] = pos(e), h = drag.resize; let [x1, y1, x2, y2] = drag.box;
      const nx = snapv(rx + (h.includes('w') ? PAD : -PAD)), ny = snapv(ry + (h.includes('n') ? PAD : -PAD));
      if (h.includes('w')) x1 = Math.min(nx, x2 - MIN); else x2 = Math.max(nx, x1 + MIN);
      if (h.includes('n')) y1 = Math.min(ny, y2 - MIN); else y2 = Math.max(ny, y1 + MIN);
      drag.objs.forEach((o, k) => Object.assign(o, scaleShape(drag.origs[k], drag.box, [x1, y1, x2, y2])));
      drag.moved = true; render();
      return;
    }
    if (drag && selected.length) {
      const [mx, my] = snapPos(e); const dx = mx - drag.x, dy = my - drag.y;
      if (dx || dy) { selected.forEach(o => moveShape(o, dx, dy)); drag.x = mx; drag.y = my; drag.moved = true; render(); }
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
    if (drag) {
      const d = drag; drag = null;
      if (d.marquee) {
        const [ax, ay, bx, by] = d.marquee, x1 = Math.min(ax, bx), x2 = Math.max(ax, bx), y1 = Math.min(ay, by), y2 = Math.max(ay, by);
        const inside = state.shapes.filter(s => { const b = bbox(s); return b[0] >= x1 && b[2] <= x2 && b[1] >= y1 && b[3] <= y2; });
        const add = []; for (const s of inside) for (const m of members(s)) if (!add.includes(m)) add.push(m);
        selected = [...d.base, ...add.filter(o => !d.base.includes(o))];
      } else if (d.moved) { history.push(d.before); state.redo = []; touch(selected); saveDraft(); }
      render(); return;
    }
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
  document.querySelectorAll('[data-tool]').forEach(b => b.addEventListener('click', () => { closeText(true); state.tool = b.dataset.tool; press('[data-tool]', b); if (state.tool !== 'select') selected = []; render(); draw.style.cursor = { text: 'text', eraser: 'cell', select: 'default' }[state.tool] || 'crosshair'; }));
  document.querySelectorAll('[data-color]').forEach(b => b.addEventListener('click', () => { state.color = b.dataset.color; press('[data-color]', b); }));
  document.querySelectorAll('[data-size]').forEach(b => b.addEventListener('click', () => { state.size = +b.dataset.size; press('[data-size]', b); }));
  const paperEl = document.getElementById('paper');
  function applyLayout() {
    lay = layout(); W = lay.W; H = lay.H;
    paperEl.style.aspectRatio = W + ' / ' + H;
    paperEl.style.width = `min(100%, calc((100vh - 230px) * ${(W / H).toFixed(4)}))`;
    renderLibrary();
    saveDraft(); requestAnimationFrame(resize);
  }
  // Incrusta el patrón como un grupo de objetos dibujados a mano (debajo de lo que ya hay). Sin
  // force, solo se regenera si sus objetos siguen sin tocar.
  function placePattern(force) {
    if (!force && !state.shapes.some(s => s.pat)) return;
    const keep = state.shapes.filter(s => !s.pat);
    const gen = state.pattern && window.PizarraBoceto ? window.PizarraBoceto.generate(lay.screens, HEADER) : [];
    const g = newGroupId(); gen.forEach(s => { s.g = g; s.pat = true; });
    state.shapes = [...gen, ...keep];
    selected = gen.length && force ? gen : [];
  }
  function usePattern(id) {
    closeText(true); selected = [];
    if (state.orient === 'c' && id) state.orient = 'h';
    mutate(() => { state.pattern = id; lay = layout(); placePattern(true); });
    pressOrient(); applyLayout();
  }

  // ── Biblioteca de patrones ───────────────────────────────────────────────────────────
  // Pantallas completas (patterns.js), componentes y campos (pieces.js), con su ficha de uso y
  // comportamiento (guia.js, extraída de m3.material.io). Las pantallas sustituyen el patrón del
  // dispositivo; los componentes y campos se insertan como un grupo más.
  const GUIA = window.PizarraGuia || { layout: { patrones: {} }, componentes: {}, campos: {} };
  const PIEZAS = {}; for (const p of (window.PizarraPiezas && window.PizarraPiezas.list) || []) PIEZAS[p.id] = p;
  const CANON = { 'lista-detalle': 'list-detail', 'panel-apoyo': 'supporting-pane', 'feed': 'feed' };
  const lib = document.getElementById('library'), libBtn = document.getElementById('lib-btn');
  const libList = document.getElementById('lib-list'), libDetail = document.getElementById('lib-detail'), libSearch = document.getElementById('lib-search');
  let libTab = 'pantalla', libOpen = null;
  const fold = (t) => String(t || '').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
  // Ficha unificada de cualquier patrón
  function ficha(kind, id) {
    if (kind === 'pantalla') {
      const p = PATTERNS[id], g = GUIA.layout.patrones[CANON[id]] || {};
      return { kind, id, name: p.name, cat: p.cat, m3: g.nombreM3 ? `${g.nombreM3} (layout canónico)` : p.info.m3, url: g.url,
        que: g.que, cuandoUsar: g.cuandoUsar, cuandoNo: g.cuandoNo, comportamiento: g.comportamiento,
        compacto: g.compacto || p.info.compacto, medio: g.medio, expandido: g.expandido || p.info.expandido, componentes: p.info.componentes, notas: g.notas };
    }
    const pz = PIEZAS[id];
    if (pz.kind === 'componente') { const g = GUIA.componentes[id] || {}; return { kind: 'componente', id, name: pz.name, cat: pz.cat, m3: g.nombreM3, url: g.url, que: g.que, variantes: g.variantes, cuandoUsar: g.cuandoUsar, cuandoNo: g.cuandoNo, comportamiento: g.comportamiento, compacto: g.compacto, expandido: g.expandido, accesibilidad: g.accesibilidad }; }
    const f = pz.ficha || {};
    return { kind: 'campo', id, name: pz.name, cat: pz.cat, m3: 'Text fields, menus, pickers (guía de campos)', url: 'https://m3.material.io/components/text-fields/guidelines',
      que: f.que, formato: f.formato, comportamiento: f.comportamiento, compacto: f.compacto, expandido: f.expandido, html: f.html };
  }
  function entries(kind) {
    if (kind === 'pantalla') return Object.values(PATTERNS).map(p => ({ kind, id: p.id, name: p.name, cat: p.cat, sub: (GUIA.layout.patrones[CANON[p.id]] || {}).nombreM3 || p.info.m3 }));
    return Object.values(PIEZAS).filter(p => p.kind === kind).map(p => ({ kind, id: p.id, name: p.name, cat: p.cat, sub: p.kind === 'componente' ? (GUIA.componentes[p.id] || {}).nombreM3 || '' : (p.ficha && p.ficha.html) || '' }));
  }
  const el = (tag, attrs = {}, ...kids) => { const e = document.createElement(tag); for (const [k, v] of Object.entries(attrs)) { if (k === 'class') e.className = v; else if (k.startsWith('on')) e.addEventListener(k.slice(2), v); else e.setAttribute(k, v); } for (const k of kids) if (k != null) e.append(k); return e; };
  function renderLibrary() {
    if (lib.hidden) return;
    document.querySelectorAll('.lib-tabs [data-tab]').forEach(b => b.setAttribute('aria-selected', String(b.dataset.tab === libTab)));
    if (libOpen) { renderDetail(); return; }
    libDetail.hidden = true; libList.hidden = false; libList.replaceChildren();
    const q = fold(libSearch.value.trim());
    const list = q ? ['pantalla', 'componente', 'campo'].flatMap(entries).filter(e => fold([e.name, e.sub, e.cat, e.id].join(' ')).includes(q)) : entries(libTab);
    if (libTab === 'pantalla' && !q && GUIA.layout.fundamentos) {
      const f = GUIA.layout.fundamentos, t = el('table', {}, el('tr', {}, el('th', {}, 'Clase'), el('th', {}, 'Ancho'), el('th', {}, 'Paneles'), el('th', {}, 'Navegación')));
      for (const k of f.clasesTamano || []) t.append(el('tr', {}, el('td', {}, k.nombre), el('td', {}, k.rango), el('td', {}, k.paneles || ''), el('td', {}, k.navegacion || '')));
      libList.append(el('details', { class: 'lib-info' }, el('summary', {}, 'Diseño adaptable: clases de tamaño de ventana'), t, el('ul', {}, ...(f.reglas || []).map(r => el('li', {}, r)))));
    }
    if (!list.length) { libList.append(el('p', { class: 'lib-empty' }, 'No hay patrones con ese nombre. Prueba con otra palabra (p. ej. «fecha», «barra», «menú»).')); return; }
    let cat = null;
    const label = { pantalla: 'Pantallas', componente: 'Componentes', campo: 'Campos y datos' };
    for (const e of list) {
      const head = q ? label[e.kind] + ' · ' + e.cat : e.cat;
      if (head !== cat) { cat = head; libList.append(el('h3', {}, head)); }
      const b = el('button', { type: 'button', class: 'lib-item' + (e.kind === 'pantalla' && state.pattern === e.id ? ' on' : ''), onclick: () => { libOpen = { kind: e.kind, id: e.id }; renderLibrary(); } }, el('b', {}, e.name), e.sub ? el('small', {}, e.sub) : null);
      libList.append(b);
    }
  }
  function renderDetail() {
    const f = ficha(libOpen.kind, libOpen.id);
    libList.hidden = true; libDetail.hidden = false; libDetail.replaceChildren(); libDetail.className = 'lib-body lib-detail';
    const back = el('button', { type: 'button', class: 'tbtn', 'aria-label': 'Volver a la lista', title: 'Volver', onclick: () => { libOpen = null; renderLibrary(); } });
    back.innerHTML = '<svg viewBox="0 0 24 24"><path d="M15 6l-6 6 6 6"/></svg>';
    libDetail.append(el('div', { class: 'lib-detail-head' }, back, el('h4', {}, f.name)));
    if (f.m3) libDetail.append(f.url ? el('a', { class: 'm3', href: f.url, target: '_blank', rel: 'noopener' }, 'Material 3: ' + f.m3 + ' ↗') : el('span', { class: 'm3' }, 'Material 3: ' + f.m3));
    const acts = el('div', { class: 'lib-actions' });
    if (f.kind === 'pantalla') {
      if (state.pattern === f.id) acts.append(el('button', { type: 'button', class: 'btn ghost', onclick: () => { usePattern(''); } }, 'Quitar de la pizarra'));
      else acts.append(el('button', { type: 'button', class: 'btn', onclick: () => { usePattern(f.id); setStatus(`Pantalla «${f.name}» en la pizarra.`, 'ok'); } }, 'Usar esta pantalla'));
    } else acts.append(el('button', { type: 'button', class: 'btn', onclick: () => insertPiece(f.id) }, 'Insertar en el dispositivo'));
    libDetail.append(acts);
    const sec = (title, v) => { if (!v || (Array.isArray(v) && !v.length)) return; libDetail.append(el('section', {}, el('h5', {}, title), Array.isArray(v) ? el('ul', {}, ...v.map(x => el('li', {}, x))) : el('p', {}, v))); };
    sec('Qué es', f.que); sec('Variantes', f.variantes); sec('Cuándo usarlo', f.cuandoUsar); sec('Cuándo no', f.cuandoNo);
    sec('Comportamiento', f.comportamiento); sec('Formato', f.formato);
    sec('Compacto (< 600 dp, móvil en vertical)', f.compacto); sec('Mediano (600–839 dp)', f.medio); sec('Expandido (≥ 840 dp, horizontal, tableta, escritorio)', f.expandido);
    sec('Componentes', f.componentes); sec('Accesibilidad', f.accesibilidad); sec('Notas', f.notas);
    if (f.html) libDetail.append(el('section', {}, el('h5', {}, 'En HTML'), el('p', {}, el('code', {}, f.html))));
    if (f.kind === 'campo' && GUIA.campos) {
      const g = GUIA.campos, d = el('details', { class: 'lib-info' }, el('summary', {}, 'Reglas comunes de los campos (Material 3)'));
      for (const [k, t] of [['etiqueta', 'Etiqueta'], ['textoAyuda', 'Texto de ayuda'], ['error', 'Error'], ['prefijoSufijo', 'Prefijo y sufijo'], ['obligatorio', 'Obligatorio'], ['tipoEntrada', 'Tipo de entrada']]) if (g[k]) d.append(el('p', {}, el('b', {}, t + ': '), g[k].join(' ')));
      libDetail.append(d);
    }
    libDetail.scrollTop = 0;
  }
  function openLibrary(v) {
    lib.hidden = !v; libBtn.setAttribute('aria-expanded', String(v));
    if (v) { renderLibrary(); libSearch.focus(); } else libBtn.focus();
  }
  libBtn.addEventListener('click', () => openLibrary(lib.hidden));
  document.getElementById('lib-close').addEventListener('click', () => openLibrary(false));
  document.querySelectorAll('.lib-tabs [data-tab]').forEach(b => b.addEventListener('click', () => { libTab = b.dataset.tab; libOpen = null; libSearch.value = ''; renderLibrary(); }));
  libSearch.addEventListener('input', () => { libOpen = null; renderLibrary(); });
  lib.addEventListener('keydown', (e) => { if (e.key === 'Escape') { e.stopPropagation(); openLibrary(false); } });

  // Inserta un componente o campo en el dispositivo, en su sitio, como grupo seleccionado
  function insertPiece(id) {
    const pz = PIEZAS[id]; if (!pz || !window.PizarraBoceto) return;
    closeText(true);
    const s = lay.screens[0] || { x: 0, y: 0, w: W, h: H }, mode = state.orient === 'v' ? 'compacto' : 'expandido';
    const [w, h] = pz.size({ w: s.w, h: s.h }, mode);
    const at = {
      top: [s.x, s.y], bottom: [s.x, s.y + s.h - h], left: [s.x, s.y + s.h - h], right: [s.x + s.w - w, s.y + s.h - h],
      fab: [s.x + s.w - w - 32, s.y + s.h - h - 32], bottomInset: [s.x + (s.w - w) / 2, s.y + s.h - h - 48],
      topInset: [s.x + (s.w - w) / 2, s.y + 16], belowBar: [s.x, s.y + HEADER],
    }[pz.place] || [s.x + (s.w - w) / 2, s.y + Math.max(HEADER, (s.h - h) / 2)];
    const r = { x: Math.round(at[0]), y: Math.round(at[1]), w, h };
    const shapes = window.PizarraBoceto.generatePiece(pz, r, mode, { x: s.x, y: s.y, w: s.w, h: s.h });
    const g = newGroupId(); shapes.forEach(sh => { sh.g = g; sh.pz = id; });
    mutate(() => { state.shapes.push(...shapes); });
    document.querySelector('[data-tool="select"]').click(); selected = shapes; render();
    setStatus(`«${pz.name}» insertado. Arrástralo para colocarlo.`, 'ok');
  }
  const pressOrient = () => document.querySelectorAll('[data-orient]').forEach(b => b.setAttribute('aria-pressed', String(b.dataset.orient === state.orient)));
  function setOrient(o) {
    if (o === 'c' && !capture) o = 'h';
    closeText(true); selected = [];
    mutate(() => { state.orient = o; lay = layout(); placePattern(false); });
    pressOrient(); applyLayout();
  }
  document.querySelectorAll('[data-orient]').forEach(b => b.addEventListener('click', () => { if (state.orient !== b.dataset.orient) setOrient(b.dataset.orient); }));
  const snapBtn = document.getElementById('snap');
  const setSnap = (v) => { state.snap = v; snapBtn.setAttribute('aria-pressed', String(v)); saveDraft(); };
  snapBtn.addEventListener('click', () => setSnap(!state.snap));
  function restore(snap) {
    const d = JSON.parse(snap); state.shapes = d.shapes; selected = [];
    if (d.orient !== state.orient || d.pattern !== state.pattern) { state.orient = d.orient; state.pattern = d.pattern; pressOrient(); applyLayout(); }
  }
  const undo = () => { if (!history.length) return; closeText(false); state.redo.push(snapshot()); restore(history.pop()); saveDraft(); render(); };
  const redo = () => { if (!state.redo.length) return; closeText(false); history.push(snapshot()); restore(state.redo.pop()); saveDraft(); render(); };
  document.getElementById('delete-sel').addEventListener('click', deleteSelected);
  document.getElementById('undo').addEventListener('click', undo);
  document.getElementById('redo').addEventListener('click', redo);
  document.addEventListener('keydown', (e) => {
    if (e.target.closest && e.target.closest('input, textarea, select, #library, #ctx-menu')) return;
    const k = e.key.toLowerCase();
    if ((e.ctrlKey || e.metaKey) && k === 'z') { e.preventDefault(); e.shiftKey ? redo() : undo(); return; }
    if ((e.ctrlKey || e.metaKey) && k === 'y') { e.preventDefault(); redo(); return; }
    if ((e.ctrlKey || e.metaKey) && k === 'g') { e.preventDefault(); e.shiftKey ? ungroupSelected() : groupSelected(); render(); return; }
    if ((e.ctrlKey || e.metaKey) && k === 'a') { e.preventDefault(); document.querySelector('[data-tool="select"]').click(); selected = state.shapes.slice(); render(); return; }
    if (!e.ctrlKey && !e.metaKey && k === 'g') { setSnap(!state.snap); return; }
    if (e.key === 'Delete' || e.key === 'Backspace') { if (selected.length) { e.preventDefault(); deleteSelected(); } return; }
    if (e.key === 'Escape') { hideMenu(); selected = []; render(); return; }
    const map = { v: 'select', p: 'pen', r: 'rect', l: 'line', a: 'arrow', t: 'text', b: 'box', e: 'eraser' };
    if (!e.ctrlKey && !e.metaKey && map[k]) document.querySelector(`[data-tool="${map[k]}"]`).click();
  });

  // Menú contextual (clic derecho): agrupar, desagrupar, orden y eliminar
  const menu = document.getElementById('ctx-menu');
  const ACTIONS = { group: groupSelected, ungroup: ungroupSelected, front: () => reorder(true), back: () => reorder(false), delete: deleteSelected };
  function hideMenu() { menu.hidden = true; }
  draw.addEventListener('contextmenu', (e) => {
    e.preventDefault(); closeText(true);
    const [x, y] = pos(e), i = pick(x, y);
    if (i >= 0 && !selected.includes(state.shapes[i])) selected = members(state.shapes[i]);
    render();
    const can = { group: canGroup(), ungroup: canUngroup(), front: !!selected.length, back: !!selected.length, delete: !!selected.length };
    menu.querySelectorAll('[data-act]').forEach(b => { b.disabled = !can[b.dataset.act]; });
    menu.hidden = false;
    const mw = menu.offsetWidth, mh = menu.offsetHeight;
    menu.style.left = Math.min(e.clientX, window.innerWidth - mw - 8) + 'px'; menu.style.top = Math.min(e.clientY, window.innerHeight - mh - 8) + 'px';
    const first = menu.querySelector('[data-act]:not(:disabled)'); if (first) first.focus();
  });
  menu.addEventListener('click', (e) => { const b = e.target.closest('[data-act]'); if (!b || b.disabled) return; hideMenu(); ACTIONS[b.dataset.act](); render(); });
  menu.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') { hideMenu(); draw.focus && draw.focus(); return; }
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return; e.preventDefault();
    const items = [...menu.querySelectorAll('[data-act]:not(:disabled)')], k = items.indexOf(document.activeElement);
    items[(k + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length].focus();
  });
  document.addEventListener('pointerdown', (e) => { if (!menu.hidden && !menu.contains(e.target)) hideMenu(); }, true);
  window.addEventListener('blur', hideMenu); window.addEventListener('resize', hideMenu);

  // Borrar todo con confirmación en la propia página
  const clearWrap = document.getElementById('clear-wrap');
  function showClear() {
    clearWrap.innerHTML = '<span class="confirm">¿Borrar el dibujo? <button class="btn danger" id="clear-yes" type="button">Borrar</button><button class="btn ghost" id="clear-no" type="button">Cancelar</button></span>';
    document.getElementById('clear-yes').onclick = () => { selected = []; mutate(() => { state.shapes = []; }); resetClear(); };
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

  // ── Copiar para Claude: explicación en Markdown + JSON ─────────────────────────────────
  // Describe el patrón en sus dos modos (compacto y expandido) y las anotaciones del usuario (los
  // objetos que no vienen del patrón) con su pantalla, zona y posición en dp (1 dp = 2 unidades).
  const COLOR_NAMES = { '#142030': 'negro', '#0284c7': 'azul', '#dc2626': 'rojo', '#059669': 'verde', '#ea580c': 'naranja', '#7c3aed': 'morado' };
  const TYPE_NAMES = { text: 'texto', box: 'caja de texto', arrow: 'flecha', line: 'línea', rect: 'rectángulo', pen: 'trazo a mano' };
  const dp = (v) => Math.round(v / 2);
  function screensOf(p, mode) {
    const specs = mode === 'compacto' ? p.compact : p.wide;
    return specs.map((sp, i) => ({
      pantalla: i + 1,
      nombre: sp.name || (specs.length > 1 ? `Pantalla ${i + 1}` : p.name),
      ...(sp.bar && sp.bar.title ? { tituloBarra: sp.bar.title } : {}),
      ...(sp.link && specs[i + 1] ? { abre: { pantalla: i + 2, cuando: sp.link } } : {}),
    }));
  }
  function zone(x, y, sc) {
    const fx = (x - sc.x) / sc.w, fy = (y - sc.y) / sc.h;
    return `${fy < 1 / 3 ? 'arriba' : fy < 2 / 3 ? 'en medio' : 'abajo'} ${fx < 1 / 3 ? 'a la izquierda' : fx < 2 / 3 ? 'en el centro' : 'a la derecha'}`;
  }
  function annotations() {
    const scs = lay.screens.length ? lay.screens : [{ x: 0, y: 0, w: W, h: H, spec: {} }];
    return state.shapes.filter(s => !s.hand).map(s => {
      const b = bbox(s), cx = (b[0] + b[2]) / 2, cy = (b[1] + b[3]) / 2;
      let k = scs.findIndex(sc => cx >= sc.x && cx <= sc.x + sc.w && cy >= sc.y && cy <= sc.y + sc.h);
      const sc = scs[k] || scs[0], a = { tipo: TYPE_NAMES[s.type] || s.type, color: COLOR_NAMES[s.color] || s.color };
      if (s.text) a.texto = s.text;
      a.pantalla = k >= 0 ? k + 1 : null; a.zona = k >= 0 ? zone(cx, cy, sc) : 'fuera de las pantallas';
      if (s.type === 'arrow' || s.type === 'line') { a.desdeDp = [dp(s.x1 - sc.x), dp(s.y1 - sc.y)]; a.hastaDp = [dp(s.x2 - sc.x), dp(s.y2 - sc.y)]; }
      else { a.xDp = dp(b[0] - sc.x); a.yDp = dp(b[1] - sc.y); a.anchoDp = dp(b[2] - b[0]); a.altoDp = dp(b[3] - b[1]); }
      if (s.g) a.grupo = s.g;
      return a;
    });
  }
  function describeForClaude() {
    const p = PATTERNS[state.pattern], mode = state.orient === 'v' ? 'compacto' : 'expandido';
    const [dw, dh] = state.orient === 'c' ? [W, H] : DEV[state.orient];
    const notes = annotations(), hand = state.shapes.filter(s => s.hand);
    const spec = {
      formato: 'pizarra-bocetos/v1',
      boceto: {
        vista: state.orient === 'c' ? 'captura de la página' : state.orient === 'v' ? 'móvil en vertical' : 'móvil en horizontal',
        ...(state.orient !== 'c' ? { modo: mode, claseTamanoVentana: mode === 'compacto' ? 'compact' : 'expanded' } : { pagina: fileName }),
        dispositivoDp: [dp(dw), dp(dh)],
      },
    };
    if (p) {
      spec.patron = {
        id: p.id, nombre: p.name, categoria: p.cat, materialDesign3: p.info.m3, componentes: p.info.componentes,
        modificadoAMano: hand.length > 0 && !hand.some(s => s.pat),
      };
      spec.modos = {
        compacto: { cuando: 'ventana compacta (< 600 dp, móvil en vertical)', comportamiento: p.info.compacto, pantallas: screensOf(p, 'compacto') },
        expandido: { cuando: 'ventana mediana o expandida (≥ 600 dp, móvil en horizontal, tableta, escritorio)', comportamiento: p.info.expandido, pantallas: screensOf(p, 'expandido') },
      };
    }
    // Componentes y campos insertados (un grupo por pieza)
    const seen = new Set(), pieces = [];
    for (const sh of state.shapes) {
      if (!sh.pz || seen.has(sh.g)) continue; seen.add(sh.g);
      const objs = state.shapes.filter(o => o.g === sh.g), bs = objs.map(bbox);
      const b = [Math.min(...bs.map(v => v[0])), Math.min(...bs.map(v => v[1])), Math.max(...bs.map(v => v[2])), Math.max(...bs.map(v => v[3]))];
      const scs = lay.screens.length ? lay.screens : [{ x: 0, y: 0, w: W, h: H }], cx = (b[0] + b[2]) / 2, cy = (b[1] + b[3]) / 2;
      const k = scs.findIndex(sc => cx >= sc.x && cx <= sc.x + sc.w && cy >= sc.y && cy <= sc.y + sc.h), sc = scs[k] || scs[0];
      const f = ficha(PIEZAS[sh.pz].kind, sh.pz);
      pieces.push({ id: f.id, nombre: f.name, tipo: f.kind, materialDesign3: f.m3, ...(f.url ? { guia: f.url } : {}), pantalla: k >= 0 ? k + 1 : null, zona: k >= 0 ? zone(cx, cy, sc) : 'fuera de las pantallas',
        xDp: dp(b[0] - sc.x), yDp: dp(b[1] - sc.y), anchoDp: dp(b[2] - b[0]), altoDp: dp(b[3] - b[1]),
        uso: f.que, comportamiento: (f.comportamiento || []).slice(0, 4), ...(f.formato ? { formato: f.formato } : {}), ...(f.html ? { html: f.html } : {}),
        ...(f.compacto ? { compacto: f.compacto } : {}), ...(f.expandido ? { expandido: f.expandido } : {}) });
    }
    if (pieces.length) spec.componentes = pieces;
    spec.estilo = 'No incluido: el patrón solo define estructura y comportamiento. Usa el tema y los estilos del proyecto (colores, tipografía, formas); los grises y el trazo a mano son del boceto.';
    spec.anotaciones = notes;

    const L = [];
    L.push('## Boceto de pantalla', '');
    if (p) {
      L.push(`Usa el patrón **${p.name}** de Material Design 3 (${p.info.m3}) como base de esta pantalla. Tiene que ser adaptativo: implementa los dos modos.`, '');
      L.push(`- **Compacto** (ventana < 600 dp, móvil en vertical): ${p.info.compacto}`);
      const cs = screensOf(p, 'compacto');
      if (cs.length > 1) L.push(`  - Pantallas: ${cs.map(x => `${x.pantalla}. ${x.nombre}${x.abre ? ` → «${x.abre.cuando}» abre la ${x.abre.pantalla}` : ''}`).join('; ')}.`);
      L.push(`- **Expandido** (ventana ≥ 600 dp, horizontal, tableta o escritorio): ${p.info.expandido}`);
      L.push(`- Componentes: ${p.info.componentes.join(', ')}.`);
      L.push('- **Estilo**: no va en el patrón. Usa el tema y los estilos del proyecto (colores, tipografía, formas); los grises y el trazo a mano son solo del boceto.');
      if (spec.patron.modificadoAMano) L.push('- He modificado el patrón a mano: si la imagen y esta descripción no coinciden, manda la imagen en la distribución.');
      L.push('');
    } else if (state.orient === 'c') {
      L.push(`Anotaciones sobre la página «${fileName}». La imagen es una captura con mis marcas encima.`, '');
    } else {
      L.push('Boceto libre, sin patrón de base.', '');
    }
    if (pieces.length) {
      L.push('**Componentes y campos que he colocado** (sigue su guía de Material 3):', '');
      pieces.forEach((pc, i) => {
        L.push(`${i + 1}. **${pc.nombre}** (${pc.materialDesign3 || pc.tipo})${pc.pantalla ? `, pantalla ${pc.pantalla}` : ''}, ${pc.zona}.`);
        if (pc.uso) L.push(`   - Uso: ${pc.uso}`);
        for (const c of pc.comportamiento) L.push(`   - ${c}`);
        for (const c of pc.formato || []) L.push(`   - Formato: ${c}`);
        if (pc.html) L.push(`   - HTML: \`${pc.html}\``);
        if (pc.compacto || pc.expandido) L.push(`   - Compacto: ${pc.compacto || 'igual'} · Expandido: ${pc.expandido || 'igual'}`);
      });
      L.push('');
    }
    L.push(`El boceto está dibujado en **${spec.boceto.vista}**${state.orient !== 'c' ? ` (modo ${mode}, ${spec.boceto.dispositivoDp.join(' × ')} dp)` : ''}.`);
    if (notes.length) {
      L.push(p ? `Mis anotaciones se refieren a ese modo; aplica lo que pidan también al otro modo cuando tenga sentido:` : 'Mis anotaciones:', '');
      notes.forEach((a, i) => L.push(`${i + 1}. ${a.tipo[0].toUpperCase() + a.tipo.slice(1)}${a.texto ? ` «${a.texto.replace(/\n/g, ' ')}»` : ''} en ${a.color}, ${a.pantalla ? `pantalla ${a.pantalla}, ` : ''}${a.zona}.`));
      if (notes.some(a => a.color === 'rojo')) L.push('', 'Lo marcado en rojo es lo que hay que cambiar.');
    } else L.push(p ? 'No hay anotaciones: implementa el patrón tal cual.' : 'No hay más anotaciones.');
    L.push('', 'Si pego también la imagen del boceto, úsala como referencia visual; la estructura y los modos son los de esta descripción.', '', '```json', JSON.stringify(spec, null, 2), '```');
    return L.join('\n');
  }
  document.getElementById('claude-btn').addEventListener('click', async () => {
    closeText(true);
    const text = describeForClaude();
    try { await navigator.clipboard.writeText(text); setStatus('Explicación y JSON copiados. Pégalos en el chat junto a la imagen.', 'ok'); }
    catch (err) { setStatus('No se ha podido copiar la explicación.', 'err'); }
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
      if (location.hash === '#captura') { state.orient = 'c'; pressOrient(); lay = layout(); placePattern(false); applyLayout(); }
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
  if (state.orient === 'c') state.orient = 'h';
  setSnap(state.snap); pressOrient(); lay = layout();
  // Borradores con el patrón como fondo fijo (versión anterior): se incrusta como objetos
  if (state.pattern && !state.shapes.some(s => s.hand)) placePattern(true);
  selected = []; applyLayout(); loadCapture();
  window.addEventListener('resize', resize);
  if (document.fonts && document.fonts.ready) document.fonts.ready.then(resize);
  resize();
})();
