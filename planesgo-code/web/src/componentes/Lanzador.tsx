import { useEffect, useRef, useState } from 'react';
import { get, post, type ConfigPrograma, type Proceso, type Proyecto } from '../api';
import * as L from '../lanzador';
import Icono from './Icono';

// Barra de prueba encima del cuadro del chat: lanza lo que se está desarrollando en la rama de la sesión.
// A la izquierda, dónde (navegador interno, navegador del sistema o un programa del proyecto); después la
// dirección, como un enlace, y el botón de abrir. En el navegador interno, además, el formato de la página
// (ordenador, tablet, móvil) y su orientación. La elección se recuerda por proyecto.

export type AbreWeb = (url: string, formato?: { dispositivo: L.Dispositivo; orientacion: L.Orientacion }) => void;

const clave = (proyecto: number) => 'pgo:prueba:' + proyecto;

const DESTINOS: Record<'interno' | 'sistema', { icono: string; nombre: string }> = {
  interno: { icono: 'web', nombre: 'Navegador interno' },
  sistema: { icono: 'externo', nombre: 'Navegador del sistema' },
};
const DISPOSITIVOS: { id: L.Dispositivo; nombre: string }[] = [
  { id: 'ordenador', nombre: 'Ordenador' }, { id: 'tablet', nombre: 'Tablet' }, { id: 'movil', nombre: 'Móvil' },
];

// Formato de la página: ordenador, tablet o móvil, y su orientación (también lo usa el navegador interno).
export function Formato({ dispositivo, orientacion, alCambiar, desactivado }: {
  dispositivo: L.Dispositivo; orientacion: L.Orientacion; desactivado?: boolean;
  alCambiar: (d: L.Dispositivo, o: L.Orientacion) => void;
}) {
  return (
    <span className="formato" role="group" aria-label="Formato de la página">
      {DISPOSITIVOS.map(d => (
        <button key={d.id} type="button" className="icono" aria-pressed={dispositivo === d.id} disabled={desactivado}
          onClick={() => alCambiar(d.id, d.id === dispositivo ? orientacion : L.orientacionDe(d.id))}
          title={desactivado ? 'El formato solo se aplica en el navegador interno' : d.nombre} aria-label={d.nombre}>
          <Icono n={d.id} t={15} />
        </button>
      ))}
      <button type="button" className={'icono girar' + (orientacion === 'horizontal' ? ' horizontal' : '')} disabled={desactivado || dispositivo === 'ordenador'}
        onClick={() => alCambiar(dispositivo, orientacion === 'horizontal' ? 'vertical' : 'horizontal')}
        title={dispositivo === 'ordenador' ? 'La orientación es para tablet y móvil' : orientacion === 'horizontal' ? 'Horizontal (pasar a vertical)' : 'Vertical (pasar a horizontal)'}
        aria-label={'Orientación ' + orientacion}>
        <Icono n="girar" t={14} />
      </button>
    </span>
  );
}

