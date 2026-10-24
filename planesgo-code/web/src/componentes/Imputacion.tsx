import { useEffect, useRef, useState } from 'react';
import { get, post } from '../api';
import Icono from './Icono';

// Imputación de tiempo en Odoo, que lleva pgocode. En Odoo hay, por día, proyecto y tarea, una línea de Hora
// Hombre y una de Hora Máquina por agente. La etiqueta (centrada en la barra) muestra lo que llevas hoy en el
// proyecto y la tarea de la sesión abierta, avanzando en vivo; su color es el estado de esa sesión.

interface Linea {
  fecha: string; proyecto: number; nombre_proyecto: string; tarea_id: number; tarea: string; clase: 'h' | 'm'; agente?: string;
  modelo?: string; linea_id: number; entrada: number; salida: number; coste: number; sesiones: string[]; error?: string; sucio: boolean;
  escrito: number; // horas ya escritas en Odoo (redondeadas)
  horas: number; ritmo: number; calculado: string;
}
interface Abierto {
  sesion: string; titulo: string; proyecto: number; nombre_proyecto: string; tarea_id: number; tarea: string;
  estado: 'trabajando' | 'activo' | 'en-espera' | 'error'; error?: string; horas: number;
  inicio: string; ultima: string; agente: boolean; maquina: number; hombre?: number; presente?: boolean; calculado: string;
}
interface Resumen { tarea_id: number; hoy_h: number; hoy_m: number; total_h: number; total_m: number }
// La sesión abierta: su proyecto y su tarea (aunque todavía no tenga periodo de trabajo).
export interface SesionMarcador { id: string; proyecto: number; nombreProyecto?: string; tareaId?: number; tarea?: string }
const JORNADA = 8; // horas de una jornada, para la barra de «hoy»
interface Validacion {
  dia: string; fichero: string; sucesos: number; errores: number;
  avisos: { t?: string; gravedad: 'error' | 'aviso' | 'info'; tipo: string; sesion?: string; texto: string }[];
  lineas: { fecha: string; proyecto: number; tarea_id: number; clase: 'h' | 'm'; agente?: string; segundos: number; horas_calculadas: number; horas_odoo: number }[];
}
const hhmm = (h: number) => { const m = Math.round(h * 60); return `${Math.floor(m / 60)}:${String(m % 60).padStart(2, '0')}`; };

interface Estado { hoy: Linea[]; abiertos: Abierto[] }

const BLOQUE = { h: 15, m: 5 }; // redondeo hacia arriba en Odoo (minutos)
const hm = (h: number) => { const m = Math.round(h * 60); return `${Math.floor(m / 60)}:${String(m % 60).padStart(2, '0')}`; };
const reloj = (h: number) => { const s = Math.max(0, Math.round(h * 3600)); return `${Math.floor(s / 3600)}:${String(Math.floor(s / 60) % 60).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`; };
const k = (n: number) => n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n);
const enOdoo = (l: Linea, h: number) => h <= 0 ? 0 : Math.ceil((h * 60) / BLOQUE[l.clase]) * BLOQUE[l.clase] / 60;

