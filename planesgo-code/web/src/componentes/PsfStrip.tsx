import { useState } from 'react';
import * as PSF from '../psf';
import Icono from './Icono';

// Strip under the messages of an automatic PSF session: the steps as dots on a line. Inside each dot, its state:
// ✓ done, the round number on the Builder (S while the Supervisor works), ! failed; what runs pulses and its segment flows; what waits for a person
// is amber. The 4 reviewers are small dots on «Revisión». The status in words is the tooltip; the details (findings,
// branch) only when unfolded. When the session needs a decision, a one-line box takes it.

const openOutside = (url?: string) => {
  if (!url) return;
  if (window.psf) window.psf.abrirFuera(url).catch(() => {});
  else window.open(url, '_blank', 'noopener');
};

const STATUS_TITLE: Record<PSF.StepStatus, string> = {
  done: 'hecho', running: 'en curso', waiting: 'espera a una persona', rejected: 'rechazado', error: 'ha fallado', pending: 'pendiente',
};

export default function PsfStrip({ state: st, busy, now, cost, onStop, onResume, onMerge, onRetry, onDiscard }: {
  state: PSF.PsfState; busy: boolean; now: string; cost: string;
  onStop: () => void; onResume: () => void; onMerge: () => void; onRetry: (text: string) => void; onDiscard: (why: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [decision, setDecision] = useState('');
  const reviews = st.reviews || {};
  const stalled = !busy && PSF.isWorkingPhase(st.phase);
  const decide = !busy && PSF.needsDecision(st);
  const approved = PSF.REVIEWERS.filter(r => reviews[r.role]?.verdict === 'APROBADO').length;
  const reviewerDot = (r: PSF.ReviewerRole) => {
    const v = reviews[r];
    return v?.verdict === 'APROBADO' ? 'ok' : v?.verdict === 'RECHAZADO' ? 'ko' : v?.running && busy ? 'curso' : '';
  };
  // What goes inside a dot: ✓ when done, the round on the Builder after the first one, ! when it failed.
  const mark = (step: PSF.PsfStep, status: PSF.StepStatus) =>
    step === 'builder' && st.phase === 'supervisor' ? 'S'
      : step === 'builder' && st.iteration > 1 && status !== 'pending' ? String(st.iteration)
      : status === 'done' ? '✓' : status === 'error' || status === 'rejected' ? '!' : '';
  const submit = (e: React.FormEvent) => { e.preventDefault(); onRetry(decision); setDecision(''); };
  return (
    <section className="psf-strip" aria-label={`Sesión automática PSF de la issue #${st.issue}`} title={PSF.statusText(st, busy)}>
      <div className="psf-head">
        <span>
          <button className="enlace" onClick={() => openOutside(st.url)} title={'Abrir la issue en GitHub: ' + st.title}>Issue #{st.issue}</button>
          {st.pr ? <> → <button className="enlace" onClick={() => openOutside(st.prUrl)} title="Abrir su pull request en GitHub (GitHub numera issues y PR juntos: por eso no coinciden)">PR #{st.pr}</button></> : null}
        </span>
        <span className="hueco" />
        <span className="suave">{PSF.elapsed(st)} · {cost}</span>
        <button className="psf-btn" onClick={() => window.dispatchEvent(new Event(PSF.SHOW_EVIDENCE))} title="Ver en Studio TV las fotos de esta issue (spec, cambios del Builder, decisión del comité)"><Icono n="imagen" t={13} /> Fotos</button>
        {busy && <button className="psf-btn" onClick={onStop} title="Parar la sesión automática (se puede reanudar)">Parar</button>}
        {stalled && <button className="psf-btn" onClick={onResume} title="Seguir desde la fase en la que se quedó">Reanudar</button>}
        {st.phase === 'ready' && !busy && <button className="psf-btn primario" onClick={onMerge} title="Fusionar el PR (squash), con los checks de GitHub en verde">Fusionar</button>}
        <button className="icono" onClick={() => setOpen(o => !o)} aria-expanded={open} title={open ? 'Ocultar el detalle' : 'Ver el detalle'}><Icono n="bajar" t={14} /></button>
      </div>
      <ol className="psf-steps">
        {PSF.STEPS.map(({ step, label }) => {
          const status = PSF.stepStatus(st, step);
          return (
            <li key={step} className={'psf-step ' + status} title={`${label}: ${STATUS_TITLE[status]}${step === 'builder' && st.iteration > 1 ? ` (iteración ${st.iteration}/${st.maxIterations})` : ''}`}>
              <span className="psf-dot">{mark(step, status)}</span>
              <span className="psf-label">
                {label}
                {step === 'review' && (
                  <span className="psf-reviewers" title={`${approved} de 4 aprobados`}>
                    {PSF.REVIEWERS.map(r => <span key={r.role} className={'psf-mini ' + reviewerDot(r.role)}
                      title={`Reviewer-${r.name}: ${reviews[r.role]?.verdict || (reviews[r.role]?.running && busy ? 'revisando' : 'pendiente')}`} />)}
                  </span>
                )}
              </span>
            </li>
          );
        })}
      </ol>
      {now && <div className="psf-now">{now}</div>}
      {decide && (
        <form className="psf-decision" onSubmit={submit}>
          <input value={decision} onChange={e => setDecision(e.target.value)} aria-label="Cómo seguir"
            placeholder={st.phase === 'blocked' && st.blockedReason === 'ambiguity' ? 'Responde a las preguntas del PR…'
              : st.heldReason === 'conflict' ? 'Indicaciones (vacío: integrar la rama base y resolver los conflictos)…' : 'Indicaciones para el agente (opcional)…'} />
          <button className="psf-btn primario">{PSF.decisionLabel(st)}</button>
          <button type="button" className="psf-btn" onClick={() => { onDiscard(decision); setDecision(''); }}
            title="La issue no se hace: el motivo (lo escrito) va al PR, el PR se cierra sin fusionar con su rama, la issue se cierra como «no se hará» y la sesión se archiva">Descartar</button>
        </form>
      )}
      {open && (
        <div className="psf-detail">
          <p className="suave">{PSF.statusText(st, busy)} · rama <code>{st.branch}</code>{st.base ? <> → <code>{st.base}</code></> : null}</p>
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
