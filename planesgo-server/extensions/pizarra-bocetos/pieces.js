// Patrones simples: componentes de Material Design 3 y campos de datos. Cada pieza se incrusta en
// el dispositivo como un grupo de objetos a mano (sketch.js), en su sitio (barras arriba o abajo,
// rieles y cajones a un lado, FAB abajo a la derecha) o en el centro para moverla.
// size(dev, mode) → [ancho, alto] en unidades (1 dp = 2); place: dónde se coloca; draw(c, r, mode).
(() => {
  const K = window.PizarraPatrones.kit;
  const { WF, PAD, rr, bar, disc, txt, icon, button, fab, toggle, chip, field, divider, vdivider, wv } = K;

  // ── Piezas auxiliares ──────────────────────────────────────────────────────────────────
  function check(c, x, y, state) { // casilla: 'on' | 'off' | 'mixed'
    rr(c, x, y, 36, 36, 6);
    if (state === 'off') { c.strokeStyle = WF.mid; c.lineWidth = 3; c.stroke(); return; }
    c.fillStyle = WF.dark; c.fill(); c.strokeStyle = '#ffffff'; c.lineWidth = 4; c.beginPath();
    if (state === 'on') { c.moveTo(x + 8, y + 18); c.lineTo(x + 15, y + 26); c.lineTo(x + 28, y + 10); } else { c.moveTo(x + 9, y + 18); c.lineTo(x + 27, y + 18); }
    c.stroke();
  }
  function iconBtn(c, cx, cy, kind, ico = 'dot') { // standard | filled | tonal | outlined
    c.beginPath(); c.arc(cx, cy, 40, 0, Math.PI * 2);
    if (kind === 'filled') { c.fillStyle = WF.dark; c.fill(); } else if (kind === 'tonal') { c.fillStyle = WF.tonal; c.fill(); } else if (kind === 'outlined') { c.strokeStyle = WF.mid; c.lineWidth = 2; c.stroke(); }
    icon(c, cx, cy, ico, kind === 'filled' ? '#ffffff' : WF.dark);
  }
  function surface(c, x, y, w, h, r = 24) { K.sheet(c, x, y, w, h, r); }
  // Campo de texto completo: etiqueta, valor, prefijo/sufijo, icono final y texto de ayuda
  function tf(c, x, y, w, o) {
    const h = o.h || 112;
    c.strokeStyle = o.error ? WF.dark : WF.mid; c.lineWidth = o.error ? 4 : 2; rr(c, x, y, w, h, 8); c.stroke();
    c.font = `600 22px ${WF.font}`; const lw = c.measureText(o.label).width + 16;
    c.fillStyle = '#ffffff'; c.fillRect(x + 20, y - 13, lw, 26);
    txt(c, o.label, x + 28, y + 1, { size: 22, color: o.error ? WF.dark : WF.mid });
    let tx = x + 32;
    if (o.lead) { icon(c, x + 48, y + h / 2, o.lead); tx = x + 88; }
    if (o.prefix) { txt(c, o.prefix, tx, y + h / 2 + 1, { size: 28, weight: 400, color: WF.mid }); c.font = `400 28px ${WF.font}`; tx += c.measureText(o.prefix).width + 10; }
    let right = x + w - 32;
    if (o.trail) { if (o.trail === 'stepper') { icon(c, right - 70, y + h / 2, 'minus'); icon(c, right - 14, y + h / 2, 'plus'); vdivider(c, right - 42, y + 30, h - 60); right -= 110; } else { icon(c, right - 14, y + h / 2, o.trail); right -= 56; } }
    if (o.suffix) { c.font = `400 28px ${WF.font}`; const sw = c.measureText(o.suffix).width; txt(c, o.suffix, right - sw, y + h / 2 + 1, { size: 28, weight: 400, color: WF.mid }); right -= sw + 12; }
    if (o.multiline) { (o.value || '').split('\n').forEach((ln, i) => txt(c, ln, tx, y + 40 + i * 38, { size: 28, weight: 400 })); }
    else if (o.value != null) txt(c, o.value, o.alignRight ? right : tx, y + h / 2 + 1, { size: 28, weight: 400, align: o.alignRight ? 'right' : 'left' });
    if (o.help) txt(c, o.help, x + 32, y + h + 30, { size: 22, weight: 400, color: o.error ? WF.dark : WF.mid });
    if (o.counter) txt(c, o.counter, x + w - 32, y + h + 30, { size: 22, weight: 400, color: WF.mid, align: 'right' });
    return y + h + (o.help || o.counter ? 48 : 0);
  }
  // Fila clave–valor de solo lectura (valores numéricos alineados a la derecha)
  function kv(c, x, y, w, k, v, right) {
    txt(c, k, x + PAD, y + 44, { size: 24, weight: 400, color: WF.mid });
    txt(c, v, right ? x + w - PAD : x + w * 0.45, y + 44, { size: 26, color: WF.dark, align: right ? 'right' : 'left' });
    divider(c, x + PAD, y + 87, w - 2 * PAD);
    return y + 88;
  }
  function calendarGrid(c, x, y, w, h, sel) {
    const cw = w / 7, ch = h / 6;
    'LMXJVSD'.split('').forEach((d, i) => txt(c, d, x + cw * (i + 0.5), y + ch / 2, { size: 22, align: 'center', color: WF.mid }));
    for (let i = 0; i < 31; i++) {
      const col = (i + 2) % 7, row = Math.floor((i + 2) / 7) + 1, cx = x + cw * (col + 0.5), cy = y + ch * (row + 0.5);
      if (i + 1 === sel) disc(c, cx, cy, Math.min(cw, ch) * 0.42, WF.dark);
      txt(c, String(i + 1), cx, cy + 1, { size: 22, align: 'center', weight: 400, color: i + 1 === sel ? '#ffffff' : WF.dark });
    }
  }
  function clockFace(c, cx, cy, r) {
    disc(c, cx, cy, r, WF.tonal);
    for (let h = 1; h <= 12; h++) { const a = (h / 12) * Math.PI * 2 - Math.PI / 2; txt(c, String(h === 12 ? 0 : h * 1), cx + Math.cos(a) * r * 0.8, cy + Math.sin(a) * r * 0.8 + 1, { size: 22, align: 'center', weight: 400 }); }
    const a = (2 / 12) * Math.PI * 2 - Math.PI / 2;
    c.strokeStyle = WF.dark; c.lineWidth = 3; c.beginPath(); c.moveTo(cx, cy); c.lineTo(cx + Math.cos(a) * r * 0.8, cy + Math.sin(a) * r * 0.8); c.stroke();
    disc(c, cx, cy, 6, WF.dark); c.strokeStyle = WF.dark; c.beginPath(); c.arc(cx + Math.cos(a) * r * 0.8, cy + Math.sin(a) * r * 0.8, 24, 0, Math.PI * 2); c.stroke();
  }
  const dialogBox0 = (c, r, title) => { surface(c, r.x, r.y, r.w, r.h, 56); if (title) txt(c, title, r.x + 48, r.y + 64, { size: 30, weight: 700 }); };
  const actions0 = (c, r, labels) => { let x = r.x + r.w - 40; for (const l of labels.slice().reverse()) { const w = l.length * 14 + 48; button(c, x - w, r.y + r.h - 40 - 80, w, 80, l, 'text'); x -= w + 8; } };

  // Cada pieza auxiliar, en su propio subgrupo al pasar a objetos (ver patterns.js)
  const grouped = (fn) => function (c, ...args) { if (c.beginGroup) c.beginGroup(); try { return fn(c, ...args); } finally { if (c.endGroup) c.endGroup(); } };
  check = grouped(check); iconBtn = grouped(iconBtn); tf = grouped(tf); kv = grouped(kv); calendarGrid = grouped(calendarGrid); clockFace = grouped(clockFace);
  const actions = grouped(actions0), dialogBox = grouped(dialogBox0);

  // ── Catálogo de componentes (Material Design 3) ────────────────────────────────────────
  const A = 'Acciones', CO = 'Comunicación', CT = 'Contención', NV = 'Navegación', SE = 'Selección', TI = 'Entrada de texto';
  const COMPONENTES = [
    { cat: CT, id: 'app-bars', name: 'Barra superior (app bar)', place: 'top', size: (d) => [d.w, 128],
      draw: (c, r) => K.appBar(c, r, { nav: 'back', title: 'Título', actions: ['more', 'search'] }) },
    { cat: CO, id: 'badges', name: 'Insignias (badges)', size: () => [300, 120],
      draw: (c, r) => { icon(c, r.x + 70, r.y + 64, 'dot', WF.dark); disc(c, r.x + 86, r.y + 46, 8, WF.dark); icon(c, r.x + 200, r.y + 64, 'dot', WF.dark); rr(c, r.x + 204, r.y + 30, 48, 32, 16); c.fillStyle = WF.dark; c.fill(); txt(c, '12', r.x + 228, r.y + 47, { size: 20, align: 'center', color: '#ffffff' }); } },
    { cat: A, id: 'button-groups', name: 'Grupo de botones', size: () => [560, 112],
      draw: (c, r) => { const w = (r.w - 16) / 3; ['Día', 'Semana', 'Mes'].forEach((l, k) => button(c, r.x + k * (w + 8), r.y + 16, w, 80, l, k === 1 ? 'filled' : 'tonal')); } },
    { cat: A, id: 'buttons', name: 'Botones', size: () => [380, 520],
      draw: (c, r) => [['Elevado', 'outlined'], ['Relleno', 'filled'], ['Tonal', 'tonal'], ['Con borde', 'outlined'], ['Texto', 'text']].forEach(([l, k], i) => button(c, r.x, r.y + i * 104, r.w, 80, l, k)) },
    { cat: A, id: 'extended-fab', name: 'FAB extendido', place: 'fab', size: () => [320, 112], draw: (c, r) => fab(c, r.x, r.y, { label: 'Nuevo' }) },
    { cat: A, id: 'fab-menu', name: 'Menú del FAB', place: 'fab', size: () => [420, 560],
      draw: (c, r) => { ['Documento', 'Foto', 'Carpeta'].forEach((l, k) => { const w = 300, y = r.y + k * 112; c.fillStyle = WF.pill; rr(c, r.x + r.w - w, y, w, 96, 48); c.fill(); icon(c, r.x + r.w - w + 52, y + 48, 'dot', WF.dark); txt(c, l, r.x + r.w - w + 92, y + 49); }); disc(c, r.x + r.w - 56, r.y + r.h - 56, 56, WF.dark); icon(c, r.x + r.w - 56, r.y + r.h - 56, 'close', '#ffffff'); } },
    { cat: A, id: 'floating-action-button', name: 'Botón de acción flotante (FAB)', place: 'fab', size: () => [112, 112], draw: (c, r) => fab(c, r.x, r.y) },
    { cat: A, id: 'icon-buttons', name: 'Botones de icono', size: () => [420, 112],
      draw: (c, r) => ['standard', 'filled', 'tonal', 'outlined'].forEach((k, i) => iconBtn(c, r.x + 52 + i * 104, r.y + 56, k)) },
    { cat: A, id: 'segmented-buttons', name: 'Botones segmentados', size: (d) => [Math.min(d.w - 64, 640), 80], draw: (c, r) => K.segmented(c, r.x, r.y, r.w, ['Día', 'Semana', 'Mes'], 1) },
    { cat: A, id: 'split-button', name: 'Botón dividido', size: () => [380, 80],
      draw: (c, r) => { c.fillStyle = WF.dark; rr(c, r.x, r.y, r.w - 96, 80, [40, 8, 8, 40]); c.fill(); txt(c, 'Guardar', r.x + (r.w - 96) / 2, r.y + 41, { align: 'center', color: '#ffffff' }); rr(c, r.x + r.w - 88, r.y, 88, 80, [8, 40, 40, 8]); c.fill(); icon(c, r.x + r.w - 44, r.y + 40, 'expand', '#ffffff'); } },
    { cat: CT, id: 'cards', name: 'Tarjeta', size: (d) => [Math.min(d.w - 64, 640), 560],
      draw: (c, r) => { K.card(c, r.x, r.y, r.w, r.h - 112, 0, { imgRatio: 0.55 }); button(c, r.x + r.w - 380, r.y + r.h - 96, 170, 80, 'Compartir', 'outlined'); button(c, r.x + r.w - 190, r.y + r.h - 96, 170, 80, 'Abrir'); } },
    { cat: CT, id: 'carousel', name: 'Carrusel', size: (d) => [d.w - 64, 380], draw: (c, r) => K.carousel(c, r.x, r.y, r.w, r.h, [0.6, 0.27, 0.13]) },
    { cat: SE, id: 'checkbox', name: 'Casillas de verificación', size: () => [460, 280],
      draw: (c, r) => [['Todos', 'mixed'], ['Avisos por correo', 'on'], ['Avisos por SMS', 'off']].forEach(([l, st], i) => { check(c, r.x + (i ? 48 : 0), r.y + 20 + i * 88, st); txt(c, l, r.x + (i ? 108 : 60), r.y + 39 + i * 88, { size: 26, weight: 400 }); }) },
    { cat: SE, id: 'chips', name: 'Chips', size: (d) => [Math.min(d.w - 64, 700), 80],
      draw: (c, r) => { let x = r.x; x += chip(c, x, r.y, 'Ayuda') + 16; x += chip(c, x, r.y, '✓ Filtro', true) + 16; x += chip(c, x, r.y, 'Entrada ✕') + 16; chip(c, x, r.y, 'Sugerencia'); } },
    { cat: SE, id: 'date-pickers', name: 'Selector de fecha', size: (d, m) => m === 'expandido' ? [720, 860] : [Math.min(d.w - 64, 700), 1040],
      draw: (c, r, m) => {
        if (m === 'expandido') { // acoplado: campo + calendario desplegado
          tf(c, r.x, r.y + 16, r.w, { label: 'Fecha', value: '01/10/2026', trail: 'calendar' });
          surface(c, r.x, r.y + 152, r.w, r.h - 152); txt(c, 'Octubre 2026  ▾', r.x + 40, r.y + 212, { size: 26 }); icon(c, r.x + r.w - 120, r.y + 212, 'chevronLeft'); icon(c, r.x + r.w - 56, r.y + 212, 'chevron');
          calendarGrid(c, r.x + 24, r.y + 260, r.w - 48, 440, 1); actions(c, { ...r, y: r.y }, ['Cancelar', 'Aceptar']); return;
        }
        dialogBox(c, r); txt(c, 'Seleccionar fecha', r.x + 48, r.y + 60, { size: 22, color: WF.mid }); txt(c, 'jue, 1 oct', r.x + 48, r.y + 130, { size: 52, weight: 400 }); icon(c, r.x + r.w - 64, r.y + 130, 'edit');
        divider(c, r.x, r.y + 190, r.w); txt(c, 'Octubre 2026  ▾', r.x + 48, r.y + 244, { size: 24 }); icon(c, r.x + r.w - 128, r.y + 244, 'chevronLeft'); icon(c, r.x + r.w - 64, r.y + 244, 'chevron');
        calendarGrid(c, r.x + 24, r.y + 290, r.w - 48, 560, 1); actions(c, r, ['Cancelar', 'Aceptar']);
      } },
    { cat: SE, id: 'time-pickers', name: 'Selector de hora', size: (d, m) => m === 'expandido' ? [1040, 640] : [Math.min(d.w - 64, 660), 1080],
      draw: (c, r, m) => {
        dialogBox(c, r); txt(c, 'Seleccionar hora', r.x + 48, r.y + 60, { size: 22, color: WF.mid });
        const box = (x, y, t, on) => { c.fillStyle = on ? WF.pill : WF.tonal; rr(c, x, y, 192, 160, 16); c.fill(); txt(c, t, x + 96, y + 82, { size: 64, weight: 400, align: 'center' }); };
        if (m === 'expandido') { box(r.x + 48, r.y + 110, '14', true); txt(c, ':', r.x + 266, r.y + 190, { size: 64, align: 'center' }); box(r.x + 288, r.y + 110, '30'); clockFace(c, r.x + r.w - 260, r.y + 280, 210); }
        else { box(r.x + (r.w - 432) / 2, r.y + 110, '14', true); txt(c, ':', r.x + r.w / 2, r.y + 190, { size: 64, align: 'center' }); box(r.x + r.w / 2 + 24, r.y + 110, '30'); clockFace(c, r.x + r.w / 2, r.y + 560, Math.min(260, r.w / 2 - 60)); }
        icon(c, r.x + 64, r.y + r.h - 80, 'keyboard'); actions(c, r, ['Cancelar', 'Aceptar']);
      } },
    { cat: CO, id: 'dialogs', name: 'Diálogo', size: (d) => [Math.min(d.w - 96, 640), 440],
      draw: (c, r) => { dialogBox(c, r, '¿Descartar el borrador?'); K.textLines(c, r.x + 48, r.y + 130, r.w - 96, r.y + 230); actions(c, r, ['Cancelar', 'Descartar']); } },
    { cat: CT, id: 'divider', name: 'Divisor', size: (d) => [d.w - 64, 120],
      draw: (c, r) => { bar(c, r.x, r.y + 14, r.w * 0.5, 16, WF.mid); divider(c, r.x, r.y + 58, r.w); bar(c, r.x, r.y + 88, r.w * 0.4, 16, WF.mid); } },
    { cat: CT, id: 'lists', name: 'Lista', size: (d) => [d.w, 336], draw: (c, r) => K.rows(c, r.x, r.y, r.w, r.y + r.h, { trail: 'chevron' }) },
    { cat: CO, id: 'loading-indicator', name: 'Indicador de carga', size: () => [180, 180],
      draw: (c, r) => { const cx = r.x + 90, cy = r.y + 90; c.fillStyle = WF.tonal; c.beginPath(); c.arc(cx, cy, 88, 0, Math.PI * 2); c.fill(); c.strokeStyle = WF.dark; c.lineWidth = 4; c.beginPath(); for (let i = 0; i <= 40; i++) { const a = (i / 40) * Math.PI * 2, rr2 = 46 + 10 * Math.cos(a * 7); c[i ? 'lineTo' : 'moveTo'](cx + rr2 * Math.cos(a), cy + rr2 * Math.sin(a)); } c.stroke(); } },
    { cat: CO, id: 'progress-indicators', name: 'Indicadores de progreso', size: (d) => [Math.min(d.w - 64, 640), 220],
      draw: (c, r) => { bar(c, r.x, r.y + 20, r.w * 0.62, 12, WF.dark); bar(c, r.x + r.w * 0.62 + 12, r.y + 20, r.w * 0.38 - 12, 12, WF.line); c.strokeStyle = WF.line; c.lineWidth = 10; c.beginPath(); c.arc(r.x + 80, r.y + 140, 56, 0, Math.PI * 2); c.stroke(); c.strokeStyle = WF.dark; c.beginPath(); c.arc(r.x + 80, r.y + 140, 56, -Math.PI / 2, Math.PI * 0.7); c.stroke(); txt(c, '62 %', r.x + 180, r.y + 141, { size: 26 }); } },
    { cat: SE, id: 'menus', name: 'Menú', size: () => [460, 520],
      draw: (c, r) => { surface(c, r.x, r.y, r.w, r.h, 16); [['Cortar', 'Ctrl+X'], ['Copiar', 'Ctrl+C'], ['Pegar', 'Ctrl+V'], null, ['Ajustes', '']].forEach((it, i) => { const y = r.y + 16 + i * 98 - (i > 3 ? 70 : 0); if (!it) { divider(c, r.x, y + 12, r.w); return; } icon(c, r.x + 52, y + 48, 'dot'); txt(c, it[0], r.x + 96, y + 49, { size: 26, weight: 400 }); if (it[1]) txt(c, it[1], r.x + r.w - 32, y + 49, { size: 22, weight: 400, color: WF.mid, align: 'right' }); }); } },
    { cat: NV, id: 'navigation-bar', name: 'Barra de navegación', place: 'bottom', size: (d) => [d.w, 160], draw: (c, r) => K.navBar(c, r) },
    { cat: NV, id: 'navigation-drawer', name: 'Cajón de navegación', place: 'left', size: (d) => [Math.min(720, Math.round(d.w * 0.82)), d.h], draw: (c, r) => K.drawer(c, r.x, r.y, r.w, r.h, true) },
    { cat: NV, id: 'navigation-rail', name: 'Riel de navegación', place: 'left', size: (d) => [176, d.h - 100], draw: (c, r) => K.rail(c, r) },
    { cat: SE, id: 'radio-button', name: 'Botones de opción (radio)', size: () => [460, 280],
      draw: (c, r) => ['Claro', 'Oscuro', 'Automático'].forEach((l, i) => { K.radio(c, r.x + 22, r.y + 40 + i * 92, i === 2); txt(c, l, r.x + 68, r.y + 41 + i * 92, { size: 26, weight: 400 }); }) },
    { cat: NV, id: 'search', name: 'Búsqueda', place: 'topInset', size: (d) => [d.w - 64, 112], draw: (c, r) => K.searchBar(c, r.x, r.y + 8, r.w, { lead: 'menu' }) },
    { cat: CT, id: 'bottom-sheets', name: 'Hoja inferior', place: 'bottom', size: (d) => [d.w, Math.round(d.h * 0.45)],
      draw: (c, r) => { surface(c, r.x, r.y, r.w, r.h, [56, 56, 0, 0]); bar(c, r.x + r.w / 2 - 32, r.y + 20, 64, 8, WF.mid); K.rows(c, r.x, r.y + 56, r.w, r.y + r.h - 16, { lead: 'icon', oneLine: true, divider: false }); } },
    { cat: CT, id: 'side-sheets', name: 'Hoja lateral', place: 'right', size: (d) => [Math.min(640, Math.round(d.w * 0.8)), d.h - 100],
      draw: (c, r) => { c.fillStyle = WF.surface; c.fillRect(r.x, r.y, r.w, r.h); vdivider(c, r.x, r.y, r.h); txt(c, 'Filtros', r.x + 48, r.y + 60, { size: 30, weight: 700 }); icon(c, r.x + r.w - 56, r.y + 60, 'close'); let y = K.chips(c, r.x + 48, r.y + 120, ['Hoy', 'Semana']) + 48; field(c, r.x + 48, y, r.w - 96, 'Cliente', 1); const bw = (r.w - 120) / 2; button(c, r.x + 48, r.y + r.h - 128, bw, 80, 'Limpiar', 'outlined'); button(c, r.x + 72 + bw, r.y + r.h - 128, bw, 80, 'Aplicar'); } },
    { cat: SE, id: 'sliders', name: 'Deslizadores', size: (d) => [Math.min(d.w - 64, 640), 280],
      draw: (c, r) => { const t = (y, a, b) => { bar(c, r.x, y - 8, r.w, 16, WF.line); bar(c, r.x + r.w * a, y - 8, r.w * (b - a), 16, WF.dark); }; t(r.y + 40, 0, 0.6); bar(c, r.x + r.w * 0.6 - 3, r.y + 8, 6, 64, WF.dark); txt(c, '60', r.x + r.w * 0.6, r.y + 100, { size: 22, align: 'center' });
        t(r.y + 190, 0.2, 0.75); for (const f of [0.2, 0.75]) bar(c, r.x + r.w * f - 3, r.y + 158, 6, 64, WF.dark); for (let k = 0; k <= 10; k++) disc(c, r.x + r.w * k / 10, r.y + 190, 3, WF.mid); } },
    { cat: CO, id: 'snackbar', name: 'Snackbar', place: 'bottomInset', size: (d) => [Math.min(d.w - 64, 1000), 112],
      draw: (c, r) => { c.fillStyle = WF.dark; rr(c, r.x, r.y, r.w, r.h, 8); c.fill(); txt(c, 'Pedido archivado', r.x + 40, r.y + 57, { size: 26, weight: 400, color: '#ffffff' }); txt(c, 'Deshacer', r.x + r.w - 40, r.y + 57, { size: 26, color: '#ffffff', align: 'right' }); } },
    { cat: SE, id: 'switch', name: 'Interruptores', size: (d) => [Math.min(d.w - 64, 560), 200],
      draw: (c, r) => [['Wi-Fi', true], ['Modo avión', false]].forEach(([l, on], i) => { txt(c, l, r.x, r.y + 50 + i * 100, { size: 26, weight: 400 }); toggle(c, r.x + r.w - 104, r.y + 18 + i * 100, on); }) },
    { cat: NV, id: 'tabs', name: 'Pestañas', place: 'belowBar', size: (d) => [d.w, 96], draw: (c, r) => K.tabs(c, r.x, r.y, r.w, ['Todos', 'Pendientes', 'Hechos']) },
    { cat: TI, id: 'text-fields', name: 'Campos de texto', size: (d) => [Math.min(d.w - 64, 640), 340],
      draw: (c, r) => { c.fillStyle = WF.tonal; rr(c, r.x, r.y, r.w, 112, [8, 8, 0, 0]); c.fill(); divider(c, r.x, r.y + 110, r.w); txt(c, 'Relleno', r.x + 32, r.y + 32, { size: 20, color: WF.mid }); txt(c, 'María López', r.x + 32, r.y + 74, { size: 28, weight: 400 });
        tf(c, r.x, r.y + 196, r.w, { label: 'Con borde', value: 'María López', help: 'Texto de ayuda' }); } },
    { cat: A, id: 'toolbars', name: 'Barra de herramientas', place: 'bottomInset', size: () => [600, 128],
      draw: (c, r) => { c.save(); c.shadowColor = 'rgba(15,23,42,.18)'; c.shadowBlur = 16; c.fillStyle = WF.pill; rr(c, r.x, r.y, r.w - 144, r.h, 64); c.fill(); c.restore(); for (let k = 0; k < 4; k++) icon(c, r.x + 64 + k * 96, r.y + 64, 'dot', WF.dark); fab(c, r.x + r.w - 128, r.y + 8); } },
    { cat: CO, id: 'tooltips', name: 'Información sobre herramientas', size: () => [560, 320],
      draw: (c, r) => { icon(c, r.x + 40, r.y + 40, 'dot', WF.dark); c.fillStyle = WF.dark; rr(c, r.x + 80, r.y + 4, 260, 64, 8); c.fill(); txt(c, 'Añadir a favoritos', r.x + 210, r.y + 37, { size: 22, color: '#ffffff', align: 'center' });
        surface(c, r.x, r.y + 112, r.w, 200, 24); txt(c, 'Copias automáticas', r.x + 32, r.y + 152, { size: 24 }); K.textLines(c, r.x + 32, r.y + 184, r.w - 64, r.y + 230); txt(c, 'Más información', r.x + 32, r.y + 278, { size: 22 }); } },
  ];

  // ── Campos y datos ─────────────────────────────────────────────────────────────────────
  const F = 'Campos de entrada', D = 'Visualización de datos';
  const fw = (d) => Math.min(d.w - 64, 640);
  const one = (o, h = 170) => ({ size: (d) => [fw(d), h], draw: (c, r) => tf(c, r.x, r.y + 16, r.w, o) });
  const CAMPOS = [
    { cat: F, id: 'campo-texto', name: 'Texto', ...one({ label: 'Nombre', value: 'María López', help: 'Obligatorio' }) },
    { cat: F, id: 'campo-multilinea', name: 'Texto largo (multilínea)', size: (d) => [fw(d), 300], draw: (c, r) => tf(c, r.x, r.y + 16, r.w, { label: 'Notas', value: 'Entregar por la mañana.\nLlamar antes de subir.', multiline: true, h: 200, counter: '47 / 500' }) },
    { cat: F, id: 'campo-entero', name: 'Número entero', ...one({ label: 'Cantidad', value: '1.250', trail: 'stepper', help: 'Unidades, sin decimales' }) },
    { cat: F, id: 'campo-decimal', name: 'Número decimal', ...one({ label: 'Peso', value: '12,75', suffix: 'kg', help: 'Hasta 2 decimales' }) },
    { cat: F, id: 'campo-moneda', name: 'Importe (moneda)', ...one({ label: 'Importe', value: '1.234,56', suffix: '€', help: 'IVA incluido' }) },
    { cat: F, id: 'campo-porcentaje', name: 'Porcentaje', ...one({ label: 'Descuento', value: '12,5', suffix: '%', help: 'Entre 0 y 100' }) },
    { cat: F, id: 'campo-fecha', name: 'Fecha', ...one({ label: 'Fecha de entrega', value: '01/10/2026', trail: 'calendar', help: 'dd/mm/aaaa' }) },
    { cat: F, id: 'campo-hora', name: 'Hora', ...one({ label: 'Hora', value: '14:30', trail: 'clock', help: 'hh:mm (24 h)' }) },
    { cat: F, id: 'campo-fecha-hora', name: 'Fecha y hora', size: (d) => [fw(d), 170],
      draw: (c, r) => { const w1 = Math.round((r.w - 24) * 0.58); tf(c, r.x, r.y + 16, w1, { label: 'Fecha de inicio', value: '01/10/2026', trail: 'calendar', help: 'dd/mm/aaaa' }); tf(c, r.x + w1 + 24, r.y + 16, r.w - w1 - 24, { label: 'Hora', value: '09:00', trail: 'clock', help: 'hh:mm' }); } },
    { cat: F, id: 'campo-rango-fechas', name: 'Rango de fechas', ...one({ label: 'Periodo', value: '01/10/2026 – 15/10/2026', trail: 'calendar', help: 'dd/mm/aaaa – dd/mm/aaaa' }) },
    { cat: F, id: 'campo-duracion', name: 'Duración', ...one({ label: 'Duración', value: '1 h 30 min', trail: 'clock', help: 'Horas y minutos' }) },
    { cat: F, id: 'campo-email', name: 'Correo electrónico', ...one({ label: 'Correo', value: 'maria@empresa.com', lead: 'mail', help: 'nombre@dominio.com' }) },
    { cat: F, id: 'campo-telefono', name: 'Teléfono', ...one({ label: 'Teléfono', prefix: '+34', value: '600 123 456', lead: 'phone' }) },
    { cat: F, id: 'campo-contrasena', name: 'Contraseña', ...one({ label: 'Contraseña', value: '••••••••••', trail: 'eye', help: 'Mínimo 8 caracteres' }) },
    { cat: F, id: 'campo-url', name: 'Dirección web (URL)', ...one({ label: 'Web', prefix: 'https://', value: 'empresa.com', lead: 'link' }) },
    { cat: F, id: 'campo-desplegable', name: 'Desplegable (selección única)', size: (d) => [fw(d), 520],
      draw: (c, r) => { tf(c, r.x, r.y + 16, r.w, { label: 'Estado', value: 'Pendiente', trail: 'expand' }); surface(c, r.x, r.y + 140, r.w, 360, 16); ['Pendiente', 'En curso', 'Entregado', 'Cancelado'].forEach((l, i) => { if (!i) { c.fillStyle = WF.tonal; c.fillRect(r.x, r.y + 156, r.w, 80); } txt(c, l, r.x + 40, r.y + 197 + i * 84, { size: 26, weight: 400 }); }); } },
    { cat: F, id: 'campo-autocompletar', name: 'Autocompletar', size: (d) => [fw(d), 440],
      draw: (c, r) => { tf(c, r.x, r.y + 16, r.w, { label: 'Cliente', value: 'Ferr', lead: 'search' }); surface(c, r.x, r.y + 140, r.w, 280, 16); ['Ferretería López', 'Ferrovial', 'Ferrer y Asociados'].forEach((l, i) => txt(c, l, r.x + 40, r.y + 192 + i * 84, { size: 26, weight: 400 })); } },
    { cat: F, id: 'campo-booleano', name: 'Sí / No (booleano)', size: (d) => [fw(d), 200],
      draw: (c, r) => { txt(c, 'Factura electrónica', r.x, r.y + 50, { size: 26, weight: 400 }); toggle(c, r.x + r.w - 104, r.y + 18, true); check(c, r.x, r.y + 120, 'on'); txt(c, 'Acepto las condiciones', r.x + 60, r.y + 139, { size: 26, weight: 400 }); } },
    { cat: F, id: 'campo-deslizador', name: 'Valor en un rango (deslizador + número)', size: (d) => [fw(d), 200],
      draw: (c, r) => { const sw = r.w - 200; bar(c, r.x, r.y + 52, sw, 16, WF.line); bar(c, r.x, r.y + 52, sw * 0.4, 16, WF.dark); bar(c, r.x + sw * 0.4 - 3, r.y + 28, 6, 64, WF.dark); tf(c, r.x + sw + 32, r.y + 4, 168, { label: 'Volumen', value: '40', suffix: '%' }); } },
    { cat: F, id: 'campo-etiquetas', name: 'Etiquetas (chips de entrada)', size: (d) => [fw(d), 200],
      draw: (c, r) => { c.strokeStyle = WF.mid; c.lineWidth = 2; rr(c, r.x, r.y + 16, r.w, 128, 8); c.stroke(); txt(c, 'Etiquetas', r.x + 28, r.y + 17, { size: 22, color: WF.mid }); let x = r.x + 24; x += chip(c, x, r.y + 48, 'Urgente ✕') + 12; chip(c, x, r.y + 48, 'Madrid ✕'); } },
    { cat: F, id: 'campo-archivo', name: 'Adjuntar archivo', size: (d) => [fw(d), 200],
      draw: (c, r) => { button(c, r.x, r.y + 16, 300, 80, 'Adjuntar archivo', 'outlined'); c.strokeStyle = WF.mid; c.lineWidth = 2; rr(c, r.x, r.y + 116, r.w, 72, 16); c.stroke(); icon(c, r.x + 40, r.y + 152, 'dot'); txt(c, 'albaran-0412.pdf · 240 KB', r.x + 80, r.y + 153, { size: 24, weight: 400 }); icon(c, r.x + r.w - 40, r.y + 152, 'close'); } },
    { cat: F, id: 'campo-error', name: 'Campo con error', ...one({ label: 'Fecha de entrega', value: '31/02/2026', trail: 'error', help: 'Esa fecha no existe. Usa dd/mm/aaaa.', error: true }) },
    { cat: D, id: 'datos-ficha', name: 'Ficha de datos (solo lectura)', size: (d) => [Math.min(d.w - 32, 760), 11 * 88],
      draw: (c, r) => { let y = r.y; [['Cliente', 'Ferretería López', 0], ['Cantidad', '1.250', 1], ['Peso', '12,75 kg', 1], ['Importe', '1.234,56 €', 1], ['Descuento', '12,5 %', 1], ['Fecha', '1 oct 2026', 0], ['Hora', '14:30', 0], ['Fecha y hora', '01/10/2026 14:30', 0], ['Duración', '1 h 30 min', 0], ['Factura electrónica', 'Sí', 0], ['Estado', 'Pendiente', 0]].forEach(([k, v, n]) => { y = kv(c, r.x, y, r.w, k, v, n); }); } },
    { cat: D, id: 'datos-tabla', name: 'Tabla de datos', size: (d) => [d.w - 32, 560],
      draw: (c, r) => { const cols = [['Pedido', 0.22, 0], ['Fecha', 0.24, 0], ['Unidades', 0.2, 1], ['Importe', 0.34, 1]]; let x = r.x;
        const cx = cols.map(([, f]) => { const a = x; x += r.w * f; return [a, x]; });
        cols.forEach(([l, , n], i) => txt(c, l, n ? cx[i][1] - 20 : cx[i][0] + 20, r.y + 40, { size: 22, color: WF.mid, align: n ? 'right' : 'left' })); divider(c, r.x, r.y + 78, r.w);
        [['#0412', '01/10/2026', '1.250', '1.234,56 €'], ['#0413', '02/10/2026', '80', '96,00 €'], ['#0414', '02/10/2026', '12', '1.020,10 €'], ['#0415', '03/10/2026', '3.400', '12.500,00 €'], ['#0416', '05/10/2026', '6', '8,40 €']].forEach((row, j) => {
          const y = r.y + 80 + j * 92; row.forEach((v, i) => txt(c, v, cols[i][2] ? cx[i][1] - 20 : cx[i][0] + 20, y + 46, { size: 24, weight: 400, align: cols[i][2] ? 'right' : 'left' })); divider(c, r.x, y + 91, r.w); }); } },
    { cat: D, id: 'datos-resumen', name: 'Cifras destacadas (KPI)', size: (d) => [d.w - 64, 220],
      draw: (c, r) => { const n = 3, w = (r.w - 2 * 24) / n; [['Ventas', '12.480 €', '+8,2 %'], ['Pedidos', '342', '−1,5 %'], ['Margen', '23,4 %', '+0,6 pp']].forEach(([k, v, dlt], i) => { const x = r.x + i * (w + 24); c.strokeStyle = WF.line; c.lineWidth = 2; rr(c, x, r.y, w, r.h, 24); c.stroke(); txt(c, k, x + 28, r.y + 44, { size: 22, color: WF.mid }); txt(c, v, x + 28, r.y + 110, { size: 40, weight: 700 }); txt(c, dlt, x + 28, r.y + 172, { size: 22, color: WF.mid }); }); } },
  ];

  // Fichas de los campos (formato es-ES). Material 3 no tiene una página por tipo de dato: se
  // apoyan en las guías de campos de texto (text fields), menús y selectores de fecha y hora.
  const FICHA = {
    'campo-texto': { que: 'Texto libre corto: nombres, referencias, títulos.', html: 'type="text"', formato: ['Etiqueta siempre visible; nunca solo marcador de posición.'],
      comportamiento: ['Valida al salir del campo, no mientras se escribe.', 'El error sustituye al texto de ayuda debajo del campo.', 'Indica «Obligatorio» en la ayuda o con asterisco y explícalo una vez.'] },
    'campo-multilinea': { que: 'Texto largo: notas, descripciones, comentarios.', html: '<textarea>', formato: ['Contador «n / máx» si hay límite de caracteres.'],
      comportamiento: ['Crece con el contenido hasta un alto máximo; después, scroll interno.', 'Intro añade línea; no envía el formulario.'] },
    'campo-entero': { que: 'Cantidades sin decimales: unidades, personas, años.', html: 'type="text" inputmode="numeric" pattern="[0-9]*"', formato: ['Al mostrar, separador de miles con punto: 1.250.', 'Al teclear, acepta el número con o sin punto de miles.'],
      comportamiento: ['Teclado numérico en móvil.', 'Botones − / + solo para valores pequeños y pasos de 1.', 'Valida mínimo y máximo y explica el rango en la ayuda.', 'Rechaza decimales con un mensaje claro.'] },
    'campo-decimal': { que: 'Números con decimales y unidad: peso, longitud, temperatura.', html: 'type="text" inputmode="decimal"', formato: ['Coma decimal y punto de miles (es-ES): 1.234,75.', 'Acepta también el punto al teclear y normaliza al salir.', 'Unidad como sufijo dentro del campo (kg, m, °C).', 'Número de decimales fijo al mostrar.'],
      comportamiento: ['Teclado decimal en móvil.', 'Redondea o avisa si hay más decimales de los permitidos.'] },
    'campo-moneda': { que: 'Importes de dinero.', html: 'type="text" inputmode="decimal"', formato: ['Símbolo detrás con espacio (es-ES): 1.234,56 €.', 'Siempre dos decimales al mostrar.', 'Indica si el importe lleva impuestos (IVA incluido / sin IVA).'],
      comportamiento: ['Símbolo € como sufijo fijo; el usuario solo teclea el número.', 'No permite más de dos decimales.', 'Negativos con signo menos (−12,00 €), no entre paréntesis.'] },
    'campo-porcentaje': { que: 'Proporciones: descuentos, impuestos, avance.', html: 'type="text" inputmode="decimal"', formato: ['Símbolo % como sufijo; al mostrar, con espacio: 12,5 %.', 'Para diferencias entre porcentajes, puntos porcentuales (pp).'],
      comportamiento: ['Valida el rango (normalmente 0–100) y dilo en la ayuda.', 'Decide si se guarda como 12,5 o 0,125 y conviértelo de forma invisible.'] },
    'campo-fecha': { que: 'Una fecha: entrega, nacimiento, vencimiento.', html: 'type="text" inputmode="numeric" o type="date"', formato: ['Entrada dd/mm/aaaa; ayuda con el formato bajo el campo.', 'En solo lectura, fecha corta «1 oct 2026» o dd/mm/aaaa según el contexto.'],
      comportamiento: ['Se puede teclear o elegir con el icono de calendario.', 'El icono abre el selector de fecha (date picker).', 'Valida fechas imposibles (31/02) y fuera de rango (pasadas o futuras).'],
      compacto: 'El calendario abre un selector de fecha modal; para fechas lejanas (nacimiento), mejor la entrada de texto (modal input).', expandido: 'Selector de fecha acoplado (docked) desplegado bajo el campo.' },
    'campo-hora': { que: 'Una hora del día.', html: 'type="text" inputmode="numeric" o type="time"', formato: ['24 horas, hh:mm (14:30).'],
      comportamiento: ['Se puede teclear o elegir con el icono de reloj.', 'El icono abre el selector de hora (time picker): dial o entrada de texto.', 'Valida 00:00–23:59.'],
      compacto: 'Selector de hora con dial en vertical.', expandido: 'Selector de hora en horizontal (dial al lado de la hora) o entrada de texto.' },
    'campo-fecha-hora': { que: 'Momento concreto: inicio de una cita, salida de un envío.', html: 'dos campos: fecha + hora', formato: ['Fecha dd/mm/aaaa y hora hh:mm en campos separados.', 'En solo lectura: 01/10/2026 14:30.', 'Indica la zona horaria si los usuarios están en zonas distintas.'],
      comportamiento: ['Cada campo abre su selector (fecha y hora).', 'Si hay inicio y fin, valida que el fin sea posterior.'],
      compacto: 'Fecha y hora pueden ir en la misma fila o apiladas si no caben.', expandido: 'Fecha y hora en la misma fila.' },
    'campo-rango-fechas': { que: 'Periodo entre dos fechas: vacaciones, informes.', html: 'dos fechas (inicio y fin)', formato: ['dd/mm/aaaa – dd/mm/aaaa.'],
      comportamiento: ['El icono abre el selector de rango (date range picker).', 'La fecha de fin no puede ser anterior a la de inicio.', 'Ofrece atajos si ayudan: hoy, esta semana, este mes.'],
      compacto: 'Selector de rango modal a pantalla completa.', expandido: 'Selector de rango acoplado o en diálogo.' },
    'campo-duracion': { que: 'Tiempo transcurrido: duración de una tarea o servicio.', html: 'type="text" inputmode="numeric" (o dos campos h y min)', formato: ['Al mostrar: 1 h 30 min.', 'No confundir con una hora del día.'],
      comportamiento: ['Teclado numérico; acepta 1:30 y lo convierte a 1 h 30 min.'] },
    'campo-email': { que: 'Dirección de correo electrónico.', html: 'type="email" inputmode="email" autocomplete="email"', formato: ['Sin mayúsculas forzadas ni autocorrección.'],
      comportamiento: ['Teclado con @ en móvil.', 'Valida el formato al salir del campo.'] },
    'campo-telefono': { que: 'Número de teléfono.', html: 'type="tel" inputmode="tel" autocomplete="tel"', formato: ['Prefijo de país como prefijo del campo (+34).', 'Al mostrar, en grupos: 600 123 456.'],
      comportamiento: ['Teclado telefónico en móvil.', 'Acepta espacios y guiones al pegar.'] },
    'campo-contrasena': { que: 'Contraseña o clave.', html: 'type="password" autocomplete="current-password" o "new-password"', formato: ['Requisitos en el texto de ayuda antes de escribir.'],
      comportamiento: ['Botón de icono para mostrar u ocultar la contraseña.', 'Permite pegar (gestores de contraseñas).', 'No borres lo escrito al mostrar un error.'] },
    'campo-url': { que: 'Dirección web.', html: 'type="url" inputmode="url"', formato: ['Prefijo https:// fijo si siempre es web.'], comportamiento: ['Valida al salir; acepta la dirección con o sin https://.'] },
    'campo-desplegable': { que: 'Elegir una opción de una lista corta (menú desplegable expuesto).', html: '<select> o combobox', formato: ['Muestra la opción elegida en el campo, con flecha ▾.'],
      comportamiento: ['Al pulsar, se abre un menú (menus) bajo el campo.', 'Con más de unas 7 opciones, mejor autocompletar.', 'Con 2–5 opciones visibles a la vez, valora botones de opción o segmentados.'] },
    'campo-autocompletar': { que: 'Elegir de una lista larga escribiendo (clientes, productos, ciudades).', html: 'combobox con lista de sugerencias', formato: ['Resalta en cada sugerencia el texto que coincide.'],
      comportamiento: ['Sugiere mientras se escribe; flechas e Intro para elegir.', 'Decide si se admiten valores que no están en la lista y dilo.', 'Muestra «Sin resultados» y, si procede, «Crear…».'] },
    'campo-booleano': { que: 'Sí o no.', html: 'checkbox o role="switch"', formato: ['En solo lectura: Sí / No (no true/false).'],
      comportamiento: ['Interruptor (switch) cuando el cambio se aplica al momento.', 'Casilla (checkbox) cuando se confirma al enviar el formulario o para aceptar condiciones.'] },
    'campo-deslizador': { que: 'Valor dentro de un rango cuando lo aproximado basta (volumen, distancia).', html: 'type="range" + campo numérico', formato: ['Muestra el valor y la unidad junto al deslizador.'],
      comportamiento: ['Deslizador (slider) para ajustar; campo numérico al lado para el valor exacto.', 'Con pasos discretos, marca los puntos de parada.'] },
    'campo-etiquetas': { que: 'Varios valores libres: etiquetas, destinatarios.', html: 'campo + lista de chips', formato: ['Cada valor es un chip de entrada con ✕.'],
      comportamiento: ['Intro o coma convierte el texto en chip.', 'Retroceso con el campo vacío quita el último chip.'] },
    'campo-archivo': { que: 'Adjuntar documentos o imágenes.', html: 'type="file"', formato: ['Lista de archivos con nombre, tamaño (240 KB) y botón para quitar.'],
      comportamiento: ['Botón «Adjuntar archivo»; indica tipos y tamaño máximo en la ayuda.', 'Muestra el progreso de subida y los errores por archivo.'], expandido: 'Admite además arrastrar y soltar sobre la zona.' },
    'campo-error': { que: 'Cómo se muestra un error de validación en cualquier campo.', formato: ['El mensaje dice qué falla y cómo arreglarlo (no solo «Campo no válido»).'],
      comportamiento: ['Borde, etiqueta y texto de ayuda en el color de error, con icono: no depende solo del color.', 'El mensaje sustituye a la ayuda y se anuncia a lectores de pantalla.', 'Se valida al salir del campo y al enviar; al corregir, el error desaparece.'] },
    'datos-ficha': { que: 'Mostrar datos ya guardados, sin editar.', formato: ['Etiqueta y valor; valores numéricos alineados a la derecha.', 'Formatos es-ES: 1.250 · 12,75 kg · 1.234,56 € · 12,5 % · 1 oct 2026 · 14:30 · Sí/No.', 'Valor vacío: «—», no «null» ni 0.'],
      comportamiento: ['Un botón «Editar» abre el formulario o hace editable la ficha.'], compacto: 'Etiqueta encima del valor si no cabe en la misma fila.', expandido: 'Etiqueta y valor en la misma fila, en una o dos columnas.' },
    'datos-tabla': { que: 'Comparar varios registros con las mismas columnas.', formato: ['Números e importes alineados a la derecha, cabecera incluida; texto a la izquierda.', 'Mismo número de decimales en toda la columna.', 'Fechas en formato corto y uniforme.'],
      comportamiento: ['Ordenar al pulsar la cabecera; filtrar con chips o búsqueda.', 'Filas pulsables para abrir el detalle.'], compacto: 'Convierte cada fila en un elemento de lista o tarjeta con los 2–3 datos clave.', expandido: 'Tabla completa; columnas fijas a la izquierda si hay scroll horizontal.' },
    'datos-resumen': { que: 'Cifras clave de un vistazo (KPI).', formato: ['Cifra grande con su unidad; variación con signo (+8,2 %, −1,5 %) y periodo de referencia.', 'Para diferencias entre porcentajes, puntos porcentuales (pp).'],
      comportamiento: ['La variación no depende solo del color: lleva signo o flecha.', 'Pulsar una cifra abre su detalle.'], compacto: 'Tarjetas en una fila con scroll horizontal o en cuadrícula de 2.', expandido: 'Todas las tarjetas en una fila.' },
  };
  for (const p of CAMPOS) p.ficha = FICHA[p.id] || {};

  window.PizarraPiezas = { list: [...COMPONENTES.map(p => ({ ...p, kind: 'componente' })), ...CAMPOS.map(p => ({ ...p, kind: 'campo' }))] };
})();
