import { useEffect, useMemo, useRef, useState } from 'react';
import { get, post, type Proyecto } from '../api';
import Icono from './Icono';

// Visor de pruebas de una campaña (.psf/qa/<campaña>/): casos con su estado, vídeo con marcadores, resultado y
// comentarios; y el panel para lanzar sus pruebas en este equipo (emiten solas al Studio TV local).

interface Marcador { t: number; txt: string }
interface Caso {
  id: string; grupo?: string; titulo?: string; canal?: string; perfil?: string; estado?: string;
  video?: string; marcadores?: Marcador[]; resultado?: string; usabilidad?: string;
}
interface Casos { actualizado?: string; fase?: string; nota?: string; casos: Caso[] }
interface Campana { id: string; nombre: string; carpeta: string; casos: number; fase?: string; actualizado?: string }
interface Ejecucion { id: string; script: string; inicio: string; fin?: string; estado: 'curso' | 'ok' | 'error' | 'parada'; codigo: number; salida: string[] }
interface Comentario { caso: string; t: number; texto: string; autor: string; fecha: string }

const ESTADOS: Record<string, { txt: string; clase: string }> = {
  pendiente: { txt: 'Pendiente', clase: 'pendiente' }, curso: { txt: 'En curso', clase: 'curso' },
  ok: { txt: 'Correcto', clase: 'ok' }, fallo: { txt: 'Fallo', clase: 'fallo' }, na: { txt: 'No aplica', clase: 'na' },
};
const estado = (e?: string) => ESTADOS[e || 'pendiente'] || { txt: e || '—', clase: 'pendiente' };
const mmss = (s: number) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, '0')}`;

export default function Pruebas({ proyecto, alVerDirecto }: { proyecto: Proyecto; alVerDirecto: () => void }) {
  const [campanas, setCampanas] = useState<Campana[] | null>(null);
  const [campana, setCampana] = useState('');
  const [datos, setDatos] = useState<Casos | null>(null);
  const [filtro, setFiltro] = useState('todos');
  const [sel, setSel] = useState<string | null>(null);
  const [comentarios, setComentarios] = useState<Comentario[]>([]);
  const [error, setError] = useState('');
  const q = `proyecto=${proyecto.id}&campana=${encodeURIComponent(campana)}`;

  useEffect(() => {
    get<Campana[]>('/api/qa/campanas?proyecto=' + proyecto.id).then(l => {
      setCampanas(l);
      let pref = '';
      try { pref = localStorage.getItem('pgo:campana:' + proyecto.id) || ''; } catch { /* sin almacenamiento */ }
      setCampana((l.find(c => c.id === pref) || l[0])?.id || '');
    }).catch(e => setError(e.message));
  }, [proyecto.id]);

  const carga = () => {
    if (!campana) return;
    get<Casos>('/api/qa/casos?' + q).then(setDatos).catch(e => setError(e.message));
    get<Comentario[]>('/api/qa/comentarios?' + q).then(setComentarios).catch(() => {});
  };
  useEffect(() => {
    setDatos(null); setSel(null);
    if (!campana) return;
    try { localStorage.setItem('pgo:campana:' + proyecto.id, campana); } catch { /* sin almacenamiento */ }
    carga();
    const id = setInterval(carga, 15000); // el agente o la prueba pueden ir actualizando casos.json
    return () => clearInterval(id);
  }, [campana]); // eslint-disable-line react-hooks/exhaustive-deps

  const casos = datos?.casos || [];
  const cuenta = useMemo(() => {
    const c: Record<string, number> = { todos: casos.length };
    casos.forEach(x => { const e = x.estado || 'pendiente'; c[e] = (c[e] || 0) + 1; });
    return c;
  }, [casos]);
  const visibles = filtro === 'todos' ? casos : casos.filter(x => (x.estado || 'pendiente') === filtro);
  const grupos = useMemo(() => {
    const m = new Map<string, Caso[]>();
    visibles.forEach(c => { const g = c.grupo || 'Sin grupo'; m.set(g, [...(m.get(g) || []), c]); });
    return [...m.entries()];
  }, [visibles]);
  const caso = casos.find(c => c.id === sel) || null;

  if (error) return <p className="error relleno">{error}</p>;
  if (!campanas) return <p className="suave relleno">Cargando…</p>;
  if (!campanas.length) return (
    <div className="relleno suave">
      <p>Este proyecto no tiene campañas de pruebas.</p>
      <p className="pequeno">Una campaña es una carpeta <code>.psf/qa/&lt;nombre&gt;/</code> en la carpeta del proyecto, con <code>visor/casos.json</code> y sus pruebas (<code>*.py</code>).</p>
    </div>
  );

  return (
    <div className="pruebas">
      <div className="pruebas-cabecera">
        <select value={campana} onChange={e => setCampana(e.target.value)} aria-label="Campaña">
          {campanas.map(c => <option key={c.id} value={c.id}>{c.nombre} · {c.casos} casos</option>)}
        </select>
        {datos?.fase && <span className="suave pequeno recorta">{datos.fase}</span>}
        <span className="hueco" />
        {datos?.actualizado && <span className="t" title="Última actualización de casos.json">{datos.actualizado}</span>}
      </div>
      <Lanzar q={q} proyecto={proyecto.id} campana={campana} alVerDirecto={alVerDirecto} />
      <div className="filtros" role="tablist" aria-label="Filtrar por estado">
        {['todos', 'pendiente', 'curso', 'ok', 'fallo', 'na'].filter(e => e === 'todos' || cuenta[e]).map(e => (
          <button key={e} role="tab" aria-selected={filtro === e} className={'filtro ' + (e === 'todos' ? '' : estado(e).clase)} onClick={() => setFiltro(e)}>
            {e === 'todos' ? 'Todos' : estado(e).txt} <b>{cuenta[e] || 0}</b>
          </button>
        ))}
      </div>
      {caso
        ? <DetalleCaso caso={caso} q={q} proyecto={proyecto.id} campana={campana} comentarios={comentarios.filter(c => c.caso === caso.id)}
            alVolver={() => setSel(null)} alComentar={c => setComentarios(l => [...l, c])} />
        : (
          <div className="lista-casos">
            {grupos.map(([g, l]) => (
              <section key={g}>
                <p className="titulo-lista">{g}</p>
                {l.map(c => (
                  <button key={c.id} className="caso-fila" onClick={() => setSel(c.id)}>
                    <span className={'punto ' + estado(c.estado).clase} aria-hidden="true" />
                    <span className="caso-texto">
                      <span className="recorta">{c.titulo || c.id}</span>
                      <span className="suave pequeno">{c.id}{c.canal ? ' · ' + c.canal : ''}</span>
                    </span>
                    {c.video && <Icono n="video" t={14} />}
                    <span className={'estado-txt ' + estado(c.estado).clase}>{estado(c.estado).txt}</span>
                  </button>
                ))}
              </section>
            ))}
          </div>
        )}
    </div>
  );
}

// Lanzar una prueba de la campaña y seguir su salida; la imagen se ve en el Directo.
function Lanzar({ q, proyecto, campana, alVerDirecto }: { q: string; proyecto: number; campana: string; alVerDirecto: () => void }) {
  const [pruebas, setPruebas] = useState<string[]>([]);
  const [script, setScript] = useState('');
  const [ejs, setEjs] = useState<Ejecucion[]>([]);
  const [verSalida, setVerSalida] = useState(true);
  const [error, setError] = useState('');
  const salida = useRef<HTMLPreElement>(null);

  const carga = () => get<{ pruebas: string[]; ejecuciones: Ejecucion[] }>('/api/qa/pruebas?' + q).then(r => {
    setPruebas(r.pruebas);
    setScript(s => s && r.pruebas.includes(s) ? s : r.pruebas[0] || '');
    setEjs(r.ejecuciones);
  }).catch(() => {});
  const ultima = ejs[0];
  const enCurso = ultima?.estado === 'curso';
  useEffect(() => { carga(); }, [q]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!enCurso) return;
    const id = setInterval(carga, 1500);
    return () => clearInterval(id);
  }, [enCurso, q]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { salida.current?.scrollTo(0, salida.current.scrollHeight); }, [ultima?.salida.length]);

  const lanza = async () => {
    setError('');
    try { await post('/api/qa/lanzar', { proyecto, campana, script }); await carga(); setVerSalida(true); }
    catch (e) { setError((e as Error).message); }
  };
  const para = () => ultima && post('/api/qa/parar', { proyecto, campana, id: ultima.id }).then(carga).catch(() => {});

  if (!pruebas.length) return null;
  return (
    <div className="lanzar">
      <div className="fila-lanzar">
        <select value={script} onChange={e => setScript(e.target.value)} aria-label="Prueba" disabled={enCurso}>
          {pruebas.map(p => <option key={p}>{p}</option>)}
        </select>
        {enCurso
          ? <button className="icono grabar activo" onClick={para} title="Detener la prueba" aria-label="Detener la prueba"><Icono n="parar" t={15} /></button>
          : <button className="primario lanzar-boton" onClick={lanza} disabled={!script} title="Lanzar en este equipo, con el navegador visible">▶ Lanzar</button>}
        {ultima && (
          <>
            <span className={'onair' + (enCurso ? ' on' : '')}>{enCurso ? 'En marcha' : { ok: 'Terminada', error: 'Con error', parada: 'Detenida', curso: '' }[ultima.estado]}</span>
            <button className="icono" onClick={alVerDirecto} title="Ver el directo" aria-label="Ver el directo"><Icono n="tv" t={15} /></button>
            <button className="icono" onClick={() => setVerSalida(v => !v)} title="Salida de la prueba" aria-label="Salida de la prueba" aria-pressed={verSalida}><Icono n="bajar" t={14} /></button>
          </>
        )}
      </div>
      {error && <p className="error pequeno">{error}</p>}
      {ultima && verSalida && <pre className="salida" ref={salida}>{ultima.salida.join('\n') || 'Arrancando…'}</pre>}
    </div>
  );
}

function DetalleCaso({ caso, q, proyecto, campana, comentarios, alVolver, alComentar }: {
  caso: Caso; q: string; proyecto: number; campana: string; comentarios: Comentario[];
  alVolver: () => void; alComentar: (c: Comentario) => void;
}) {
  const video = useRef<HTMLVideoElement>(null);
  const [dur, setDur] = useState(0);
  const [texto, setTexto] = useState('');
  const marcas = caso.marcadores || [];
  const src = caso.video ? `/api/qa/media?${q}&archivo=${encodeURIComponent(caso.video)}` : '';
  const salta = (t: number) => { if (video.current) { video.current.currentTime = t; video.current.play().catch(() => {}); } };

  const comenta = async () => {
    const t = video.current?.currentTime || 0;
    if (!texto.trim()) return;
    try { alComentar(await post<Comentario>('/api/qa/comentario', { proyecto, campana, caso: caso.id, t, texto })); setTexto(''); }
    catch { /* el error se ve al reintentar */ }
  };

  return (
    <div className="detalle-caso">
      <div className="detalle-barra">
        <button className="icono" onClick={alVolver} title="Volver a los casos" aria-label="Volver a los casos"><Icono n="atras" /></button>
        <span className="caso-id">{caso.id}</span>
        <span className={'estado-txt marco ' + estado(caso.estado).clase}>{estado(caso.estado).txt}</span>
      </div>
      <h3>{caso.titulo || caso.id}</h3>
      <dl className="ficha">
        {caso.grupo && <><dt>Grupo</dt><dd>{caso.grupo}</dd></>}
        {(caso.canal || caso.perfil) && <><dt>Canal y perfil</dt><dd>{[caso.canal, caso.perfil].filter(Boolean).join(' · ')}</dd></>}
      </dl>
      {src ? (
        <>
          <video ref={video} src={src} controls className="reproductor" onLoadedMetadata={e => setDur(e.currentTarget.duration || 0)}
            onFocus={() => video.current?.pause()} />
          {dur > 0 && marcas.length > 0 && (
            <div className="linea-tiempo" aria-hidden="true">
              {marcas.map((m, i) => <button key={i} className="marca-linea" style={{ left: `${Math.min(100, (m.t / dur) * 100)}%` }} onClick={() => salta(m.t)} title={m.txt} tabIndex={-1} />)}
            </div>
          )}
          <div className="chips-marcas">
            {marcas.map((m, i) => <button key={i} className="chip-marca" onClick={() => salta(m.t)}><span className="t">{mmss(m.t)}</span> {m.txt}</button>)}
          </div>
          <a className="enlace-descarga" href={src} download={`${caso.id}.mp4`}><Icono n="descargar" t={14} /> Descargar este vídeo</a>
        </>
      ) : <p className="suave pequeno">Este caso aún no tiene vídeo.</p>}
      {caso.resultado && <div className="bloque"><p className="titulo-lista">Resultado</p><p>{caso.resultado}</p></div>}
      {caso.usabilidad && <div className="bloque"><p className="titulo-lista">Usabilidad</p><p>{caso.usabilidad}</p></div>}
      <div className="bloque">
        <p className="titulo-lista">Comentarios</p>
        {comentarios.map((c, i) => (
          <p key={i} className="comentario">
            <button className="enlace t" onClick={() => salta(c.t)} title="Ir a este segundo">{mmss(c.t)}</button> {c.texto} <span className="suave pequeno">— {c.autor}</span>
          </p>
        ))}
        <div className="nuevo-comentario">
          <textarea rows={2} value={texto} onChange={e => setTexto(e.target.value)} onFocus={() => video.current?.pause()}
            placeholder={src ? 'Comentario en el segundo actual del vídeo (al escribir se pausa)' : 'Comentario sobre este caso'} />
          <button className="redondo primario" onClick={comenta} disabled={!texto.trim()} title="Guardar comentario" aria-label="Guardar comentario"><Icono n="enviar" t={15} /></button>
        </div>
      </div>
    </div>
  );
}
