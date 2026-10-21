import { useEffect, useRef, useState } from 'react';
import { get, post } from '../api';
import Icono from './Icono';

// Imputación de tiempo en Odoo, que lleva pgocode: los partes abiertos (proyecto · tarea · tiempo) en la barra y,
// al pulsar, el panel con la regla, los partes de hoy y el interruptor «PlanesGo en los agentes».

interface Parte {
  sesion: string; titulo: string; proyecto: number; nombre_proyecto: string; tarea_id: number; tarea: string; linea_id: number;
  inicio: string; ultima: string; fin?: string; horas: number; tokens: number; coste: number; estado: 'trabajando' | 'en-espera' | 'cerrado' | 'error'; error?: string; desc: string;
}
interface Estado { abiertos: Parte[]; cerrados: Parte[]; gestor_pgocode: boolean }

const hm = (h: number) => { const m = Math.round(h * 60); return `${Math.floor(m / 60)}:${String(m % 60).padStart(2, '0')}`; };
const k = (n: number) => n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n);

export default function Imputacion({ version }: { version: number }) {
  const [e, setE] = useState<Estado | null>(null);
  const [abierto, setAbierto] = useState(false);
  const [, setTic] = useState(0);
  const caja = useRef<HTMLDivElement>(null);
  const carga = () => get<Estado>('/api/imputacion').then(setE).catch(() => {});
  useEffect(() => { carga(); }, [version]);
  // El tiempo de los partes abiertos avanza en pantalla (sin pedir nada al servidor).
  useEffect(() => { const t = setInterval(() => setTic(x => x + 1), 30000); return () => clearInterval(t); }, []);
  useEffect(() => {
    if (!abierto) return;
    const fuera = (ev: MouseEvent) => { if (!caja.current?.contains(ev.target as Node)) setAbierto(false); };
    document.addEventListener('mousedown', fuera);
    return () => document.removeEventListener('mousedown', fuera);
  }, [abierto]);

  const cambiaGestor = async (pgocode: boolean) => {
    setE(x => x && { ...x, gestor_pgocode: pgocode });
    await post('/api/imputacion/gestor', { pgocode }).catch(() => {});
    carga();
  };

  const duracion = (p: Parte) => (Date.now() - new Date(p.inicio).getTime()) / 3600000;
  const ab = e?.abiertos || [];
  const hoy = (e?.cerrados || []).reduce((t, p) => t + p.horas, 0) + ab.reduce((t, p) => t + duracion(p), 0);
  const principal = ab.find(p => p.estado === 'trabajando') || ab[0];

  return (
    <div className="imputacion" ref={caja}>
      <button className={'chip-imputa' + (principal ? ' ' + principal.estado : '')} onClick={() => setAbierto(a => !a)} aria-expanded={abierto}
        title="Tiempo imputado en Odoo por pgocode">
        <Icono n="reloj" t={14} />
        {principal ? <span className="recorta">{principal.nombre_proyecto} · {principal.tarea} · {hm(duracion(principal))}</span> : <span>Hoy {hm(hoy)}</span>}
        {ab.length > 1 && <b>+{ab.length - 1}</b>}
      </button>
      {abierto && e && (
        <div className="panel-imputa" role="dialog" aria-label="Imputación de tiempo">
          <p className="suave pequeno regla">
            Cada sesión imputa en Odoo a <b>su proyecto</b> y a <b>su tarea</b> (en los ajustes de la sesión; por defecto «pgocode - Sesión IA»).
            Un parte se abre con el primer turno del agente y se cierra a los 5 minutos sin turnos, con las horas reales y los tokens.
            Mientras se trabaja no se envía nada a Odoo.
          </p>
          <p className="titulo-lista">Abiertos <span className="gh-cuenta">{ab.length}</span></p>
          {ab.length === 0 && <p className="suave pequeno">Ninguno: se abre uno al hablar con un agente.</p>}
          {ab.map(p => <Fila key={p.sesion} p={p} horas={duracion(p)} />)}
          <p className="titulo-lista">Cerrados hoy <span className="gh-cuenta">{e.cerrados.length}</span> <span className="suave">· {hm(hoy)} en total</span></p>
          {e.cerrados.map((p, i) => <Fila key={p.sesion + i} p={p} horas={p.horas} />)}
          <label className="gestor">
            <input type="checkbox" checked={e.gestor_pgocode} onChange={ev => cambiaGestor(ev.target.checked)} />
            <span><b>PlanesGo dormido en los agentes</b>
              <span className="suave pequeno">pgocode lleva la imputación. Los agentes que se usen fuera de pgocode (terminal, Antigravity, OpenCode Desktop) no imputan ni piden vincular el proyecto. Desmárcalo para que vuelvan a hacerlo.</span></span>
          </label>
        </div>
      )}
    </div>
  );
}

function Fila({ p, horas }: { p: Parte; horas: number }) {
  return (
    <div className={'fila-parte ' + p.estado} title={p.desc + (p.error ? '\n⚠ ' + p.error : '')}>
      <span className={'punto-parte ' + p.estado} />
      <span className="parte-texto">
        <b className="recorta">{p.nombre_proyecto || 'Proyecto ' + p.proyecto} · {p.tarea}</b>
        <span className="suave pequeno recorta">{p.titulo || 'Sesión sin nombre'}{p.linea_id ? ` · parte ${p.linea_id}` : ''}{p.tokens ? ` · ${k(p.tokens)} tokens` : ''}
          {p.estado === 'en-espera' ? ' · se cierra si no hay más turnos' : ''}{p.error ? ' · ⚠ ' + p.error : ''}</span>
      </span>
      <span className="parte-horas">{hm(horas)}</span>
    </div>
  );
}
