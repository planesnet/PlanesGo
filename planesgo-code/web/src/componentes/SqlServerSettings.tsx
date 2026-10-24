import { useEffect, useState } from 'react';
import { authKind, readLastSql, saveLastSql, SQL_MAIN, sqlFormError, sqlNameError, sqlNames, sqlStatusText, type SqlConfig, type SqlView } from '../sqlConnection';
import { CampoClave } from './Acceso';
import { ConnectionSection, ModeSwitch, OpenConnection, useSessionConnection } from './SessionConnection';

// «SQL Server» (session properties): the connections of this work session, each with its name and its own mode
// (one against the test server read only, another against the local container that allows changes…). Its agents use
// them with «pgo sql [--conexion nombre]», its programs and pgocode's services from the backend; the password stays in
// the backend's memory until pgocode closes.

const MODE_TEXTS = {
  onText: 'Activado: los agentes pueden guardar cambios pidiéndolo expresamente (pgo sql --escribir). Cada orden queda en la auditoría.',
  offText: 'Desactivado (solo lectura): lo que cambie una orden se deshace al acabar. Cada orden queda en la auditoría.',
};

const TEST_SQL = 'SELECT @@SERVERNAME AS servidor, DB_NAME() AS base_de_datos, SUSER_SNAME() AS usuario';

export default function SqlServerSettings({ project, session, login }: { project: number; session: string; login: string }) {
  const [name, setName] = useState(SQL_MAIN);
  const [newName, setNewName] = useState('');
  const { view: v, busy, error, setError, note, act, probe } = useSessionConnection<SqlView>('/api/sql', project, session, name === SQL_MAIN ? '' : name);
  const [c, setC] = useState<SqlConfig>(() => readLastSql(login, project));
  const [password, setPassword] = useState('');
  const [names, setNames] = useState<string[]>([SQL_MAIN]);
  useEffect(() => { if (v) setNames(n => sqlNames([...n, ...(v.names || [])], name)); }, [v, name]);

  const choose = (n: string) => { setName(n); setC(readLastSql(login, project, n)); setPassword(''); };
  const add = () => {
    const n = newName.trim().toLowerCase();
    const why = sqlNameError(n);
    if (why) { setError(why); return; }
    setNewName('');
    setNames(l => sqlNames(l, n));
    choose(n);
  };
  const connect = () => {
    const why = sqlFormError(c, password);
    if (why) { setError(why); return; }
    saveLastSql(login, project, c, name);
    act('connect', { ...c, password }).finally(() => setPassword(''));
  };
  const set = (k: keyof SqlConfig, value: string | boolean) => setC(x => ({ ...x, [k]: value }));

  return (
    <ConnectionSection title="SQL Server" error={error}
      intro="Las conexiones de esta sesión, cada una con su nombre y su modo: las usan sus agentes («pgo sql», o «pgo sql --conexion nombre» para otra que no sea la principal), sus programas y los servicios de pgocode. Las contraseñas se quedan en la memoria de pgocode hasta que lo cierras; nadie más las ve."
      chooser={
        <div className="fila-config">
          <label>Conexión
            <select value={name} disabled={busy} onChange={e => choose(e.target.value)}>
              {names.map(n => <option key={n} value={n}>{n}{n === SQL_MAIN ? ' (la de pgo sql sin --conexion)' : ''}{v?.names?.includes(n) ? ' · conectada' : ''}</option>)}
            </select>
          </label>
          <label>Otra
            <input value={newName} onChange={e => setNewName(e.target.value)} placeholder="nombre, p. ej. pruebas" spellCheck={false}
              onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); add(); } }} />
          </label>
          <button type="button" disabled={busy || !newName.trim()} onClick={add}>Añadir</button>
        </div>
      }
      connected={v?.connected && (
        <OpenConnection busy={busy} note={note}
          rows={[['Conexión', sqlStatusText(v)], ...(v.server_version ? [['Servidor', `SQL Server ${v.server_version}`] as [string, string]] : [])]}
          onProbe={() => probe<{ sets: { rows: unknown[][] }[] }>('run', { sql: TEST_SQL }, r => {
            const x = r.sets[0]?.rows[0] || [];
            return `Responde: servidor ${x[0] ?? '—'}, base de datos ${x[1] ?? '—'}, usuario ${x[2] ?? '—'}.`;
          })}
          onDisconnect={() => act('disconnect')}>
          <label>Base de datos
            <select value={v.config?.database || ''} disabled={busy} onChange={e => act('database', { database: e.target.value })}>
              <option value="">La del usuario (por defecto)</option>
              {(v.databases || []).map(d => <option key={d} value={d}>{d}</option>)}
            </select>
          </label>
          <ModeSwitch changes={!!v.config?.allow_changes} disabled={busy} onChange={on => act('mode', { allow_changes: on })} {...MODE_TEXTS} />
        </OpenConnection>
      )}>
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
      <ModeSwitch changes={c.allow_changes} onChange={on => set('allow_changes', on)} {...MODE_TEXTS} />
      <label className="casilla"><input type="checkbox" checked={c.trust_certificate} onChange={e => set('trust_certificate', e.target.checked)} /> Aceptar el certificado del servidor sin comprobarlo</label>
      <span className="suave pequeno">Lo habitual en un servidor de la red local con certificado propio. La conexión va cifrada igualmente.</span>
      <div className="fila-config app-actions">
        <button type="button" className="primario" disabled={busy} onClick={connect}>{busy ? 'Conectando…' : 'Conectar'}</button>
      </div>
    </ConnectionSection>
  );
}
