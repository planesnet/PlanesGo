import { useState } from 'react';
import { authKind, readLastSql, saveLastSql, sqlFormError, sqlStatusText, type SqlConfig, type SqlView } from '../sqlConnection';
import { CampoClave } from './Acceso';
import { ConnectionSection, ModeSwitch, OpenConnection, useSessionConnection } from './SessionConnection';

// «SQL Server» (session properties): the connection of this work session. Its agents use it with «pgo sql» and
// pgocode's services from the backend; the password stays in the backend's memory until pgocode closes.

const MODE_TEXTS = {
  onText: 'Activado: los agentes pueden guardar cambios pidiéndolo expresamente (pgo sql --escribir). Cada orden queda en la auditoría.',
  offText: 'Desactivado (solo lectura): lo que cambie una orden se deshace al acabar. Cada orden queda en la auditoría.',
};

const TEST_SQL = 'SELECT @@SERVERNAME AS servidor, DB_NAME() AS base_de_datos, SUSER_SNAME() AS usuario';

export default function SqlServerSettings({ project, session, login }: { project: number; session: string; login: string }) {
  const { view: v, busy, error, setError, note, act, probe } = useSessionConnection<SqlView>('/api/sql', project, session);
  const [c, setC] = useState<SqlConfig>(() => readLastSql(login, project));
  const [password, setPassword] = useState('');

  const connect = () => {
    const why = sqlFormError(c, password);
    if (why) { setError(why); return; }
    saveLastSql(login, project, c);
    act('connect', { ...c, password }).finally(() => setPassword(''));
  };
  const set = (k: keyof SqlConfig, value: string | boolean) => setC(x => ({ ...x, [k]: value }));

  return (
    <ConnectionSection title="SQL Server" error={error}
      intro="La conexión de esta sesión: la usan sus agentes («pgo sql») y los servicios de pgocode. La contraseña se queda en la memoria de pgocode hasta que lo cierras; nadie más la ve."
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
