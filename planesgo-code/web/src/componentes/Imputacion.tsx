import { useEffect, useRef, useState } from 'react';
import { get, post } from '../api';
import Icono from './Icono';

// Imputación de tiempo en Odoo, que lleva pgocode. En Odoo hay, por día, proyecto y tarea, una línea de Hora
// Hombre y una de Hora Máquina por agente. La etiqueta (centrada en la barra) muestra lo que llevas hoy en el
// proyecto y la tarea de la sesión abierta, avanzando en vivo; su color es el estado de esa sesión.

interface Linea {
  fecha: string; proyecto: number; nombre_proyecto: string; tarea_id: number; tarea: string; clase: 'h' | 'm'; agente?: string;
  modelo?: string; linea_id: number; entrada: number; salida: number; coste: number; sesiones: string[]; error?: string; sucio: boolean;
  horas: number; ritmo: number; calculado: string;
}
interface Abierto {
  sesion: string; titulo: string; proyecto: number; nombre_proyecto: string; tarea_id: number; tarea: string;
  estado: 'trabajando' | 'activo' | 'en-espera' | 'error'; error?: string; horas: number;
}
interface Estado { hoy: Linea[]; abiertos: Abierto[]; gestor_pgocode: boolean }

const BLOQUE = { h: 15, m: 5 }; // redondeo hacia arriba en Odoo (minutos)
const hm = (h: number) => { const m = Math.round(h * 60); return `${Math.floor(m / 60)}:${String(m % 60).padStart(2, '0')}`; };
const reloj = (h: number) => { const s = Math.max(0, Math.round(h * 3600)); return `${Math.floor(s / 3600)}:${String(Math.floor(s / 60) % 60).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`; };
const k = (n: number) => n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n);
const enOdoo = (l: Linea, h: number) => h <= 0 ? 0 : Math.ceil((h * 60) / BLOQUE[l.clase]) * BLOQUE[l.clase] / 60;

