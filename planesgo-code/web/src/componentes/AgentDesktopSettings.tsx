import { useEffect, useState } from 'react';
import { get, post } from '../api';
import { desktopFormError, desktopStatusText, parseSize, readLastDesktop, saveLastDesktop, type AgentDesktopConfig, type AgentDesktopView } from '../agentDesktop';
import { CampoClave } from './Acceso';
import Icono from './Icono';
import Switch from './Switch';

// «Escritorio del agente» (session properties): a Windows desktop over RDP on a virtual screen of this computer,
// which the session's agents drive with «pgo escritorio». The password goes to FreeRDP and nowhere else.

const POLL_MS = 2000;

export default function AgentDesktopSettings({ project, session, login }: { project: number; session: string; login: string }) {
  const [v, setV] = useState<AgentDesktopView | null>(null);
  const [c, setC] = useState<AgentDesktopConfig>(() => readLastDesktop(login, project));
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const url = `/api/agent-desktop?project=${project}&session=${encodeURIComponent(session)}`;

  useEffect(() => {
    get<AgentDesktopView>(url).then(setV).catch(e => setError(String(e.message || e)));
  }, [url]);
  // While it runs, its state changes on its own (login, recording, end).
  useEffect(() => {
    if (!v?.running) return;
    const t = setInterval(() => get<AgentDesktopView>(url).then(setV).catch(() => {}), POLL_MS);
    return () => clearInterval(t);
  }, [url, v?.running]);

  const act = (f: () => Promise<AgentDesktopView>) => {
    setBusy(true); setError('');
    f().then(setV).catch(e => setError(String(e.message || e))).finally(() => { setBusy(false); setPassword(''); });
  };
  const start = () => {
    const why = desktopFormError(c, password);
    if (why) { setError(why); return; }
    saveLastDesktop(login, project, c);
    const [width, height] = parseSize(c.size) || [0, 0];
    act(() => post<AgentDesktopView>('/api/agent-desktop/start', { project, session, host: c.host, user: c.user, keyboard: c.keyboard, width, height, password }));
  };
  const set = (k: keyof AgentDesktopConfig, value: string) => setC(x => ({ ...x, [k]: value }));
  const s = v?.status;

  return <>
    <h3>Escritorio del agente</h3>
    <span className="suave pequeno">Un escritorio de Windows por RDP en una pantalla virtual de este equipo: lo manejan los agentes de esta sesión («pgo escritorio»: capturas, ratón y teclado). La contraseña va a FreeRDP y no se guarda; cada orden queda en la auditoría.</span>
    {v && !v.supported && <div className="aviso-txt pequeno" role="alert">Solo funciona en Linux (pantalla virtual Xvfb y X11).</div>}
    {v?.supported && v.missing && <div className="aviso-txt pequeno" role="alert">{v.missing}</div>}
    {s && <dl className="session-ids">
      <div><dt>Estado</dt><dd>{desktopStatusText(v)}</dd></div>
      <div><dt>Intercambio</dt><dd><code className="recorta" title={v.exchange}>{v.exchange}</code> <span className="suave pequeno">(en Windows, \\tsclient\Intercambio)</span></dd></div>
      {s.video && <div><dt>Vídeo</dt><dd><code className="recorta" title={s.video}>{s.video}</code></dd></div>}
    </dl>}
    {s?.error && <div className="aviso-txt pequeno" role="alert">{s.error}</div>}
    {v?.running ? <div className="fila-config app-actions">
      <button type="button" className="boton-secundario" disabled={busy} onClick={() => get<AgentDesktopView>(url).then(setV).catch(e => setError(String(e.message || e)))}><Icono n="recargar" t={13} /> Actualizar</button>
      <button type="button" className="boton-secundario peligro" disabled={busy}
        onClick={() => act(() => post<AgentDesktopView>('/api/agent-desktop/stop', { project, session }))}>Parar</button>
    </div> : <>
      <label>Equipo
        <input value={c.host} onChange={e => set('host', e.target.value)} placeholder="nombre o IP (y :puerto si no es el 3389)" spellCheck={false} />
      </label>
      <label>Usuario
        <input value={c.user} onChange={e => set('user', e.target.value)} placeholder="DOMINIO\usuario o usuario" autoComplete="off" spellCheck={false} />
      </label>
      <label>Contraseña
        <CampoClave value={password} onChange={setPassword} autoComplete="new-password" />
      </label>
      <label>Tamaño de la pantalla
        <input value={c.size} onChange={e => set('size', e.target.value)} placeholder="1600x900" spellCheck={false} />
      </label>
      <Switch on={c.keyboard === 'unicode'} onChange={on => set('keyboard', on ? 'unicode' : 'keys')}>Teclado Unicode</Switch>
      <span className="suave pequeno">{c.keyboard === 'unicode'
        ? 'Activado: el texto llega tal cual (acentos, ñ, €) sea cual sea la distribución de teclado de Windows.'
        : 'Desactivado: el texto va como pulsaciones de tecla; solo es fiable en ASCII.'}</span>
      {!s && <span className="suave pequeno">Carpeta de intercambio: <code>{v?.exchange}</code> (en Windows, \\tsclient\Intercambio).</span>}
      <div className="fila-config app-actions">
        <button type="button" className="primario" disabled={busy || !v?.supported || !!v?.missing} onClick={start}>{busy ? 'Iniciando…' : 'Iniciar'}</button>
      </div>
    </>}
    {error && <div className="aviso-txt pequeno" role="alert">{error}</div>}
  </>;
}
