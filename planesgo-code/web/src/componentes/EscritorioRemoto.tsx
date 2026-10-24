// Escritorio remoto (pgocode --rdp): la ventana de pgocode para entrar en otro equipo por escritorio remoto de Windows
// (RDP) o por AnyDesk. Usa la sesión de pgocode (la misma app y el mismo backend); si no hay, el acceso de siempre. El
// proyecto es obligatorio: si la conexión ya lo tiene, se muestra; si no, el buscador de la barra de proyectos. El
// tiempo se imputa en él con el servicio de pgocode.
//  - RDP: la ventana de Windows la abre el ayudante de escritorio de la app (FreeRDP, en este equipo) con todo
//    permitido (portapapeles, carpeta «Intercambio», sonido, micrófono e impresoras) y se cierra desde ella misma.
//    La contraseña va de esta ventana al ayudante y de él a FreeRDP: no pasa por el servidor ni se guarda.
//  - AnyDesk (AnyDesk.tsx): se abre el cliente instalado, que pide él la contraseña; las sesiones entrantes, o sin
//    proyecto, lo piden en un diálogo.
// Las conexiones, sin contraseña, se recuerdan en este navegador. Nada debajo de «Conectar». Los visores solo están en
// la app de escritorio: en el navegador, un aviso.
import { useEffect, useRef, useState, type FormEvent } from 'react';
import { escuchaAvisos, get, remoteOrder, remoteSessions, VIEWERS_ONLY_DESKTOP, type Proyecto, type Usuario } from '../api';
import Acceso, { CampoClave } from './Acceso';
import { AnyDeskForm, AnyDeskUnassigned, useAnyDesk } from './AnyDesk';
import { Dialogs, showMessage } from './Modal';
import ProjectField from './ProjectField';
import { readRemoteKind, readSaved, remember, REMOTE_KIND_KEY, writeSaved, type RemoteKind } from '../remoteConnections';

interface Conexion { equipo: string; usuario: string; proyecto: number }
type Kind = RemoteKind;

export default function EscritorioRemoto() {
  const [usuario, setUsuario] = useState<Usuario | null | undefined>(undefined);
  useEffect(() => { get<Usuario>('/api/yo').then(setUsuario).catch(() => setUsuario(null)); }, []);
  if (usuario === undefined) return null;
  if (!usuario) return <Acceso aviso="" alEntrar={setUsuario} />;
  return <Ventana usuario={usuario} />;
}

function Ventana({ usuario }: { usuario: Usuario }) {
  const [proyectos, setProyectos] = useState<Proyecto[]>([]);
  const [versionApp, setVersionApp] = useState('');
  const [kind, setKind] = useState<Kind>(readRemoteKind);
  // Chosen from the main window (the test bar's list) while this one is open.
  useEffect(() => {
    const f = (e: StorageEvent) => { if (e.key === REMOTE_KIND_KEY) setKind(readRemoteKind()); };
    window.addEventListener('storage', f);
    return () => window.removeEventListener('storage', f);
  }, []);
  const [anydesk, reloadAnyDesk] = useAnyDesk();

  // Es pgocode: su versión, en el título de la ventana (como la principal) y en la tarjeta.
  useEffect(() => {
    get<{ version: string }>('/api/version').then(v => { setVersionApp(v.version); document.title = 'Escritorio remoto · Planes Go Code ' + v.version; }).catch(() => {});
  }, []);
  useEffect(() => { get<Proyecto[]>('/api/proyectos').then(setProyectos).catch(() => {}); }, []);
  const choose = (k: Kind) => { setKind(k); try { localStorage.setItem(REMOTE_KIND_KEY, k); } catch { /* sin almacenamiento */ } };

  return (
    <main className="acceso rdp">
      <div className="tarjeta-acceso">
        <header className="rdp-cabecera">
          <img className="logo" src="/logo.svg" alt="" width="40" height="40" />
          <h1>Escritorio <span>remoto</span>{versionApp && <small className="rdp-version">Planes Go Code {versionApp}</small>}</h1>
        </header>
        <div className="subpestanas rdp-tipo" role="tablist" aria-label="Tipo de conexión">
          <button type="button" role="tab" aria-selected={kind === 'rdp'} onClick={() => choose('rdp')}>Windows (RDP)</button>
          <button type="button" role="tab" aria-selected={kind === 'anydesk'} onClick={() => choose('anydesk')}>AnyDesk</button>
        </div>
        {kind === 'rdp'
          ? <FormularioRdp usuario={usuario} proyectos={proyectos} />
          : <AnyDeskForm user={usuario} projects={proyectos} state={anydesk} reload={reloadAnyDesk} />}
        {!window.psf?.remoto && <p className="rdp-nota" role="note">{VIEWERS_ONLY_DESKTOP}</p>}
      </div>
      <AnyDeskUnassigned login={usuario.login} projects={proyectos} />
      <Dialogs />
    </main>
  );
}

