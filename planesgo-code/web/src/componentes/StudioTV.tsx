import { useEffect, useRef, useState, type FormEvent } from 'react';
import { get, post, type Proyecto } from '../api';
import Icono from './Icono';

interface Marcador { t: number; txt: string }
interface Estado { caso?: string; titulo?: string; paso?: string; estado?: string; marcadores?: Marcador[] }
interface Canal {
  canal: string; en_directo: boolean; ultima: string; espectadores: number; con_imagen: boolean;
  grabando?: boolean; graba_desde?: string; estado: Estado;
}
interface Grabacion { id: string; canal: string; inicio: string; dur: number; bytes: number; marcadores: Marcador[] }
interface Conexion { url: string; local: boolean }
interface Local { url: string; pco_url: string; pco_token: string }
interface InfoConexion { conexion?: Conexion; local?: Local }

const mmss = (s: number) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, '0')}`;
const tamano = (b: number) => b > 1 << 20 ? (b / (1 << 20)).toFixed(1) + ' MB' : Math.max(1, Math.round(b / 1024)) + ' KB';
const fecha = (iso: string) => new Date(iso).toLocaleString('es-ES', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });

// Artefacto Studio TV: directo de los canales del estudio (el local del arnés o uno externo) y, si se pide,
// grabación en vídeo con marcadores. Por defecto solo directo. Todo pasa por el backend: el token de ver no
// llega al navegador.
export default function StudioTV({ proyecto }: { proyecto: Proyecto }) {
  const [info, setInfo] = useState<InfoConexion | null>(null);
  const [error, setError] = useState('');
  const q = 'proyecto=' + proyecto.id;

  const carga = () => get<InfoConexion>('/api/studio/conexion?' + q).then(setInfo).catch(e => setError(e.message));
  useEffect(() => { carga(); }, [proyecto.id]);

  const conecta = async (cuerpo: object) => {
    setError('');
    try { await post('/api/studio/conexion', { proyecto: proyecto.id, ...cuerpo }); await carga(); }
    catch (e) { setError((e as Error).message); }
  };

  if (!info) return <p className="suave relleno">{error || 'Cargando…'}</p>;
  if (!info.conexion) return <Conectar local={info.local} error={error} alConectar={conecta} />;
  return <Directo q={q} conexion={info.conexion} local={info.local} alDesconectar={() => conecta({ quitar: true })} />;
}

function Conectar({ local, error, alConectar }: { local?: Local; error: string; alConectar: (c: object) => void }) {
  const [url, setUrl] = useState('https://planesgo.autopyme.com/studio-tv');
  const [token, setToken] = useState('');
  const enviar = (e: FormEvent) => { e.preventDefault(); alConectar({ url, token }); };
  return (
    <div className="studio-conectar relleno">
      {local && (
        <section className="caja">
          <div className="caja-titulo"><Icono n="tv" /> <b>Estudio local</b></div>
          <p className="suave">Las pruebas de este equipo emiten aquí.</p>
          <Emisor local={local} />
          <button className="primario" onClick={() => alConectar({ local: true })}>Ver</button>
        </section>
      )}
      <form className="caja" onSubmit={enviar}>
        <div className="caja-titulo"><Icono n="web" /> <b>Otro estudio</b></div>
        <label>URL<input value={url} onChange={e => setUrl(e.target.value)} required /></label>
        <label>Token de ver<input type="password" value={token} onChange={e => setToken(e.target.value)} required autoComplete="off" /></label>
        <button className="primario">Conectar</button>
      </form>
      {error && <p className="error" role="alert">{error}</p>}
    </div>
  );
}

function Emisor({ local }: { local: Local }) {
  const linea = `export PCO_URL=${local.pco_url} PCO_TOKEN=${local.pco_token} PCO_CANAL=<nombre>`;
  const [copiado, setCopiado] = useState(false);
  return (
    <div className="emisor">
      <code>{linea}</code>
      <button className="icono" title={copiado ? 'Copiado' : 'Copiar la configuración del emisor'} aria-label="Copiar"
        onClick={() => navigator.clipboard.writeText(linea).then(() => { setCopiado(true); setTimeout(() => setCopiado(false), 1500); })}>
        <Icono n={copiado ? 'hecho' : 'copiar'} t={16} />
      </button>
    </div>
  );
}

function Directo({ q, conexion, local, alDesconectar }: { q: string; conexion: Conexion; local?: Local; alDesconectar: () => void }) {
  const [canales, setCanales] = useState<Canal[] | null>(null);
  const [error, setError] = useState('');
  const [canal, setCanal] = useState<string | null>(null);
  const [tic, setTic] = useState(0);
  const [verEmisor, setVerEmisor] = useState(false);

  // Consulta encadenada (la siguiente sale cuando acaba la anterior, con 5 s de límite): un estudio lento no
  // acumula peticiones. Mientras se ve un canal no se consulta la parrilla.
  useEffect(() => {
    if (canal) return;
    let vivo = true;
    let espera: ReturnType<typeof setTimeout>;
    const carga = async () => {
      try {
        const r = await fetch('/api/studio/canales?' + q, { credentials: 'same-origin', signal: AbortSignal.timeout(5000) });
        if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || r.statusText);
        const l = await r.json() as Canal[];
        if (vivo) { setCanales(l); setError(''); setTic(t => t + 1); }
      } catch (e) {
        if (vivo) setError((e as Error).name === 'TimeoutError' ? 'El estudio no responde' : (e as Error).message);
      }
      if (vivo) espera = setTimeout(carga, 2000);
    };
    carga();
    return () => { vivo = false; clearTimeout(espera); };
  }, [q, canal]);

  if (canal) return <UnCanal q={q} canal={canal} alVolver={() => setCanal(null)} />;

  const enDirecto = canales?.filter(c => c.en_directo).length || 0;
  return (
    <div className="studio">
      <div className="studio-barra">
        <span className={'onair' + (enDirecto ? ' on' : '')}>{enDirecto ? `${enDirecto} en directo` : 'Sin señal'}</span>
        <span className="suave pequeno recorta" title={conexion.url}>{conexion.local ? 'Local' : conexion.url}</span>
        <span className="hueco" />
        {conexion.local && local && (
          <button className="icono" title="Configuración del emisor" aria-label="Configuración del emisor" aria-pressed={verEmisor} onClick={() => setVerEmisor(v => !v)}>
            <Icono n="copiar" t={16} />
          </button>
        )}
        <button className="icono" title="Cambiar de estudio" aria-label="Cambiar de estudio" onClick={alDesconectar}><Icono n="desconectar" t={16} /></button>
      </div>
      {error && <p className="error relleno">{error}</p>}
      {verEmisor && local && <div className="relleno"><Emisor local={local} /></div>}
      <div className="parrilla">
        {canales?.length === 0 && <p className="suave relleno">Sin canales. Aparecen al llegar la primera imagen.</p>}
        {canales?.map(c => {
          const on = c.en_directo && c.estado?.estado !== 'fin';
          return (
            <button key={c.canal} className="canal" onClick={() => setCanal(c.canal)}>
              <div className="mini">
                {c.con_imagen
                  ? <img alt={'Canal ' + c.canal} src={`/api/studio/snapshot?${q}&canal=${encodeURIComponent(c.canal)}&n=${tic}`} />
                  : <span className="suave">Sin imagen</span>}
                {c.grabando && <span className="rec" title="Grabando">REC</span>}
              </div>
              <div className="pie">
                <span className="nombre">{c.canal}{c.estado?.caso ? ' · ' + c.estado.caso : ''}</span>
                <span className={'onair' + (on ? ' on' : '')}>{on ? 'Directo' : c.estado?.estado === 'fin' ? 'Fin' : 'Sin señal'}</span>
                <span className="paso">{c.estado?.titulo || c.estado?.paso || ''}</span>
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}

function UnCanal({ q, canal, alVolver }: { q: string; canal: string; alVolver: () => void }) {
  const [estado, setEstado] = useState<Estado>({});
  const [n, setN] = useState(0);
  const base = `${q}&canal=${encodeURIComponent(canal)}`;

  // Si el canal aún no existe o se corta, EventSource no siempre reintenta: se reconecta a mano.
  useEffect(() => {
    let es: EventSource | null = null;
    let espera: ReturnType<typeof setTimeout>;
    const escucha = () => {
      es = new EventSource('/api/studio/events?' + base);
      es.onmessage = ev => { try { const e = JSON.parse(ev.data); if (e && Object.keys(e).length) setEstado(e); } catch { /* ignorar */ } };
      es.onerror = () => { if (es?.readyState === EventSource.CLOSED) espera = setTimeout(escucha, 3000); };
    };
    escucha();
    return () => { clearTimeout(espera); es?.close(); };
  }, [base]);

  const on = estado.estado === 'curso';
  return (
    <div className="studio uno">
      <div className="studio-barra">
        <button className="icono" title="Canales" aria-label="Volver a los canales" onClick={alVolver}><Icono n="atras" /></button>
        <b className="recorta">{canal}</b>
        <span className={'onair' + (on ? ' on' : '')}>{on ? 'Directo' : estado.estado === 'fin' ? 'Fin' : 'Sin señal'}</span>
        <span className="hueco" />
        <Grabar base={base} />
      </div>
      <div className="pantalla">
        <img alt={'Directo del canal ' + canal} src={`/api/studio/stream?${base}&n=${n}`} onError={() => setTimeout(() => setN(x => x + 1), 2000)} />
      </div>
      <div className="escaleta relleno">
        {estado.caso && <div className="caso">{estado.caso}</div>}
        {(estado.titulo || estado.caso) && <h3>{estado.titulo || 'Caso ' + estado.caso}</h3>}
        {estado.paso && <div className="paso-actual">{estado.paso}</div>}
        <ol>{(estado.marcadores || []).map((m, i) => <li key={i}>{m.txt} <span className="t">{mmss(m.t || 0)}</span></li>)}</ol>
      </div>
      <Grabaciones base={base} />
    </div>
  );
}

// Botón de grabación del canal: por defecto no se graba; se activa y se para a mano.
function Grabar({ base }: { base: string }) {
  const [desde, setDesde] = useState<number | null>(null);
  const [ahora, setAhora] = useState(Date.now());
  const [error, setError] = useState('');
  const [ocupado, setOcupado] = useState(false);

  // Estado inicial (el canal puede estar ya grabándose desde otra ventana).
  useEffect(() => {
    const canal = new URLSearchParams(base).get('canal');
    get<Canal[]>('/api/studio/canales?' + base).then(l => {
      const c = l.find(x => x.canal === canal);
      setDesde(c?.grabando && c.graba_desde ? Date.parse(c.graba_desde) : null);
    }).catch(() => {});
  }, [base]);

  useEffect(() => {
    if (desde === null) return;
    const id = setInterval(() => setAhora(Date.now()), 1000);
    return () => clearInterval(id);
  }, [desde]);

  const cambia = async () => {
    setOcupado(true); setError('');
    try {
      const r = await post<{ grabando: boolean; graba_desde?: string }>('/api/studio/grabar?' + base, { grabar: desde === null });
      setDesde(r.grabando && r.graba_desde ? Date.parse(r.graba_desde) : null);
      if (!r.grabando) window.dispatchEvent(new Event('pgo:grabaciones'));
    } catch (e) { setError((e as Error).message); }
    finally { setOcupado(false); }
  };

  const grabando = desde !== null;
  return (
    <>
      {error && <span className="error pequeno recorta" title={error}>{error}</span>}
      <button className={'icono grabar' + (grabando ? ' activo' : '')} onClick={cambia} disabled={ocupado}
        title={grabando ? 'Parar la grabación' : 'Grabar en vídeo'} aria-label={grabando ? 'Parar la grabación' : 'Grabar en vídeo'} aria-pressed={grabando}>
        <Icono n={grabando ? 'parar' : 'grabar'} t={16} />
        {grabando && <span className="t">{mmss(Math.max(0, (ahora - desde) / 1000))}</span>}
      </button>
    </>
  );
}

// Vídeos grabados del canal, con sus marcadores (pulsar un marcador salta a ese segundo).
function Grabaciones({ base }: { base: string }) {
  const [lista, setLista] = useState<Grabacion[] | null>(null);
  const [abierta, setAbierta] = useState<string | null>(null);
  const video = useRef<HTMLVideoElement>(null);

  useEffect(() => {
    const carga = () => get<Grabacion[]>('/api/studio/grabaciones?' + base).then(setLista).catch(() => setLista([]));
    carga();
    window.addEventListener('pgo:grabaciones', carga);
    return () => window.removeEventListener('pgo:grabaciones', carga);
  }, [base]);

  if (!lista?.length) return null;
  return (
    <div className="grabaciones relleno">
      <p className="titulo-lista">Grabaciones</p>
      {lista.map(g => {
        const src = `/api/studio/grabacion?${base}&id=${encodeURIComponent(g.id)}`;
        return (
          <div key={g.id} className="grabacion">
            <div className="fila">
              <button className="icono" title={abierta === g.id ? 'Cerrar' : 'Ver'} aria-label={abierta === g.id ? 'Cerrar' : 'Ver'} onClick={() => setAbierta(a => a === g.id ? null : g.id)}>
                <Icono n={abierta === g.id ? 'cerrar' : 'video'} t={16} />
              </button>
              <span className="recorta">{fecha(g.inicio)}</span>
              <span className="t">{mmss(g.dur)} · {tamano(g.bytes)}</span>
              <span className="hueco" />
              <a className="icono" href={src} download={`${g.canal}-${g.id}.mp4`} title="Descargar" aria-label="Descargar"><Icono n="descargar" t={16} /></a>
            </div>
            {abierta === g.id && (
              <>
                <video ref={video} src={src} controls className="reproductor" />
                <ol className="marcas">
                  {g.marcadores.map((m, i) => (
                    <li key={i}><button className="enlace" onClick={() => { if (video.current) { video.current.currentTime = m.t; video.current.play(); } }}>
                      <span className="t">{mmss(m.t)}</span> {m.txt}
                    </button></li>
                  ))}
                </ol>
              </>
            )}
          </div>
        );
      })}
    </div>
  );
}
