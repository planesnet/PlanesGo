import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { get, type Proveedor, type Proyecto, type Usuario } from '../api';
import { lista as sesionesDe, titulo, type Repo, type Sesion } from '../sesiones';
import { guardaAjustes, leeAjustes, type AjustesProyecto, type Detalle } from '../ajustes';
import Icono from './Icono';
import { Agente, Hosting } from './Marca';
import './config-sesion.css';

interface Props {
  usuario: Usuario; actual: Proyecto | null; inicial: number; alElegir: (p: Proyecto) => void;
  repo: Repo | null; proveedores: Proveedor[];
  sesiones: Sesion[]; sesion: Sesion | null;
  alAjustes: (p: Proyecto, a: AjustesProyecto) => void;
  alElegirSesion: (p: Proyecto, s: Sesion) => void; alNuevaSesion: (p: Proyecto) => void; alCambiarSesion: (s: Sesion) => void; alBorrarSesion: (s: Sesion) => void;
}

// Proyectos recientes de cada usuario (los últimos que ha abierto, el más reciente primero).
const MAX_RECIENTES = 10;
const claveRecientes = (login: string) => 'pgo:recientes:' + login.toLowerCase();
function leeRecientes(login: string): number[] {
  try { return (JSON.parse(localStorage.getItem(claveRecientes(login)) || '[]') as number[]).filter(n => Number.isInteger(n)); } catch { return []; }
}
function guardaReciente(login: string, id: number): number[] {
  const l = [id, ...leeRecientes(login).filter(x => x !== id)].slice(0, MAX_RECIENTES);
  try { localStorage.setItem(claveRecientes(login), JSON.stringify(l)); } catch { /* sin almacenamiento */ }
  return l;
}

const sinAcentos = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();

