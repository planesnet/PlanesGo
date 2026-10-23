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
          <p className="cleaner-what suave pequeno">El contexto crece con cada paso del agente; el hilo retira lo que ya no hace falta y lo hace caer. Sin gastar tokens.</p>
          <div className="cleaner-now">
            <b>{last ? fmt(last.tokens) : '—'}</b>
            <span className="suave"> {last ? `de contexto · ${String(last.pct).replace('.', ',')} % de ${fmt(window)}` : 'sin datos aún'}</span>
          </div>
          {series.length > 1 && (
            <figure className="cleaner-spark">
              <svg width={W} height={H + 4} viewBox={`0 -2 ${W} ${H + 4}`} role="img" aria-label="Evolución del contexto: sube en cada turno y cae en cada limpieza">
                <path d={CL.sparkPath(series, W, H)} fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" />
                {series.map((p, i) => p.drop && (
                  <circle key={i} cx={pts[i].x} cy={pts[i].y} r="3" className={p.relay ? 'mark-relay' : 'mark-prune'}>
                    <title>{CL.pruneLabel(series, i)}</title>
                  </circle>
                ))}
              </svg>
              <figcaption className="suave pequeno">Cada caída <span className="mark-prune-txt">●</span> es una limpieza{series.some(p => p.relay) && <> (<span className="mark-relay-txt">●</span> relevo)</>}
                {series.map((_, i) => CL.pruneLabel(series, i)).filter(Boolean).slice(-1).map((t, i) => <span key={i}> · la última, {t}</span>)}</figcaption>
            </figure>
          )}
          <dl className="cleaner-dl">
            <dt>Retirado</dt><dd>{fmt(st?.retirados_sesion || 0)} en esta sesión{st?.ultimo_barrido ? ` · última limpieza ${CL.ago(st.ultimo_barrido)}` : ''}</dd>
            <dt>Ahora</dt><dd>{pending > 0 && last ? <>{fmt(pending)} ya no hacen falta → quedaría en {fmt(after)}</> : 'nada que retirar'}</dd>
          </dl>
          {onSweep && (
            <div className="cleaner-sweep">
              <button type="button" className="boton-secundario" onClick={onSweep} disabled={working || (agent === 'antigravity' && relayForced)}
                title={warn ? 'Barrer ahora: ' + warn : 'Retira ya lo que no hace falta, sin esperar al próximo turno'}>
                {sweeping ? 'Barriendo…' : 'Barrer ahora'}
              </button>
              {warn && <span className="aviso-txt pequeno">{warn}</span>}
              {sweepResult && <span className="suave pequeno">{sweepResult}</span>}
            </div>
          )}
          <details className="cleaner-more">
            <summary className="suave pequeno">Detalles</summary>
            <dl className="cleaner-dl">
              <dt>Cómo</dt><dd>{CL.HOW[agent] || 'sin gestión del contexto'}{hint && !sweepResult ? ` · ${hint}` : ''}</dd>
              <dt>Motor</dt><dd className={rules ? 'aviso-txt' : ''}>{CL.engineLine(st)}</dd>
              <dt>Piezas</dt><dd>{counts.in} en contexto · {counts.out} ya retiradas</dd>
              {cal && <>
                <dt>Ajuste</dt><dd title="antigüedad en intercambios · en pasos · tiempo · relación (pivote)">{cal.generales ? 'general (aún poco historial del proyecto)' : `del proyecto, ${CL.ago(cal.fecha)}`}: {CL.paramsLine(cal)}</dd>
                {!cal.generales && <><dt>Relecturas</dt><dd>{CL.overLine(cal)}</dd><dt>Ahorro</dt><dd>{CL.savedLine(cal)}</dd></>}
              </>}
            </dl>
            <div className="cleaner-legend suave pequeno" aria-label="Leyenda de la vigencia">
              <span>Vigencia en el chat:</span>
              <span className="vig-chip" style={{ opacity: 1 }}>0 vital</span>
              <span className="vig-chip" style={{ opacity: 0.8 }}>1</span>
              <span className="vig-chip" style={{ opacity: 0.6 }}>2</span>
              <span className="vig-chip" style={{ opacity: 0.4 }}>3+ se retira</span>
            </div>
          </details>
        </div>
      )}
    </span>
  );
}

function fmt(n: number): string {
  return n >= 1_000_000 ? (n / 1_000_000).toFixed(1).replace('.', ',') + ' M' : n >= 1000 ? Math.round(n / 1000) + ' k' : String(n);
}
