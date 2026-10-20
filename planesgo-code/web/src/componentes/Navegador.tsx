import { createElement, useState, type FormEvent } from 'react';
import { enEscritorio, post, type Proyecto } from '../api';
import Icono from './Icono';

// Navegador del arnés: dentro de Electron usa <webview> (carga cualquier web); en el navegador normal, un iframe
// (muchas webs no se dejan incrustar: para esas está «Abrir en el navegador del sistema»).
export default function Navegador({ proyecto }: { proyecto: Proyecto }) {
  const [entrada, setEntrada] = useState('');
  const [url, setUrl] = useState('');
  const [error, setError] = useState('');

  const normaliza = (s: string) => {
    s = s.trim();
    if (!s) return '';
    if (!/^https?:\/\//i.test(s)) s = 'https://' + s;
    try { const u = new URL(s); return u.protocol === 'http:' || u.protocol === 'https:' ? u.toString() : ''; } catch { return ''; }
  };

  const ir = (e: FormEvent) => {
    e.preventDefault();
    const u = normaliza(entrada);
    setError(u ? '' : 'Dirección no válida (solo http o https)');
    if (u) { setUrl(u); setEntrada(u); }
  };

  const fuera = async () => {
    const u = normaliza(entrada) || url;
    if (!u) return;
    setError('');
    try {
      if (window.psf) await window.psf.abrirFuera(u);
      else await post('/api/abrir', { proyecto: proyecto.id, url: u });
    } catch (err) { setError((err as Error).message); }
  };

  return (
    <div className="navegador">
      <form className="barra" onSubmit={ir}>
        <input value={entrada} onChange={e => setEntrada(e.target.value)} placeholder="https://…" aria-label="Dirección" />
        <button className="icono" title="Ir" aria-label="Ir"><Icono n="ir" t={16} /></button>
        <button type="button" className="icono" onClick={fuera} title="Abrir en el navegador del sistema" aria-label="Abrir en el navegador del sistema"><Icono n="externo" t={16} /></button>
      </form>
      {error && <p className="error relleno">{error}</p>}
      {url
        ? (enEscritorio()
          ? createElement('webview', { src: url, className: 'vista', partition: 'persist:navegador-' + proyecto.id })
          : <iframe className="vista" src={url} title="Navegador" sandbox="allow-scripts allow-same-origin allow-forms allow-popups" />)
        : <p className="suave relleno">Escribe una dirección.{enEscritorio() ? '' : ' Algunas webs solo se ven en la app de escritorio.'}</p>}
    </div>
  );
}