export default function Proyectos(props: Props) {
  const { usuario, actual, inicial, alElegir } = props;
  const [lista, setLista] = useState<Proyecto[] | null>(null);
  const [error, setError] = useState('');
  const [filtro, setFiltro] = useState('');
  const [recientes, setRecientes] = useState<number[]>(() => leeRecientes(usuario.login));
  // Proyectos plegados en la lista (se recuerdan).
  const [plegados, setPlegados] = useState<Set<number>>(() => {
    try { return new Set(JSON.parse(localStorage.getItem('pgo:plegados:' + usuario.login.toLowerCase()) || '[]') as number[]); } catch { return new Set(); }
  });
  const pliega = (id: number) => setPlegados(prev => {
    const n = new Set(prev);
    if (n.has(id)) n.delete(id); else n.add(id);
    try { localStorage.setItem('pgo:plegados:' + usuario.login.toLowerCase(), JSON.stringify([...n])); } catch { /* sin almacenamiento */ }
    return n;
  });
  // Al cambiar o borrar una sesión de otro proyecto, se vuelve a leer su lista.
  const [, setCambios] = useState(0);
  const cambia = (s: Sesion) => { props.alCambiarSesion(s); setCambios(c => c + 1); };
  // Menú ⋮ de un proyecto y ventana de propiedades.
  const [menuP, setMenuP] = useState<{ id: number; ancla: HTMLElement } | null>(null);
  const [propiedades, setPropiedades] = useState<Proyecto | null>(null);
  const borra = (s: Sesion) => { props.alBorrarSesion(s); setCambios(c => c + 1); };

  // Cada proyecto que se abre pasa a ser el más reciente.
  useEffect(() => { if (actual) setRecientes(guardaReciente(usuario.login, actual.id)); }, [actual?.id, usuario.login]);

  const carga = (recargar = false) => {
    setError('');
    get<Proyecto[]>('/api/proyectos' + (recargar ? '?recargar=1' : ''))
      .then(setLista).catch(e => setError(e.message));
  };
  useEffect(() => carga(), []);

  // Tras F5: se vuelve a elegir el proyecto guardado si Odoo sigue dándolo.
  useEffect(() => {
    if (!actual && lista && inicial) {
      const p = lista.find(x => x.id === inicial);
      if (p) alElegir(p);
    }
  }, [lista]); // eslint-disable-line react-hooks/exhaustive-deps

  // Sin buscar: solo los recientes, en orden de uso. Buscando: todos los proyectos de Odoo.
  const buscando = filtro.trim() !== '';
  const visibles = useMemo(() => {
    const todos = lista || [];
    if (!buscando) {
      const porId = new Map(todos.map(p => [p.id, p]));
      return recientes.map(id => porId.get(id)).filter((p): p is Proyecto => Boolean(p));
    }
    const f = sinAcentos(filtro.trim());
    return todos.filter(p => sinAcentos(p.nombre + ' ' + (p.cliente?.nombre || '')).includes(f));
  }, [lista, filtro, recientes, buscando]);

  return (
    <nav className="proyectos" aria-label="Proyectos">
      <div className="busca">
        <input placeholder="Buscar proyecto…" value={filtro} onChange={e => setFiltro(e.target.value)} aria-label="Buscar proyecto" />
        <button className="icono" title="Volver a cargar de Odoo" aria-label="Volver a cargar de Odoo" onClick={() => carga(true)}><Icono n="recargar" t={16} /></button>
      </div>
      <div className="lista">
        {error && <p className="error">{error}</p>}
        {!lista && !error && <p className="suave">Cargando proyectos de Odoo…</p>}
        {lista && !buscando && <p className="titulo-lista">Recientes</p>}
        {lista && visibles.length === 0 && <p className="suave">{buscando ? 'Ningún proyecto coincide.' : 'Aún no has abierto ningún proyecto. Búscalo arriba para empezar.'}</p>}
        {buscando ? visibles.map(p => (
          <button key={p.id} className={'proyecto' + (actual?.id === p.id ? ' activo' : '')} onClick={() => { alElegir(p); setFiltro(''); }}>
            <span className="nombre">{p.nombre}</span>
            <span className="detalle">
              {p.cliente?.nombre || 'Sin cliente'}{p.carpetas.length > 0 && <span className="local" title={p.carpetas.join('\n')}> · local</span>}
            </span>
          </button>
        )) : visibles.map(p => {
          // Como los grupos de Claude Code: el proyecto y, debajo, sus sesiones abiertas.
          const activo = actual?.id === p.id;
          const plegado = plegados.has(p.id);
          const ss = activo ? props.sesiones : sesionesDe(usuario.login, p.id);
          return (
            <section key={p.id} className={'grupo' + (activo ? ' activo' : '')}>
              <div className="grupo-cabecera">
                <button className="icono plegar" onClick={() => pliega(p.id)} aria-expanded={!plegado} aria-label={plegado ? 'Desplegar' : 'Plegar'}>
                  <Icono n="bajar" t={12} />
                </button>
                <button className="grupo-nombre recorta" onClick={() => alElegir(p)} title={(p.cliente?.nombre || 'Sin cliente') + (p.carpetas.length ? '\n' + p.carpetas.join('\n') : '')}>
                  {p.nombre}
                </button>
                <button className="icono grupo-nueva" onClick={() => props.alNuevaSesion(p)} title="Nueva sesión" aria-label={'Nueva sesión en ' + p.nombre}>
                  <Icono n="nuevo" t={14} />
                </button>
                <button className="icono puntos grupo-puntos" onClick={e => { const ancla = e.currentTarget; setMenuP(m => m?.id === p.id ? null : { id: p.id, ancla }); }}
                  title="Más opciones" aria-label={'Más opciones de ' + p.nombre} aria-haspopup="menu" aria-expanded={menuP?.id === p.id}>
                  <Icono n="puntos" t={15} />
                </button>
                {menuP?.id === p.id && (
                  <MenuSesion ancla={menuP.ancla} alCerrar={() => setMenuP(null)} opciones={[
                    { icono: 'nuevo', texto: 'Nueva sesión', accion: () => props.alNuevaSesion(p) },
                    null,
                    { icono: 'ajustes', texto: 'Propiedades', accion: () => setPropiedades(p) },
                  ]} />
                )}
              </div>
              {!plegado && <Sesiones {...props} proyecto={p} lista={ss} alCambiarSesion={cambia} alBorrarSesion={borra} />}
            </section>
          );
        })}
      </div>
      {propiedades && <PropiedadesProyecto p={propiedades} login={usuario.login} alCerrar={() => setPropiedades(null)}
        alGuardar={a => props.alAjustes(propiedades, a)} />}
    </nav>
  );
}

