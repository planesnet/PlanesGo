import { useEffect, useRef, useState, type DragEvent } from 'react';
import { get, post } from '../api';
import Icono from './Icono';
import { clientFileProblem, clientText, googleActions, googleStatusText, scopeNames, type GoogleStatus } from '../googleWorkspace';

// «Google Workspace» (Propiedades): Drive, Docs y Sheets en solo lectura para pgocode y sus agentes («pgo drive»).
// Cada usuario conecta su cuenta con «Conectar con Google» (la página de Google se abre en el navegador del sistema:
// Google no deja iniciar sesión en el navegador integrado). La aplicación de Google («PlanesGo») se registra una vez
// por empresa; su fichero del cliente se carga aquí (botón o arrastrándolo), nunca pegado en un campo.

// openOutside: la página de Google, en el navegador del sistema (escritorio) o en otra pestaña (navegador).
const openOutside = (url: string) => window.psf ? window.psf.abrirFuera(url) : Promise.resolve(void window.open(url, '_blank', 'noopener'));

export default function GoogleSettings() {
  const [s, setS] = useState<GoogleStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [authUrl, setAuthUrl] = useState('');
  const [dragging, setDragging] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);

  const load = (fresh: boolean) => {
    setBusy(true); setError('');
    return get<GoogleStatus>('/api/google' + (fresh ? '?probar=1' : '')).then(setS).catch(e => setError(String(e.message || e))).finally(() => setBusy(false));
  };
  useEffect(() => { load(false); }, []);

  // Mientras se espera a Google, se pregunta cada 2 s (el backend recibe la vuelta en su propio puerto).
  const pending = Boolean(s?.en_curso);
  useEffect(() => {
    if (!pending) { setAuthUrl(''); return; }
    const t = setInterval(() => { get<GoogleStatus>('/api/google').then(setS).catch(() => {}); }, 2000);
    return () => clearInterval(t);
  }, [pending]);

  const connect = async () => {
    setBusy(true); setError(''); setNotice('');
    try {
      const r = await post<{ url: string }>('/api/google/conectar', {});
      setAuthUrl(r.url);
      await openOutside(r.url).catch(() => {});
      await load(false);
    } catch (e) { setError(String((e as Error).message || e)); setBusy(false); }
  };

  const disconnect = async () => {
    setBusy(true); setError(''); setNotice('');
    try {
      const r = await post<{ aviso?: string }>('/api/google/desconectar', {});
      if (r.aviso) setNotice(r.aviso);
      await load(false);
    } catch (e) { setError(String((e as Error).message || e)); setBusy(false); }
  };

  const sendClientFile = async (f: File | undefined) => {
    if (!f) return;
    setError(''); setNotice('');
    const problem = clientFileProblem(f.name, f.size);
    if (problem) { setError(problem); return; }
    setBusy(true);
    try {
      await post('/api/google/cliente', { contenido: await f.text() });
      setNotice('Aplicación de Google registrada en este equipo. Ya puedes conectar tu cuenta.');
      await load(false);
    } catch (e) { setError(String((e as Error).message || e)); setBusy(false); }
  };

  const onDrop = (e: DragEvent) => { e.preventDefault(); setDragging(false); sendClientFile(e.dataTransfer.files[0]); };
  const a = googleActions(s);

  return <div className={'google-settings' + (dragging ? ' google-drop' : '')}
    onDragOver={e => { if (a.loadClient) { e.preventDefault(); setDragging(true); } }} onDragLeave={() => setDragging(false)} onDrop={a.loadClient ? onDrop : undefined}>
    <h3>Google Workspace</h3>
    <span className="suave pequeno">Drive, Docs y Sheets en <b>solo lectura</b>, para ti y para los agentes (<code>pgo drive buscar|listar|leer</code>), con tu cuenta de Google. El permiso se guarda solo en este equipo.</span>
    <dl className="session-ids">
      <div><dt>Estado</dt><dd className={s?.conectado ? '' : 'aviso-txt'}>{busy && !s ? 'Comprobando…' : googleStatusText(s)}</dd></div>
      {s?.conectado && !!s.ambitos?.length && <div><dt>Permisos</dt><dd>{scopeNames(s.ambitos).join(' · ')}</dd></div>}
      {s?.cliente && <div><dt>Aplicación de Google</dt><dd>{clientText(s)}</dd></div>}
    </dl>
    {s?.problema && <div className="aviso-txt pequeno">{s.problema}</div>}
    {s && !s.cliente && <span className="suave pequeno">
      La aplicación «PlanesGo» se registra una vez para toda la empresa (Google Cloud → APIs y servicios → Credenciales → cliente
      OAuth de tipo «Aplicación de escritorio» → Descargar JSON). Pide ese fichero a quien administra Google Cloud y cárgalo
      aquí con el botón o arrastrándolo a esta sección.
    </span>}
    {pending && <span className="suave pequeno">
      Se ha abierto la página de Google en tu navegador: elige tu cuenta y acepta. {authUrl && <a href={authUrl} onClick={e => { e.preventDefault(); openOutside(authUrl).catch(() => {}); }}>Abrirla otra vez</a>}
    </span>}
    {notice && <div className="suave pequeno" role="status">{notice}</div>}
    {error && <div className="aviso-txt pequeno" role="alert">{error}</div>}
    <div className="fila-config app-actions">
      {a.connect && <button type="button" className="primario" disabled={busy} onClick={connect}><Icono n="conectar" t={13} /> Conectar con Google</button>}
      {a.disconnect && <button type="button" className="boton-secundario" disabled={busy} onClick={disconnect}><Icono n="desconectar" t={13} /> Desconectar</button>}
      {a.loadClient && <button type="button" className="boton-secundario" disabled={busy} onClick={() => fileInput.current?.click()}>
        <Icono n="carpeta" t={13} /> {s?.cliente ? 'Cambiar el fichero del cliente…' : 'Cargar el fichero del cliente…'}</button>}
      <button type="button" className="boton-secundario" disabled={busy} onClick={() => load(true)}><Icono n="recargar" t={13} /> Comprobar</button>
      <input ref={fileInput} type="file" accept=".json,application/json" hidden onChange={e => { sendClientFile(e.target.files?.[0]); e.target.value = ''; }} />
    </div>
  </div>;
}
