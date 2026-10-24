import { useState } from 'react';
import * as PSF from '../psf';
import Icono from './Icono';

// Strip above the chat of an automatic PSF session: the steps as dots on a line (done, running, waiting for a
// person, rejected, pending), the 4 reviewers as small dots on «Revisión» and what is working right now. The
// details (findings, links, error) only when unfolded.

const openOutside = (url?: string) => {
  if (!url) return;
  if (window.psf) window.psf.abrirFuera(url).catch(() => {});
  else window.open(url, '_blank', 'noopener');
};

const STATUS_TITLE: Record<PSF.StepStatus, string> = {
  done: 'hecho', running: 'en curso', waiting: 'espera a una persona', rejected: 'rechazado', error: 'ha fallado', pending: 'pendiente',
};

export default function PsfStrip({ state: st, busy, now, cost, onStop, onResume, onMerge, onRetry }: {
  state: PSF.PsfState; busy: boolean; now: string; cost: string;
  onStop: () => void; onResume: () => void; onMerge: () => void; onRetry: () => void;
}) {
  const [open, setOpen] = useState(false);
  const reviews = st.reviews || {};
  const stalled = !busy && (PSF.isWorkingPhase(st.phase) || st.phase === 'error');
  const reviewerDot = (r: PSF.ReviewerRole) => {
    const v = reviews[r];
    return v?.verdict === 'APROBADO' ? 'ok' : v?.verdict === 'RECHAZADO' ? 'ko' : v?.running && busy ? 'curso' : '';
  };
  return (
    <section className="psf-strip" aria-label={`Sesión automática PSF de la issue #${st.issue}`}>
      <div className="psf-head">
        <span>
          <button className="enlace" onClick={() => openOutside(st.url)} title={st.title}>#{st.issue}</button>
          {st.pr ? <> · <button className="enlace" onClick={() => openOutside(st.prUrl)} title="Abrir el PR en GitHub">PR #{st.pr}</button></> : null}
          {st.iteration > 0 && <> · iteración {st.iteration}/{st.maxIterations}</>}
        </span>
        <span className="hueco" />
        <span className="suave">{PSF.elapsed(st)} · {cost}</span>
        {busy && <button className="psf-btn" onClick={onStop} title="Parar la sesión automática (se puede reanudar)">Parar</button>}
        {stalled && <button className="psf-btn" onClick={onResume} title="Seguir desde la fase en la que se quedó">Reanudar</button>}
        {st.phase === 'ready' && <button className="psf-btn primario" onClick={onMerge} title="Fusionar el PR (squash), con los checks de GitHub en verde">Fusionar</button>}
        {st.phase === 'blocked' && <button className="psf-btn" onClick={onRetry}>{st.blockedReason === 'ambiguity' ? 'Rehacer la spec' : 'Otra iteración'}</button>}
        <button className="icono" onClick={() => setOpen(o => !o)} aria-expanded={open} title={open ? 'Ocultar el detalle' : 'Ver el detalle'}><Icono n="bajar" t={14} /></button>
      </div>
      <ol className="psf-steps">
        {PSF.STEPS.map(({ step, label }) => {
          const status = PSF.stepStatus(st, step);
          return (
            <li key={step} className={'psf-step ' + status} title={`${label}: ${STATUS_TITLE[status]}`}>
              <span className="psf-dot" />
              <span>
                {label}
                {step === 'review' && (
                  <span className="psf-reviewers">
                    {PSF.REVIEWERS.map(r => <span key={r.role} className={'psf-mini ' + reviewerDot(r.role)}
                      title={`Reviewer-${r.name}: ${reviews[r.role]?.verdict || (reviews[r.role]?.running && busy ? 'revisando' : 'pendiente')}`} />)}
                  </span>
                )}
              </span>
            </li>
          );
        })}
      </ol>
      {now && <div className="psf-now">Ahora: {now}</div>}
      {st.phase === 'error' && st.error && <div className="psf-now psf-error">⚠ {st.error}</div>}
      {open && (
        <div className="psf-detail">
          <p className="suave">Rama <code>{st.branch}</code>{st.base ? <> → <code>{st.base}</code></> : null} · spec <code>{st.specPath}</code></p>
          {PSF.REVIEWERS.filter(r => reviews[r.role]?.verdict).map(r => (
            <details key={r.role}>
              <summary>Reviewer-{r.name}: {reviews[r.role]!.verdict}</summary>
              <pre>{reviews[r.role]!.findings || '—'}</pre>
            </details>
          ))}
        </div>
      )}
    </section>
  );
}