export default function Lanzador({ proyecto, carpeta, version, alAbrirWeb }: {
  proyecto: Proyecto; carpeta: string; version: number; alAbrirWeb: AbreWeb;
}) {
  const [p, setP] = useState<L.Prueba>(() => { try { return L.leePrueba(localStorage.getItem(clave(proyecto.id))); } catch { return { ...L.PRUEBA_DEFECTO }; } });
  useEffect(() => { try { setP(L.leePrueba(localStorage.getItem(clave(proyecto.id)))); } catch { /* sin almacenamiento */ } }, [proyecto.id]);
  const cambia = (c: Partial<L.Prueba>) => setP(x => {
    const n = { ...x, ...c };
    try { localStorage.setItem(clave(proyecto.id), JSON.stringify(n)); } catch { /* sin almacenamiento */ }
    return n;
  });
  const [url, setUrl] = useState(p.url);
  useEffect(() => setUrl(p.url), [p.url]);

  // Programas del proyecto (los guardados y los que están en marcha) para proponer direcciones y ejecutarlos.
  const [procs, setProcs] = useState<Proceso[]>([]);
  const [cfg, setCfg] = useState<ConfigPrograma[]>([]);
  const [error, setError] = useState('');
  const carga = () => {
    if (!carpeta) return;
    get<{ procesos: Proceso[]; configuraciones: ConfigPrograma[] }>(`/api/procesos?proyecto=${proyecto.id}&carpeta=${encodeURIComponent(carpeta)}`)
      .then(r => { setProcs(r.procesos); setCfg(r.configuraciones); }).catch(() => {});
  };
  useEffect(carga, [proyecto.id, carpeta, version]); // eslint-disable-line react-hooks/exhaustive-deps

  const [menu, setMenu] = useState(false);
  const caja = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!menu) return;
    const fuera = (e: MouseEvent) => { if (!caja.current?.contains(e.target as Node)) setMenu(false); };
    document.addEventListener('mousedown', fuera);
    return () => document.removeEventListener('mousedown', fuera);
  }, [menu]);

  const enMarcha = procs.filter(x => x.estado === 'curso');
  const sug = L.sugerencias(enMarcha.map(x => x.url), cfg.map(c => c.url), p.url);
  const programa = p.destino === 'programa' ? cfg.find(c => c.nombre === p.programa) || procs.find(x => x.nombre === p.programa) : undefined;
  const proc = programa ? procs.find(x => x.nombre === programa.nombre) : undefined;
  const corriendo = proc?.estado === 'curso';

  const abre = async (u = url) => {
    const n = L.normalizaUrl(u);
    if (!n) { setError('Dirección no válida'); return; }
    setError('');
    if (n !== p.url) cambia({ url: n });
    setUrl(n);
    if (p.destino === 'sistema') {
      try {
        if (window.psf) await window.psf.abrirFuera(n);
        else await post('/api/abrir', { proyecto: proyecto.id, url: n });
      } catch (e) { setError((e as Error).message); }
    } else {
      alAbrirWeb(n, { dispositivo: p.dispositivo, orientacion: p.orientacion });
    }
  };

  const ejecuta = async () => {
    if (!programa) return;
    setError('');
    try {
      await post('/api/procesos/lanzar', { proyecto: proyecto.id, carpeta, nombre: programa.nombre, orden: programa.orden, url: programa.url });
      carga();
    } catch (e) { setError((e as Error).message); }
  };
  const para = async () => {
    if (!programa) return;
    try { await post('/api/procesos/parar', { nombre: programa.nombre }); carga(); } catch (e) { setError((e as Error).message); }
  };

  const icono = p.destino === 'programa' ? 'ejecutar' : DESTINOS[p.destino].icono;
  const titulo = p.destino === 'programa' ? 'Programa: ' + (programa?.nombre || 'elige uno') : DESTINOS[p.destino].nombre;
  const programas = [...cfg, ...procs.filter(x => !cfg.some(c => c.nombre === x.nombre)).map(x => ({ nombre: x.nombre, orden: x.orden, url: x.url }))];

  return (
    <div className="lanzador" ref={caja}>
      <div className="lanzador-destino">
        <button type="button" className="lanzador-elige" onClick={() => setMenu(m => !m)} title={titulo + ' (cambiar)'} aria-label={'Probar en: ' + titulo} aria-haspopup="listbox" aria-expanded={menu}>
          <Icono n={icono} t={15} /><Icono n="bajar" t={10} />
        </button>
        {menu && (
          <ul className="menu-lanzador" role="listbox" aria-label="Dónde probar">
            {(['interno', 'sistema'] as const).map(d => (
              <li key={d}><button role="option" aria-selected={p.destino === d} onClick={() => { cambia({ destino: d }); setMenu(false); }}>
                <Icono n={DESTINOS[d].icono} t={15} /> {DESTINOS[d].nombre}{p.destino === d && <Icono n="hecho" t={13} />}
              </button></li>
            ))}
            {programas.length > 0 && <li className="menu-titulo">Programas del proyecto</li>}
            {programas.map(c => (
              <li key={c.nombre}><button role="option" aria-selected={p.destino === 'programa' && p.programa === c.nombre} title={c.orden}
                onClick={() => { cambia({ destino: 'programa', programa: c.nombre }); setMenu(false); }}>
                <Icono n="ejecutar" t={14} /> <span className="recorta">{c.nombre}</span>{p.destino === 'programa' && p.programa === c.nombre && <Icono n="hecho" t={13} />}
              </button></li>
            ))}
            {programas.length === 0 && <li className="menu-titulo suave">Sin programas guardados (pestaña Programas)</li>}
          </ul>
        )}
      </div>
      {p.destino === 'programa' ? <>
        <span className={'punto ' + (corriendo ? 'ok' : proc?.estado === 'error' ? 'fallo' : '')} title={proc?.estado || 'parado'} />
        <code className="lanzador-orden recorta" title={programa?.orden}>{programa ? programa.orden : 'Elige un programa del proyecto'}</code>
        {corriendo && proc?.url && <button className="lanzador-enlace recorta" onClick={() => alAbrirWeb(proc.url!, { dispositivo: p.dispositivo, orientacion: p.orientacion })} title={'Abrir ' + proc.url}>{proc.url}</button>}
        {corriendo
          ? <button className="icono" onClick={para} title="Parar" aria-label="Parar el programa"><Icono n="parar" t={14} /></button>
          : <button className="icono primario-suave" onClick={ejecuta} disabled={!programa || !carpeta} title="Ejecutar" aria-label="Ejecutar el programa"><Icono n="ejecutar" t={14} /></button>}
      </> : <>
        <form className="lanzador-url" onSubmit={e => { e.preventDefault(); abre(); }}>
          <input value={url} onChange={e => setUrl(e.target.value)} list={'sug-prueba-' + proyecto.id} spellCheck={false}
            placeholder="Dirección de la prueba (p. ej. localhost:8888)" aria-label="Dirección de la prueba" />
          <datalist id={'sug-prueba-' + proyecto.id}>{sug.map(s => <option key={s} value={s} />)}</datalist>
          <button type="submit" className="icono" disabled={!url.trim()} title={p.destino === 'sistema' ? 'Abrir en el navegador del sistema' : 'Abrir en el navegador interno'} aria-label="Abrir">
            <Icono n="externo" t={14} />
          </button>
        </form>
        <Formato dispositivo={p.dispositivo} orientacion={p.orientacion} desactivado={p.destino === 'sistema'}
          alCambiar={(d, o) => { cambia({ dispositivo: d, orientacion: o }); if (p.destino === 'interno' && L.normalizaUrl(url)) alAbrirWeb(L.normalizaUrl(url), { dispositivo: d, orientacion: o }); }} />
      </>}
      {error && <span className="error pequeno">{error}</span>}
    </div>
  );
}