// Ventana de propiedades del proyecto: secciones a la izquierda y su detalle a la derecha.
const SECCIONES = [{ id: 'conversacion', nombre: 'Conversación', icono: 'comentario' }] as const;
type Seccion = typeof SECCIONES[number]['id'];
const DETALLES: { id: Detalle; nombre: string; ayuda: string }[] = [
  { id: 'conciso', nombre: 'Conciso', ayuda: 'Solo la respuesta del agente. Lo que hace (leer, editar, ejecutar…) queda en una línea «N acciones» que se puede desplegar.' },
  { id: 'normal', nombre: 'Normal', ayuda: 'La respuesta y cada acción en una línea (nombre y fichero u orden), plegada.' },
  { id: 'detallado', nombre: 'Detallado', ayuda: 'Todo desplegado: entrada y salida de cada acción y el razonamiento del agente.' },
];

function PropiedadesProyecto({ p, login, alCerrar, alGuardar }: { p: Proyecto; login: string; alCerrar: () => void; alGuardar: (a: AjustesProyecto) => void }) {
  const [seccion, setSeccion] = useState<Seccion>('conversacion');
  const [a, setA] = useState<AjustesProyecto>(() => leeAjustes(login, p.id));
  const cambia = (c: Partial<AjustesProyecto>) => {
    const n = { ...a, ...c };
    setA(n);
    guardaAjustes(login, p.id, n);
    alGuardar(n);
  };
  useEffect(() => {
    const tecla = (e: KeyboardEvent) => { if (e.key === 'Escape') alCerrar(); };
    document.addEventListener('keydown', tecla);
    return () => document.removeEventListener('keydown', tecla);
  }, [alCerrar]);
  return createPortal(
    <div className="fondo-modal" onMouseDown={e => { if (e.target === e.currentTarget) alCerrar(); }}>
      <div className="modal propiedades" role="dialog" aria-modal="true" aria-label={'Propiedades de ' + p.nombre}>
        <div className="modal-cabecera">
          <h2><span className="suave">Propiedades ·</span> {p.nombre}</h2>
          <button className="icono" onClick={alCerrar} title="Cerrar" aria-label="Cerrar"><Icono n="cerrar" /></button>
        </div>
        <div className="propiedades-cuerpo">
          <nav className="propiedades-secciones" aria-label="Secciones">
            {SECCIONES.map(s => (
              <button key={s.id} aria-current={seccion === s.id} onClick={() => setSeccion(s.id)}><Icono n={s.icono} t={15} /> {s.nombre}</button>
            ))}
          </nav>
          <section className="propiedades-detalle">
            {seccion === 'conversacion' && <>
              <h3>Conversación</h3>
              <fieldset className="opciones-detalle">
                <legend>Nivel de detalle de la conversación</legend>
                <p className="suave pequeno">Cuánto se muestra en el chat de lo que hace el agente. No cambia lo que se le pide: no gasta tokens.</p>
                {DETALLES.map(d => (
                  <label key={d.id} className={'opcion-detalle' + (a.detalle === d.id ? ' elegida' : '')}>
                    <input type="radio" name="detalle" checked={a.detalle === d.id} onChange={() => cambia({ detalle: d.id })} />
                    <span><b>{d.nombre}</b><span className="suave pequeno">{d.ayuda}</span></span>
                  </label>
                ))}
              </fieldset>
            </>}
          </section>
        </div>
      </div>
    </div>,
    document.body,
  );
}

