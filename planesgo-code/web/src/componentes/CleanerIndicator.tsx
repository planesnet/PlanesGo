// El indicador del hilo de limpieza (el recolector de vigencia de la sesión) junto al pie del redactor: gira mientras
// marca o barre, atenuado sin Ollama («solo reglas»). Al pulsarlo, una ventanita con el estado, el contexto medido
// contra el umbral de compactación (con el objetivo de la sesión), su evolución con las podas marcadas, lo retirado
// (también por los relevos), lo que quedaría tras el próximo barrido y lo que da y cuesta barrer ahora. El hilo no
// gasta tokens; barrer con la caché caliente puede costar caché.
import { useEffect, useRef, useState } from 'react';
import type { Mensaje } from '../api';
import type { PruneReport } from '../prune';
import * as CL from '../cleaner';
import CacheLine from './CacheLine';

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
  const [confirm, setConfirm] = useState(false);
  const box = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (!open) return;
    const fuera = (e: MouseEvent) => { if (box.current && !box.current.contains(e.target as Node)) setOpen(false); };
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('mousedown', fuera);
    document.addEventListener('keydown', esc);
    return () => { document.removeEventListener('mousedown', fuera); document.removeEventListener('keydown', esc); };
  }, [open]);
  // El resultado del último barrido pedido: se quita al empezar otro turno, al cambiar el informe (otra sesión, el
  // marcado del turno) o pasados unos minutos.
  const users = CL.userCount(messages);
  const [shown, setShown] = useState<CL.SweepShown | null>(null);
  useEffect(() => { setShown(sweepResult ? { text: sweepResult, users, report, at: Date.now() } : null); }, [sweepResult]);
  useEffect(() => { setConfirm(false); }, [report]);
  const st = report?.estado;
  const last0 = messages[messages.length - 1];
  const agentWorking = Boolean(last0 && last0.rol === 'assistant' && !last0.fin && !last0.error && messages.length > 1);
  const working = Boolean(busy || sweeping || (st && st.fase));
  const rules = st?.motor === 'reglas';
  const window = CL.contextWindow(agent, model);
  const threshold = CL.compactAt(report);
  const target = report?.objetivo || 0;
  const series = CL.contextSeries(messages, threshold);
  const last = series[series.length - 1];
  const counts = CL.pieceCounts(report);
  const pending = report?.pendiente || 0;
  const after = last ? CL.projected(last.tokens, pending) : 0;
  const top = CL.sparkTop(series, target, threshold);
  const pts = CL.sparkPoints(series, W, H, top);
  const ticks = CL.turnTicks(series, W, H);
  const line = sweeping ? 'barriendo' : working ? CL.statusLine(st, busy) : agentWorking ? 'el agente trabaja' : CL.statusLine(st, busy);
  const hint = CL.sweepHint(agent, report, relayForced);
  // Qué parte del contexto de la última llamada cuesta: leído de caché (barato), escrito en ella o sin caché.
  const lastUse = [...messages].reverse().find(m => m.rol === 'assistant' && (m.vivo?.contexto || m.uso?.contexto));
  const ctxCost = CL.contextCost(lastUse?.vivo?.contexto ? lastUse.vivo : lastUse?.uso, agent, model);
  const note = CL.pendingNote(agent, report, relayForced);
  const warn = CL.cacheWarning(agent, report);
  const cost = CL.sweepCost(agent, model, report);
  const advice = CL.sweepAdvice(report);
  const cannot = CL.canSweep(agent, report, relayForced);
  const relays = CL.relayRemoved(messages);
  const removed = (st?.retirados_sesion || 0) + relays.tokens;
  const result = CL.sweepResultVisible(shown, users, report, pending) ? shown!.text : '';
  const cal = report?.calibracion;
  const progress = cal ? CL.calibProgress(cal) : '';
  const sweep = () => {
    if (advice && !confirm) { setConfirm(true); return; }
    setConfirm(false);
    onSweep?.();
  };
  return (
    <span className="cleaner" ref={box}>
      <button className={'cleaner-btn' + (working ? ' working' : '') + (rules ? ' rules' : '')} onClick={() => setOpen(o => !o)}
        aria-label={'Hilo de limpieza: ' + line} aria-expanded={open}
        title={`Hilo de limpieza: ${line}${rules ? ' · solo reglas' : ''}${last ? ` · ${CL.contextLine(last.tokens, threshold)} hasta compactar` : ''}`}>
        <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden className="cleaner-icon">
          <path d="M8 2a6 6 0 1 1-5.2 3" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
          <path d="M2.2 1.8v3.6h3.6" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
        {last && <span className="cleaner-pct">{Math.round(last.pct)} %</span>}
      </button>
      {open && (
        <div className="cleaner-pop" role="dialog" aria-label="Hilo de limpieza">
          <div className="cleaner-title">Hilo de limpieza <span className={'cleaner-state' + (working ? ' on' : '')}>{line}</span></div>
          <p className="cleaner-what suave pequeno">El contexto crece con cada paso del agente; el hilo retira lo que ya no hace falta y lo hace caer. El hilo no gasta tokens (usa la IA local); barrer con la caché caliente puede costar caché.</p>
          <div className="cleaner-now">
            <b>{last ? CL.contextLine(last.tokens, threshold) : '—'}</b>
            <span className="suave"> {CL.nowLabel(last, agentWorking)}</span>
          </div>
          {ctxCost && <ContextCostBar c={ctxCost} agent={agent} />}
          {!agentWorking && <CacheLine messages={messages} agent={agent} />}
          <div className="suave pequeno">compacta a {fmt(threshold)}{target ? ` · objetivo ${fmt(target)}: por encima, el hilo retira también lo grande y viejo` : ''}</div>
          {series.length > 1 && (
            <figure className="cleaner-spark">
              <svg width={W} height={H + 4} viewBox={`0 -2 ${W} ${H + 4}`} role="img" aria-label={`Evolución del contexto (escala hasta ${fmt(top)}): sube en cada turno y cae en cada limpieza`}>
                {threshold <= top && <line x1="0" x2={W} y1={CL.sparkY(threshold, top, H)} y2={CL.sparkY(threshold, top, H)} stroke="currentColor" strokeWidth="0.8" strokeDasharray="3 3" opacity="0.6"><title>compacta a {fmt(threshold)}</title></line>}
                {target > 0 && target <= top && <line x1="0" x2={W} y1={CL.sparkY(target, top, H)} y2={CL.sparkY(target, top, H)} stroke="currentColor" strokeWidth="0.6" strokeDasharray="1 3" opacity="0.5"><title>objetivo {fmt(target)}</title></line>}
                {ticks.map((x, i) => <line key={'t' + i} x1={x} x2={x} y1={H - 1} y2={H + 2} stroke="currentColor" strokeWidth="1" opacity="0.7" />)}
                <path d={CL.sparkPath(series, W, H, top)} fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" />
                {series.map((p, i) => p.drop && (
                  <circle key={i} cx={pts[i].x} cy={pts[i].y} r="3" className={p.relay ? 'mark-relay' : 'mark-prune'}>
                    <title>{CL.pruneLabel(series, i)}</title>
                  </circle>
                ))}
              </svg>
              <figcaption className="suave pequeno">Escala hasta {fmt(top)}{threshold <= top ? ' (- - compacta' + (target ? ', ··· objetivo' : '') + ')' : target ? ' (··· objetivo)' : ''}; un tic por turno. Cada caída <span className="mark-prune-txt">●</span> es una limpieza{series.some(p => p.relay) && <> (<span className="mark-relay-txt">●</span> relevo)</>}
                {series.map((_, i) => CL.pruneLabel(series, i)).filter(Boolean).slice(-1).map((t, i) => <span key={i}> · la última, {t}</span>)}</figcaption>
            </figure>
          )}
          <dl className="cleaner-dl">
            <dt>Retirado</dt><dd>{fmt(removed)} en esta sesión{relays.count ? ` (${relays.count === 1 ? '1 relevo' : relays.count + ' relevos'} incluidos)` : ''}{st?.ultimo_barrido ? ` · última limpieza ${CL.ago(st.ultimo_barrido)}` : ''}</dd>
            <dt>Ahora</dt><dd>{pending > 0 && last ? <>{fmt(pending)} ya no hacen falta → quedaría en {fmt(after)}</> : 'nada que retirar'}{report?.por_reponer ? ` · ${fmt(report.por_reponer)} por reponer (el agente los ha vuelto a citar)` : ''}</dd>
          </dl>
          {onSweep && (
            <div className="cleaner-sweep">
              <button type="button" className="boton-secundario" onClick={sweep} disabled={working || Boolean(cannot)}
                title={cannot || (warn ? 'Barrer ahora: ' + warn : 'Retira ya lo que no hace falta, sin esperar al próximo turno')}>
                {sweeping ? 'Barriendo…' : confirm ? 'Barrer igualmente' : 'Barrer ahora'}
              </button>
              {confirm && <button type="button" className="boton-secundario" onClick={() => setConfirm(false)}>Esperar</button>}
              {note && <span className="aviso-txt pequeno">{note}</span>}
              {!note && cannot && <span className="suave pequeno">{cannot}</span>}
              {!cannot && cost && <span className="suave pequeno">{cost}</span>}
              {confirm && <span className="aviso-txt pequeno">{advice}</span>}
              {!confirm && !cannot && warn && <span className="aviso-txt pequeno">{warn}</span>}
              {result && <span className="suave pequeno">{result}</span>}
            </div>
          )}
          <details className="cleaner-more">
            <summary className="suave pequeno">Detalles</summary>
            <dl className="cleaner-dl">
              <dt>Cómo</dt><dd>{CL.HOW[agent] || 'sin gestión del contexto'}{hint && !result && !note ? ` · ${hint}` : ''}</dd>
              <dt>Ventana</dt><dd>{fmt(window)} del modelo{last ? ` (${String(CL.pct(last.tokens, window)).replace('.', ',')} %)` : ''}; se compacta antes, a {fmt(threshold)}</dd>
              <dt>Motor</dt><dd className={rules ? 'aviso-txt' : ''}>{CL.engineLine(st)}</dd>
              <dt>Piezas</dt><dd>{counts.in} en contexto · {counts.out} ya retiradas</dd>
              {cal && <>
                <dt>Ajuste</dt><dd title="antigüedad en intercambios · en pasos · tiempo · relación (pivote)">{cal.generales ? `general${progress ? ` (${progress})` : ' (aún poco historial del proyecto)'}` : `del proyecto, ${CL.ago(cal.fecha)}`}: {CL.paramsLine(cal)}</dd>
                {(cal.retiradas || cal.retiradas_generales) ? <><dt>Relecturas</dt><dd>{CL.overLine(cal)}</dd><dt>Ahorro</dt><dd>{CL.savedLine(cal)}</dd></> : null}
              </>}
            </dl>
            <div className="cleaner-legend suave pequeno" aria-label="Leyenda de la vigencia">
              <span>Vigencia en el chat:</span>
              <span className="vig-chip" style={{ opacity: 1 }}>0 vital</span>
              <span className="vig-chip" style={{ opacity: 0.87 }}>1</span>
              <span className="vig-chip" style={{ opacity: 0.73 }}>2</span>
              <span className="vig-chip" style={{ opacity: 0.6 }}>3+ se retirará</span>
              <span className="vig-chip" style={{ opacity: 0.4 }}>⤓ fuera</span>
            </div>
            <p className="suave pequeno">{CL.HYSTERESIS}</p>
          </details>
        </div>
      )}
    </span>
  );
}

