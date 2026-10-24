import { useEffect, useState } from 'react';
import { get, post } from '../api';
import { externalUntil, maskToken, type ExternalAccess } from '../externalAccess';
import { askConfirm } from './Modal';

// «Acceso externo» (session properties): the person lets an agent that runs outside pgocode (another Claude Code
// session, say) use «pgo» on this session: its agent's desktop, its SQL Server connections, its remote PowerShell…
// the same orders as its own agents, with its time logged here. pgocode leaves the access in a file of the session's
// folder (only this user can read it); it lasts the hours chosen and can be taken back at any moment
// (internal/servidor/externalaccess.go).

const HOURS = [1, 4, 8, 24];

export default function ExternalAccessSettings({ project, session, folder }: { project: number; session: string; folder: string }) {
  const [list, setList] = useState<ExternalAccess[] | null>(null);
  const [label, setLabel] = useState('');
  const [hours, setHours] = useState(8);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [shown, setShown] = useState('');
  const [copied, setCopied] = useState('');
  const copy = (a: ExternalAccess) => navigator.clipboard.writeText(a.env).then(() => setCopied(a.id)).catch(() => setError('No se ha podido copiar: cópialas a mano.'));

  useEffect(() => {
    get<ExternalAccess[]>(`/api/acceso-externo?project=${project}&session=${encodeURIComponent(session)}`).then(setList).catch(e => setError(String(e.message || e)));
  }, [project, session]);

  const run = (f: () => Promise<ExternalAccess[]>) => {
    setBusy(true); setError('');
    f().then(setList).catch(e => setError(String(e.message || e))).finally(() => setBusy(false));
  };
  const give = async () => {
    if (!(await askConfirm(`¿Dar acceso a «${label.trim() || 'agente externo'}» durante ${hours} h? Podrá usar «pgo» en esta sesión como sus agentes: el escritorio del agente, las conexiones con SQL Server y el PowerShell remoto, con su tiempo imputado a tu nombre.`, { okText: 'Dar acceso' }))) return;
    run(() => post<ExternalAccess[]>('/api/acceso-externo', { project, session, folder, label, hours }));
  };
  const revoke = (a: ExternalAccess) => run(() => post<ExternalAccess[]>('/api/acceso-externo/retirar', { project, session, id: a.id }));

  return <>
    <h3>Acceso externo</h3>
    <span className="suave pequeno">
      Deja que un agente de fuera de pgocode (otra sesión de Claude Code, por ejemplo) use «pgo» en esta sesión, como sus agentes:
      el escritorio del agente, las conexiones con SQL Server y el PowerShell remoto. Lo que haga se imputa en esta sesión, a tu nombre.
      El acceso se guarda con la sesión (aquí ves su token y copias las variables para dárselas al agente) y dura, aunque pgocode
      se reinicie, hasta su hora o hasta que lo retires. También queda en <code>.pgocode/acceso-externo.env</code> de la carpeta de
      la sesión (solo lo lee tu usuario; los agentes de pgocode no): el agente lo carga con <code>. .pgocode/acceso-externo.env</code>
      en esa carpeta y usa <code>pgo</code>.
    </span>
    {!!list?.length && (
      <ul className="lista-clientes">
        {list.map(a => (
          <li key={a.id}>
            <span className="recorta"><b>{a.label}</b> · {externalUntil(a)} · token <code>{shown === a.id ? a.token : maskToken(a.token)}</code></span>
            <button type="button" className="enlace pequeno" onClick={() => setShown(shown === a.id ? '' : a.id)}>{shown === a.id ? 'Ocultar' : 'Ver'}</button>
            <button type="button" className="enlace pequeno" onClick={() => copy(a)}>{copied === a.id ? 'Copiadas' : 'Copiar variables'}</button>
            <button type="button" className="enlace pequeno" disabled={busy} onClick={() => revoke(a)}>Retirar</button>
            {shown === a.id && <pre className="pequeno">{a.env}</pre>}
          </li>
        ))}
      </ul>
    )}
    <div className="fila-config">
      <label>Para
        <input value={label} onChange={e => setLabel(e.target.value)} placeholder="agente externo (p. ej. Claude de Sage)" maxLength={80} />
      </label>
      <label>Durante
        <select value={hours} onChange={e => setHours(Number(e.target.value))}>
          {HOURS.map(h => <option key={h} value={h}>{h} h</option>)}
        </select>
      </label>
      <button type="button" className="primario" disabled={busy || !folder} onClick={give}>Dar acceso</button>
    </div>
    {!folder && <p className="suave pequeno">La sesión necesita su carpeta para dejar el acceso.</p>}
    {error && <div className="aviso-txt pequeno" role="alert">{error}</div>}
  </>;
}
