import { useEffect, useRef, useState } from 'react';
import { get, post } from '../api';
import Icono from './Icono';

// Imputación de tiempo en Odoo, que lleva pgocode: los partes abiertos (proyecto · tarea · tiempo) en la barra y,
// al pulsar, el panel con la regla, los partes de hoy y el interruptor «PlanesGo en los agentes».

interface Parte {
  sesion: string; titulo: string; proyecto: number; nombre_proyecto: string; tarea_id: number; tarea: string;
  linea_maquina: number; linea_hombre: number; horas_maquina: number; horas_hombre: number;
  inicio: string; ultima: string; fin?: string; horas: number; tokens: number; coste: number; estado: 'trabajando' | 'activo' | 'en-espera' | 'cerrado' | 'pendiente' | 'error'; error?: string; desc: string;
}
interface Estado { abiertos: Parte[]; cerrados: Parte[]; gestor_pgocode: boolean }

const hm = (h: number) => { const m = Math.round(h * 60); return `${Math.floor(m / 60)}:${String(m % 60).padStart(2, '0')}`; };
const k = (n: number) => n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n);

export default function Imputacion({ version, sesion }: { version: number; sesion?: string }) {
  const [e, setE] = useState<Estado | null>(null);
  const [abierto, setAbierto] = useState(false);
  const [, setTic] = useState(0);
  const caja = useRef<HTMLDivElement>(null);
  const carga = () => get<Estado>('/api/imputacion').then(setE).catch(() => {});
  useEffect(() => { carga(); }, [version]);
  // El tiempo avanza en pantalla cada segundo (sin pedir nada al servidor).
  useEffect(() => { const t = setInterval(() => setTic(x => x + 1), 1000); return () => clearInterval(t); }, []);
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

  // Lo que se está imputando: desde el inicio hasta ahora, sin pasar del último trabajo + 5 min (la espera cuenta).
  const duracion = (p: Parte) => (Math.min(Date.now(), new Date(p.ultima).getTime() + 5 * 60000) - new Date(p.inicio).getTime()) / 3600000;
  const ab = e?.abiertos || [];
  const hoy = (e?.cerrados || []).reduce((t, p) => t + p.horas, 0) + ab.reduce((t, p) => t + duracion(p), 0);
  // La etiqueta es la de la sesión abierta: verde (imputando), amarillo (espera activa, cuenta), gris (sin parte:
  // no se imputa, a cero) o rojo (error). Sin sesión abierta, el parte que esté trabajando o el total de hoy.
  const principal = sesion ? ab.find(p => p.sesion === sesion) : (ab.find(p => p.estado === 'trabajando' || p.estado === 'activo') || ab[0]);
  // Para la etiqueta, al segundo: h:mm:ss.
  const reloj = (h: number) => { const s = Math.max(0, Math.round(h * 3600)); return `${Math.floor(s / 3600)}:${String(Math.floor(s / 60) % 60).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`; };

  return (
    <div className="imputacion" ref={caja}>
      <button className={'chip-imputa' + (principal ? ' ' + (principal.estado === 'activo' ? 'trabajando' : principal.estado) : '')} onClick={() => setAbierto(a => !a)} aria-expanded={abierto}
        title="Tiempo imputado en Odoo por pgocode">
        <Icono n="reloj" t={14} />
        {principal
          ? <><span className="recorta">{principal.nombre_proyecto} · {principal.tarea}</span><b className="reloj-vivo">{reloj(duracion(principal))}</b></>
          : sesion ? <><span>Sin imputar</span><b className="reloj-vivo">0:00:00</b></> : <span>Hoy {hm(hoy)}</span>}
        {ab.length > 1 && <b>+{ab.length - 1}</b>}
      </button>
      {abierto && e && (
        <div className="panel-imputa" role="dialog" aria-label="Imputación de tiempo">
          <p className="suave pequeno regla">
            Cada sesión imputa en Odoo a <b>su proyecto</b> y a <b>su tarea</b> (en los ajustes de la sesión; por defecto la canónica «Desarrollo»),
            en dos líneas: <b>Hora Máquina</b> (los turnos del agente, con sus tokens) y <b>Hora Hombre</b> (tu trabajo y las esperas).
            Las líneas están vivas en Odoo: sus horas se ponen al día cada 3 minutos.
            Hay trabajo cuando el agente está con un turno o cuando tú usas la sesión (teclado, ratón, con la ventana en primer plano).
            Tras el último trabajo hay una espera activa de 5 minutos que también cuenta; si no hay más, el parte se cierra con el tiempo
            del inicio al último trabajo + 5 min. Si pgocode se cierra mal, al volver se cierra con su último trabajo; si Odoo falla, se reintenta.
            <br /><span className="leyenda"><i className="trabajando" /> imputando (agente o tú) <i className="en-espera" /> espera activa (cuenta) <i className="gris" /> sin imputar <i className="error" /> error</span>
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
        <span className="suave pequeno recorta">{p.titulo || 'Sesión sin nombre'} · máquina {hm(p.horas_maquina || 0)} · hombre {hm(p.horas_hombre || 0)}{p.tokens ? ` · ${k(p.tokens)} tokens` : ''}
          {p.estado === 'activo' ? ' · trabajando tú' : p.estado === 'trabajando' ? ' · trabajando el agente' : p.estado === 'en-espera' ? ' · espera activa: cuenta; se cierra a los 5 min sin trabajo' : p.estado === 'pendiente' ? ' · Odoo no lo aceptó: se reintenta' : ''}{p.error ? ' · ⚠ ' + p.error : ''}</span>
      </span>
      <span className="parte-horas">{hm(horas)}</span>
    </div>
  );
}
