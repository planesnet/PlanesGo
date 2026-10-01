// Patrones de diseño de Material Design 3 para la pizarra de bocetos.
// Cada patrón define sus pantallas en modo compacto (móvil en vertical) y expandido (horizontal).
// En compacto un patrón se puede descomponer en varias pantallas: la pizarra pinta la primera en
// el dispositivo y las demás en el tapete, enlazadas con una flecha desde el punto que devuelve
// draw() de la pantalla anterior. Todo va en grises (wireframe) para que el boceto destaque.
// Coordenadas: 1 dp = 2 unidades (el dispositivo en vertical mide 780 × 1560).
(() => {
  const WF = {
    line: '#cbd5e1', mid: '#94a3b8', dark: '#64748b', soft: '#f1f5f9', surface: '#f8fafc', tonal: '#e2e8f0',
    container: '#eef2f6', pill: '#dbe4ee', sel: '#e0f2fe', selLine: '#38bdf8', selFill: '#bae6fd',
    scrim: 'rgba(15,23,42,.32)', title: '#b6c2d1', font: 'system-ui, sans-serif',
  };
  const PAD = 32;
  const WV = [0.62, 0.48, 0.7, 0.55, 0.42, 0.66, 0.5, 0.58, 0.45, 0.68, 0.52, 0.6];
  const wv = (i) => WV[((i % WV.length) + WV.length) % WV.length];
  const snap = (v) => Math.round(v / 20) * 20;

  // ── Piezas básicas ─────────────────────────────────────────────────────────────────────
  const rr = (c, x, y, w, h, r) => { c.beginPath(); c.roundRect(x, y, w, h, r); };
  const bar = (c, x, y, w, h, color) => { c.fillStyle = color; rr(c, x, y, Math.max(h, w), h, h / 2); c.fill(); };
  const disc = (c, x, y, r, color) => { c.fillStyle = color; c.beginPath(); c.arc(x, y, r, 0, Math.PI * 2); c.fill(); };
  function txt(c, s, x, y, o = {}) {
    c.save(); c.fillStyle = o.color || WF.dark; c.font = `${o.weight || 600} ${o.size || 24}px ${WF.font}`;
    c.textAlign = o.align || 'left'; c.textBaseline = 'middle'; c.fillText(s, x, y); c.restore();
  }
  function icon(c, x, y, kind, color = WF.mid) {
    c.save(); c.strokeStyle = color; c.fillStyle = color; c.lineWidth = 3; c.lineCap = 'round'; c.lineJoin = 'round'; c.beginPath();
    switch (kind) {
      case 'menu': for (const d of [-10, 0, 10]) { c.moveTo(x - 15, y + d); c.lineTo(x + 15, y + d); } break;
      case 'search': c.arc(x - 3, y - 3, 11, 0, Math.PI * 2); c.moveTo(x + 5, y + 5); c.lineTo(x + 14, y + 14); break;
      case 'back': c.moveTo(x + 14, y); c.lineTo(x - 14, y); c.moveTo(x - 4, y - 11); c.lineTo(x - 15, y); c.lineTo(x - 4, y + 11); break;
      case 'close': c.moveTo(x - 11, y - 11); c.lineTo(x + 11, y + 11); c.moveTo(x + 11, y - 11); c.lineTo(x - 11, y + 11); break;
      case 'plus': c.moveTo(x - 12, y); c.lineTo(x + 12, y); c.moveTo(x, y - 12); c.lineTo(x, y + 12); break;
      case 'history': c.arc(x, y, 13, 0, Math.PI * 2); c.moveTo(x, y - 7); c.lineTo(x, y); c.lineTo(x + 6, y + 4); break;
      case 'chevron': c.moveTo(x - 6, y - 12); c.lineTo(x + 6, y); c.lineTo(x - 6, y + 12); break;
      case 'more': for (const d of [-10, 0, 10]) { c.moveTo(x + 3, y + d); c.arc(x, y + d, 3, 0, Math.PI * 2); } c.fill(); c.restore(); return;
      default: c.arc(x, y, 13, 0, Math.PI * 2);
    }
    c.stroke(); c.restore();
  }
  // Imagen (rectángulo con aspa)
  function ph(c, x, y, w, h, r = 16) {
    c.save(); rr(c, x, y, w, h, r); c.fillStyle = WF.soft; c.fill(); c.clip();
    c.strokeStyle = WF.line; c.lineWidth = 2; c.beginPath(); c.moveTo(x, y); c.lineTo(x + w, y + h); c.moveTo(x + w, y); c.lineTo(x, y + h); c.stroke();
    c.restore(); c.strokeStyle = WF.line; c.lineWidth = 2; rr(c, x, y, w, h, r); c.stroke();
  }
  function textLines(c, x, y, w, yMax) {
    let i = 0;
    for (; y + 14 <= yMax; y += 36, i++) bar(c, x, y, w * (0.92 - (i % 3) * 0.07), 14, WF.line);
    return y;
  }
  function button(c, x, y, w, h, label, kind = 'filled') {
    rr(c, x, y, w, h, h / 2);
    if (kind === 'filled') { c.fillStyle = WF.dark; c.fill(); }
    else if (kind === 'tonal') { c.fillStyle = WF.tonal; c.fill(); }
    else if (kind === 'outlined') { c.strokeStyle = WF.mid; c.lineWidth = 2; c.stroke(); }
    txt(c, label, x + w / 2, y + h / 2 + 1, { align: 'center', color: kind === 'filled' ? '#ffffff' : WF.dark });
  }
  function fab(c, x, y, o = {}) {
    const h = o.size || 112, w = o.label ? h + o.label.length * 14 + 12 : h;
    c.save(); c.shadowColor = 'rgba(15,23,42,.18)'; c.shadowBlur = 16; c.shadowOffsetY = 4;
    c.fillStyle = WF.pill; rr(c, x, y, w, h, h * 0.29); c.fill(); c.restore();
    icon(c, o.label ? x + h / 2 : x + w / 2, y + h / 2, 'plus', WF.dark);
    if (o.label) txt(c, o.label, x + h - 8, y + h / 2 + 1);
    return [x + w, y + h / 2];
  }
  function toggle(c, x, y, on) {
    rr(c, x, y, 104, 64, 32);
    if (on) { c.fillStyle = WF.dark; c.fill(); disc(c, x + 72, y + 32, 24, '#ffffff'); }
    else { c.strokeStyle = WF.mid; c.lineWidth = 3; c.stroke(); disc(c, x + 32, y + 32, 16, WF.mid); }
  }
  function chip(c, x, y, label, sel) {
    const w = label.length * 13 + 48; rr(c, x, y, w, 64, 16);
    if (sel) { c.fillStyle = WF.tonal; c.fill(); } else { c.strokeStyle = WF.mid; c.lineWidth = 2; c.stroke(); }
    txt(c, label, x + w / 2, y + 33, { align: 'center', size: 22 });
    return w;
  }
  function chips(c, x, y, labels) { for (const [i, l] of labels.entries()) x += chip(c, x, y, l, i === 0) + 16; return y + 64; }
  // Campo de texto con borde y etiqueta sobre el borde
  function field(c, x, y, w, label, i = 0) {
    c.strokeStyle = WF.mid; c.lineWidth = 2; rr(c, x, y, w, 96, 8); c.stroke();
    c.font = `600 20px ${WF.font}`; const lw = c.measureText(label).width + 16;
    c.fillStyle = '#ffffff'; c.fillRect(x + 20, y - 12, lw, 24);
    txt(c, label, x + 28, y + 1, { size: 20, color: WF.mid });
    bar(c, x + 28, y + 40, (w - 56) * wv(i) * 0.7, 16, WF.line);
    return y + 96;
  }
  function searchBar(c, x, y, w, o = {}) {
    const h = o.h || 96; rr(c, x, y, w, h, h / 2); c.fillStyle = '#e8edf3'; c.fill();
    icon(c, x + 48, y + h / 2, o.lead || 'search');
    txt(c, 'Buscar…', x + 92, y + h / 2 + 1, { size: 26, weight: 400, color: WF.mid });
    if (o.avatar !== false) disc(c, x + w - 48, y + h / 2, 22, WF.line);
    return y + h;
  }
  // Fila de lista (avatar o icono, una o dos líneas, y opcionalmente flecha o interruptor)
  function listRow(c, x, y, w, i, o = {}) {
    const h = o.h || 112;
    if (o.sel) { c.fillStyle = WF.sel; c.fillRect(x, y, w, h); c.fillStyle = WF.selLine; c.fillRect(x, y, 6, h); }
    let tx = x + PAD;
    if (o.lead === 'icon') { icon(c, x + PAD + 16, y + h / 2, o.icon || 'dot'); tx += 64; }
    else if (o.lead !== false) { disc(c, x + PAD + 30, y + h / 2, 30, o.sel ? WF.selFill : WF.soft); tx += 84; }
    const tw = x + w - PAD - (o.trail === 'switch' ? 124 : o.trail ? 48 : 0) - tx;
    if (o.oneLine) bar(c, tx, y + h / 2 - 9, tw * wv(i), 18, WF.mid);
    else { bar(c, tx, y + h / 2 - 22, tw * wv(i), 18, WF.mid); bar(c, tx, y + h / 2 + 12, tw * wv(i + 5) * 0.8, 14, WF.line); }
    if (o.trail === 'chevron') icon(c, x + w - PAD - 10, y + h / 2, 'chevron');
    else if (o.trail === 'switch') toggle(c, x + w - PAD - 104, y + h / 2 - 32, i % 3 !== 1);
    if (o.divider !== false) { c.fillStyle = WF.line; c.fillRect(x + PAD, y + h - 1, w - 2 * PAD, 2); }
    return y + h;
  }
  // Filas hasta llenar el alto (o max); devuelve el punto de salida de la fila seleccionada
  function rows(c, x, y, w, yMax, o = {}) {
    const h = o.h || 112; let anchor = null;
    for (let i = 0; y + h <= yMax && (o.max == null || i < o.max); i++) {
      const sel = o.sel === i; if (sel) anchor = [x + w, y + h / 2];
      y = listRow(c, x, y, w, i + (o.start || 0), { ...o, sel });
    }
    return o.returnY ? y : anchor;
  }
  function card(c, x, y, w, h, i, o = {}) {
    c.save(); rr(c, x, y, w, h, 24); c.fillStyle = WF.surface; c.fill(); c.clip();
    const ih = o.img === false ? 0 : Math.round(h * (o.imgRatio || 0.55));
    if (ih) ph(c, x, y, w, ih, 0);
    const ty = y + ih + 28;
    if (ty + 20 < y + h - 12) bar(c, x + 28, ty, (w - 56) * wv(i), 20, WF.mid);
    if (ty + 54 < y + h - 12) bar(c, x + 28, ty + 40, (w - 56) * wv(i + 3) * 0.85, 14, WF.line);
    if (ty + 82 < y + h - 12) bar(c, x + 28, ty + 68, (w - 56) * wv(i + 7) * 0.6, 14, WF.line);
    c.restore(); c.strokeStyle = WF.line; c.lineWidth = 2; rr(c, x, y, w, h, 24); c.stroke();
  }
  // Cuadrícula de tarjetas que llena el rectángulo (la última fila queda cortada, como al hacer scroll)
  function grid(c, r, cols, ch, o = {}) {
    const gap = 24, pad = o.pad ?? PAD, cw = (r.w - 2 * pad - (cols - 1) * gap) / cols;
    c.save(); c.beginPath(); c.rect(r.x, r.y, r.w, r.h); c.clip();
    for (let y = r.y + (o.top ?? pad), i = 0; y < r.y + r.h; y += ch + gap)
      for (let k = 0; k < cols; k++) card(c, r.x + pad + k * (cw + gap), y, cw, ch, i++, o);
    c.restore();
  }
  function scrim(c, s) { c.fillStyle = WF.scrim; c.fillRect(s.x, s.y, s.w, s.h); }
  function sheet(c, x, y, w, h, radii) {
    c.save(); c.shadowColor = 'rgba(15,23,42,.2)'; c.shadowBlur = 24; c.fillStyle = WF.surface; rr(c, x, y, w, h, radii); c.fill(); c.restore();
  }
  function divider(c, x, y, w) { c.fillStyle = WF.line; c.fillRect(x, y, w, 2); }
  function vdivider(c, x, y, h) { c.fillStyle = WF.line; c.fillRect(x - 1, y, 2, h); }
  // Dos paneles lado a lado (ratio = ancho del primero)
  function split(c, r, ratio, fa, fb, o = {}) {
    const w1 = snap(r.w * ratio), b = { x: r.x + w1, y: r.y, w: r.w - w1, h: r.h };
    if (o.bg1) { c.fillStyle = o.bg1; c.fillRect(r.x, r.y, w1, r.h); }
    if (o.bg2) { c.fillStyle = o.bg2; c.fillRect(b.x, b.y, b.w, b.h); }
    const a = fa(c, { x: r.x, y: r.y, w: w1, h: r.h }); fb(c, b); vdivider(c, b.x, r.y, r.h);
    return a;
  }

  function radio(c, cx, cy, on) {
    c.strokeStyle = on ? WF.dark : WF.mid; c.lineWidth = 3; c.beginPath(); c.arc(cx, cy, 20, 0, Math.PI * 2); c.stroke();
    if (on) disc(c, cx, cy, 10, WF.dark);
  }
  // Botón segmentado (la opción activa, rellena y con marca)
  function segmented(c, x, y, w, labels, active) {
    const h = 80, sw = w / labels.length;
    labels.forEach((l, k) => {
      const sx = x + k * sw;
      if (k === active) { c.fillStyle = WF.tonal; rr(c, sx, y, sw, h, k === 0 ? [40, 0, 0, 40] : k === labels.length - 1 ? [0, 40, 40, 0] : 0); c.fill(); }
      if (k) vdivider(c, sx, y, h);
      const tx = sx + sw / 2 + (k === active ? 16 : 0);
      if (k === active) { c.strokeStyle = WF.dark; c.lineWidth = 3; c.beginPath(); c.moveTo(tx - 64 - l.length * 6.5, y + 41); c.lineTo(tx - 56 - l.length * 6.5, y + 49); c.lineTo(tx - 42 - l.length * 6.5, y + 31); c.stroke(); }
      txt(c, l, tx, y + h / 2 + 1, { align: 'center', color: k === active ? WF.dark : WF.mid });
    });
    c.strokeStyle = WF.mid; c.lineWidth = 2; rr(c, x, y, w, h, h / 2); c.stroke();
    return y + h;
  }
  // Fila de ajuste con texto real (título y valor)
  function settingRow(c, x, y, w, title, value, o = {}) {
    const h = 128;
    if (o.sel) { c.fillStyle = WF.sel; c.fillRect(x, y, w, h); }
    icon(c, x + PAD + 16, y + h / 2, 'dot');
    txt(c, title, x + PAD + 64, y + 46, { size: 26, color: WF.dark });
    if (value) txt(c, value, x + PAD + 64, y + 86, { size: 22, weight: 400, color: WF.mid });
    if (o.trail === 'switch') toggle(c, x + w - PAD - 104, y + h / 2 - 32, true);
    divider(c, x + PAD, y + h - 1, w - 2 * PAD);
    return y + h;
  }
  // Miniatura de una pantalla en claro u oscuro
  function themeThumb(c, x, y, w, h, dark, label, on) {
    rr(c, x, y, w, h, 20); c.fillStyle = dark ? WF.dark : '#ffffff'; c.fill(); c.strokeStyle = on ? WF.dark : WF.line; c.lineWidth = on ? 4 : 2; c.stroke();
    const fg = dark ? WF.mid : WF.line;
    bar(c, x + 24, y + 28, w * 0.5, 14, fg);
    for (let k = 0; k < 3; k++) { disc(c, x + 40, y + 84 + k * 52, 14, fg); bar(c, x + 68, y + 78 + k * 52, w * wv(k) * 0.6, 12, fg); }
    txt(c, label, x + w / 2, y + h + 34, { align: 'center', size: 22, color: on ? WF.dark : WF.mid });
  }
  function appearancePane(c, r) {
    const x = r.x + 48, w = r.w - 96; let y = r.y + 40;
    txt(c, 'Apariencia', x, y + 10, { size: 30, weight: 700 }); y += 60;
    txt(c, 'Tema', x, y + 10, { size: 22, color: WF.mid }); y += 36;
    y = segmented(c, x, y, Math.min(w, 760), ['Claro', 'Oscuro', 'Automático'], 2) + 48;
    const tw = Math.min(220, (w - 48) / 3), th = Math.round(tw * 1.1);
    themeThumb(c, x, y, tw, th, false, 'Claro', false);
    themeThumb(c, x + tw + 24, y, tw, th, true, 'Oscuro', false);
    // Automático: mitad clara, mitad oscura
    const ax = x + 2 * (tw + 24);
    c.save(); rr(c, ax, y, tw, th, 20); c.clip(); c.fillStyle = '#ffffff'; c.fillRect(ax, y, tw / 2, th); c.fillStyle = WF.dark; c.fillRect(ax + tw / 2, y, tw / 2, th); c.restore();
    c.strokeStyle = WF.dark; c.lineWidth = 4; rr(c, ax, y, tw, th, 20); c.stroke();
    txt(c, 'Automático', ax + tw / 2, y + th + 34, { align: 'center', size: 22 });
    y += th + 90;
    if (y + 128 <= r.y + r.h) settingRow(c, r.x + 16, y, r.w - 32, 'Colores dinámicos', 'Según el fondo de pantalla', { trail: 'switch' });
  }
  function themeSettings(c, r) {
    let y = r.y + 16;
    txt(c, 'Apariencia', r.x + PAD, y + 28, { size: 22, color: WF.dark }); y += 56;
    const anchorY = y + 64;
    y = settingRow(c, r.x, y, r.w, 'Tema', 'Automático (según el sistema)');
    y = settingRow(c, r.x, y, r.w, 'Colores dinámicos', 'Según el fondo de pantalla', { trail: 'switch' });
    y = settingRow(c, r.x, y, r.w, 'Tamaño del texto', 'Mediano');
    txt(c, 'General', r.x + PAD, y + 44, { size: 22, color: WF.dark });
    rows(c, r.x, y + 72, r.w, r.y + r.h, { lead: 'icon', trail: 'chevron', oneLine: true });
    return [r.x + r.w, anchorY];
  }

  // ── Barras de app y navegación ─────────────────────────────────────────────────────────
  // Barra superior: icono de navegación, título, acciones (iconos) y un botón opcional
  function appBar(c, r, b = {}) {
    const cy = r.y + r.h / 2 + 2; let x = r.x + 40;
    if (b.nav) { icon(c, r.x + 52, cy, b.nav); x = r.x + 96; }
    if (b.title !== '') txt(c, b.title || 'Título', x, cy, { size: 26, color: WF.title });
    let ax = r.x + r.w - 52;
    if (b.action) { const w = b.action.length * 14 + 56; button(c, r.x + r.w - 32 - w, cy - 32, w, 64, b.action); ax -= w + 8; }
    for (const a of b.actions || []) { icon(c, ax, cy, a); ax -= 72; }
    divider(c, r.x, r.y + r.h - 1, r.w);
  }
  function navBar(c, r, n = 4, active = 0) {
    const h = 160, y = r.y + r.h - h; c.fillStyle = WF.container; c.fillRect(r.x, y, r.w, h);
    for (let k = 0; k < n; k++) {
      const cx = r.x + (k + 0.5) * r.w / n;
      if (k === active) { c.fillStyle = WF.pill; rr(c, cx - 56, y + 24, 112, 64, 32); c.fill(); }
      icon(c, cx, y + 56, 'dot', k === active ? WF.dark : WF.mid);
      bar(c, cx - 36, y + 112, 72, 12, k === active ? WF.dark : WF.line);
    }
    return h;
  }
  function rail(c, r, active = 0) {
    const w = 176; c.fillStyle = WF.container; c.fillRect(r.x, r.y, w, r.h);
    icon(c, r.x + w / 2, r.y + 52, 'menu');
    fab(c, r.x + (w - 112) / 2, r.y + 100);
    for (let k = 0, y = r.y + 290; k < 4 && y + 100 < r.y + r.h; k++, y += 130) {
      if (k === active) { c.fillStyle = WF.pill; rr(c, r.x + w / 2 - 56, y - 32, 112, 64, 32); c.fill(); }
      icon(c, r.x + w / 2, y, 'dot', k === active ? WF.dark : WF.mid);
      bar(c, r.x + w / 2 - 34, y + 52, 68, 12, k === active ? WF.dark : WF.line);
    }
    vdivider(c, r.x + w, r.y, r.h);
    return w;
  }
  function drawer(c, x, y, w, h, modal) {
    if (modal) sheet(c, x, y, w, h, [0, 32, 32, 0]); else { c.fillStyle = WF.container; c.fillRect(x, y, w, h); }
    txt(c, 'Mi app', x + 56, y + 70, { size: 28, color: WF.dark });
    let yy = y + 130;
    for (let k = 0; k < 9 && yy + 112 <= y + h; k++) {
      if (k === 4) { divider(c, x + 56, yy + 16, w - 112); bar(c, x + 56, yy + 50, 120, 14, WF.mid); yy += 90; continue; }
      if (k === 0) { c.fillStyle = WF.pill; rr(c, x + 24, yy + 4, w - 48, 104, 52); c.fill(); }
      icon(c, x + 80, yy + 56, 'dot', k === 0 ? WF.dark : WF.mid);
      bar(c, x + 124, yy + 47, (w - 220) * wv(k + 2), 18, k === 0 ? WF.dark : WF.mid);
      yy += 112;
    }
    if (!modal) vdivider(c, x + w, y, h);
  }
  function tabs(c, x, y, w, labels, active = 0) {
    const tw = w / labels.length;
    labels.forEach((l, k) => {
      const cx = x + (k + 0.5) * tw;
      txt(c, l, cx, y + 48, { align: 'center', color: k === active ? WF.dark : WF.mid });
      if (k === active) { const lw = l.length * 14 + 16; c.fillStyle = WF.dark; rr(c, cx - lw / 2, y + 88, lw, 8, [8, 8, 0, 0]); c.fill(); }
    });
    divider(c, x, y + 95, w);
    return y + 96;
  }
  function bottomAppBar(c, r) {
    const h = 160, y = r.y + r.h - h; c.fillStyle = WF.container; c.fillRect(r.x, y, r.w, h);
    for (let k = 0; k < 4; k++) icon(c, r.x + 64 + k * 96, y + h / 2, 'dot');
    fab(c, r.x + r.w - PAD - 112, y + 24);
    return h;
  }
  function keyboard(c, x, y, w, h) {
    c.fillStyle = WF.tonal; c.fillRect(x, y, w, h);
    const rowsN = 4, kh = (h - 50) / rowsN;
    for (let k = 0; k < rowsN; k++) {
      const n = k === 3 ? 3 : 10 - k, kw = (w - 24) / 10;
      const keys = k === 3 ? [1.5, 6, 1.5] : Array(n).fill(1);
      let kx = x + 12 + (k === 1 ? kw / 2 : k === 2 ? kw : 0) + (k === 3 ? kw / 2 : 0);
      for (const f of keys) { c.fillStyle = '#ffffff'; rr(c, kx + 4, y + 20 + k * kh, kw * f - 8, kh - 16, 10); c.fill(); kx += kw * f; }
    }
  }

  // ── Contenidos reutilizados ────────────────────────────────────────────────────────────
  function listPane(c, r, o = {}) {
    const y = searchBar(c, r.x + PAD, r.y + PAD, r.w - 2 * PAD, { h: 80, avatar: false }) + 16;
    return rows(c, r.x, y, r.w, r.y + r.h - 8, { sel: 1, trail: o.chevron ? 'chevron' : null });
  }
  function detailPane(c, r) {
    const pad = 40; let y = r.y + pad;
    bar(c, r.x + pad, y, Math.min(420, r.w * 0.5), 30, WF.dark); y += 50;
    bar(c, r.x + pad, y, Math.min(300, r.w * 0.36), 16, WF.line); y += 44;
    const ih = Math.round(Math.min(260, r.h * 0.26)); ph(c, r.x + pad, y, r.w - 2 * pad, ih, 12); y += ih + 40;
    const cols = r.w > 900 ? 2 : 1, cw = (r.w - 2 * pad - (cols - 1) * 40) / cols, btnY = r.y + r.h - pad - 72;
    for (let i = 0; y + 70 <= btnY - 24; i++) {
      const cx = r.x + pad + (i % cols) * (cw + 40);
      bar(c, cx, y, cw * 0.3, 12, WF.line); bar(c, cx, y + 26, cw * [0.8, 0.6, 0.72, 0.5][i % 4], 18, WF.mid);
      if (i % cols === cols - 1) y += 84;
    }
    const bw = r.w > 900 ? 200 : (r.w - 2 * pad - 24) / 2;
    button(c, r.x + r.w - pad - bw * 2 - 24, btnY, bw, 72, 'Cancelar', 'outlined');
    button(c, r.x + r.w - pad - bw, btnY, bw, 72, 'Guardar');
  }
  function articlePane(c, r, o = {}) {
    let y = r.y + PAD; const ih = Math.round(Math.min(340, r.h * 0.3));
    ph(c, r.x + PAD, y, r.w - 2 * PAD, ih); y += ih + 36;
    bar(c, r.x + PAD, y, (r.w - 2 * PAD) * 0.6, 30, WF.dark); y += 60;
    textLines(c, r.x + PAD, y, r.w - 2 * PAD, r.y + r.h - PAD - (o.bottom || 0));
  }
  function relatedPane(c, r) {
    txt(c, 'Relacionado', r.x + PAD, r.y + 52, { size: 24 });
    rows(c, r.x, r.y + 88, r.w, r.y + r.h, { h: 104 });
  }
  function settingsList(c, r) {
    txt(c, 'General', r.x + PAD, r.y + 44, { size: 22, color: WF.dark });
    let y = rows(c, r.x, r.y + 72, r.w, r.y + r.h, { lead: 'icon', trail: 'switch', max: 3, returnY: true });
    txt(c, 'Cuenta', r.x + PAD, y + 44, { size: 22, color: WF.dark });
    rows(c, r.x, y + 72, r.w, r.y + r.h, { lead: 'icon', trail: 'chevron', start: 4 });
  }
  function heroContent(c, x, y, w, yMax) {
    bar(c, x, y, w * 0.62, 34, WF.dark); y += 58;
    bar(c, x, y, w * 0.4, 16, WF.mid); y += 44;
    y = chips(c, x, y, ['Hoy', 'Urgente', 'Madrid']) + 40;
    textLines(c, x, y, w, yMax - 120);
    const bw = (w - 24) / 2;
    button(c, x, yMax - 80, bw, 80, 'Compartir', 'outlined');
    button(c, x + bw + 24, yMax - 80, bw, 80, 'Reservar');
  }
  function loginForm(c, x, y, w, h) {
    const top = y + Math.max(40, (h - 680) / 2), cx = x + w / 2;
    disc(c, cx, top + 56, 56, WF.tonal); icon(c, cx, top + 56, 'dot', WF.dark);
    txt(c, 'Inicia sesión', cx, top + 166, { align: 'center', size: 40, weight: 700 });
    txt(c, 'Usa tu cuenta', cx, top + 214, { align: 'center', size: 22, weight: 400, color: WF.mid });
    field(c, x, top + 270, w, 'Correo', 2); field(c, x, top + 396, w, 'Contraseña', 4);
    button(c, x, top + 528, w, 84, 'Entrar');
    txt(c, '¿Has olvidado la contraseña?', cx, top + 656, { align: 'center', size: 22 });
  }
  function carousel(c, x, y, w, h, fr) {
    let cx = x;
    for (const f of fr) { const iw = (w - 16 * (fr.length - 1)) * f; ph(c, cx, y, iw, h, 28); cx += iw + 16; }
    return y + h;
  }
  function formFields(c, r, cols) {
    const labels = ['Cliente', 'Dirección de entrega', 'Fecha', 'Hora', 'Referencia', 'Notas'];
    const gap = 40, cw = (r.w - 2 * 48 - (cols - 1) * gap) / cols;
    let y = r.y + 64;
    for (let i = 0; i < labels.length && y + 96 <= r.y + r.h - 40; i++) {
      field(c, r.x + 48 + (i % cols) * (cw + gap), y, cw, labels[i], i);
      if (i % cols === cols - 1) y += 96 + 48;
    }
  }

  // ── Catálogo ───────────────────────────────────────────────────────────────────────────
  // Cada pantalla: { bar: opciones de la barra superior | false (sin barra), draw(c, contenido,
  // pantalla) → punto de salida para la flecha o null, link: texto de la flecha a la siguiente }
  const CANON = 'Layouts canónicos', NAV = 'Navegación', BARS = 'Barras de app', CONT = 'Contenido',
    INPUT = 'Entrada de datos', OVER = 'Superposiciones';
  const PATRONES = [
    { cat: CANON, id: 'lista-detalle', name: 'Lista + detalle con búsqueda',
      compact: [
        { draw: (c, r) => listPane(c, r, { chevron: true }), link: 'Al pulsar' },
        { bar: { title: 'Detalle', nav: 'back', actions: ['more'] }, draw: detailPane },
      ],
      wide: [{ draw: (c, r) => split(c, r, 0.36, listPane, detailPane) }] },
    { cat: CANON, id: 'panel-apoyo', name: 'Panel de apoyo',
      compact: [
        { draw: (c, r) => { articlePane(c, r, { bottom: 120 }); const y = r.y + r.h - PAD - 80; button(c, r.x + PAD, y, r.w - 2 * PAD, 80, 'Ver relacionados', 'tonal'); return [r.x + r.w, y + 40]; }, link: 'Al pulsar' },
        { draw: (c, r, s) => { articlePane(c, r); scrim(c, s); const y = s.y + Math.round(s.h * 0.42); sheet(c, s.x, y, s.w, s.y + s.h - y, [56, 56, 0, 0]); bar(c, s.x + s.w / 2 - 32, y + 20, 64, 8, WF.mid); relatedPane(c, { x: s.x, y: y + 24, w: s.w, h: s.y + s.h - y - 24 }); } },
      ],
      wide: [{ draw: (c, r) => split(c, r, 0.64, articlePane, relatedPane, { bg2: WF.surface }) }] },
    { cat: CANON, id: 'feed', name: 'Feed',
      compact: [{ draw: (c, r) => grid(c, r, 1, 420, { imgRatio: 0.6 }) }],
      wide: [{ draw: (c, r) => grid(c, r, 3, 400, { imgRatio: 0.55 }) }] },

    { cat: NAV, id: 'barra-riel', name: 'Barra de navegación ↔ riel',
      compact: [{ draw: (c, r) => { grid(c, { ...r, h: r.h - 160 }, 1, 400); const h = navBar(c, r); fab(c, r.x + r.w - PAD - 112, r.y + r.h - h - PAD - 112); } }],
      wide: [{ draw: (c, r) => { const w = rail(c, r); grid(c, { x: r.x + w, y: r.y, w: r.w - w, h: r.h }, 3, 360); } }] },
    { cat: NAV, id: 'cajon', name: 'Cajón de navegación',
      compact: [
        { bar: { nav: 'menu', actions: ['search'] }, draw: (c, r) => { rows(c, r.x, r.y + 8, r.w, r.y + r.h); return [r.x + r.w, r.y - 50]; }, link: 'Menú' },
        { bar: { nav: 'menu', actions: ['search'] }, draw: (c, r, s) => { rows(c, r.x, r.y + 8, r.w, r.y + r.h); scrim(c, s); drawer(c, s.x, s.y, Math.round(s.w * 0.82), s.h, true); } },
      ],
      wide: [{ bar: { actions: ['search'] }, draw: (c, r) => { drawer(c, r.x, r.y, 480, r.h, false); grid(c, { x: r.x + 480, y: r.y, w: r.w - 480, h: r.h }, 2, 340); } }] },
    { cat: NAV, id: 'pestanas', name: 'Pestañas',
      compact: [{ draw: (c, r) => { const y = tabs(c, r.x, r.y, r.w, ['Todos', 'Pendientes', 'Hechos']); rows(c, r.x, y + 8, r.w, r.y + r.h); } }],
      wide: [{ draw: (c, r) => { const y = tabs(c, r.x, r.y, r.w, ['Todos', 'Pendientes', 'Hechos', 'Archivados']); grid(c, { x: r.x, y, w: r.w, h: r.y + r.h - y }, 3, 300); } }] },

    { cat: BARS, id: 'barra-grande', name: 'Barra superior grande',
      compact: [{ bar: { nav: 'back', title: '', actions: ['more', 'search'] }, draw: (c, r) => { txt(c, 'Pedidos', r.x + 40, r.y + 72, { size: 60, weight: 700 }); rows(c, r.x, r.y + 150, r.w, r.y + r.h); } }],
      wide: [{ bar: { nav: 'back', title: '', actions: ['more', 'search'] }, draw: (c, r) => { txt(c, 'Pedidos', r.x + 40, r.y + 72, { size: 60, weight: 700 }); grid(c, { x: r.x, y: r.y + 130, w: r.w, h: r.h - 130 }, 2, 220, { img: false, top: 0 }); } }] },
    { cat: BARS, id: 'barra-inferior', name: 'Barra inferior con FAB',
      compact: [{ draw: (c, r) => { rows(c, r.x, r.y + 8, r.w, r.y + r.h - 160); bottomAppBar(c, r); } }],
      wide: [{ draw: (c, r) => { grid(c, { ...r, h: r.h - 160 }, 3, 300); bottomAppBar(c, r); } }] },

    { cat: CONT, id: 'cuadricula', name: 'Cuadrícula de tarjetas',
      compact: [{ draw: (c, r) => grid(c, r, 2, 380, { imgRatio: 0.62 }) }],
      wide: [{ draw: (c, r) => grid(c, r, 4, 360, { imgRatio: 0.62 }) }] },
    { cat: CONT, id: 'carrusel', name: 'Carrusel',
      compact: [{ draw: (c, r) => {
        txt(c, 'Destacados', r.x + PAD, r.y + 52, { size: 26 });
        const y = carousel(c, r.x + PAD, r.y + 92, r.w - PAD, 400, [0.62, 0.26, 0.12]);
        txt(c, 'Recientes', r.x + PAD, y + 56, { size: 26 }); rows(c, r.x, y + 92, r.w, r.y + r.h);
      } }],
      wide: [{ draw: (c, r) => {
        txt(c, 'Destacados', r.x + PAD, r.y + 52, { size: 26 });
        const y = carousel(c, r.x + PAD, r.y + 92, r.w - PAD, 300, [0.36, 0.24, 0.24, 0.16]);
        grid(c, { x: r.x, y: y + 32, w: r.w, h: r.y + r.h - y - 32 }, 4, 240, { top: 0, imgRatio: 0.5 });
      } }] },
    { cat: CONT, id: 'detalle-imagen', name: 'Detalle con imagen',
      compact: [{ bar: { nav: 'back', title: '', actions: ['more'] }, draw: (c, r) => { const ih = Math.round(r.h * 0.34); ph(c, r.x, r.y, r.w, ih, 0); heroContent(c, r.x + 40, r.y + ih + 40, r.w - 80, r.y + r.h - 40); } }],
      wide: [{ bar: { nav: 'back', title: '', actions: ['more'] }, draw: (c, r) => { const iw = snap(r.w * 0.46); ph(c, r.x + PAD, r.y + PAD, iw - PAD, r.h - 2 * PAD, 24); heroContent(c, r.x + iw + 48, r.y + 48, r.w - iw - 96, r.y + r.h - 40); } }] },
    { cat: CONT, id: 'ajustes', name: 'Ajustes',
      compact: [{ bar: { nav: 'back', title: 'Ajustes' }, draw: settingsList }],
      wide: [{ bar: { title: 'Ajustes' }, draw: (c, r) => split(c, r, 0.34, (c2, a) => rows(c2, a.x, a.y + 16, a.w, a.y + a.h, { lead: 'icon', oneLine: true, sel: 0, divider: false }), settingsList) }] },

    { cat: CONT, id: 'tema', name: 'Tema: claro, oscuro, automático',
      compact: [
        { bar: { nav: 'back', title: 'Ajustes' }, draw: themeSettings, link: 'Al pulsar' },
        { bar: { nav: 'back', title: 'Ajustes' }, draw: (c, r, s) => {
          themeSettings(c, r); scrim(c, s);
          const w = s.w - 96, h = 560, x = s.x + 48, y = s.y + (s.h - h) / 2;
          sheet(c, x, y, w, h, 56); txt(c, 'Tema', x + 48, y + 72, { size: 34, weight: 700 });
          ['Claro', 'Oscuro', 'Automático (según el sistema)'].forEach((l, k) => { radio(c, x + 76, y + 160 + k * 96, k === 2); txt(c, l, x + 124, y + 161 + k * 96, { size: 26, weight: 400 }); });
          button(c, x + w - 48 - 380, y + h - 48 - 80, 180, 80, 'Cancelar', 'text'); button(c, x + w - 48 - 180, y + h - 48 - 80, 180, 80, 'Aceptar', 'text');
        } },
      ],
      wide: [{ bar: { title: 'Ajustes' }, draw: (c, r) => split(c, r, 0.32, (c2, a) => { ['Apariencia', 'Notificaciones', 'Cuenta', 'Privacidad', 'Acerca de'].forEach((l, k) => { const y = a.y + 16 + k * 104; if (k === 0) { c2.fillStyle = WF.pill; rr(c2, a.x + 16, y + 8, a.w - 32, 88, 44); c2.fill(); } icon(c2, a.x + 64, y + 52, 'dot', k === 0 ? WF.dark : WF.mid); txt(c2, l, a.x + 104, y + 53, { size: 24, color: k === 0 ? WF.dark : WF.mid }); }); }, appearancePane) }] },

    { cat: INPUT, id: 'busqueda', name: 'Búsqueda',
      compact: [
        { bar: false, draw: (c, r) => { const y = searchBar(c, r.x + PAD, r.y + PAD + 20, r.w - 2 * PAD, { lead: 'menu' }); grid(c, { x: r.x, y: y + 16, w: r.w, h: r.y + r.h - y - 16 }, 2, 360, { top: 16 }); return [r.x + r.w, r.y + PAD + 68]; }, link: 'Al pulsar' },
        { bar: false, draw: (c, r) => {
          icon(c, r.x + 52, r.y + 70, 'back'); txt(c, 'pedidos', r.x + 96, r.y + 71, { size: 28, weight: 400 }); icon(c, r.x + r.w - 52, r.y + 70, 'close');
          divider(c, r.x, r.y + 138, r.w);
          const kh = Math.round(r.h * 0.3); rows(c, r.x, r.y + 148, r.w, r.y + r.h - kh, { lead: 'icon', icon: 'history', oneLine: true, h: 96, divider: false });
          keyboard(c, r.x, r.y + r.h - kh, r.w, kh);
        } },
      ],
      wide: [{ bar: false, draw: (c, r) => {
        const sw = snap(r.w * 0.5), y = searchBar(c, r.x + PAD, r.y + PAD + 20, sw, { lead: 'menu' });
        grid(c, { x: r.x, y: y + 16, w: r.w, h: r.y + r.h - y - 16 }, 3, 320, { top: 16 });
        sheet(c, r.x + PAD, y + 12, sw, 420, 28);
        rows(c, r.x + PAD, y + 28, sw, y + 420, { lead: 'icon', icon: 'history', oneLine: true, h: 96, divider: false });
      } }] },
    { cat: INPUT, id: 'formulario', name: 'Formulario',
      compact: [{ bar: { nav: 'close', title: 'Nuevo pedido', action: 'Guardar' }, draw: (c, r) => formFields(c, r, 1) }],
      wide: [{ bar: { nav: 'close', title: 'Nuevo pedido', action: 'Guardar' }, draw: (c, r) => formFields(c, r, 2) }] },
    { cat: INPUT, id: 'inicio-sesion', name: 'Inicio de sesión',
      compact: [{ bar: false, draw: (c, r) => loginForm(c, r.x + 56, r.y, r.w - 112, r.h) }],
      wide: [{ bar: false, draw: (c, r) => { const iw = snap(r.w * 0.5); ph(c, r.x, r.y, iw, r.h, 0); loginForm(c, r.x + iw + 120, r.y, r.w - iw - 240, r.h); } }] },

    { cat: OVER, id: 'dialogo', name: 'Diálogo',
      compact: [
        { draw: (c, r) => { rows(c, r.x, r.y + 8, r.w, r.y + r.h); const [, y] = fab(c, r.x + r.w - PAD - 112, r.y + r.h - PAD - 112); return [r.x + r.w, y]; }, link: 'Al pulsar +' },
        { bar: { nav: 'close', title: 'Nuevo elemento', action: 'Guardar' }, draw: (c, r) => formFields(c, r, 1) },
      ],
      wide: [{ draw: (c, r, s) => {
        rows(c, r.x, r.y + 8, r.w, r.y + r.h); scrim(c, s);
        const w = 800, h = 440, x = s.x + (s.w - w) / 2, y = s.y + (s.h - h) / 2;
        sheet(c, x, y, w, h, 56); txt(c, '¿Descartar el borrador?', x + 56, y + 86, { size: 34, weight: 700 });
        textLines(c, x + 56, y + 150, w - 112, y + 260);
        button(c, x + w - 56 - 380, y + h - 56 - 80, 180, 80, 'Cancelar', 'text'); button(c, x + w - 56 - 180, y + h - 56 - 80, 180, 80, 'Descartar', 'text');
      } }] },
    { cat: OVER, id: 'hojas', name: 'Hoja inferior ↔ lateral',
      compact: [{ draw: (c, r, s) => {
        grid(c, r, 1, 400); scrim(c, s);
        const y = s.y + Math.round(s.h * 0.5); sheet(c, s.x, y, s.w, s.y + s.h - y, [56, 56, 0, 0]); bar(c, s.x + s.w / 2 - 32, y + 20, 64, 8, WF.mid);
        rows(c, s.x, y + 56, s.w, s.y + s.h - 16, { lead: 'icon', oneLine: true, divider: false });
      } }],
      wide: [{ draw: (c, r) => {
        const sw = 560, x = r.x + r.w - sw; grid(c, { ...r, w: r.w - sw }, 2, 300);
        c.fillStyle = WF.surface; c.fillRect(x, r.y, sw, r.h); vdivider(c, x, r.y, r.h);
        txt(c, 'Filtros', x + 48, r.y + 60, { size: 30, weight: 700 }); icon(c, x + sw - 56, r.y + 60, 'close');
        let y = chips(c, x + 48, r.y + 120, ['Hoy', 'Semana', 'Mes']) + 40;
        y = field(c, x + 48, y, sw - 96, 'Cliente', 1) + 40; field(c, x + 48, y, sw - 96, 'Estado', 3);
        const bw = (sw - 96 - 24) / 2;
        button(c, x + 48, r.y + r.h - 48 - 80, bw, 80, 'Limpiar', 'outlined'); button(c, x + 48 + bw + 24, r.y + r.h - 48 - 80, bw, 80, 'Aplicar');
      } }] },
  ];

  window.PizarraPatrones = { list: PATRONES, appBar };
})();
