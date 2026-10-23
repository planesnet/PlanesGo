// El indicador del hilo de limpieza (el recolector de vigencia de la sesión) junto al pie del redactor: gira mientras
// marca o barre, atenuado sin Ollama («solo reglas»). Al pulsarlo, una ventanita con el estado, cómo se aplica en
// este agente, el motor, las piezas dentro y fuera, lo retirado, el % de contexto y su evolución con las podas
// marcadas, y lo que quedaría tras el próximo barrido. Nada de esto gasta tokens.
import { useEffect, useRef, useState } from 'react';
import type { Mensaje } from '../api';
import type { PruneReport } from '../prune';
import * as CL from '../cleaner';

interface Props {
  agent: string;
  model?: string;
  report?: PruneReport | null;
  busy?: boolean;         // una consulta al recolector en curso (marcando)
  messages: Mensaje[];
  onSweep?: () => void;   // «Barrer ahora»
  sweeping?: boolean;     // el barrido pedido está en marcha
  sweepResult?: string;   // lo que salió del último barrido pedido
  relayForced?: boolean;  // Antigravity: relevo pedido para el próximo envío
}

const W = 220, H = 40;

export default function CleanerIndicator({ agent, model, report, busy, messages, onSweep, sweeping, sweepResult, relayForced }: Props) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (!open) return;
    const fuera = (e: MouseEvent) => { if (box.current && !box.current.contains(e.target as Node)) setOpen(false); };
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('mousedown', fuera);
    document.addEventListener('keydown', esc);
    return () => { document.removeEventListener('mousedown', fuera); document.removeEventListener('keydown', esc); };
  }, [open]);
  const st = report?.estado;
  const working = Boolean(busy || sweeping || (st && st.fase));
  const rules = st?.motor === 'reglas';
  const window = CL.contextWindow(agent, model);
  const series = CL.contextSeries(messages, window);
  const last = series[series.length - 1];
  const counts = CL.pieceCounts(report);
  const pending = report?.pendiente || 0;
  const after = last ? CL.projected(last.tokens, pending) : 0;
  const pts = CL.sparkPoints(series, W, H);
  const line = sweeping ? 'barriendo' : CL.statusLine(st, busy);
  const hint = CL.sweepHint(agent, report, relayForced);
  const warn = CL.cacheWarning(agent, report);
  const cal = report?.calibracion;
  return (
    <span className="cleaner" ref={box}>
      <button className={'cleaner-btn' + (working ? ' working' : '') + (rules ? ' rules' : '')} onClick={() => setOpen(o => !o)}
        aria-label={'Hilo de limpieza: ' + line} aria-expanded={open} title={`Hilo de limpieza: ${line}${rules ? ' · solo reglas' : ''}`}>
        <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden className="cleaner-icon">
          <path d="M8 2a6 6 0 1 1-5.2 3" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
          <path d="M2.2 1.8v3.6h3.6" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
        {last && <span className="cleaner-pct">{String(last.pct).replace('.', ',')} %</span>}
      </button>
      {open && (
        <div className="cleaner-pop" role="dialog" aria-label="Hilo de limpieza">
          <div className="cleaner-title">Hilo de limpieza <span className={'cleaner-state' + (working ? ' on' : '')}>{line}</span></div>
          <dl className="cleaner-dl">
            <dt>Agente</dt><dd>{CL.HOW[agent] || 'sin gestión del contexto'}</dd>
            <dt>Motor</dt><dd className={rules ? 'aviso-txt' : ''}>{CL.engineLine(st)}</dd>
            <dt>Contexto</dt><dd>{last ? <>{String(last.pct).replace('.', ',')} % · {fmt(last.tokens)} de {fmt(window)}</> : 'sin datos aún'}</dd>
            <dt>Piezas</dt><dd>{counts.in} en contexto · {counts.out} fuera</dd>
            {pending > 0 && last && <><dt>Próximo barrido</dt><dd>−{fmt(pending)} fuera de vigencia → {String(CL.pct(after, window)).replace('.', ',')} %{agent === 'antigravity' ? ' (con el relevo)' : ''}</dd></>}
            <dt>Retirados</dt><dd>{fmt(st?.retirados_sesion || 0)} en la sesión{st?.ultimos_retirados ? ` · ${fmt(st.ultimos_retirados)} en el último barrido` : ''}</dd>
            <dt>Último barrido</dt><dd>{CL.ago(st?.ultimo_barrido)}</dd>
          </dl>
          {onSweep && (
            <div className="cleaner-sweep">
              <button type="button" className="boton-secundario" onClick={onSweep} disabled={working || (agent === 'antigravity' && relayForced)}
                title={warn ? 'Barrer ahora: ' + warn : 'Retira ya lo que está fuera de vigencia (3 o más), sin esperar al próximo turno'}>
                {sweeping ? 'Barriendo…' : 'Barrer ahora'}
              </button>
              {warn && <span className="aviso-txt pequeno">{warn}</span>}
              {(sweepResult || hint) && <span className="suave pequeno">{sweepResult || hint}</span>}
            </div>
          )}
          {cal && (
            <dl className="cleaner-dl cleaner-cal" aria-label="Calibración de la vigencia del proyecto">
              <dt>Pesos</dt><dd title="antigüedad en intercambios · en pasos · tiempo · relación (pivote)">{cal.generales ? 'generales' : 'del proyecto'}: {CL.paramsLine(cal)}</dd>
              <dt>Calibrado</dt><dd>{cal.generales ? 'aún no (poco historial)' : `${CL.ago(cal.fecha)} · ${cal.piezas || 0} piezas de ${cal.sesiones || 0} ${cal.sesiones === 1 ? 'sesión' : 'sesiones'}`}</dd>
              {!cal.generales && <><dt>Relecturas</dt><dd>{CL.overLine(cal)}</dd>
                <dt>Ahorro</dt><dd>{CL.savedLine(cal)}</dd></>}
            </dl>
          )}
          {series.length > 1 && (
            <figure className="cleaner-spark">
              <svg width={W} height={H + 4} viewBox={`0 -2 ${W} ${H + 4}`} role="img" aria-label="Evolución del contexto por turno">
                <path d={CL.sparkPath(series, W, H)} fill="none" stroke="currentColor" strokeWidth="1.4" />
                {series.map((p, i) => (p.pruned || p.relay) && (
                  <circle key={i} cx={pts[i].x} cy={pts[i].y} r="3" className={p.pruned ? 'mark-prune' : 'mark-relay'}>
                    <title>{p.pruned ? CL.pruneLabel(series, i) : 'relevo: conversación nueva del agente'}</title>
                  </circle>
                ))}
              </svg>
              <figcaption className="suave pequeno">% de contexto por turno · <span className="mark-prune-txt">●</span> poda{series.some(p => p.relay) && <> · <span className="mark-relay-txt">●</span> relevo</>}
                {series.map((_, i) => CL.pruneLabel(series, i)).filter(Boolean).slice(-2).map((t, i) => <span key={i}> · {t}</span>)}</figcaption>
            </figure>
          )}
          <div className="cleaner-legend suave pequeno" aria-label="Leyenda de la vigencia">
            <span>Vigencia:</span>
            <span className="vig-chip" style={{ opacity: 1 }}>0 vital</span>
            <span className="vig-chip" style={{ opacity: 0.8 }}>1</span>
            <span className="vig-chip" style={{ opacity: 0.6 }}>2</span>
            <span className="vig-chip" style={{ opacity: 0.4 }}>3+ fuera</span>
          </div>
        </div>
      )}
    </span>
  );
}

function fmt(n: number): string {
  return n >= 1_000_000 ? (n / 1_000_000).toFixed(1).replace('.', ',') + ' M' : n >= 1000 ? Math.round(n / 1000) + ' k' : String(n);
}
