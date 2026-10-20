import { createElement, useEffect, useRef, useState, type FormEvent } from 'react';
import { enEscritorio, post, type Proyecto } from '../api';
import Icono from './Icono';

// Navegador integrado, como el de Claude: pestañas, atrás/adelante/recargar y dirección, ocupando todo el panel.
// La página se dibuja con ancho de escritorio y se escala al ancho del panel (se ve más pequeña, pero entera).
// En la app de escritorio usa <webview> (carga cualquier web); en el navegador normal, un iframe.

const ANCHO_PAGINA = 1280;

interface Pestana { id: number; url: string; titulo: string }

interface WebView extends HTMLElement {
  src: string;
  getURL(): string;
  canGoBack(): boolean; canGoForward(): boolean;
  goBack(): void; goForward(): void; reload(): void; stop(): void;
  setZoomFactor(f: number): void;
}

const pareceDominio = (s: string) => /^[\w.-]+\.[a-z]{2,}(:\d+)?(\/|$)/i.test(s) || /^localhost(:\d+)?(\/|$)/i.test(s);

// Dirección escrita → URL: con esquema, tal cual; con pinta de dominio, https://; si no, búsqueda.
const normaliza = (s: string) => {
  s = s.trim();
  if (!s) return '';
  if (!/^https?:\/\//i.test(s)) s = pareceDominio(s) ? 'https://' + s : 'https://www.google.com/search?q=' + encodeURIComponent(s);
  try { const u = new URL(s); return u.protocol === 'http:' || u.protocol === 'https:' ? u.toString() : ''; } catch { return ''; }
};

let siguiente = 1;
const pestanaNueva = (): Pestana => ({ id: siguiente++, url: '', titulo: 'Nueva pestaña' });

export default function Navegador({ proyecto, pedido }: { proyecto: Proyecto; pedido?: { url: string; n: number } | null }) {
  const [pestanas, setPestanas] = useState<Pestana[]>(() => [pestanaNueva()]);
  const [activa, setActiva] = useState(pestanas[0].id);
  const [entrada, setEntrada] = useState('');
  const [nav, setNav] = useState({ atras: false, adelante: false, cargando: false });
  const vistas = useRef(new Map<number, WebView>());
  const caja = useRef<HTMLDivElement>(null);
  const activaRef = useRef(activa);
  activaRef.current = activa;
  const escritorio = enEscritorio();

  const actual = pestanas.find(p => p.id === activa) || pestanas[0];
  const pon = (id: number, cambios: Partial<Pestana>) => setPestanas(l => l.map(p => p.id === id ? { ...p, ...cambios } : p));
  const vista = () => vistas.current.get(activaRef.current);

  // Escala: la página se dibuja a ANCHO_PAGINA y se reduce al ancho del panel.
  const escala = () => {
    const f = Math.min(1, (caja.current?.clientWidth || ANCHO_PAGINA) / ANCHO_PAGINA);
    vistas.current.forEach(v => { try { v.setZoomFactor(f); } catch { /* aún sin cargar */ } });
  };
  useEffect(() => {
    const ro = new ResizeObserver(escala);
    if (caja.current) ro.observe(caja.current);
    return () => ro.disconnect();
  }, []);

  const refrescaNav = () => {
    const v = vista();
    if (!v || !escritorio) { setNav(n => ({ ...n, atras: false, adelante: false })); return; }
    try { setNav(n => ({ ...n, atras: v.canGoBack(), adelante: v.canGoForward() })); } catch { /* aún sin cargar */ }
  };

  useEffect(() => { setEntrada(actual.url); refrescaNav(); }, [activa]); // eslint-disable-line react-hooks/exhaustive-deps

  // Engancha los eventos de cada <webview> una sola vez.
  const engancha = (id: number) => (el: WebView | null) => {
    if (!el || vistas.current.get(id) === el) return;
    vistas.current.set(id, el);
    const url = () => {
      const u = el.getURL();
      pon(id, { url: u });
      if (id === activaRef.current) { setEntrada(u); refrescaNav(); }
    };
    el.addEventListener('did-navigate', url);
    el.addEventListener('did-navigate-in-page', url);
    el.addEventListener('page-title-updated', (e => pon(id, { titulo: (e as Event & { title: string }).title })) as EventListener);
    el.addEventListener('did-start-loading', () => { if (id === activaRef.current) setNav(n => ({ ...n, cargando: true })); });
    el.addEventListener('did-stop-loading', () => { if (id === activaRef.current) { setNav(n => ({ ...n, cargando: false })); refrescaNav(); } });
    el.addEventListener('dom-ready', escala);
  };

  const ir = (e: FormEvent) => {
    e.preventDefault();
    const u = normaliza(entrada);
    if (!u) return;
    setEntrada(u);
    pon(activa, { url: u, titulo: u.replace(/^https?:\/\//, '') });
    const v = vista();
    if (v && escritorio) v.src = u;
  };

  const nueva = () => { const p = pestanaNueva(); setPestanas(l => [...l, p]); setActiva(p.id); };

  // URL pedida desde fuera (un programa que arranca, el agente con «pgo abrir»): si ya está abierta en una
  // pestaña, se recarga; si la activa está vacía, se usa; si no, pestaña nueva.
  useEffect(() => {
    if (!pedido) return;
    const u = normaliza(pedido.url);
    if (!u) return;
    const ya = pestanas.find(p => p.url && new URL(p.url).origin === new URL(u).origin);
    if (ya) {
      setActiva(ya.id);
      const v = vistas.current.get(ya.id);
      if (v && escritorio) v.src = u;
      pon(ya.id, { url: u });
      return;
    }
    const vacia = pestanas.find(p => p.id === activaRef.current && !p.url);
    if (vacia) { pon(vacia.id, { url: u, titulo: u.replace(/^https?:\/\//, '') }); setEntrada(u); const v = vistas.current.get(vacia.id); if (v && escritorio) v.src = u; return; }
    const p = { ...pestanaNueva(), url: u, titulo: u.replace(/^https?:\/\//, '') };
    setPestanas(l => [...l, p]);
    setActiva(p.id);
  }, [pedido?.n]); // eslint-disable-line react-hooks/exhaustive-deps
  const cierra = (id: number) => {
    vistas.current.delete(id);
    setPestanas(l => {
      const i = l.findIndex(p => p.id === id);
      const resto = l.filter(p => p.id !== id);
      if (!resto.length) { const p = pestanaNueva(); setActiva(p.id); return [p]; }
      if (id === activaRef.current) setActiva(resto[Math.max(0, i - 1)].id);
      return resto;
    });
  };

  const fuera = async () => {
    if (!actual.url) return;
    try {
      if (window.psf) await window.psf.abrirFuera(actual.url);
      else await post('/api/abrir', { proyecto: proyecto.id, url: actual.url });
    } catch { /* sin navegador del sistema */ }
  };

  return (
    <div className="navegador">
      <div className="pestanas-web" role="tablist">
        {pestanas.map(p => (
          <div key={p.id} className={'pestana-web' + (p.id === activa ? ' activa' : '')} role="tab" aria-selected={p.id === activa}>
            <button className="titulo-pestana" onClick={() => setActiva(p.id)} title={p.url || p.titulo}>
              <Icono n="web" t={13} /> <span className="recorta">{p.titulo || p.url}</span>
            </button>
            <button className="cerrar-pestana" onClick={() => cierra(p.id)} title="Cerrar pestaña" aria-label="Cerrar pestaña"><Icono n="cerrar" t={12} /></button>
          </div>
        ))}
        <button className="icono" onClick={nueva} title="Nueva pestaña" aria-label="Nueva pestaña"><Icono n="nuevo" t={15} /></button>
      </div>
      <form className="barra" onSubmit={ir}>
        <button type="button" className="icono" disabled={!nav.atras} onClick={() => vista()?.goBack()} title="Atrás" aria-label="Atrás"><Icono n="atras" t={16} /></button>
        <button type="button" className="icono" disabled={!nav.adelante} onClick={() => vista()?.goForward()} title="Adelante" aria-label="Adelante"><span className="espejo"><Icono n="atras" t={16} /></span></button>
        <button type="button" className="icono" disabled={!actual.url} onClick={() => nav.cargando ? vista()?.stop() : vista()?.reload()}
          title={nav.cargando ? 'Detener' : 'Recargar'} aria-label={nav.cargando ? 'Detener' : 'Recargar'}>
          <Icono n={nav.cargando ? 'cerrar' : 'recargar'} t={15} />
        </button>
        <input value={entrada} onChange={e => setEntrada(e.target.value)} onFocus={e => e.target.select()}
          placeholder="Busca o escribe una dirección" aria-label="Dirección" spellCheck={false} />
        <button type="button" className="icono" disabled={!actual.url} onClick={fuera} title="Abrir en el navegador del sistema" aria-label="Abrir en el navegador del sistema">
          <Icono n="externo" t={15} />
        </button>
      </form>
      <div className="vistas" ref={caja}>
        {pestanas.map(p => (
          <div key={p.id} className="vista-pestana" hidden={p.id !== activa}>
            {!p.url
              ? <p className="suave relleno">Escribe una dirección o una búsqueda.</p>
              : escritorio
                ? createElement('webview', { ref: engancha(p.id), src: p.url, className: 'vista', partition: 'persist:navegador-' + proyecto.id })
                : <iframe className="vista" src={p.url} title={p.titulo} sandbox="allow-scripts allow-same-origin allow-forms allow-popups" />}
          </div>
        ))}
      </div>
    </div>
  );
}
