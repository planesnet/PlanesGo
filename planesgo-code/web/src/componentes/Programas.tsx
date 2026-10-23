import { useEffect, useRef, useState } from 'react';
import { get, post, type ConfigPrograma, type Proceso, type Proyecto } from '../api';
import Icono from './Icono';
import { askConfirm } from './Modal';

// Programas del proyecto en local (un Odoo, un servicio, lo que se está desarrollando): los guardados en
// .psf/ejecutar.json de la carpeta de la sesión, los que están en marcha (lanzados aquí o por el agente con «pgo»)
// y su salida. Cuando un programa dice su URL, pgocode la abre en el navegador integrado. Aquí solo se ejecutan,
// se paran y se ve su salida: se definen en Propiedades de la sesión → Programas. asSection (Actividad): con su
// solo los que están en marcha (los parados, plegados) y nada si no hay ninguno.
export default function Programas({ proyecto, carpeta, version, alAbrir, asSection }: {
  proyecto: Proyecto; carpeta: string; version: number; alAbrir: (url: string) => void; asSection?: boolean;
}) {
  const [procs, setProcs] = useState<Proceso[]>([]);
  const [cfg, setCfg] = useState<ConfigPrograma[]>([]);
  const [error, setError] = useState('');
  const [verSalida, setVerSalida] = useState<string | null>(null);

  const carga = () => get<{ procesos: Proceso[]; configuraciones: ConfigPrograma[] }>(
    `/api/procesos?proyecto=${proyecto.id}&carpeta=${encodeURIComponent(carpeta)}`)
    .then(r => { setProcs(r.procesos || []); setCfg(r.configuraciones || []); }).catch(e => setError(e.message));
  useEffect(() => { carga(); }, [proyecto.id, carpeta, version]); // eslint-disable-line react-hooks/exhaustive-deps

  const lanza = async (c: ConfigPrograma, guardar = false) => {
    setError('');
    try { await post('/api/procesos/lanzar', { proyecto: proyecto.id, carpeta, ...c, guardar }); setVerSalida(c.nombre); carga(); }
    catch (e) { setError((e as Error).message); }
  };
  const para = async (nombre: string, quitar = false) => {
    try { await post('/api/procesos/parar', { nombre, quitar }); carga(); } catch (e) { setError((e as Error).message); }
  };

  const enMarcha = new Map(procs.map(p => [p.nombre, p]));
  const sinGuardar = procs.filter(p => !cfg.some(c => c.nombre === p.nombre));

  const filas = [...cfg.map(c => ({ c, p: enMarcha.get(c.nombre) })), ...sinGuardar.map(p => ({ c: { nombre: p.nombre, orden: p.orden, url: p.url }, p }))];
  const lista = filas.map(({ c, p }) => (
    <div key={c.nombre} className="programa">
      <div className="programa-fila">
        <span className={'punto ' + (p?.estado === 'curso' ? 'ok' : p?.estado === 'error' ? 'fallo' : '')} title={p?.estado || 'parado'} />
        <span className="programa-texto">
          <b className="recorta">{c.nombre}{p?.lanzado === 'agente' && <span className="etiqueta">agente</span>}</b>
          <code className="recorta" title={c.orden}>{c.orden}</code>
        </span>
        {p?.url && p.estado === 'curso' && (
          <button className="icono" onClick={() => alAbrir(p.url!)} title={'Abrir ' + p.url} aria-label="Abrir en el navegador"><Icono n="web" t={15} /></button>
        )}
        {p && <button className="icono" onClick={() => setVerSalida(v => v === c.nombre ? null : c.nombre)} aria-pressed={verSalida === c.nombre} title="Salida" aria-label="Salida"><Icono n="bajar" t={14} /></button>}
        {p?.estado === 'curso'
          ? <button className="icono" onClick={() => para(c.nombre)} title="Parar" aria-label="Parar"><Icono n="parar" t={14} /></button>
          : <button className="icono" onClick={() => lanza(c)} disabled={!carpeta} title="Ejecutar" aria-label="Ejecutar"><Icono n="ejecutar" t={14} /></button>}
        {p && p.estado !== 'curso' && !cfg.some(x => x.nombre === c.nombre) && (
          <button className="icono" onClick={() => para(c.nombre, true)} title="Quitar de la lista" aria-label="Quitar"><Icono n="cerrar" t={13} /></button>
        )}
      </div>
      {p && verSalida === c.nombre && <Salida nombre={c.nombre} enCurso={p.estado === 'curso'} />}
    </div>
  ));

  // En Actividad: solo los que están en marcha; los guardados y parados, plegados (para ejecutarlos desde aquí).
  if (asSection) {
    const corriendo = lista.filter((_, i) => filas[i].p?.estado === 'curso');
    const resto = lista.filter((_, i) => filas[i].p?.estado !== 'curso');
    return <>
      {(filas.length > 0 || error) && (
        <section>
          {corriendo.length > 0 && <h4 title="Fuera del agente y sin tokens: los lanzados aquí y los que el agente dejó en marcha">Programas en marcha <span className="gh-cuenta">{corriendo.length}</span></h4>}
          <div className="programas">
            {error && <p className="error">{error}</p>}
            {corriendo}
            {resto.length > 0 && (
              <details className="act-hechas">
                <summary className="suave pequeno">{resto.length} {resto.length === 1 ? 'programa parado' : 'programas parados'}</summary>
                {resto}
              </details>
            )}
          </div>
        </section>
      )}
      <Externos proyecto={proyecto} version={version} alAbrir={alAbrir} asSection />
    </>;
  }
  return (
    <div className="programas">
      {error && <p className="error">{error}</p>}
      {lista}
      {filas.length === 0 && !error && <p className="suave pequeno">No hay programas guardados ni en marcha.</p>}
      <Externos proyecto={proyecto} version={version} alAbrir={alAbrir} />
    </div>
  );
}

