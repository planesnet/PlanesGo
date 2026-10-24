// AnyDesk in pgocode: the form that opens the installed client on a project (AnyDesk asks for the password itself:
// pgocode never sees it) and the dialog that gives a project to the sessions without one (incoming ones, or opened
// from AnyDesk itself): their time is not logged until then. The desktop app's helper (internal/desktop) opens AnyDesk
// and follows its traces to know when each session starts and ends; the server keeps the sessions and their projects.
import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { escuchaAvisos, get, post, remoteOrder, type Proyecto, type RemoteSession, type Usuario } from '../api';
import { Modal, showMessage } from './Modal';
import { BuscaProyecto } from './Proyectos';
import ProjectField from './ProjectField';
import { formatPeer, normalizePeer, readSaved, remember, writeSaved } from '../remoteConnections';

// available: AnyDesk is installed on this computer (only known in the desktop app).
export interface AnyDeskState { available: boolean; sessions: RemoteSession[]; default_project: number; default_project_name: string }
interface SavedPeer { peer: string; project: number }

const TITLE = 'AnyDesk';

// useAnyDesk: the AnyDesk sessions and settings, up to date with the server's notices (and every 15 s, in case one is
// lost), and whether this computer has AnyDesk.
export function useAnyDesk(): [AnyDeskState | null, () => void] {
  const [state, setState] = useState<AnyDeskState | null>(null);
  const load = useCallback(() => {
    Promise.all([get<Omit<AnyDeskState, 'available'>>('/api/anydesk'), window.psf?.visores?.().catch(() => null)])
      .then(([s, v]) => setState({ ...s, available: Boolean(v?.anydesk) })).catch(() => {});
  }, []);
  useEffect(() => {
    load();
    const off = escuchaAvisos(a => { if (a.tipo === 'remoto') load(); });
    const t = setInterval(load, 15000);
    return () => { clearInterval(t); off(); };
  }, [load]);
  return [state, load];
}

const peerLabel = (s: RemoteSession) => {
  const id = s.peer ? formatPeer(s.peer) : 'un equipo desconocido';
  return s.peer_name ? `${s.peer_name} (${id})` : id;
};

export function AnyDeskForm({ user, projects, state, reload }: { user: Usuario; projects: Proyecto[]; state: AnyDeskState | null; reload: () => void }) {
  const [saved, setSaved] = useState<SavedPeer[]>(() => readSaved<SavedPeer>('anydesk', user.login));
  const [c, setC] = useState<SavedPeer>(() => saved[0] || { peer: '', project: 0 });
  const [sending, setSending] = useState(false);
  const unavailable = state !== null && !state.available;

  const changePeer = (peer: string) => {
    const g = saved.find(x => formatPeer(x.peer) === peer || x.peer === peer); // a saved one: its project
    setC(g ? { ...g, peer: formatPeer(g.peer) } : x => ({ ...x, peer }));
  };

  async function connect(e: FormEvent) {
    e.preventDefault();
    const peer = normalizePeer(c.peer);
    if (!c.project) { showMessage('Elige el proyecto al que se imputa el tiempo de la sesión.', { level: 'error', title: TITLE }); return; }
    if (!peer) { showMessage('El ID de AnyDesk son solo cifras (p. ej. 123 456 789) o un alias como equipo@ad.', { level: 'error', title: TITLE }); return; }
    setSending(true);
    try {
      await remoteOrder({ op: 'anydesk', project: c.project, peer });
      const l = remember(saved, { peer, project: c.project }, (a, b) => a.peer === b.peer);
      setSaved(l);
      writeSaved('anydesk', user.login, l);
    } catch (err) {
      showMessage((err as Error).message, { level: 'error', title: TITLE });
    } finally {
      setSending(false);
    }
  }

  async function clearDefault() {
    try { await post('/api/anydesk/default', { project: 0 }); reload(); } catch (err) { showMessage((err as Error).message, { level: 'error', title: TITLE }); }
  }

  return (
    <form onSubmit={connect} className="rdp-formulario">
      <ProjectField login={user.login} projects={projects} value={c.project} onChange={p => setC(x => ({ ...x, project: p.id }))} />
      <label>ID de AnyDesk
        <input list="anydesk-equipos" value={c.peer} onChange={e => changePeer(e.target.value)} placeholder="123 456 789 o equipo@ad" autoComplete="off" required />
        <datalist id="anydesk-equipos">{saved.map(x => <option key={x.peer} value={formatPeer(x.peer)}>{projects.find(p => p.id === x.project)?.nombre}</option>)}</datalist>
      </label>
      {state?.default_project ? (
        <p className="rdp-nota">Entrantes: <b>{state.default_project_name}</b><button type="button" className="enlace" onClick={clearDefault}>Quitar</button></p>
      ) : null}
      <button className="primario" disabled={sending || unavailable} title={unavailable && window.psf ? 'AnyDesk no está instalado en este equipo' : undefined}>
        {sending ? 'Abriendo AnyDesk…' : 'Conectar'}
      </button>
    </form>
  );
}

// AnyDeskUnassigned: asks for the project of an open session without one (from any pgocode window).
export function AnyDeskUnassigned({ login, projects: given }: { login: string; projects?: Proyecto[] }) {
  const [state, reload] = useAnyDesk();
  const [dismissed, setDismissed] = useState<string[]>([]);
  const [loaded, setLoaded] = useState<Proyecto[]>([]);
  const [chosen, setChosen] = useState(0);
  const [asDefault, setAsDefault] = useState(false);
  const [sending, setSending] = useState(false);
  const session = state?.sessions.find(s => !s.project && !dismissed.includes(s.id));
  const projects = given || loaded;
  useEffect(() => { if (session && !given && !loaded.length) get<Proyecto[]>('/api/proyectos').then(setLoaded).catch(() => {}); }, [session, given, loaded.length]);
  if (!session) return null;
  const incoming = session.direction === 'incoming';
  const later = () => { setDismissed(d => [...d, session.id]); setChosen(0); };

  async function assign() {
    if (!session || !chosen) return;
    setSending(true);
    try {
      await post('/api/anydesk/assign', { id: session.id, project: chosen, default: incoming && asDefault });
      setChosen(0);
      setAsDefault(false);
      reload();
    } catch (err) {
      showMessage((err as Error).message, { level: 'error', title: TITLE });
    } finally {
      setSending(false);
    }
  }

  return (
    <Modal title="Sesión de AnyDesk sin proyecto" className="dialog anydesk-asignar" onClose={later}
      footer={<><span className="hueco" /><button className="enlace" onClick={later}>Ahora no</button>
        <button className="primario" disabled={!chosen || sending} onClick={assign}>Imputar</button></>}>
      <div className="anydesk-asignar-cuerpo">
        <p>{incoming ? 'Sesión entrante desde ' : 'Sesión hacia '}<b>{peerLabel(session)}</b>. Su tiempo no se imputa hasta que elijas el proyecto.</p>
        <BuscaProyecto login={login} lista={projects} elegido={chosen} alElegir={p => setChosen(p.id)} />
        {incoming && <label className="casilla"><input type="checkbox" checked={asDefault} onChange={e => setAsDefault(e.target.checked)} /> Usarlo para todas las sesiones entrantes</label>}
      </div>
    </Modal>
  );
}
