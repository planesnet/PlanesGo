import { useState } from 'react';
import { readLastWinRM, saveLastWinRM, winrmFormError, winrmStatusText, winrmTransport, type WinRMConfig, type WinRMResult, type WinRMView } from '../winrmConnection';
import { CampoClave } from './Acceso';
import { ConnectionSection, ModeSwitch, OpenConnection, useSessionConnection } from './SessionConnection';

// «PowerShell remoto» (session properties): the WinRM connection of this work session with a Windows computer. Its
// agents use it with «pgo ps» and pgocode's services from the backend; the password stays in the backend's memory
// until pgocode closes.

const MODE_TEXTS = {
  onText: 'Activado: los agentes pueden cambiar el equipo pidiéndolo expresamente (pgo ps --escribir) y subir ficheros. PowerShell no deshace nada. Cada orden queda en la auditoría.',
  offText: 'Desactivado (solo lectura): solo órdenes de consulta (Get-*, Select-*, Where-*…), en el modo restringido de PowerShell. Evita accidentes, no es una garantía: para eso, un usuario de Windows sin permisos de escritura. Cada orden queda en la auditoría.',
};

const TEST_PS = '"$env:COMPUTERNAME · $env:USERDOMAIN\\$env:USERNAME · PowerShell $($PSVersionTable.PSVersion)"';

export default function WinRMSettings({ project, session, login }: { project: number; session: string; login: string }) {
  const { view: v, busy, error, setError, note, act, probe } = useSessionConnection<WinRMView>('/api/winrm', project, session);
  const [c, setC] = useState<WinRMConfig>(() => readLastWinRM(login, project));
  const [password, setPassword] = useState('');

  const connect = () => {
    const why = winrmFormError(c, password);
    if (why) { setError(why); return; }
    saveLastWinRM(login, project, c);
    act('connect', { ...c, host: c.host.trim(), user: c.user.trim(), password }).finally(() => setPassword(''));
  };
  const set = (k: keyof WinRMConfig, value: string | boolean) => setC(x => ({ ...x, [k]: value }));

  return (
    <ConnectionSection title="PowerShell remoto" error={error}
      intro="La conexión WinRM de esta sesión con un equipo Windows: la usan sus agentes («pgo ps») y los servicios de pgocode. La contraseña se queda en la memoria de pgocode hasta que lo cierras; nadie más la ve."
      connected={v?.connected && (
        <OpenConnection busy={busy} note={note}
          rows={[['Conexión', winrmStatusText(v)], ...(v.server ? [['Equipo', v.server] as [string, string]] : [])]}
          onProbe={() => probe<WinRMResult>('run', { script: TEST_PS }, r =>
            r.exit_code === 0 ? `Responde: ${r.stdout.trim()}.` : `Responde con error: ${r.stderr.trim() || 'código ' + r.exit_code}`)}
          onDisconnect={() => act('disconnect')}>
          <ModeSwitch changes={!!v.config?.allow_changes} disabled={busy} onChange={on => act('mode', { allow_changes: on })} {...MODE_TEXTS} />
        </OpenConnection>
      )}>
      <label>Equipo
        <input value={c.host} onChange={e => set('host', e.target.value)} placeholder="nombre o IP; equipo:puerto si no es 5985 (HTTP) o 5986 (HTTPS)" spellCheck={false} />
      </label>
      <label>Usuario <span className="suave pequeno">(local, o DOMINIO\usuario)</span>
        <input value={c.user} onChange={e => set('user', e.target.value)} autoComplete="off" spellCheck={false} />
      </label>
      <label>Contraseña
        <CampoClave value={password} onChange={setPassword} autoComplete="new-password" />
      </label>
      <ModeSwitch changes={c.allow_changes} onChange={on => set('allow_changes', on)} {...MODE_TEXTS} />
      <label className="casilla"><input type="checkbox" checked={c.https} onChange={e => set('https', e.target.checked)} /> HTTPS (5986)</label>
      <span className="suave pequeno">Sin HTTPS va por HTTP (5985) con cada mensaje cifrado por NTLM. HTTPS necesita un listener HTTPS de WinRM en el equipo.</span>
      {c.https && <>
        <label className="casilla"><input type="checkbox" checked={c.trust_certificate} onChange={e => set('trust_certificate', e.target.checked)} /> Aceptar el certificado del servidor sin comprobarlo</label>
        <span className="suave pequeno">Lo habitual con un certificado propio de la red local. Va cifrado igualmente: {winrmTransport(c)}.</span>
      </>}
      <div className="fila-config app-actions">
        <button type="button" className="primario" disabled={busy} onClick={connect}>{busy ? 'Conectando…' : 'Conectar'}</button>
      </div>
    </ConnectionSection>
  );
}
