// Convierte un patrón (patterns.js) en objetos de la pizarra «dibujados a mano».
// Los patrones se dibujan sobre un lienzo de mentira (Recorder) que, en vez de pintar, apunta
// rectángulos, líneas, círculos y textos como objetos editables: las barras finas (texto de
// relleno) pasan a ser líneas, los círculos a trazos de lápiz y los rellenos a contornos.
// Las capas opacas (hojas, diálogos, barras de navegación) tapan lo que había debajo.
(() => {
  const HAND = "'Segoe Print', 'Bradley Hand', 'Comic Sans MS', cursive";
  const INK = { dark: '#334155', mid: '#64748b', light: '#94a3b8', blue: '#0284c7' };

  // Color del rotulador según el gris del patrón (null = no se dibuja: blancos y velos)
  function ink(col, forText) {
    if (typeof col !== 'string' || col.startsWith('rgba')) return null;
    const h = col.replace('#', '');
    if (h.length !== 6) return INK.dark;
    const r = parseInt(h.slice(0, 2), 16), g = parseInt(h.slice(2, 4), 16), b = parseInt(h.slice(4, 6), 16);
    const L = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
    if (L > 0.98) return forText ? INK.dark : null;
    if (b - r > 40) return INK.blue;
    return L < 0.5 ? INK.dark : L < 0.7 ? INK.mid : INK.light;
  }
  function boxOf(s) {
    if (s.type === 'pen') { const xs = s.pts.map(p => p[0]), ys = s.pts.map(p => p[1]); return [Math.min(...xs), Math.min(...ys), Math.max(...xs), Math.max(...ys)]; }
    if (s.type === 'text') return [s.x, s.y, s.x + s.w, s.y + s.h];
    return [Math.min(s.x1, s.x2), Math.min(s.y1, s.y2), Math.max(s.x1, s.x2), Math.max(s.y1, s.y2)];
  }
  const inter = (a, b) => { const x = Math.max(a.x, b.x), y = Math.max(a.y, b.y); return { x, y, w: Math.min(a.x + a.w, b.x + b.w) - x, h: Math.min(a.y + a.h, b.y + b.h) - y }; };
  // Tramo [t0, t1] de un segmento que cae dentro de un rectángulo (Liang–Barsky), o null
  function clipT(x1, y1, x2, y2, r) {
    let t0 = 0, t1 = 1; const dx = x2 - x1, dy = y2 - y1;
    for (const [p, q] of [[-dx, x1 - r.x], [dx, r.x + r.w - x1], [-dy, y1 - r.y], [dy, r.y + r.h - y1]]) {
      if (p === 0) { if (q < 0) return null; continue; }
      const t = q / p; if (p < 0) { if (t > t1) return null; if (t > t0) t0 = t; } else { if (t < t0) return null; if (t < t1) t1 = t; }
    }
    return t1 > t0 ? [t0, t1] : null;
  }
  function clipSeg(x1, y1, x2, y2, r) {
    const t = clipT(x1, y1, x2, y2, r); if (!t) return null; const dx = x2 - x1, dy = y2 - y1;
    return [x1 + t[0] * dx, y1 + t[0] * dy, x1 + t[1] * dx, y1 + t[1] * dy];
  }

  class Recorder {
    constructor(clip, rnd) {
      this.out = []; this.stack = []; this.path = []; this.cur = null; this.clipR = clip; this.rnd = rnd;
      this.groups = []; this.gid = 0; this.gbase = 's' + Date.now().toString(36) + Math.floor(rnd() * 1e6).toString(36);
      this.fillStyle = '#000'; this.strokeStyle = '#000'; this.lineWidth = 1; this.font = '10px sans-serif';
      this.textAlign = 'left'; this.textBaseline = 'alphabetic';
      this.m = document.createElement('canvas').getContext('2d');
    }
    // Subgrupos: las piezas de dibujo (patterns.js, pieces.js) abren y cierran un grupo por pieza
    beginGroup() { this.groups.push(this.gbase + '-' + (++this.gid)); }
    endGroup() { this.groups.pop(); }
    save() { this.stack.push({ fillStyle: this.fillStyle, strokeStyle: this.strokeStyle, lineWidth: this.lineWidth, font: this.font, textAlign: this.textAlign, textBaseline: this.textBaseline, clipR: this.clipR }); }
    restore() { const s = this.stack.pop(); if (s) Object.assign(this, s); }
    beginPath() { this.path = []; this.cur = null; }
    moveTo(x, y) { this.cur = [[x, y]]; this.path.push({ k: 'poly', pts: this.cur }); }
    lineTo(x, y) { if (!this.cur) this.moveTo(x, y); else this.cur.push([x, y]); }
    closePath() { if (this.cur && this.cur.length) this.cur.push([...this.cur[0]]); }
    rect(x, y, w, h) { this.path.push({ k: 'rect', x, y, w, h, r: 0 }); this.cur = null; }
    roundRect(x, y, w, h, r) { this.path.push({ k: 'rect', x, y, w, h, r: Math.max(...[].concat(r || 0)) }); this.cur = null; }
    arc(cx, cy, r) { this.path.push({ k: 'circle', cx, cy, r }); this.cur = null; }
    clip() { const p = this.path.find(q => q.k === 'rect'); if (p) this.clipR = inter(this.clipR, p); }
    setLineDash() {} quadraticCurveTo(x, y, x2, y2) { this.lineTo(x2, y2); } drawImage() {}
    measureText(t) { this.m.font = this.font; return this.m.measureText(t); }
    stroke() { for (const p of this.path) this.emit(p, false, this.strokeStyle, this.lineWidth); }
    fill() { for (const p of this.path) this.emit(p, true, this.fillStyle, 2); }
    fillRect(x, y, w, h) { this.emit({ k: 'rect', x, y, w, h, r: 0 }, true, this.fillStyle, 2); }
    strokeRect(x, y, w, h) { this.emit({ k: 'rect', x, y, w, h, r: 0 }, false, this.strokeStyle, this.lineWidth); }
    fillText(text, x, y) {
      const color = ink(this.fillStyle, true); if (!color) return;
      const px = +(/(\d+(?:\.\d+)?)px/.exec(this.font) || [0, 22])[1];
      this.m.font = `${px}px ${HAND}`; const w = this.m.measureText(text).width;
      if (this.textAlign === 'center') x -= w / 2; else if (this.textAlign === 'right' || this.textAlign === 'end') x -= w;
      if (this.textBaseline === 'middle') y -= px * 0.62; else if (this.textBaseline !== 'top') y -= px;
      const c = this.clipR; if (x + w / 2 < c.x || x + w / 2 > c.x + c.w || y + px / 2 < c.y || y + px / 2 > c.y + c.h) return;
      this.push({ type: 'text', text, x, y, color, size: 2, k: px / 22, w, h: px });
    }
    // Opaco grande: lo que queda debajo deja de verse. Las líneas se cortan en el borde; el resto
    // desaparece si queda tapado en más de la mitad (o su centro, si es pequeño).
    occlude(r) {
      const next = [];
      for (const s of this.out) {
        if (s.type === 'line') {
          const t = clipT(s.x1, s.y1, s.x2, s.y2, r);
          if (!t) { next.push(s); continue; }
          const at = (k) => [s.x1 + (s.x2 - s.x1) * k, s.y1 + (s.y2 - s.y1) * k];
          for (const [a, b] of [[0, t[0]], [t[1], 1]]) {
            const p1 = at(a), p2 = at(b);
            if (Math.hypot(p2[0] - p1[0], p2[1] - p1[1]) > 6) next.push({ ...s, x1: p1[0], y1: p1[1], x2: p2[0], y2: p2[1] });
          }
          continue;
        }
        const b = boxOf(s), w = b[2] - b[0], h = b[3] - b[1];
        const ov = Math.max(0, Math.min(b[2], r.x + r.w) - Math.max(b[0], r.x)) * Math.max(0, Math.min(b[3], r.y + r.h) - Math.max(b[1], r.y));
        const cx = (b[0] + b[2]) / 2, cy = (b[1] + b[3]) / 2, centre = cx > r.x && cx < r.x + r.w && cy > r.y && cy < r.y + r.h;
        if (w * h > 0 ? ov / (w * h) > 0.5 : centre) continue;
        next.push(s);
      }
      this.out = next;
    }
    emit(p, filled, col, lw) {
      const white = typeof col === 'string' && /^#fff(fff)?$/i.test(col);
      let color = ink(col);
      if (!color && white && (p.k === 'circle' || (p.k === 'rect' && p.h >= 40))) color = INK.light; // teclas, tiradores
      if (!color) return;
      const size = Math.max(2, Math.min(6, Math.round(lw)));
      if (p.k === 'rect') {
        if (filled && p.w * p.h > 40000 && !white) this.occlude(p);
        // Barra fina = línea (texto de relleno, divisores)
        if (filled && p.h <= 34 && p.w > 2.2 * p.h) { const t = Math.max(2, Math.min(9, p.h * 0.5)); return this.line(p.x + p.h / 2, p.y + p.h / 2, p.x + p.w - p.h / 2, p.y + p.h / 2, color, Math.round(t)); }
        if (filled && p.w <= 8 && p.h > 3 * p.w) return this.line(p.x + p.w / 2, p.y, p.x + p.w / 2, p.y + p.h, color, Math.max(2, Math.round(p.w)));
        const r = inter(this.clipR, p); if (r.w < 4 || r.h < 4) return;
        const sh = { type: 'rect', x1: r.x, y1: r.y, x2: r.x + r.w, y2: r.y + r.h, r: Math.min(p.r || 0, r.w / 2, r.h / 2), color, size };
        if (filled && p.w * p.h > 40000 && !white) sh.fill = '#ffffff'; // superficie opaca: tapa lo de debajo
        this.push(sh);
      } else if (p.k === 'circle') {
        const c = this.clipR; if (p.cx < c.x || p.cx > c.x + c.w || p.cy < c.y || p.cy > c.y + c.h) return;
        if (filled && p.r <= 6) return this.push({ type: 'pen', pts: [[p.cx, p.cy]], color, size: Math.round(p.r * 2) });
        // Círculo a mano: un poco irregular y sin cerrar del todo
        const n = Math.max(12, Math.min(32, Math.round(p.r / 2))), a0 = this.rnd() * Math.PI * 2, pts = [];
        for (let i = 0; i <= n + 1; i++) { const a = a0 + (i / n) * Math.PI * 2 * 1.04, rr = p.r * (1 + (this.rnd() - 0.5) * 0.06); pts.push([Math.round((p.cx + rr * Math.cos(a)) * 10) / 10, Math.round((p.cy + rr * Math.sin(a)) * 10) / 10]); }
        this.push({ type: 'pen', pts, color, size });
      } else if (p.k === 'poly' && p.pts.length >= 2) {
        if (p.pts.length === 2) return this.line(p.pts[0][0], p.pts[0][1], p.pts[1][0], p.pts[1][1], color, size);
        const c = this.clipR, pts = p.pts.filter(q => q[0] >= c.x && q[0] <= c.x + c.w && q[1] >= c.y && q[1] <= c.y + c.h);
        if (pts.length >= 2) this.push({ type: 'pen', pts: pts.map(q => [q[0], q[1]]), color, size });
      }
    }
    line(x1, y1, x2, y2, color, size) {
      const s = clipSeg(x1, y1, x2, y2, this.clipR); if (!s) return;
      if (Math.hypot(s[2] - s[0], s[3] - s[1]) < 2) return;
      this.push({ type: 'line', x1: s[0], y1: s[1], x2: s[2], y2: s[3], color, size });
    }
    push(s) { if (this.groups.length) s.gs = this.groups.slice(); this.out.push(s); }
    // Quita duplicados (relleno + borde de la misma figura): se queda con el último
    done() {
      const seen = new Map();
      for (const s of this.out) {
        const b = (s.type === 'line' ? [s.x1, s.y1, s.x2, s.y2] : boxOf(s)).map(v => Math.round(v / 4));
        const key = s.type === 'text' ? 't' + b.join(',') + s.text : s.type + b.join(','), prev = seen.get(key);
        if (prev && prev.fill && !s.fill) s.fill = prev.fill;
        seen.set(key, s);
      }
      const keep = new Set(seen.values()), out = this.out.filter(s => keep.has(s));
      // Un grupo con un solo objeto no aporta nada: se quita ese nivel
      const count = {}; for (const s of out) for (const g of s.gs || []) count[g] = (count[g] || 0) + 1;
      return out.map(s => {
        if (s.type === 'text') { delete s.w; delete s.h; }
        if (s.gs) { s.gs = s.gs.filter(g => count[g] > 1); if (!s.gs.length) delete s.gs; }
        return s;
      });
    }
  }

  // Genera los objetos de un patrón para la disposición dada (pantallas del tapete)
  function generate(screens, header) {
    const P = window.PizarraPatrones; if (!P) return [];
    let seed = 1; const rnd = () => { seed = (seed * 16807) % 2147483647; return (seed - 1) / 2147483646; };
    const out = [], anchors = [];
    for (const s of screens) {
      const full = { x: s.x, y: s.y, w: s.w, h: s.h }, rec = new Recorder(full, rnd), spec = s.spec;
      let r = full;
      if (screens.length > 1) rec.beginGroup();
      if (spec.bar !== false) { P.appBar(rec, { x: s.x, y: s.y, w: s.w, h: header }, spec.bar || {}); r = { x: s.x, y: s.y + header, w: s.w, h: s.h - header }; }
      anchors.push((spec.draw && spec.draw(rec, r, full)) || null);
      if (screens.length > 1) rec.endGroup();
      out.push(...rec.done());
    }
    // Flechas entre pantallas, con su texto
    screens.forEach((s, i) => {
      if (!i) return; const prev = screens[i - 1], a = anchors[i - 1] || [prev.x + prev.w, prev.y + prev.h / 2];
      const lg = ['enlace' + i + '-' + Math.floor(rnd() * 1e9).toString(36)];
      out.push({ type: 'arrow', x1: a[0] + 14, y1: a[1], x2: s.x - 10, y2: a[1], color: INK.dark, size: 3, gs: lg });
      const label = prev.spec.link || '';
      if (label) { const px = 22, m = document.createElement('canvas').getContext('2d'); m.font = `${px}px ${HAND}`; const w = m.measureText(label).width; out.push({ type: 'text', text: label, x: (a[0] + s.x) / 2 - w / 2, y: a[1] - 44, color: INK.dark, size: 2, k: 1, gs: lg }); }
    });
    return out.map(sh => Object.assign(sh, { hand: true, seed: Math.floor(rnd() * 1e9) }));
  }

  // Un patrón simple (pieza) dibujado en el rectángulo r, recortado al dispositivo
  function generatePiece(piece, r, mode, clip) {
    let seed = 7; const rnd = () => { seed = (seed * 16807) % 2147483647; return (seed - 1) / 2147483646; };
    const rec = new Recorder(clip, rnd); piece.draw(rec, r, mode);
    return rec.done().map(sh => Object.assign(sh, { hand: true, seed: Math.floor(rnd() * 1e9) }));
  }

  window.PizarraBoceto = { generate, generatePiece };
})();