// Sesiones de un proyecto, como en Claude: icono del hosting, título, rama y menú ⋮ (renombrar, borrar, ajustes).
function Sesiones({ proyecto, lista, actual, repo, proveedores, sesion, alElegirSesion, alCambiarSesion, alBorrarSesion }: Props & { proyecto: Proyecto; lista: Sesion[] }) {
  // Sesión con el menú ⋮ abierto y el botón que lo abrió (para colocar el menú flotante a su lado).
  const [menu, setMenu] = useState<{ id: string; ancla: HTMLElement } | null>(null);
  const [ajustes, setAjustes] = useState<string | null>(null);
  const [renombrando, setRenombrando] = useState<string | null>(null);
  const enAjustes = lista.find(s => s.id === ajustes);
  return (
    <div className="sesiones">
      {lista.length === 0 && <p className="sin-sesiones">Sin sesiones</p>}
      {lista.map(s => (
        <div key={s.id} className={'sesion' + (sesion?.id === s.id ? ' activa' : '')}>
          {renombrando === s.id ? (
            <Renombra s={s} alFin={nombre => { setRenombrando(null); if (nombre !== null) alCambiarSesion({ ...s, nombre }); }} />
          ) : (
            <button className="sesion-boton" onClick={() => alElegirSesion(proyecto, s)} title={s.carpeta + (s.rama ? ' · ' + s.rama : '')}>
              <Hosting tipo={s.tipo} t={13} enBlanco />
              <span className={'recorta' + (s.nombre ? '' : ' suave')}>{titulo(s)}</span>
              {s.rama && <span className="rama-sesion recorta"><Icono n="rama" t={11} />{s.rama}</span>}
            </button>
          )}
          <button className="icono puntos" onClick={e => { const ancla = e.currentTarget; setMenu(m => m?.id === s.id ? null : { id: s.id, ancla }); }}
            title="Más opciones" aria-label="Más opciones de la sesión" aria-haspopup="menu" aria-expanded={menu?.id === s.id}>
            <Icono n="puntos" t={15} />
          </button>
          {menu?.id === s.id && (
            <MenuSesion ancla={menu.ancla} alCerrar={() => setMenu(null)} opciones={[
              { icono: 'lapiz', texto: 'Renombrar', accion: () => setRenombrando(s.id) },
              { icono: 'borrar', texto: 'Borrar', peligro: true, accion: () => { if (confirm(`¿Borrar la sesión «${titulo(s)}» y su conversación?`)) alBorrarSesion(s); } },
              null,
              { icono: 'ajustes', texto: 'Ajustes', accion: () => setAjustes(s.id) },
            ]} />
          )}
        </div>
      ))}
      {enAjustes && (
        <AjustesSesion s={enAjustes} repo={actual?.id === proyecto.id && repo?.carpeta === enAjustes.carpeta ? repo : null}
          carpetas={proyecto.carpetas} proveedores={proveedores} alCerrar={() => setAjustes(null)} alCambiar={alCambiarSesion} />
      )}
    </div>
  );
}

// Renombrar en la propia fila: Intro guarda, Escape cancela.
function Renombra({ s, alFin }: { s: Sesion; alFin: (nombre: string | null) => void }) {
  const [v, setV] = useState(s.nombre);
  const hecho = useRef(false);
  const fin = (n: string | null) => { if (!hecho.current) { hecho.current = true; alFin(n); } };
  return (
    <input className="renombra" autoFocus value={v} onChange={e => setV(e.target.value)} aria-label="Nombre de la sesión"
      onFocus={e => e.currentTarget.select()} onBlur={() => fin(v.trim() || null)}
      onKeyDown={e => { if (e.key === 'Enter') fin(v.trim() || null); if (e.key === 'Escape') fin(null); }} />
  );
}

type Opcion = { icono: string; texto: string; accion: () => void; peligro?: boolean } | null;
const ANCHO_MENU = 200, MARGEN = 8;

// Menú flotante del ⋮ (portal sobre document.body, position: fixed), como el de las sesiones de Claude.
function MenuSesion({ ancla, opciones, alCerrar }: { ancla: HTMLElement; opciones: Opcion[]; alCerrar: () => void }) {
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);
  const caja = useRef<HTMLDivElement>(null);

  // A la derecha del botón; si no cabe, a su izquierda; y siempre dentro de la ventana.
  useLayoutEffect(() => {
    const r = ancla.getBoundingClientRect();
    const alto = caja.current?.offsetHeight ?? 0;
    let left = r.right + 4, top = r.top;
    if (left + ANCHO_MENU > window.innerWidth - MARGEN) left = r.left - ANCHO_MENU - 4;
    left = Math.max(MARGEN, left);
    if (top + alto > window.innerHeight - MARGEN) top = window.innerHeight - alto - MARGEN;
    setPos({ left, top: Math.max(MARGEN, top) });
  }, [ancla]);

  useEffect(() => {
    const fuera = (e: MouseEvent) => { const t = e.target as Node; if (!caja.current?.contains(t) && !ancla.contains(t)) alCerrar(); };
    const tecla = (e: KeyboardEvent) => { if (e.key === 'Escape') alCerrar(); };
    document.addEventListener('mousedown', fuera);
    document.addEventListener('keydown', tecla);
    window.addEventListener('resize', alCerrar);
    window.addEventListener('scroll', alCerrar, true);
    return () => {
      document.removeEventListener('mousedown', fuera);
      document.removeEventListener('keydown', tecla);
      window.removeEventListener('resize', alCerrar);
      window.removeEventListener('scroll', alCerrar, true);
    };
  }, [ancla, alCerrar]);

  return createPortal(
    <div ref={caja} className="menu-flotante" role="menu"
      style={{ left: pos?.left ?? 0, top: pos?.top ?? 0, width: ANCHO_MENU, visibility: pos ? 'visible' : 'hidden' }}>
      {opciones.map((o, i) => o === null
        ? <div key={i} className="separador" role="separator" />
        : <button key={i} role="menuitem" className={o.peligro ? 'peligro' : ''} onClick={() => { alCerrar(); o.accion(); }}>
            <Icono n={o.icono} t={15} /> {o.texto}
          </button>)}
    </div>,
    document.body,
  );
}

