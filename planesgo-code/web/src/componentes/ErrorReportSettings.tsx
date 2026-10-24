import { useEffect, useState } from 'react';
import { get, post } from '../api';
import { failureText, statusText, type ErrorReportsView } from '../errorReports';
import Icono from './Icono';

// «Errores» (Propiedades): whether the failures of pgocode open GitHub issues on their own, and the latest ones.

const open = (url: string) => { if (window.psf) void window.psf.abrirFuera(url); else window.open(url, '_blank', 'noopener'); };

export default function ErrorReportSettings() {
  const [v, setV] = useState<ErrorReportsView | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => { get<ErrorReportsView>('/api/errores').then(setV).catch(e => setError(String(e.message || e))); }, []);
  const toggle = (enabled: boolean) => {
    setBusy(true); setError('');
    post<ErrorReportsView>('/api/errores', { enabled }).then(setV).catch(e => setError(String(e.message || e))).finally(() => setBusy(false));
  };

  return <>
    <h3>Errores</h3>
    <label className="casilla"><input type="checkbox" checked={!!v?.enabled} disabled={!v || busy} onChange={e => toggle(e.target.checked)} />
      Abrir una issue en {v?.repo || 'GitHub'} cuando falle pgocode</label>
    <span className="suave pequeno">
      Solo los fallos del propio pgocode: excepciones de la interfaz y del servidor, y cierres inesperados. No los de fuera (Odoo, GitHub, la red o los agentes).
      Si ya hay una issue abierta para el mismo fallo, se comenta en ella como mucho una vez al día; como mucho {v?.max_per_day ?? 5} issues nuevas al día.
      Antes de enviar nada se tapan contraseñas, tokens y tu carpeta personal.
    </span>
    {v && <p className={'pequeno' + (v.enabled && !v.access ? ' aviso-txt' : ' suave')}>{statusText(v)}</p>}
    <span className="suave pequeno">Para que se resuelvan solas, activa «Resolución automática» en los ajustes de Issues del proyecto PLANESGO: cada issue nueva arranca una sesión automática PSF.</span>
    {error && <div className="aviso-txt pequeno" role="alert">{error}</div>}
    {!!v?.recent?.length && <>
      <h4>Últimos fallos en este equipo</h4>
      <ul className="lista-fallos">
        {v.recent.map(k => (
          <li key={k.first + k.title}>
            <span className="recorta" title={k.title}>{k.title.replace(/^Fallo de pgocode: /, '')}</span>
            <span className="suave pequeno">{failureText(k)}</span>
            {k.url && <button type="button" className="enlace pequeno" onClick={() => open(k.url!)}>#{k.issue} <Icono n="externo" t={11} /></button>}
          </li>
        ))}
      </ul>
    </>}
  </>;
}