export default function Imputacion({ version, sesion }: { version: number; sesion?: string }) {
  const [e, setE] = useState<Estado | null>(null);
  const [abierto, setAbierto] = useState(false);
  const [, setTic] = useState(0);
  const caja = useRef<HTMLDivElement>(null);
  const carga = () => get<Estado>('/api/imputacion').then(setE).catch(() => {});
  useEffect(() => { carga(); }, [version]);
  // Avanza cada segundo en pantalla y se refresca del servidor cada 30 s (sin tocar Odoo).
  useEffect(() => {
    const t = setInterval(() => setTic(x => x + 1), 1000);
    const r = setInterval(carga, 30000);
    return () => { clearInterval(t); clearInterval(r); };
  }, []);
  useEffect(() => {
    if (!abierto) return;
    const fuera = (ev: MouseEvent) => { if (!caja.current?.contains(ev.target as Node)) setAbierto(false); };
    document.addEventListener('mousedown', fuera);
    return () => document.removeEventListener('mousedown', fuera);
  }, [abierto]);

  const [unificando, setUnificando] = useState(false);
  const [unificado, setUnificado] = useState('');
  const unifica = async () => {
    setUnificando(true);
    try { const r = await post<{ mensaje: string }>('/api/imputacion/unificar', {}); setUnificado(r.mensaje); }
    catch (x) { setUnificado('⚠ ' + (x as Error).message); }
    finally { setUnificando(false); carga(); }
  };

  const cambiaGestor = async (pgocode: boolean) => {
    setE(x => x && { ...x, gestor_pgocode: pgocode });
    await post('/api/imputacion/gestor', { pgocode }).catch(() => {});
    carga();
  };

  // Horas de una línea ahora mismo: lo calculado + lo que ha crecido desde entonces.
  const ahora = (l: Linea) => l.horas + l.ritmo * Math.max(0, Date.now() - new Date(l.calculado).getTime()) / 3600000;
  const hoy = e?.hoy || [];
  const ab = e?.abiertos || [];
  const mia = ab.find(a => a.sesion === sesion);
  // La etiqueta: lo de hoy en el proyecto y la tarea de la sesión abierta (máquina + hombre).
  const delaTarea = mia ? hoy.filter(l => l.proyecto === mia.proyecto && l.tarea_id === mia.tarea_id) : [];
  const total = delaTarea.reduce((t, l) => t + ahora(l), 0);
  const estado = mia ? (mia.estado === 'activo' ? 'trabajando' : mia.estado) : '';
  const totalDia = hoy.reduce((t, l) => t + ahora(l), 0);

  // Agrupado por proyecto y tarea para el panel.
  const grupos = new Map<string, Linea[]>();
  for (const l of hoy) {
    const c = `${l.nombre_proyecto} · ${l.tarea}`;
    grupos.set(c, [...(grupos.get(c) || []), l]);
  }

  return (
    <div className="imputacion" ref={caja}>
      <button className={'chip-imputa' + (estado ? ' ' + estado : '')} onClick={() => setAbierto(a => !a)} aria-expanded={abierto}
        title="Tiempo de hoy imputado en Odoo por pgocode">
        <Icono n="reloj" t={14} />
        {mia
          ? <><span className="recorta">{mia.nombre_proyecto} · {mia.tarea}</span><b className="reloj-vivo">{reloj(total)}</b></>
          : <><span>Hoy</span><b className="reloj-vivo">{reloj(totalDia)}</b></>}
      </button>
      {abierto && e && (
        <div className="panel-imputa" role="dialog" aria-label="Imputación de tiempo">
          <p className="suave pequeno regla">
            En Odoo hay, por <b>día, proyecto y tarea</b>, una línea de <b>Hora Hombre</b> (tu trabajo y las esperas) y una de
            <b> Hora Máquina</b> por agente (sus turnos, con tokens y coste). Todas las sesiones suman en ellas y se ponen al día cada 3 minutos.
            Redondeo hacia arriba del total del día: hombre a 15 min, máquina a 5 min. Los periodos de cada sesión se guardan en tu equipo, no en Odoo.
            <br /><span className="leyenda"><i className="trabajando" /> trabajando (agente o tú) <i className="en-espera" /> espera activa (cuenta) <i className="gris" /> sin trabajo <i className="error" /> error</span>
          </p>
          <p className="titulo-lista">Hoy <span className="suave">· {hm(totalDia)} de trabajo</span></p>
          {grupos.size === 0 && <p className="suave pequeno">Todavía nada: empieza en cuanto hables con un agente o uses una sesión.</p>}
          {[...grupos.entries()].map(([titulo, ls]) => (
            <div key={titulo} className="grupo-imputa">
              <b className="recorta">{titulo}</b>
              {ls.map(l => {
                const h = ahora(l);
                return (
                  <div key={l.clase + (l.agente || '')} className="fila-parte" title={(l.sesiones || []).join(', ') + (l.error ? '\n⚠ ' + l.error : '')}>
                    <span className={'punto-parte ' + (l.ritmo > 0 ? 'trabajando' : l.error ? 'pendiente' : '')} />
                    <span className="parte-texto">
                      <span>{l.clase === 'h' ? 'Hora Hombre' : `Hora Máquina · ${l.agente}`}</span>
                      <span className="suave pequeno recorta">
                        {(l.sesiones || []).length} {(l.sesiones || []).length === 1 ? 'sesión' : 'sesiones'}
                        {l.clase === 'm' && l.entrada + l.salida > 0 ? ` · ${k(l.entrada + l.salida)} tokens` : ''}{l.coste ? ` · $${l.coste.toFixed(2)}` : ''}
                        {l.linea_id ? ` · en Odoo ${hm(enOdoo(l, h))}` : ' · aún no está en Odoo'}{l.error ? ' · ⚠ se reintenta' : ''}
                      </span>
                    </span>
                    <span className="parte-horas">{reloj(h)}</span>
                  </div>
                );
              })}
            </div>
          ))}
          {ab.length > 0 && <>
            <p className="titulo-lista">Sesiones trabajando ahora <span className="gh-cuenta">{ab.length}</span></p>
            {ab.map(a => (
              <div key={a.sesion} className="fila-parte" title={a.error || ''}>
                <span className={'punto-parte ' + (a.estado === 'activo' ? 'trabajando' : a.estado)} />
                <span className="parte-texto">
                  <span className="recorta">{a.titulo || 'Sesión sin nombre'}</span>
                  <span className="suave pequeno recorta">{a.nombre_proyecto} · {a.tarea} · {a.estado === 'trabajando' ? 'el agente trabaja' : a.estado === 'activo' ? 'trabajas tú' : a.estado === 'error' ? '⚠ ' + a.error : 'espera activa (cuenta)'}</span>
                </span>
                <span className="parte-horas">{hm(a.horas)}</span>
              </div>
            ))}
          </>}
          <div className="unificar">
            <button className="boton-secundario" onClick={unifica} disabled={unificando}>{unificando ? 'Unificando…' : 'Unificar líneas sueltas'}</button>
            <span className="suave pequeno">{unificado || 'Suma a la línea del día las líneas de pgocode de menos de 5 minutos (de versiones anteriores) y las borra.'}</span>
          </div>
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