export default function Imputacion({ version, sesion }: { version: number; sesion?: SesionMarcador }) {
  const [e, setE] = useState<Estado | null>(null);
  const [abierto, setAbierto] = useState(false);
  const [, setTic] = useState(0);
  const caja = useRef<HTMLDivElement>(null);
  const carga = () => get<Estado>('/api/imputacion').then(setE).catch(() => {});
  useEffect(() => { carga(); }, [version]);
  // Avanza cada segundo en pantalla y se refresca del servidor cada 30 s (sin tocar Odoo).
  useEffect(() => {
    const t = setInterval(() => { if (!document.hidden) setTic(x => x + 1); }, 1000);
    const r = setInterval(() => { if (!document.hidden) carga(); }, 30000);
    return () => { clearInterval(t); clearInterval(r); };
  }, []);
  useEffect(() => {
    if (!abierto) return;
    const fuera = (ev: MouseEvent) => { if (!caja.current?.contains(ev.target as Node)) setAbierto(false); };
    document.addEventListener('mousedown', fuera);
    return () => document.removeEventListener('mousedown', fuera);
  }, [abierto]);

  // Validación del registro de tiempos de hoy (todo lo que interviene en la imputación, para encontrar fallos).
  const [validacion, setValidacion] = useState<Validacion | null>(null);
  const [validando, setValidando] = useState(false);
  const valida = async () => {
    setValidando(true);
    try { setValidacion(await get<Validacion>('/api/imputacion/validar')); }
    catch (err) { setValidacion({ dia: '', fichero: '', sucesos: 0, errores: 1, avisos: [{ gravedad: 'error', tipo: 'validar', texto: (err as Error).message }], lineas: [] }); }
    finally { setValidando(false); }
  };
  const [unificando, setUnificando] = useState(false);
  const [unificado, setUnificado] = useState('');
  const unifica = async () => {
    setUnificando(true);
    try { const r = await post<{ mensaje: string }>('/api/imputacion/unificar', {}); setUnificado(r.mensaje); }
    catch (x) { setUnificado('⚠ ' + (x as Error).message); }
    finally { setUnificando(false); carga(); }
  };

  // Horas de una línea ahora mismo: lo calculado + lo que ha crecido desde entonces.
  const ahora = (l: Linea) => l.horas + l.ritmo * Math.max(0, Date.now() - new Date(l.calculado).getTime()) / 3600000;
  // Lo que hay en Odoo se lee una vez (al abrir la sesión o cambiar de proyecto o tarea). Desde entonces se suma
  // lo que pgocode lleva en cada línea del día por encima de lo que ya estaba escrito en ese momento.
  const clave = (l: Linea) => `${l.fecha}|${l.proyecto}|${l.tarea_id}|${l.clase}|${l.agente || ''}`;
  const base = useRef<Map<string, number>>(new Map());
  const eActual = useRef(e);
  eActual.current = e;
  const baseFalta = useRef(false); // el resumen llegó antes que el estado de pgocode: la base se toma al llegar este
  const tomaBase = (est: Estado) => { base.current = new Map(est.hoy.map(l => [clave(l), l.linea_id ? (l.escrito || 0) : 0])); };
  useEffect(() => { if (e && baseFalta.current) { baseFalta.current = false; tomaBase(e); } }, [e]); // eslint-disable-line react-hooks/exhaustive-deps
  const pendiente = (l: Linea) => Math.max(0, ahora(l) - (base.current.get(clave(l)) ?? 0));
  const hoy = e?.hoy || [];
  const ab = e?.abiertos || [];
  const mia = sesion ? ab.find(a => a.sesion === sesion.id) : undefined;
  const totalDia = hoy.reduce((t, l) => t + ahora(l), 0);

  // Marcador de la sesión abierta: lo que hay en Odoo (resumen, cada minuto) + lo pendiente del cronómetro.
  const proyecto = mia?.proyecto || sesion?.proyecto || 0;
  const [res, setRes] = useState<Resumen | null>(null);
  const tareaPedida = mia?.tarea_id || sesion?.tareaId || 0;
  const nombreTarea = mia?.tarea || sesion?.tarea || 'Desarrollo';
  useEffect(() => {
    if (!proyecto) { setRes(null); return; }
    let vivo = true;
    get<Resumen>(`/api/imputacion/resumen?proyecto=${proyecto}&tarea_id=${tareaPedida}&tarea=${encodeURIComponent(nombreTarea)}`)
      .then(r => {
        if (!vivo) return;
        // Lo escrito en Odoo de cada línea de pgocode en el momento de la lectura (ya va dentro del resumen).
        if (eActual.current) tomaBase(eActual.current); else baseFalta.current = true;
        setRes(r);
      }).catch(() => {});
    return () => { vivo = false; };
  }, [proyecto, tareaPedida, nombreTarea]); // eslint-disable-line react-hooks/exhaustive-deps
  const tareaID = res?.tarea_id || tareaPedida;
  const pendTarea = (clase: 'h' | 'm') => hoy.filter(l => l.proyecto === proyecto && l.tarea_id === tareaID && l.clase === clase).reduce((t, l) => t + pendiente(l), 0);
  const pendProy = (clase: 'h' | 'm') => hoy.filter(l => l.proyecto === proyecto && l.clase === clase).reduce((t, l) => t + pendiente(l), 0);
  // Cada tiempo suma hombre y máquina; el desglose se lleva por dentro (líneas de Odoo y registro de tiempos),
  // no en la etiqueta (Luis: «no queda claro»).
  const hoyH = (res?.hoy_h || 0) + pendTarea('h');
  const hoyM = (res?.hoy_m || 0) + pendTarea('m');
  const hoyTarea = hoyH + hoyM;
  const totalH = (res?.total_h || 0) + pendProy('h');
  const totalM = (res?.total_m || 0) + pendProy('m');
  // Cronómetro: el periodo de trabajo actual de la sesión (lo calcula el backend; sigue mientras trabaja el agente
  // o hay presencia humana, con su mínimo de 15 minutos).
  const crono = mia ? mia.horas + ((mia.agente || mia.presente) ? Math.max(0, Date.now() - new Date(mia.calculado).getTime()) / 3600000 : 0) : 0;
  const estado = mia ? (mia.estado === 'activo' ? 'trabajando' : mia.estado) : 'parado';
  const nombreProy = mia?.nombre_proyecto || sesion?.nombreProyecto || '';
  const pctHoy = Math.min(100, (hoyTarea / JORNADA) * 100);

  // Agrupado por proyecto y tarea para el panel.
  const grupos = new Map<string, Linea[]>();
  for (const l of hoy) {
    const c = `${l.nombre_proyecto} · ${l.tarea}`;
    grupos.set(c, [...(grupos.get(c) || []), l]);
  }

  return (
    <div className="imputacion" ref={caja}>
      {proyecto ? (
        <button className={'marcador ' + estado} onClick={() => setAbierto(a => !a)} aria-expanded={abierto}
          title={`${nombreProy} / ${nombreTarea}\n⏱ Cronómetro de esta sesión: ${reloj(crono)}${mia ? (estado === 'trabajando' ? ' (trabajando)' : mia.presente ? ' (presencia: cuenta, mínimo 15 min)' : ' (en espera: no cuenta)') : ' (sin trabajo ahora)'}\n☀ Hoy en la tarea: ${hm(hoyTarea)} (Odoo ${hm(res ? res.hoy_h + res.hoy_m : 0)} + pendiente ${hm(pendTarea('h') + pendTarea('m'))})\nΣ Esta semana en el proyecto: ${hm(totalH + totalM)}`}>
          <span className="m-tarea recorta"><b>{nombreProy}</b> / {nombreTarea}</span>
          <span className="m-seg m-crono"><i className="m-punto" />
            <span className="m-cifras"><span>{reloj(crono)}</span></span></span>
          <span className="m-seg m-hoy"><span className="m-icono">☀</span>
            <span className="m-cifras"><span>{hm(hoyTarea)}</span></span>
            <span className="m-barra"><span style={{ width: pctHoy + '%' }} /></span></span>
          <span className="m-seg m-total" ><span className="m-icono" title="Esta semana en el proyecto">Σ</span>
            <span className="m-cifras"><span>{hm(totalH + totalM)}</span></span></span>
        </button>
      ) : (
        <button className="chip-imputa" onClick={() => setAbierto(a => !a)} aria-expanded={abierto} title="Tiempo de hoy imputado por pgocode">
          <Icono n="reloj" t={14} /><span>Hoy</span><b className="reloj-vivo">{reloj(totalDia)}</b>
        </button>
      )}
      {abierto && e && (
        <div className="panel-imputa" role="dialog" aria-label="Imputación de tiempo">
          <p className="suave pequeno regla">
            En Odoo hay, por <b>día, proyecto y tarea</b>, una línea de <b>Hora Hombre</b> (tu trabajo y las esperas) y una de
            <b> Hora Máquina</b> por agente (sus turnos, con tokens y coste). Todas las sesiones suman en ellas y se ponen al día cada 3 minutos.
            Hombre: tu presencia (cualquier intervención abre un tramo de 15 min como mínimo; sigue hasta 5 min sin intervenir). Máquina: los turnos del agente. Se cuentan por separado. Redondeo hacia arriba del total del día: hombre a 15 min, máquina a 5 min. Los periodos de cada sesión se guardan en tu equipo, no en Odoo.
            <br /><span className="leyenda"><i className="trabajando" /> trabajando (agente o tú) <i className="en-espera" /> presencia (cuenta) o en espera <i className="gris" /> sin trabajo <i className="error" /> error</span>
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
                  <span className="suave pequeno recorta">{a.nombre_proyecto} · {a.tarea} · {a.estado === 'trabajando' ? 'el agente trabaja' : a.estado === 'activo' ? 'trabajas tú' : a.estado === 'error' ? '⚠ ' + a.error : a.presente ? 'presencia (cuenta, mínimo 15 min)' : 'en espera (no cuenta)'}</span>
                </span>
                <span className="parte-horas">{hm(a.horas)}</span>
              </div>
            ))}
          </>}
          <div className="unificar">
            <button className="boton-secundario" onClick={unifica} disabled={unificando}>{unificando ? 'Unificando…' : 'Unificar líneas sueltas'}</button>
            <span className="suave pequeno">{unificado || 'Suma a la línea del día las líneas de pgocode de menos de 5 minutos (de versiones anteriores) y las borra.'}</span>
          </div>
          <div className="validar-tiempos">
            <button className="boton-secundario" onClick={valida} disabled={validando}>{validando ? 'Validando…' : 'Validar el registro de hoy'}</button>
            {validacion && <>
              <span className="suave pequeno" title={validacion.fichero}>
                {validacion.sucesos} sucesos · {validacion.errores ? <b className="v-error">{validacion.errores} {validacion.errores === 1 ? 'error' : 'errores'}</b> : 'sin errores'} · {validacion.avisos.filter(a => a.gravedad === 'aviso').length} avisos
              </span>
              {validacion.lineas.length > 0 && (
                <table className="v-lineas">
                  <thead><tr><th>Línea</th><th>Calculado</th><th>En Odoo</th></tr></thead>
                  <tbody>{validacion.lineas.map(l => (
                    <tr key={`${l.fecha}${l.proyecto}${l.tarea_id}${l.clase}${l.agente || ''}`} className={l.horas_odoo >= 0 && Math.abs(l.horas_odoo - l.horas_calculadas) > 0.0001 ? 'v-distinto' : ''}>
                      <td>{l.proyecto}/{l.tarea_id} · {l.clase === 'h' ? 'Hombre' : 'Máquina ' + (l.agente || '')}</td>
                      <td>{hhmm(l.horas_calculadas)} <span className="suave">({Math.round(l.segundos / 60)} min)</span></td>
                      <td>{l.horas_odoo < 0 ? '—' : hhmm(l.horas_odoo)}</td>
                    </tr>
                  ))}</tbody>
                </table>
              )}
              {validacion.avisos.length > 0 && (
                <ul className="v-avisos">{validacion.avisos.map((a, i) => (
                  <li key={i} className={'v-' + a.gravedad}><span className="suave">{a.t}</span> {a.texto}</li>
                ))}</ul>
              )}
            </>}
          </div>
        </div>
      )}
    </div>
  );
}