function FormularioRdp({ usuario, proyectos }: { usuario: Usuario; proyectos: Proyecto[] }) {
  const [conexiones, setConexiones] = useState<Conexion[]>(() => readSaved<Conexion>('rdp', usuario.login));
  const [c, setC] = useState<Conexion>(() => conexiones[0] || { equipo: '', usuario: '', proyecto: 0 });
  const [contrasena, setContrasena] = useState('');
  const [enviando, setEnviando] = useState(false);
  const mias = useRef(new Set<string>()); // las conexiones abiertas desde esta ventana (para avisar de sus errores)

  useEffect(() => {
    // Por qué terminó una conexión abierta desde esta ventana: lo dice el ayudante al momento y el servidor en su
    // lista (por si se pierde el aviso).
    const avisa = (id: string, error?: string) => {
      if (mias.current.delete(id) && error) showMessage(error, { level: 'error', title: 'Escritorio remoto' });
    };
    const carga = () => remoteSessions().then(l => {
      for (const a of l) if (a.state === 'ended') avisa(a.id, a.error);
    }).catch(() => {});
    const fuera = escuchaAvisos(a => { if (a.tipo === 'remoto') carga(); });
    const sinAyudante = window.psf?.alRemoto?.(ev => { if (ev.event === 'state' && ev.id) avisa(ev.id, ev.error); });
    const t = setInterval(carga, 5000);
    return () => { clearInterval(t); fuera(); sinAyudante?.(); };
  }, []);

  const cambiaEquipo = (equipo: string) => {
    const g = conexiones.find(x => x.equipo === equipo); // una conexión guardada: su usuario y su proyecto
    setC(g ? { ...g } : x => ({ ...x, equipo }));
  };

  async function conectar(e: FormEvent) {
    e.preventDefault();
    if (!c.proyecto) return;
    setEnviando(true);
    try {
      const r = await remoteOrder({ op: 'rdp', project: c.proyecto, name: c.equipo, host: c.equipo, user: c.usuario, password: contrasena });
      mias.current.add(r.id);
      const l = remember(conexiones, c, (a, b) => a.equipo === b.equipo);
      setConexiones(l);
      writeSaved('rdp', usuario.login, l);
    } catch (err) {
      showMessage((err as Error).message, { level: 'error', title: 'Escritorio remoto' });
    } finally {
      setEnviando(false);
      setContrasena('');
    }
  }

  return (
    <form onSubmit={conectar} className="rdp-formulario">
      <ProjectField login={usuario.login} projects={proyectos} value={c.proyecto} onChange={p => setC(x => ({ ...x, proyecto: p.id }))} />
      <label>Equipo
        <input list="rdp-equipos" value={c.equipo} onChange={e => cambiaEquipo(e.target.value)} placeholder="nombre o IP[:puerto]" required />
        <datalist id="rdp-equipos">{conexiones.map(x => <option key={x.equipo} value={x.equipo}>{x.usuario}</option>)}</datalist>
      </label>
      <label>Usuario
        <input autoComplete="off" value={c.usuario} onChange={e => setC(x => ({ ...x, usuario: e.target.value }))} placeholder="DOMINIO\usuario o usuario" required />
      </label>
      <label>Contraseña
        <CampoClave value={contrasena} onChange={setContrasena} autoComplete="new-password" />
      </label>
      <button className="primario" disabled={enviando || !c.proyecto || !window.psf?.remoto}>{enviando ? 'Conectando…' : 'Conectar'}</button>
    </form>
  );
}