interface EnRed { grupo: number; pid: number; puertos: number[]; nombre: string; orden: string; carpeta: string; segundos: number }

const edad = (s: number) => s < 3600 ? Math.max(1, Math.round(s / 60)) + ' min' : s < 86400 ? Math.round(s / 3600) + ' h' : Math.round(s / 86400) + ' d';

// Programas externos: los que tienes escuchando en un puerto y no lanzó pgocode (un Odoo que arrancó un agente
// fuera de pgocode…), del proyecto y del resto de tus carpetas de trabajo, para verlos y pararlos.
function Externos({ proyecto, version, alAbrir, asSection }: { proyecto: Proyecto; version: number; alAbrir: (url: string) => void; asSection?: boolean }) {
  const [l, setL] = useState<{ proyecto: EnRed[]; otros: EnRed[] } | null>(null);
  const [error, setError] = useState('');
  const [parando, setParando] = useState<number[]>([]);
  const carga = () => get<{ proyecto: EnRed[]; otros: EnRed[] }>(`/api/procesos/externos?proyecto=${proyecto.id}`).then(setL).catch(() => {});
  useEffect(() => {
    carga();
    const t = setInterval(() => { if (!document.hidden) carga(); }, 10000); // con la ventana oculta, nada
    return () => clearInterval(t);
  }, [proyecto.id, version]); // eslint-disable-line react-hooks/exhaustive-deps
  const para = async (x: EnRed) => {
    if (!(await askConfirm(`¿Parar «${x.orden.slice(0, 120)}» (puerto ${x.puertos.join(', ')})?`, { okText: 'Parar', danger: true }))) return;
    setError('');
    setParando(p => [...p, x.grupo]);
    try { await post('/api/procesos/externos/parar', { proyecto: proyecto.id, grupo: x.grupo }); setTimeout(carga, 1500); setTimeout(carga, 6000); }
    catch (e) { setError((e as Error).message); }
  };
  if (!l || (l.proyecto.length === 0 && l.otros.length === 0)) return null;
  const fila = (x: EnRed) => (
    <div key={x.grupo} className="programa externo">
      <div className="programa-fila">
        <span className={'punto ' + (parando.includes(x.grupo) ? '' : 'ok')} title={parando.includes(x.grupo) ? 'parando…' : 'en marcha'} />
        <span className="programa-texto">
          <b className="recorta" title={x.carpeta}>{x.carpeta.split('/').filter(Boolean).slice(-2).join('/')} <span className="suave pequeno">· {edad(x.segundos)}</span></b>
          <code className="recorta" title={x.orden}>{x.orden}</code>
        </span>
        {x.puertos.slice(0, 2).map(p => (
          <button key={p} className="enlace pequeno" onClick={() => alAbrir(`http://localhost:${p}/`)} title={`Abrir http://localhost:${p}/`}>:{p}</button>
        ))}
        <button className="icono" onClick={() => para(x)} disabled={parando.includes(x.grupo)} title="Parar (todo su grupo de procesos)" aria-label="Parar"><Icono n="parar" t={14} /></button>
      </div>
    </div>
  );
  // En Actividad, si no hay ninguno de este proyecto: una línea plegada.
  if (asSection && l.proyecto.length === 0) return (
    <details className="act-hechas externos" title="Programas escuchando en un puerto que no lanzó pgocode, de tus otras carpetas de trabajo">
      <summary className="suave pequeno">{l.otros.length} {l.otros.length === 1 ? 'programa' : 'programas'} en marcha en otras carpetas (fuera de pgocode)</summary>
      {error && <p className="error">{error}</p>}
      {l.otros.map(fila)}
    </details>
  );
  const Caja = asSection ? 'section' : 'div';
  const titulo = <>Otros programas en marcha <span className="suave pequeno">(fuera de pgocode)</span></>;
  return (
    <Caja className="externos">
      {asSection ? <h4>{titulo}</h4> : <h5>{titulo}</h5>}
      {error && <p className="error">{error}</p>}
      {l.proyecto.length > 0 ? l.proyecto.map(fila) : asSection ? null : <p className="suave pequeno">Ninguno de este proyecto.</p>}
      {l.otros.length > 0 && (
        <details>
          <summary className="suave pequeno">De otras carpetas de trabajo ({l.otros.length})</summary>
          {l.otros.map(fila)}
        </details>
      )}
    </Caja>
  );
}


// Salida del programa: se va actualizando mientras está en marcha.
function Salida({ nombre, enCurso }: { nombre: string; enCurso: boolean }) {
  const [lineas, setLineas] = useState<string[]>([]);
  const caja = useRef<HTMLPreElement>(null);
  useEffect(() => {
    let vivo = true;
    const lee = () => get<{ lineas: string[] }>(`/api/procesos/salida?nombre=${encodeURIComponent(nombre)}&n=400`)
      .then(r => { if (vivo) setLineas(r.lineas || []); }).catch(() => {});
    lee();
    const t = enCurso ? setInterval(() => { if (!document.hidden) lee(); }, 1500) : undefined;
    return () => { vivo = false; if (t) clearInterval(t); };
  }, [nombre, enCurso]);
  useEffect(() => {
    const c = caja.current;
    if (c && c.scrollHeight - c.scrollTop - c.clientHeight < 60) c.scrollTop = c.scrollHeight;
  }, [lineas]);
  return <pre className="salida" ref={caja}>{lineas.join('\n') || 'Sin salida todavía…'}</pre>;
}
