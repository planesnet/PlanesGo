import { createElement, useEffect, useRef, useState, type FormEvent } from 'react';
import { enEscritorio, post, type Proyecto } from '../api';
import * as L from '../lanzador';
import Icono from './Icono';
import { SelectorFormato } from './Lanzador';

// Navegador integrado, como el de Claude: pestañas, atrás/adelante/recargar y dirección, ocupando todo el panel.
// La página se dibuja con ancho de escritorio y se escala al ancho del panel (se ve más pequeña, pero entera).
// En la app de escritorio usa <webview> (carga cualquier web); en el navegador normal, un iframe.

const ANCHO_PAGINA = 1280;

// Cada pestaña con su formato: ordenador (la página a ANCHO_PAGINA, reducida al panel) o tablet/móvil emulados.
interface Pestana { id: number; url: string; titulo: string; dispositivo?: L.Dispositivo; orientacion?: L.Orientacion }
export type PedidoWeb = { url: string; n: number; formato?: { dispositivo: L.Dispositivo; orientacion: L.Orientacion } };

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

export default function Navegador({ proyecto, pedido }: { proyecto: Proyecto; pedido?: PedidoWeb | null }) {
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
  // Las emuladas (tablet, móvil) van a su tamaño real dentro de un marco que se reduce con CSS.
  const emuladas = useRef(new Set<number>());
  const escala = () => {
    const f = Math.min(1, (caja.current?.clientWidth || ANCHO_PAGINA) / ANCHO_PAGINA);
    vistas.current.forEach((v, id) => { try { v.setZoomFactor(emuladas.current.has(id) ? 1 : f); } catch { /* aún sin cargar */ } });
    if (caja.current) setHueco({ ancho: caja.current.clientWidth, alto: caja.current.clientHeight });
  };
  const [hueco, setHueco] = useState({ ancho: 0, alto: 0 });
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
  // Duplicar: otra pestaña con la misma dirección y el mismo formato, justo a su derecha.
  const duplica = (id: number) => {
    const o = pestanas.find(p => p.id === id);
    if (!o) return;
    let url = o.url;
    try { const v = vistas.current.get(id); if (v && escritorio) url = v.getURL() || url; } catch { /* aún sin cargar */ }
    const d = { ...pestanaNueva(), url, titulo: o.titulo, dispositivo: o.dispositivo, orientacion: o.orientacion };
    setPestanas(l => { const i = l.findIndex(p => p.id === id); return [...l.slice(0, i + 1), d, ...l.slice(i + 1)]; });
    setActiva(d.id);
  };

  // URL pedida desde fuera (un programa que arranca, el agente con «pgo abrir»): si ya está abierta en una
  // pestaña, se recarga; si la activa está vacía, se usa; si no, pestaña nueva.
  useEffect(() => {
    if (!pedido) return;
    const u = normaliza(pedido.url);
    if (!u) return;
    const fmt = pedido.formato ? { dispositivo: pedido.formato.dispositivo, orientacion: pedido.formato.orientacion } : {};
    const ya = pestanas.find(p => p.url && new URL(p.url).origin === new URL(u).origin);
    if (ya) {
      setActiva(ya.id);
      const v = vistas.current.get(ya.id);
      const mismo = !pedido.formato || (pedido.formato.dispositivo === (ya.dispositivo || 'ordenador') && pedido.formato.orientacion === (ya.orientacion || 'horizontal'));
      if (v && escritorio && mismo) v.src = u; // con otro formato la vista se vuelve a crear (otro agente de usuario)
      pon(ya.id, { url: u, ...fmt });
      return;
    }
    const vacia = pestanas.find(p => p.id === activaRef.current && !p.url);
    if (vacia) { pon(vacia.id, { url: u, titulo: u.replace(/^https?:\/\//, ''), ...fmt }); setEntrada(u); const v = vistas.current.get(vacia.id); if (v && escritorio) v.src = u; return; }
    const p = { ...pestanaNueva(), url: u, titulo: u.replace(/^https?:\/\//, ''), ...fmt };
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
          <div key={p.id} className={'pestana-web' + (p.id === activa ? ' activa' : '')} role="tab" aria-selected={p.id === activa}
            onContextMenu={e => { e.preventDefault(); if (p.url) duplica(p.id); }} title={p.url ? 'Clic derecho: duplicar la pestaña' : undefined}>
            <button className="titulo-pestana" onClick={() => setActiva(p.id)} title={p.url || p.titulo}>
              <Icono n="web" t={13} /> <span className="recorta">{p.titulo || p.url}</span>
            </button>
            {p.url && <button className="cerrar-pestana duplicar-pestana" onClick={() => duplica(p.id)} title="Duplicar la pestaña" aria-label="Duplicar la pestaña"><Icono n="copiar" t={11} /></button>}
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
        <SelectorFormato dispositivo={actual.dispositivo || 'ordenador'} orientacion={actual.orientacion || 'horizontal'}
          alCambiar={(d, o) => pon(actual.id, { dispositivo: d, orientacion: o })} />
        <button type="button" className="icono" disabled={!actual.url} onClick={fuera} title="Abrir en el navegador del sistema" aria-label="Abrir en el navegador del sistema">
          <Icono n="externo" t={15} />
        </button>
      </form>
      <div className="vistas" ref={caja}>
        {pestanas.map(p => {
          const d = p.dispositivo || 'ordenador';
          const t = L.tamano(d, p.orientacion || 'horizontal');
          if (t) emuladas.current.add(p.id); else emuladas.current.delete(p.id);
          // Con otro formato, otra vista (key): el agente de usuario solo se fija al crearla.
          const vista = !p.url
            ? <p className="suave relleno">Escribe una dirección o una búsqueda.</p>
            : escritorio
              ? createElement('webview', { key: d, ref: engancha(p.id), src: p.url, className: 'vista', partition: 'persist:navegador-' + proyecto.id, useragent: L.agenteUsuario(d) })
              : <iframe key={d} className="vista" src={p.url} title={p.titulo} sandbox="allow-scripts allow-same-origin allow-forms allow-popups" />;
          if (!t || !p.url) return <div key={p.id} className="vista-pestana" hidden={p.id !== activa}>{vista}</div>;
          const f = L.escalaMarco(t, hueco, 48); // margen: el marco del dispositivo y la medida de debajo
          return (
            <div key={p.id} className="vista-pestana emulada" hidden={p.id !== activa}>
              <div className="hueco-dispositivo" style={{ width: t.ancho * f, height: t.alto * f }}>
                <div className={'marco-dispositivo ' + d} style={{ width: t.ancho, height: t.alto, transform: `scale(${f})` }}>{vista}</div>
              </div>
              <span className="medida-dispositivo">{t.ancho} × {t.alto}{f < 1 ? ` · ${Math.round(f * 100)} %` : ''}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
