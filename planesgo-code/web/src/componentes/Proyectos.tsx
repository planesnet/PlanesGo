import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { get, type Proveedor, type Proyecto, type Usuario } from '../api';
import { lista as sesionesDe, titulo, type Repo, type Sesion } from '../sesiones';
import Icono from './Icono';
import { Agente, Hosting } from './Marca';
import './config-sesion.css';

interface Props {
  usuario: Usuario; actual: Proyecto | null; inicial: number; alElegir: (p: Proyecto) => void;
  repo: Repo | null; proveedores: Proveedor[];
  sesiones: Sesion[]; sesion: Sesion | null;
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
              </div>
              {!plegado && <Sesiones {...props} proyecto={p} lista={ss} alCambiarSesion={cambia} alBorrarSesion={borra} />}
            </section>
          );
        })}
      </div>
    </nav>
  );
}

// Sesiones de un proyecto, como en Claude: icono del hosting, título, rama y menú ⋮ con su configuración.
function Sesiones({ proyecto, lista, actual, repo, proveedores, sesion, alElegirSesion, alCambiarSesion, alBorrarSesion }: Props & { proyecto: Proyecto; lista: Sesion[] }) {
  // Sesión con el menú ⋮ abierto y el botón que lo abrió (para colocar el panel flotante a su lado).
  const [menu, setMenu] = useState<{ id: string; ancla: HTMLElement } | null>(null);
  return (
    <div className="sesiones">
      {lista.length === 0 && <p className="sin-sesiones">Sin sesiones</p>}
      {lista.map(s => (
        <div key={s.id} className={'sesion' + (sesion?.id === s.id ? ' activa' : '')}>
          <button className="sesion-boton" onClick={() => alElegirSesion(proyecto, s)} title={s.carpeta + (s.rama ? ' · ' + s.rama : '')}>
            <Hosting tipo={s.tipo} t={13} enBlanco />
            <span className={'recorta' + (s.nombre ? '' : ' suave')}>{titulo(s)}</span>
            {s.rama && <span className="rama-sesion recorta"><Icono n="rama" t={11} />{s.rama}</span>}
          </button>
          <button className="icono puntos" onClick={e => { const ancla = e.currentTarget; setMenu(m => m?.id === s.id ? null : { id: s.id, ancla }); }}
            title="Configuración de la sesión" aria-label="Configuración de la sesión" aria-expanded={menu?.id === s.id}>
            <Icono n="puntos" t={15} />
          </button>
          {menu?.id === s.id && (
            <ConfigSesion s={s} ancla={menu.ancla} repo={actual?.id === proyecto.id && repo?.carpeta === s.carpeta ? repo : null}
              carpetas={proyecto.carpetas} proveedores={proveedores} alCerrar={() => setMenu(null)}
              alCambiar={alCambiarSesion} alBorrar={x => { setMenu(null); alBorrarSesion(x); }} />
          )}
        </div>
      ))}
    </div>
  );
}

const ANCHO_CONFIG = 280, MARGEN = 8;

