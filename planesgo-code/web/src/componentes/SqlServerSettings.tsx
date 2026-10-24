import { useEffect, useState } from 'react';
import { get, post } from '../api';
import { authKind, readLastSql, saveLastSql, sqlFormError, sqlStatusText, type SqlConfig, type SqlView } from '../sqlConnection';
import { CampoClave } from './Acceso';
import Icono from './Icono';
import Switch from './Switch';

// «SQL Server» (session properties): the connection of this work session. Its agents use it with «pgo sql» and
// pgocode's services from the backend; the password stays in the backend's memory until pgocode closes.

// Read only (off) or updatable (on): before connecting and while connected.
function ModeSwitch({ changes, disabled, onChange }: { changes: boolean; disabled?: boolean; onChange: (changes: boolean) => void }) {
  return <>
    <Switch on={changes} disabled={disabled} onChange={onChange}>Actualizable</Switch>
    <span className="suave pequeno">{changes
      ? 'Activado: los agentes pueden guardar cambios pidiéndolo expresamente (pgo sql --escribir). Cada orden queda en la auditoría.'
      : 'Desactivado (solo lectura): lo que cambie una orden se deshace al acabar. Cada orden queda en la auditoría.'}</span>
  </>;
}

const TEST_SQL = 'SELECT @@SERVERNAME AS servidor, DB_NAME() AS base_de_datos, SUSER_SNAME() AS usuario';

export default function SqlServerSettings({ project, session, login }: { project: number; session: string; login: string }) {
  const [v, setV] = useState<SqlView | null>(null);
  const [c, setC] = useState<SqlConfig>(() => readLastSql(login, project));
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [test, setTest] = useState('');

  useEffect(() => {
    get<SqlView>(`/api/sql?project=${project}&session=${encodeURIComponent(session)}`).then(setV).catch(e => setError(String(e.message || e)));
  }, [project, session]);

  const act = (f: () => Promise<SqlView>) => {
    setBusy(true); setError(''); setTest('');
    f().then(setV).catch(e => setError(String(e.message || e))).finally(() => { setBusy(false); setPassword(''); });
  };
  const connect = () => {
    const why = sqlFormError(c, password);
    if (why) { setError(why); return; }
    saveLastSql(login, project, c);
    act(() => post<SqlView>('/api/sql/connect', { project, session, ...c, password }));
  };
  const probe = () => {
    setBusy(true); setError(''); setTest('');
    post<{ sets: { rows: unknown[][] }[] }>('/api/sql/run', { project, session, sql: TEST_SQL })
      .then(r => { const x = r.sets[0]?.rows[0] || []; setTest(`Responde: servidor ${x[0] ?? '—'}, base de datos ${x[1] ?? '—'}, usuario ${x[2] ?? '—'}.`); })
      .catch(e => setError(String(e.message || e))).finally(() => setBusy(false));
  };
  const set = (k: keyof SqlConfig, value: string | boolean) => setC(x => ({ ...x, [k]: value }));

  return <>
    <h3>SQL Server</h3>
    <span className="suave pequeno">La conexión de esta sesión: la usan sus agentes («pgo sql») y los servicios de pgocode. La contraseña se queda en la memoria de pgocode hasta que lo cierras; nadie más la ve.</span>
    {v?.connected ? <>
      <dl className="session-ids">
        <div><dt>Conexión</dt><dd>{sqlStatusText(v)}</dd></div>
        {v.server_version && <div><dt>Servidor</dt><dd>SQL Server {v.server_version}</dd></div>}
      </dl>
      <label>Base de datos
        <select value={v.config?.database || ''} disabled={busy}
          onChange={e => act(() => post<SqlView>('/api/sql/database', { project, session, database: e.target.value }))}>
          <option value="">La del usuario (por defecto)</option>
          {(v.databases || []).map(d => <option key={d} value={d}>{d}</option>)}
        </select>
      </label>
      <ModeSwitch changes={!!v.config?.allow_changes} disabled={busy}
        onChange={on => act(() => post<SqlView>('/api/sql/mode', { project, session, allow_changes: on }))} />
      {test && <p className="suave pequeno" role="status">{test}</p>}
      <div className="fila-config app-actions">
        <button type="button" className="boton-secundario" disabled={busy} onClick={probe}><Icono n="recargar" t={13} /> Probar</button>
        <button type="button" className="boton-secundario peligro" disabled={busy}
          onClick={() => act(() => post<SqlView>('/api/sql/disconnect', { project, session }))}>Desconectar</button>
      </div>
    </> : <>
      <label>Servidor
        <input value={c.server} onChange={e => set('server', e.target.value)} placeholder="equipo, equipo:puerto o equipo\instancia" spellCheck={false} />
      </label>
      <label>Base de datos
        <input value={c.database} onChange={e => set('database', e.target.value)} placeholder="vacío: la del usuario; se puede cambiar después de conectar" spellCheck={false} />
      </label>
      <label>Usuario <span className="suave pequeno">({c.user ? authKind(c.user) : 'de SQL Server, o DOMINIO\\usuario'})</span>
        <input value={c.user} onChange={e => set('user', e.target.value)} autoComplete="off" spellCheck={false} />
      </label>
      <label>Contraseña
        <CampoClave value={password} onChange={setPassword} autoComplete="new-password" />
      </label>
      <ModeSwitch changes={c.allow_changes} onChange={on => set('allow_changes', on)} />
      <label className="casilla"><input type="checkbox" checked={c.trust_certificate} onChange={e => set('trust_certificate', e.target.checked)} /> Aceptar el certificado del servidor sin comprobarlo</label>
      <span className="suave pequeno">Lo habitual en un servidor de la red local con certificado propio. La conexión va cifrada igualmente.</span>
      <div className="fila-config app-actions">
        <button type="button" className="primario" disabled={busy} onClick={connect}>{busy ? 'Conectando…' : 'Conectar'}</button>
      </div>
    </>}
    {error && <div className="aviso-txt pequeno" role="alert">{error}</div>}
  </>;
}