function fmt(n: number): string {
  return n >= 1_000_000 ? (n / 1_000_000).toFixed(1).replace('.', ',') + ' M' : n >= 1000 ? Math.round(n / 1000) + ' k' : String(n);
}

// ContextCostBar: el contexto de la última llamada partido en lo leído de caché, lo escrito en ella y lo que fue sin
// caché (en rojo). Solo tokens: en esta ventana no se habla de dinero.
function ContextCostBar({ c }: { c: CL.ContextCost; agent: string }) {
  const pct = (n: number) => `${(n / c.total) * 100}%`;
  return (
    <div className="ctx-cost" aria-label="El contexto de la última llamada: leído de caché, escrito en caché y sin caché">
      <div className="ctx-cost-bar" role="img" aria-hidden>
        {c.cached > 0 && <span className="ctx-cached" style={{ width: pct(c.cached) }} />}
        {c.written > 0 && <span className="ctx-written" style={{ width: pct(c.written) }} />}
        {c.fresh > 0 && <span className="ctx-fresh" style={{ width: pct(c.fresh) }} />}
      </div>
      <div className="ctx-cost-legend pequeno">
        {c.cached > 0 && <span><i className="ctx-cached" /> {fmt(c.cached)} en caché</span>}
        {c.written > 0 && <span><i className="ctx-written" /> {fmt(c.written)} escritos en caché</span>}
        {c.fresh > 0 && <span className="ctx-fresh-txt"><i className="ctx-fresh" /> {fmt(c.fresh)} sin caché</span>}
      </div>
      <div className="suave pequeno">Con la caché caducada (unos minutos parado), los {fmt(c.total)} irían sin caché.</div>
    </div>
  );
}