// Panel flotante de configuración de la sesión: va en un portal sobre document.body con position: fixed,
// para que no lo recorten la lista de sesiones ni la columna lateral.
function ConfigSesion({ s, ancla, repo, carpetas, proveedores, alCerrar, alCambiar, alBorrar }: {
  s: Sesion; ancla: HTMLElement; repo: Repo | null; carpetas: string[]; proveedores: Proveedor[]; alCerrar: () => void; alCambiar: (s: Sesion) => void; alBorrar: (s: Sesion) => void;
}) {
  const [nombre, setNombre] = useState(s.nombre);
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);
  const panel = useRef<HTMLDivElement>(null);
  const cambia = (c: Partial<Sesion>) => alCambiar({ ...s, ...c });
  const prov = proveedores.find(p => p.id === s.agente);

  // Coloca el panel a la derecha del botón ⋮; si no cabe, debajo; y siempre dentro de la ventana.
  const coloca = () => {
    const r = ancla.getBoundingClientRect();
    const alto = panel.current?.offsetHeight ?? 0;
    const an = window.innerWidth, al = window.innerHeight;
    let left = r.right + 6, top = r.top;
    if (left + ANCHO_CONFIG > an - MARGEN) { left = r.right - ANCHO_CONFIG; top = r.bottom + 4; }
    left = Math.max(MARGEN, Math.min(left, an - ANCHO_CONFIG - MARGEN));
    if (top + alto > al - MARGEN) top = al - alto - MARGEN;
    top = Math.max(MARGEN, top);
    setPos(p => p && p.left === left && p.top === top ? p : { left, top });
  };
  // Se recoloca si cambia el contenido (p. ej. aparece «Permisos» al cambiar de agente).
  useLayoutEffect(coloca);

  // Se cierra al pulsar fuera, con Escape, al hacer scroll fuera del panel o al redimensionar la ventana.
  useEffect(() => {
    const fuera = (e: MouseEvent) => {
      const t = e.target as Node;
      if (!panel.current?.contains(t) && !ancla.contains(t)) alCerrar();
    };
    const tecla = (e: KeyboardEvent) => { if (e.key === 'Escape') alCerrar(); };
    const scroll = (e: Event) => { if (!panel.current?.contains(e.target as Node)) alCerrar(); };
    document.addEventListener('mousedown', fuera);
    document.addEventListener('keydown', tecla);
    window.addEventListener('scroll', scroll, true);
    window.addEventListener('resize', alCerrar);
    return () => {
      document.removeEventListener('mousedown', fuera);
      document.removeEventListener('keydown', tecla);
      window.removeEventListener('scroll', scroll, true);
      window.removeEventListener('resize', alCerrar);
    };
  }, [ancla, alCerrar]);

  return createPortal(
    <div ref={panel} className="config-sesion config-sesion-flotante" role="dialog" aria-label="Configuración de la sesión"
      style={{ left: pos?.left ?? 0, top: pos?.top ?? 0, width: ANCHO_CONFIG, visibility: pos ? 'visible' : 'hidden' }}>
      <label>Nombre
        <input value={nombre} onChange={e => setNombre(e.target.value)} onBlur={() => nombre.trim() && cambia({ nombre: nombre.trim() })}
          onKeyDown={e => { if (e.key === 'Enter') { (e.target as HTMLInputElement).blur(); } }} />
      </label>
      <label>Agente por defecto
        <span className="con-icono"><Agente id={s.agente} t={15} />
          <select value={s.agente} onChange={e => cambia({ agente: e.target.value, sesionAgente: undefined })}>
            {proveedores.map(p => <option key={p.id} value={p.id} disabled={!p.disponible}>{p.nombre}{p.disponible ? '' : ' — ' + p.motivo}</option>)}
          </select>
        </span>
      </label>
      {prov?.tipo === 'agente' && (
        <label>Permisos del agente
          <select value={s.permisos || 'lectura'} onChange={e => cambia({ permisos: e.target.value as Sesion['permisos'] })}>
            <option value="lectura">Solo lectura</option>
            <option value="editar">Editar ficheros</option>
            <option value="auto">Auto: ejecuta comandos (pruebas)</option>
          </select>
        </label>
      )}
      {carpetas.length > 0 && (
        <label>Carpeta
          <select value={s.carpeta} onChange={e => cambia({ carpeta: e.target.value, sesionAgente: undefined })}
            disabled={carpetas.length < 2 && carpetas[0] === s.carpeta} title={s.carpeta}>
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
      <div className="fila-config">
        <button className="enlace peligro" onClick={() => alBorrar(s)}><Icono n="borrar" t={14} /> Borrar</button>
        <span className="hueco" />
        <button className="enlace" onClick={alCerrar}>Cerrar</button>
      </div>
    </div>,
    document.body,
  );
}
