import { useEffect, useRef, useState, type KeyboardEvent, type MouseEvent, type WheelEvent } from 'react';
import { post } from '../api';
import { keyAction, screenPoint } from '../agentScreen';
import { Modal } from './Modal';

// «Pantalla del agente»: the screen of the session's agent desktop, live, and the person's own mouse and keyboard on it
// (to type a password, accept a notice, see what the agent is doing). While the person uses it, the agent waits; on
// closing, the screen goes back to it. What is typed here is never kept: the audit only counts its characters.

const FRAME_MS = 400;
const TYPE_MS = 120; // characters typed close together go in one order
const MAX_KEY = 'pgo:pantalla-agente:maximizada';
const readMax = () => { try { return localStorage.getItem(MAX_KEY) === '1'; } catch { return false; } };

export default function AgentScreen({ project, session, onClose }: { project: number; session: string; onClose: () => void }) {
  const [src, setSrc] = useState('');
  const [size, setSize] = useState<[number, number]>([0, 0]);
  const [error, setError] = useState('');
  const [held, setHeld] = useState(false);
  // The window is resized from its corner, or fills the app's window (remembered in this browser).
  const [max, setMax] = useState(readMax);
  const toggleMax = () => setMax(m => { try { localStorage.setItem(MAX_KEY, m ? '0' : '1'); } catch { /* no storage */ } return !m; });
  const img = useRef<HTMLImageElement>(null);
  const pending = useRef('');
  const timer = useRef(0);
  const down = useRef<[number, number] | null>(null);
  const q = `project=${project}&session=${encodeURIComponent(session)}`;

  // The screen, one picture after another (the next one when the last has arrived).
  useEffect(() => {
    let stop = false, url = '';
    const next = async () => {
      while (!stop) {
        try {
          const r = await fetch(`/api/agent-desktop/frame?${q}`, { credentials: 'same-origin', headers: { 'X-PGO': '1' } });
          if (!r.ok) {
            const j = await r.json().catch(() => ({}));
            setError(j.error || 'No se puede ver la pantalla.');
          } else {
            const w = Number(r.headers.get('X-Width')), h = Number(r.headers.get('X-Height'));
            const u = URL.createObjectURL(await r.blob());
            if (url) URL.revokeObjectURL(url);
            url = u;
            setSrc(u); setSize([w, h]); setError('');
          }
        } catch (e) { setError(String((e as Error).message || e)); }
        await new Promise(res => setTimeout(res, FRAME_MS));
      }
    };
    next();
    return () => { stop = true; if (url) URL.revokeObjectURL(url); };
  }, [q]);

  // On closing, the screen goes back to the agent.
  useEffect(() => () => { post('/api/agent-desktop/hold', { project, session, hold: false }).catch(() => {}); }, [project, session]);

  const send = (desktop: object) => {
    setHeld(true);
    post('/api/agent-desktop/input', { project, session, desktop }).catch(e => setError(String((e as Error).message || e)));
  };
  const at = (e: MouseEvent<HTMLImageElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    return screenPoint(e.clientX - r.left, e.clientY - r.top, r.width, r.height, size[0], size[1]);
  };
  const flush = () => {
    const text = pending.current;
    pending.current = '';
    if (text) send({ action: 'type', text });
  };
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const a = keyAction(e);
    if (!a) return;
    e.preventDefault();
    e.stopPropagation(); // Escape and the rest go to Windows, not to this window
    if ('text' in a) {
      pending.current += a.text;
      window.clearTimeout(timer.current);
      timer.current = window.setTimeout(flush, TYPE_MS);
    } else {
      flush();
      send({ action: 'key', keys: [a.key] });
    }
  };
  const onDown = (e: MouseEvent<HTMLImageElement>) => { if (e.button === 0) down.current = at(e); };
  const onUp = (e: MouseEvent<HTMLImageElement>) => {
    const from = down.current;
    down.current = null;
    if (e.button !== 0 || !from) return;
    const [x, y] = at(e);
    if (Math.abs(x - from[0]) > 4 || Math.abs(y - from[1]) > 4) send({ action: 'drag', x: from[0], y: from[1], x2: x, y2: y });
    else send({ action: 'click', x, y });
  };
  const onDouble = (e: MouseEvent<HTMLImageElement>) => { const [x, y] = at(e); send({ action: 'click', x, y, double: true }); };
  const onRight = (e: MouseEvent<HTMLImageElement>) => { e.preventDefault(); const [x, y] = at(e); send({ action: 'click', x, y, right: true }); };
  const onWheel = (e: WheelEvent<HTMLImageElement>) => send({ action: 'wheel', up: e.deltaY < 0, count: Math.min(5, Math.max(1, Math.round(Math.abs(e.deltaY) / 100))) });
  const giveBack = () => post('/api/agent-desktop/hold', { project, session, hold: false }).then(() => setHeld(false)).catch(() => {});

  return (
    <Modal title="Pantalla del agente" className={'pantalla-agente' + (max ? ' maximizada' : '')} onClose={onClose}
      footer={<>
        <span className="suave pequeno">{held
          ? 'La estás usando tú: el agente espera (hasta que la sueltes, cierres o pasen 2 minutos sin usarla). Lo que escribes no se guarda en ningún sitio.'
          : 'Haz clic en la imagen y escribe: el ratón y el teclado van a Windows. Mientras la uses, el agente espera.'}</span>
        {src && error && <span className="aviso-txt pequeno" role="alert">{error}</span>}
        <span className="hueco" />
        <button type="button" onClick={toggleMax} title={max ? 'Volver al tamaño anterior' : 'Ocupar toda la ventana'}>{max ? 'Restaurar' : 'Maximizar'}</button>
        {held && <button type="button" onClick={giveBack}>Devolvérsela al agente</button>}
        <button type="button" className="primario" onClick={onClose}>Cerrar</button>
      </>}>
      <div className="pantalla-agente-cuerpo" tabIndex={0} onKeyDown={onKey} autoFocus>
        {src
          ? <img ref={img} src={src} alt="Pantalla del escritorio del agente" draggable={false}
              onMouseDown={onDown} onMouseUp={onUp} onDoubleClick={onDouble} onContextMenu={onRight} onWheel={onWheel} />
          : <p className="suave">{error || 'Cargando la pantalla…'}</p>}
      </div>
    </Modal>
  );
}
