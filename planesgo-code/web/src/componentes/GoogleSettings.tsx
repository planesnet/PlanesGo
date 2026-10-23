import { useEffect, useState } from 'react';
import { get } from '../api';
import Icono from './Icono';
import { googleStatusText, scopeNames, sourceText, type GoogleStatus } from '../googleWorkspace';

// «Google Workspace» (Propiedades): Drive, Docs y Sheets en solo lectura para pgocode y sus agentes («pgo drive»),
// con la cuenta de Google que ya usa gcloud en este equipo (sin cliente propio). Si falta el acceso, la orden que
// hay que ejecutar en un terminal.

function CommandLine({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const copy = () => navigator.clipboard.writeText(text).then(() => { setCopied(true); setTimeout(() => setCopied(false), 1500); }).catch(() => {});
  return (
    <div className="fila-config google-command">
      <code>{text}</code>
      <button type="button" className="boton-secundario" onClick={copy} aria-label="Copiar la orden">
        <Icono n={copied ? 'hecho' : 'copiar'} t={13} /> {copied ? 'Copiada' : 'Copiar'}
      </button>
    </div>
  );
}

export default function GoogleSettings() {
  const [s, setS] = useState<GoogleStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const load = (fresh: boolean) => {
    setBusy(true); setError('');
    get<GoogleStatus>('/api/google' + (fresh ? '?probar=1' : '')).then(setS).catch(e => setError(String(e.message || e))).finally(() => setBusy(false));
  };
  useEffect(() => load(false), []);

  return <>
    <h3>Google Workspace</h3>
    <span className="suave pequeno">Drive, Docs y Sheets en <b>solo lectura</b>, para ti y para los agentes (<code>pgo drive buscar|listar|leer</code>). Usa la cuenta de Google con la que has entrado en gcloud en este equipo; el token no se guarda aparte.</span>
    <dl className="session-ids">
      <div><dt>Estado</dt><dd className={s?.conectado ? '' : 'aviso-txt'}>{busy && !s ? 'Comprobando…' : googleStatusText(s)}</dd></div>
      {s?.conectado && <div><dt>Credenciales</dt><dd>{sourceText(s.fuente)}</dd></div>}
      {!!s?.ambitos?.length && <div><dt>Ámbitos</dt><dd>{scopeNames(s.ambitos).join(' · ')}</dd></div>}
      {s?.cuota_en_uso && s.proyecto_cuota && <div><dt>Proyecto de cuota</dt><dd><code>{s.proyecto_cuota}</code></dd></div>}
    </dl>
    {s && !s.conectado && <>
      {s.problema && <div className="aviso-txt pequeno">{s.problema}</div>}
      {s.gcloud ? <>
        <span className="suave pequeno">Ejecuta esto en un terminal (abre la página de Google para dar permiso) y pulsa «Comprobar»:</span>
        <CommandLine text={s.orden} />
        <span className="suave pequeno">Si Google responde «Esta aplicación está bloqueada», usa en su lugar la sesión de gcloud con acceso a Drive (Drive completo; pgocode solo lee):</span>
        <CommandLine text={s.orden_alternativa} />
      </> : <span className="suave pequeno">Instala la CLI de Google Cloud (<code>gcloud</code>) y vuelve aquí.</span>}
    </>}
    {s?.orden_servicios && (s.cuota_en_uso || /gcloud services enable/.test(s.problema || '')) && <>
      <span className="suave pequeno">Si Google pide habilitar las APIs en el proyecto de cuota:</span>
      <CommandLine text={s.orden_servicios} />
    </>}
    {error && <div className="aviso-txt pequeno" role="alert">{error}</div>}
    <div className="fila-config app-actions">
      <button type="button" className="boton-secundario" disabled={busy} onClick={() => load(true)}><Icono n="recargar" t={13} /> Comprobar</button>
    </div>
  </>;
}
