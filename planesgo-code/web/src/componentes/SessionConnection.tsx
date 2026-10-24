import { useEffect, useState, type ReactNode } from 'react';
import { get, post } from '../api';
import Icono from './Icono';
import Switch from './Switch';

// The parts every connection of a work session shares in its properties (SQL Server, PowerShell remoto…): its state
// from the backend, the actions on it, the read-only/updatable switch and the buttons of an open connection.

// useSessionConnection: the connection of a session at `${api}` (GET state; POST `${api}/<action>`). name: which of
// the session's connections, when it can have several (SQL Server); "" the main one.
export function useSessionConnection<V>(api: string, project: number, session: string, name = '') {
  const [view, setView] = useState<V | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [note, setNote] = useState('');

  useEffect(() => {
    setView(null); setError(''); setNote('');
    get<V>(`${api}?project=${project}&session=${encodeURIComponent(session)}${name ? '&name=' + encodeURIComponent(name) : ''}`).then(setView).catch(e => setError(String(e.message || e)));
  }, [api, project, session, name]);

  // act: an action that answers with the new state (connect, disconnect, mode…).
  const act = (action: string, body: object = {}) => {
    setBusy(true); setError(''); setNote('');
    return post<V>(`${api}/${action}`, { project, session, ...(name ? { name } : {}), ...body }).then(setView)
      .catch(e => setError(String(e.message || e))).finally(() => setBusy(false));
  };
  // probe: an order whose answer becomes the note under the connection («Probar»).
  const probe = <R,>(action: string, body: object, describe: (r: R) => string) => {
    setBusy(true); setError(''); setNote('');
    post<R>(`${api}/${action}`, { project, session, ...(name ? { name } : {}), ...body }).then(r => setNote(describe(r)))
      .catch(e => setError(String(e.message || e))).finally(() => setBusy(false));
  };
  return { view, busy, error, setError, note, act, probe };
}

// ModeSwitch: read only (off) or updatable (on), before connecting and while connected.
export function ModeSwitch({ changes, disabled, onChange, onText, offText }: {
  changes: boolean; disabled?: boolean; onChange: (changes: boolean) => void; onText: string; offText: string;
}) {
  return <>
    <Switch on={changes} disabled={disabled} onChange={onChange}>Actualizable</Switch>
    <span className="suave pequeno">{changes ? onText : offText}</span>
  </>;
}

// ConnectionSection: the title, what the connection is for, and the open connection or the form to open it.
// chooser: above it, which of several connections (SQL Server).
export function ConnectionSection({ title, intro, connected, error, children, chooser }: {
  title: string; intro: string; connected: ReactNode; error: string; children: ReactNode; chooser?: ReactNode;
}) {
  return <>
    <h3>{title}</h3>
    <span className="suave pequeno">{intro}</span>
    {chooser}
    {connected || children}
    {error && <div className="aviso-txt pequeno" role="alert">{error}</div>}
  </>;
}

// OpenConnection: what an open connection shows: its data, the mode switch and «Probar» / «Desconectar».
export function OpenConnection({ rows, note, busy, onProbe, onDisconnect, children }: {
  rows: [string, string][]; note: string; busy: boolean; onProbe: () => void; onDisconnect: () => void; children?: ReactNode;
}) {
  return <>
    <dl className="session-ids">
      {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
    </dl>
    {children}
    {note && <p className="suave pequeno" role="status">{note}</p>}
    <div className="fila-config app-actions">
      <button type="button" className="boton-secundario" disabled={busy} onClick={onProbe}><Icono n="recargar" t={13} /> Probar</button>
      <button type="button" className="boton-secundario peligro" disabled={busy} onClick={onDisconnect}>Desconectar</button>
    </div>
  </>;
}