// Ventana de preferencias de la sesión (diálogo modal).
function AjustesSesion({ s, repo, carpetas, proveedores, alCerrar, alCambiar }: {
  s: Sesion; repo: Repo | null; carpetas: string[]; proveedores: Proveedor[]; alCerrar: () => void; alCambiar: (s: Sesion) => void;
}) {
  const [nombre, setNombre] = useState(s.nombre);
  const cambia = (c: Partial<Sesion>) => alCambiar({ ...s, ...c });
  const prov = proveedores.find(p => p.id === s.agente);
  const modelos = prov?.modelos || [];

  useEffect(() => {
    const tecla = (e: KeyboardEvent) => { if (e.key === 'Escape') alCerrar(); };
    document.addEventListener('keydown', tecla);
    return () => document.removeEventListener('keydown', tecla);
  }, [alCerrar]);

  return createPortal(
    <div className="fondo-modal" onMouseDown={e => { if (e.target === e.currentTarget) alCerrar(); }}>
      <div className="modal ajustes-sesion" role="dialog" aria-modal="true" aria-label="Ajustes de la sesión">
        <div className="modal-cabecera">
          <h2>Ajustes de la sesión</h2>
          <button className="icono" onClick={alCerrar} title="Cerrar" aria-label="Cerrar"><Icono n="cerrar" /></button>
        </div>
        <label>Nombre
          <input value={nombre} placeholder="Se toma del primer mensaje" onChange={e => setNombre(e.target.value)}
            onBlur={() => nombre.trim() !== s.nombre && cambia({ nombre: nombre.trim() })}
            onKeyDown={e => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }} />
        </label>
        {carpetas.length > 0 && (
          <label>Carpeta
            <select value={s.carpeta} onChange={e => cambia({ carpeta: e.target.value, sesionAgente: undefined })} disabled={carpetas.length < 2 && carpetas[0] === s.carpeta}>
              {!carpetas.includes(s.carpeta) && s.carpeta && <option value={s.carpeta}>{s.carpeta}</option>}
              {carpetas.map(c => <option key={c} value={c}>{c}</option>)}
            </select>
          </label>
        )}
        {repo && repo.ramas.length > 0 && (
          <label>Rama
            <select value={s.rama} onChange={e => cambia({ rama: e.target.value, sesionAgente: undefined })}>
              {repo.ramas.map(r => <option key={r}>{r}</option>)}
            </select>
          </label>
        )}
        <label>Agente
          <span className="con-icono"><Agente id={s.agente} t={15} />
            <select value={s.agente} onChange={e => cambia({ agente: e.target.value, modelo: undefined, esfuerzo: undefined, sesionAgente: undefined })}>
              {proveedores.map(p => <option key={p.id} value={p.id} disabled={!p.disponible}>{p.nombre}{p.disponible ? '' : ' — ' + p.motivo}</option>)}
            </select>
          </span>
        </label>
        {prov?.modos && (
          <label>Modo
            <select value={s.permisos || 'lectura'} onChange={e => cambia({ permisos: e.target.value as Sesion['permisos'] })}>
              <option value="lectura">Solo lectura</option>
              <option value="editar">Editar ficheros</option>
              <option value="auto">Auto: ejecuta comandos</option>
            </select>
          </label>
        )}
        {modelos.length > 1 && (
          <label>Modelo
            <select value={s.modelo || prov?.por_defecto} onChange={e => cambia({ modelo: e.target.value })}>
              {modelos.map(m => <option key={m} value={m}>{m}</option>)}
            </select>
          </label>
        )}
        {prov?.esfuerzos?.length ? (
          <label>Esfuerzo
            <select value={s.esfuerzo || ''} onChange={e => cambia({ esfuerzo: e.target.value || undefined })}>
              <option value="">Por defecto</option>
              {prov.esfuerzos.map(x => <option key={x} value={x}>{x}</option>)}
            </select>
          </label>
        ) : null}
        <p className="suave pequeno">Cambiar la carpeta, la rama o el agente empieza una conversación nueva con el agente.</p>
      </div>
    </div>,
    document.body,
  );
}
