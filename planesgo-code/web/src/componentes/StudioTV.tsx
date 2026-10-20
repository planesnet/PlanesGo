import { useEffect, useState, type FormEvent } from 'react';
import { get, post, type Proyecto } from '../api';

interface Marcador { t: number; txt: string }
interface Estado { caso?: string; titulo?: string; paso?: string; estado?: string; marcadores?: Marcador[] }
interface Canal { canal: string; en_directo: boolean; ultima: string; espectadores: number; con_imagen: boolean; estado: Estado }
interface Conexion { url: string; local: boolean }
interface Local { url: string; pco_url: string; pco_token: string }
interface InfoConexion { conexion?: Conexion; local?: Local }

const mmss = (s: number) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, '0')}`;

// Artefacto Studio TV: conecta con un estudio (el local del arnés o uno externo con su token de ver) y muestra la
// parrilla de canales y el directo de cada uno. Todo pasa por el backend: el token de ver no llega al navegador.
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
  const [url, setUrl] = useState('http://127.0.0.1:8080');
  const [token, setToken] = useState('');
  const enviar = (e: FormEvent) => { e.preventDefault(); alConectar({ url, token }); };
  return (
    <div className="studio-conectar relleno">
      <h3>Conectar con un estudio</h3>
      {local && (
        <section className="caja">
          <h4>Estudio local de PSFCode</h4>
          <p className="suave">Lo arranca el propio arnés. Las pruebas que se ejecuten en este equipo emiten aquí con:</p>
          <Emisor local={local} />
          <button className="primario" onClick={() => alConectar({ local: true })}>Ver el estudio local</button>
        </section>
      )}
      <form className="caja" onSubmit={enviar}>
        <h4>Otro estudio (studio-tv)</h4>
        <p className="suave">Copia la URL y el token de la línea «Ver:» que muestra studio-tv al arrancar (<code>…/?t=<b>token</b></code>).</p>
        <label>URL del estudio<input value={url} onChange={e => setUrl(e.target.value)} required /></label>
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
      <button className="enlace" onClick={() => navigator.clipboard.writeText(linea).then(() => { setCopiado(true); setTimeout(() => setCopiado(false), 1500); })}>
        {copiado ? 'Copiado' : 'Copiar'}
      </button>
    </div>
  );
}

function Directo({ q, conexion, local, alDesconectar }: { q: string; conexion: Conexion; local?: Local; alDesconectar: () => void }) {
  const [canales, setCanales] = useState<Canal[] | null>(null);
  const [error, setError] = useState('');
  const [canal, setCanal] = useState<string | null>(null);
  const [tic, setTic] = useState(0);

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
        <span className="suave pequeno" title={conexion.url}>{conexion.local ? 'Estudio local' : conexion.url}</span>
        <button className="enlace" onClick={alDesconectar}>Cambiar</button>
      </div>
      {error && <p className="error relleno">{error}</p>}
      {conexion.local && local && <div className="relleno"><Emisor local={local} /></div>}
      <div className="parrilla">
        {canales?.length === 0 && <p className="suave relleno">Ningún canal todavía. Las pruebas aparecen aquí en cuanto envían la primera imagen.</p>}
        {canales?.map(c => {
          const on = c.en_directo && c.estado?.estado !== 'fin';
          return (
            <button key={c.canal} className="canal" onClick={() => setCanal(c.canal)}>
              <div className="mini">
                {c.con_imagen
                  ? <img alt={'Canal ' + c.canal} src={`/api/studio/snapshot?${q}&canal=${encodeURIComponent(c.canal)}&n=${tic}`} />
                  : <span className="suave">Sin imagen</span>}
              </div>
              <div className="pie">
                <span className="nombre">{c.canal}{c.estado?.caso ? ' · ' + c.estado.caso : ''}</span>
                <span className={'onair' + (on ? ' on' : '')}>{on ? 'En directo' : c.estado?.estado === 'fin' ? 'Terminado' : 'Sin señal'}</span>
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
        <button className="enlace" onClick={alVolver}>← Canales</button>
        <b>{canal}</b>
        <span className={'onair' + (on ? ' on' : '')}>{on ? 'En directo' : estado.estado === 'fin' ? 'Caso terminado' : 'Sin señal'}</span>
      </div>
      <div className="pantalla">
        <img alt={'Directo del canal ' + canal} src={`/api/studio/stream?${base}&n=${n}`} onError={() => setTimeout(() => setN(x => x + 1), 2000)} />
      </div>
      <div className="escaleta relleno">
        <div className="caso">{estado.caso || '—'}</div>
        <h3>{estado.titulo || (estado.caso ? 'Caso ' + estado.caso : 'Sin caso en emisión')}</h3>
        <div className="paso-actual"><span>Paso actual</span>{estado.paso || '—'}</div>
        <ol>{(estado.marcadores || []).map((m, i) => <li key={i}>{m.txt} <span className="t">{mmss(m.t || 0)}</span></li>)}</ol>
      </div>
    </div>
  );
}
