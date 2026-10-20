import { useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { chat, type Herramienta, type Mensaje, type Parte, type Proveedor, type Proyecto } from '../api';
import type { Detalle } from '../ajustes';
import { conversacion, guardaConversacion, nombreDe, titulo, type Repo, type Sesion } from '../sesiones';
import Icono from './Icono';
import Texto from './Texto';
import { Agente, Hosting, nombreHosting } from './Marca';

interface Props {
  detalle: Detalle;
  hosting?: string; // alojamiento elegido en las propiedades del proyecto (si no, el del repositorio)
  // Mensaje que hay que enviar solo en cuanto la sesión indicada esté abierta (p. ej. «Resuelve la issue…»).
  envioPendiente: { sesion: string; texto: string } | null; alEnviado: () => void;
  proyecto: Proyecto | null; sesion: Sesion | null; repo: Repo | null; proveedores: Proveedor[];
  panelAbierto: boolean; alPanel: () => void;
  alCambiarSesion: (s: Sesion) => void; alNuevaSesion: (rama?: string) => void;
}

const nombreCarpeta = (c: string) => c.split('/').filter(Boolean).pop() || c;

const MODOS = { lectura: 'Solo lectura', editar: 'Editar ficheros', auto: 'Auto: ejecuta comandos' } as const;
const ESFUERZOS: Record<string, string> = { low: 'bajo', medium: 'medio', high: 'alto', xhigh: 'muy alto', max: 'máximo' };
const ALTO_MAX = 240;

export default function Chat({ hosting, detalle, envioPendiente, alEnviado, proyecto, sesion, repo, proveedores, panelAbierto, alPanel, alCambiarSesion, alNuevaSesion }: Props) {
  const [texto, setTexto] = useState('');
  const [mensajes, setMensajes] = useState<Mensaje[]>([]);
  const [enCurso, setEnCurso] = useState<AbortController | null>(null);
  const [verAgentes, setVerAgentes] = useState(false);
  const fin = useRef<HTMLDivElement>(null);
  const lista = useRef<HTMLDivElement>(null);
  const idActual = useRef(sesion?.id);
  const area = useRef<HTMLTextAreaElement>(null);

  // El campo empieza en una línea y crece hacia arriba según lo escrito (hasta un máximo, luego scroll).
  useLayoutEffect(() => {
    const a = area.current;
    if (!a) return;
    a.style.height = 'auto';
    a.style.height = Math.min(a.scrollHeight, ALTO_MAX) + 'px';
    a.style.overflowY = a.scrollHeight > ALTO_MAX ? 'auto' : 'hidden';
  }, [texto, sesion?.id]);

  const prov = proveedores.find(p => p.id === sesion?.agente);
  const modelos = useMemo(() => prov?.modelos?.length ? prov.modelos : (prov?.por_defecto ? [prov.por_defecto] : []), [prov]);

  // Al cambiar de sesión: su conversación; y se corta la respuesta en curso (no se escribe en otra sesión).
  useEffect(() => {
    idActual.current = sesion?.id;
    setMensajes(sesion ? conversacion(sesion.id) : []);
    return () => { setEnCurso(ac => { ac?.abort(); return null; }); };
  }, [sesion?.id]);

  // Solo se baja al final si el usuario ya estaba abajo.
  useEffect(() => {
    const l = lista.current;
    if (!l || l.scrollHeight - l.scrollTop - l.clientHeight < 120) fin.current?.scrollIntoView({ block: 'end' });
  }, [mensajes]);

  // Envío automático pedido desde fuera, cuando su sesión ya es la abierta y no hay nada en curso.
  useEffect(() => {
    if (!envioPendiente || !sesion || sesion.id !== envioPendiente.sesion || enCurso || !prov?.disponible) return;
    alEnviado();
    envia(envioPendiente.texto);
  }, [envioPendiente, sesion?.id, prov?.disponible]); // eslint-disable-line react-hooks/exhaustive-deps

  async function envia(forzado?: string) {
    const t = (forzado ?? texto).trim();
    if (!t || !proyecto || !sesion || !prov || enCurso) return;
    const id = sesion.id;
    const pon = (m: Mensaje[], final = false) => {
      if (idActual.current === id) setMensajes(m);
      if (final) guardaConversacion(id, m);
    };
    // Envío automático: la sesión acaba de abrirse y el estado aún puede ser el de la anterior; se lee la guardada.
    const previos = forzado !== undefined ? conversacion(id) : mensajes;
    const base: Mensaje[] = [...previos.filter(m => !m.error), { rol: 'user', texto: t }];
    let actual: Mensaje[] = [...base, { rol: 'assistant', texto: '' }];
    pon(actual, true);
    if (forzado === undefined) setTexto('');
    const ac = new AbortController();
    setEnCurso(ac);
    let sesionAgente = sesion.sesionAgente;
    const ultimo = (f: (m: Mensaje) => Mensaje) => { actual = [...actual.slice(0, -1), f(actual[actual.length - 1])]; pon(actual); };
    // Añade un trozo a las partes del mensaje: el texto seguido se junta; la herramienta se actualiza por su id.
    const parte = (p: Parte) => ultimo(m => {
      const ps = [...(m.partes || [])];
      const u = ps[ps.length - 1];
      if (p.tipo !== 'herramienta' && u && u.tipo === p.tipo) ps[ps.length - 1] = { ...u, texto: u.texto + p.texto };
      else if (p.tipo === 'herramienta') {
        const i = ps.findIndex(x => x.tipo === 'herramienta' && x.h.id === p.h.id);
        if (i >= 0) { const v = (ps[i] as { h: Herramienta }).h; ps[i] = { tipo: 'herramienta', h: { ...v, ...p.h, nombre: p.h.nombre || v.nombre, resumen: v.resumen || p.h.resumen, entrada: v.entrada || p.h.entrada } }; }
        else ps.push(p);
      } else ps.push(p);
      return { ...m, partes: ps, texto: p.tipo === 'texto' ? m.texto + p.texto : m.texto };
    });
    try {
      await chat({
        proyecto: proyecto.id, proveedor: prov.id, modelo: sesion.modelo || prov.por_defecto, carpeta: sesion.carpeta, rama: sesion.rama,
        permisos: sesion.permisos || 'lectura', sesion_agente: sesion.sesionAgente, esfuerzo: sesion.esfuerzo || '',
        mensajes: prov.tipo === 'agente' ? base.slice(-1) : base, // los agentes llevan su propio historial
      }, e => {
        if (e.texto) parte({ tipo: 'texto', texto: e.texto });
        if (e.pensamiento) parte({ tipo: 'pensamiento', texto: e.pensamiento });
        if (e.herramienta) parte({ tipo: 'herramienta', h: e.herramienta });
        if (e.uso) ultimo(m => ({ ...m, uso: e.uso }));
        if (e.sesion_agente) sesionAgente = e.sesion_agente;
        if (e.error) ultimo(m => ({ ...m, texto: (m.texto ? m.texto + '\n\n' : '') + '⚠ ' + e.error, error: true }));
      }, ac.signal);
    } catch (err) {
      if (!ac.signal.aborted) ultimo(m => ({ ...m, texto: '⚠ ' + (err as Error).message, error: true }));
    } finally {
      pon(actual, true);
      setEnCurso(c => (c === ac ? null : c));
      alCambiarSesion({ ...sesion, nombre: sesion.nombre || nombreDe(t), sesionAgente, ultima: Date.now() });
    }
  }

  const teclas = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); envia(); }
  };

  const eligeAgente = (p: Proveedor) => {
    setVerAgentes(false);
    if (sesion && p.id !== sesion.agente) alCambiarSesion({ ...sesion, agente: p.id, modelo: undefined, sesionAgente: undefined });
    try { localStorage.setItem('pgo:proveedor', p.id); } catch { /* sin almacenamiento */ }
  };

  return (
    <main className="chat">
      <header className="cabecera">
        {proyecto && sesion ? (
          <nav className="miga" aria-label="Ubicación de la sesión">
            <span className="miga-item" title={nombreHosting(hosting || repo?.tipo)}><Hosting tipo={hosting || repo?.tipo} t={16} /></span>
            {repo?.repositorio && <><span className="sep">/</span><span className="miga-item" title={repo.remoto}><Icono n="repo" t={14} /> {repo.repositorio}</span></>}
            {sesion.carpeta && <><span className="sep">/</span><span className="miga-item" title={sesion.carpeta}><Icono n="carpeta" t={14} /> {nombreCarpeta(sesion.carpeta)}</span></>}
            {repo && repo.ramas.length > 0 && (
              <><span className="sep">/</span>
                <label className="miga-item rama" title="Cambiar de rama abre una sesión nueva">
                  <Icono n="rama" t={14} />
                  <select value={sesion.rama} onChange={e => alNuevaSesion(e.target.value)} disabled={Boolean(enCurso)}>
                    {repo.ramas.map(r => <option key={r}>{r}</option>)}
                  </select>
                </label></>
            )}
          </nav>
        ) : <span className="suave titulo">{proyecto ? proyecto.nombre : 'Elige un proyecto de Odoo'}</span>}
        <span className="hueco" />
        {sesion && mensajes.length > 0 && !enCurso && (
          <button className="icono" onClick={() => alNuevaSesion(sesion.rama)} title="Nueva sesión" aria-label="Nueva sesión"><Icono n="nuevo" /></button>
        )}
        <button className="icono" onClick={alPanel} title={panelAbierto ? 'Ocultar artefactos' : 'Mostrar artefactos'} aria-label="Artefactos" aria-pressed={panelAbierto}><Icono n="panel" /></button>
      </header>

      {!proyecto ? (
        <div className="vacio">
          <h2>Sin proyecto, no hay trabajo</h2>
          <p>Todo en Planes Go Code va ligado a un proyecto de Odoo. Elige uno a la izquierda.</p>
        </div>
      ) : !sesion ? (
        <div className="vacio"><p>Preparando la sesión…</p></div>
      ) : (
        <>
          <div className="mensajes" aria-live="polite" ref={lista}>
            {mensajes.length === 0 && (
              <div className="vacio pequeno">
                <p><b>{titulo(sesion)}</b></p>
                <p className="suave">{prov?.nombre || 'Elige un agente'}{prov?.tipo === 'agente' ? ` · ${{ lectura: 'solo lectura', editar: 'puede editar', auto: 'auto: ejecuta comandos' }[sesion.permisos || 'lectura']}` : ''}</p>
              </div>
            )}
            {mensajes.map((m, i) => (
              <div key={i} className={'mensaje ' + m.rol + (m.error ? ' con-error' : '')}>
                {m.partes?.length
                  ? <Partes partes={m.partes} detalle={detalle} />
                  : m.texto ? (m.rol === 'assistant' && !m.error ? <Texto md={m.texto} /> : m.texto)
                    : (enCurso && i === mensajes.length - 1 ? <span className="pensando">Pensando…</span> : '')}
                {m.partes?.length && m.error ? <p className="con-error">{m.texto.split('⚠').slice(1).map(x => '⚠' + x).join('')}</p> : null}
                {enCurso && i === mensajes.length - 1 && m.partes?.length ? <span className="pensando">Trabajando…</span> : null}
                {m.uso && detalle !== 'conciso' && <div className="uso">{usoTexto(m.uso)}</div>}
              </div>
            ))}
            <div ref={fin} />
          </div>
          <div className="redactor">
            <div className="entrada">
              <div className="selector-agente">
                <button className="agente-actual" onClick={() => setVerAgentes(v => !v)} title={prov ? prov.nombre : 'Elegir agente'} aria-label="Agente" aria-haspopup="listbox" aria-expanded={verAgentes}>
                  <Agente id={sesion.agente} t={18} /><Icono n="bajar" t={12} />
                </button>
                {verAgentes && (
                  <ul className="lista-agentes" role="listbox" aria-label="Agentes">
                    {proveedores.map(p => (
                      <li key={p.id}>
                        <button role="option" aria-selected={p.id === sesion.agente} disabled={!p.disponible} onClick={() => eligeAgente(p)} title={p.disponible ? '' : p.motivo}>
                          <Agente id={p.id} t={16} /> <span className="recorta">{p.nombre}</span>
                          {p.id === sesion.agente && <Icono n="hecho" t={14} />}
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
              <textarea ref={area} rows={1} value={texto} onChange={e => setTexto(e.target.value)} onKeyDown={teclas}
                placeholder={prov?.disponible ? 'Escribe…' : 'Elige un agente disponible'} disabled={!prov?.disponible} />
              {enCurso
                ? <button className="redondo" onClick={() => enCurso.abort()} title="Parar" aria-label="Parar"><Icono n="parar" t={16} /></button>
                : <button className="redondo primario" onClick={() => envia()} disabled={!texto.trim() || !prov?.disponible} title="Enviar (Intro)" aria-label="Enviar"><Icono n="enviar" t={16} /></button>}
            </div>
            {/* Debajo del campo, como en Claude Code: el modo a la izquierda; el modelo y su esfuerzo a la derecha. */}
            <div className="pie-redactor">
              {prov?.modos && (
                <select className="pie-select" value={sesion.permisos || 'lectura'} aria-label="Modo"
                  onChange={e => alCambiarSesion({ ...sesion, permisos: e.target.value as Sesion['permisos'] })}>
                  {Object.entries(MODOS).map(([v, t]) => <option key={v} value={v}>{t}</option>)}
                </select>
              )}
              <span className="hueco" />
              {modelos.length > 1 && (
                <select className="pie-select" value={sesion.modelo || prov?.por_defecto} aria-label="Modelo"
                  onChange={e => alCambiarSesion({ ...sesion, modelo: e.target.value })}>
                  {modelos.map(m => <option key={m} value={m}>{m === 'por defecto' ? 'Modelo por defecto' : m}</option>)}
                </select>
              )}
              {modelos.length <= 1 && prov && <span className="pie-texto">{prov.nombre}</span>}
              {prov?.esfuerzos?.length ? (
                <select className="pie-select" value={sesion.esfuerzo || ''} aria-label="Esfuerzo"
                  onChange={e => alCambiarSesion({ ...sesion, esfuerzo: e.target.value || undefined })}>
                  <option value="">Esfuerzo por defecto</option>
                  {prov.esfuerzos.map(x => <option key={x} value={x}>Esfuerzo {ESFUERZOS[x] || x}</option>)}
                </select>
              ) : null}
            </div>
          </div>
        </>
      )}
    </main>
  );
}

const miles = (n: number) => n >= 1000 ? (n / 1000).toFixed(n >= 10000 ? 0 : 1) + 'k' : String(n);
const usoTexto = (u: { entrada: number; salida: number; coste?: number; segundos?: number }) =>
  [`${miles(u.entrada)} ↑ · ${miles(u.salida)} ↓ tokens`, u.coste ? `$${u.coste.toFixed(3)}` : '', u.segundos ? `${u.segundos.toFixed(1)} s` : ''].filter(Boolean).join(' · ');

// Una parte de la respuesta: texto, razonamiento (plegado) o una herramienta (una línea que se despliega), como en Claude Code.
// Las partes según el nivel de detalle del proyecto: conciso (solo texto y una línea con las acciones), normal
// (cada acción plegada) o detallado (todo desplegado).
function Partes({ partes, detalle }: { partes: Parte[]; detalle: Detalle }) {
  if (detalle !== 'conciso') return <>{partes.map((p, j) => <ParteMensaje key={j} p={p} abierta={detalle === 'detallado'} />)}</>;
  const acciones = partes.filter(p => p.tipo === 'herramienta');
  const enCurso = acciones.some(p => p.tipo === 'herramienta' && p.h.estado === 'curso');
  return <>
    {acciones.length > 0 && (
      <details className="acciones">
        <summary><span className={'punto-h' + (enCurso ? ' curso' : '')} /> {acciones.length} {acciones.length === 1 ? 'acción' : 'acciones'}</summary>
        {acciones.map((p, j) => <ParteMensaje key={j} p={p} abierta={false} />)}
      </details>
    )}
    {partes.filter(p => p.tipo === 'texto').map((p, j) => <ParteMensaje key={j} p={p} abierta={false} />)}
  </>;
}

function ParteMensaje({ p, abierta }: { p: Parte; abierta: boolean }) {
  if (p.tipo === 'texto') return <Texto md={p.texto} />;
  if (p.tipo === 'pensamiento') return <details className="pensamiento" open={abierta}><summary>Razonamiento</summary><div>{p.texto}</div></details>;
  const h = p.h;
  return (
    <details className={'herramienta ' + h.estado} open={abierta}>
      <summary>
        <span className="punto-h" aria-label={h.estado} />
        <b>{h.nombre}</b>{h.resumen && <span className="resumen-h recorta">{h.resumen}</span>}
      </summary>
      {h.entrada && <pre className="entrada-h">{h.entrada}</pre>}
      {h.salida && <pre className="salida-h">{h.salida}</pre>}
    </details>
  );
}
