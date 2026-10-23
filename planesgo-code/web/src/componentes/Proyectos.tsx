import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { get, type Proveedor, type Proyecto, type Usuario } from '../api';
import { identificador, lista as sesionesDe, ordenaConHijas, titulo, type Repo, type Sesion } from '../sesiones';
import { guardaAjustes, leeAjustes, TIPOS_HOSTING, type AjustesProyecto, type Detalle, type SeleccionSkills, type TipoHosting } from '../ajustes';
import Icono from './Icono';
import { Hosting } from './Marca';
import { askConfirm, PropertiesModal, type PropertiesSection } from './Modal';
import './config-sesion.css';

interface Props {
  usuario: Usuario; actual: Proyecto | null; inicial: number; alElegir: (p: Proyecto) => void;
  repo: Repo | null; proveedores: Proveedor[];
  sesiones: Sesion[]; sesion: Sesion | null;
  alAjustes: (p: Proyecto, a: AjustesProyecto) => void;
  alElegirSesion: (p: Proyecto, s: Sesion) => void;
  alAjustesSesion: (s: Sesion) => void; alNuevaSesion: (p: Proyecto) => void; alCambiarSesion: (s: Sesion) => void; alBorrarSesion: (s: Sesion) => void;
  onDuplicateSession: (p: Proyecto, s: Sesion) => void;
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
  const guardaPlegados = (n: Set<number>) => {
    try { localStorage.setItem('pgo:plegados:' + usuario.login.toLowerCase(), JSON.stringify([...n])); } catch { /* sin almacenamiento */ }
    return n;
  };
  const pliega = (id: number) => setPlegados(prev => {
    const n = new Set(prev);
    if (n.has(id)) n.delete(id); else n.add(id);
    return guardaPlegados(n);
  });
  // Al cambiar o borrar una sesión de otro proyecto, se vuelve a leer su lista.
  const [, setCambios] = useState(0);
  const cambia = (s: Sesion) => { props.alCambiarSesion(s); setCambios(c => c + 1); };
  // Menú ⋮ de un proyecto y ventana de propiedades.
  const [menuP, setMenuP] = useState<{ id: number; ancla: HTMLElement } | null>(null);
  const [propiedades, setPropiedades] = useState<Proyecto | null>(null);
  const borra = (s: Sesion) => { props.alBorrarSesion(s); setCambios(c => c + 1); };

  // Cada proyecto que se abre pasa a ser el más reciente.
  // El proyecto elegido (o el de la sesión elegida) queda arriba y desplegado; el resto, debajo y plegados.
  useEffect(() => {
    if (!actual) return;
    const r = guardaReciente(usuario.login, actual.id);
    setRecientes(r);
    setPlegados(guardaPlegados(new Set(r.filter(id => id !== actual.id))));
  }, [actual?.id, usuario.login]); // eslint-disable-line react-hooks/exhaustive-deps

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

  // Un proyecto (como los grupos de Claude Code) con sus sesiones abiertas debajo.
  const grupo = (p: Proyecto) => {
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
              {!plegado && <Sesiones {...props} hostingDe={id => leeAjustes(usuario.login, id).hosting?.tipo} proyecto={p} lista={ss} alCambiarSesion={cambia} alBorrarSesion={borra} />}
            </section>
          );
  };

  return (
    <nav className="proyectos" aria-label="Proyectos">
      <div className="busca">
        <input placeholder="Buscar proyecto…" value={filtro} onChange={e => setFiltro(e.target.value)} aria-label="Buscar proyecto" />
        <button className="icono" title="Volver a cargar de Odoo" aria-label="Volver a cargar de Odoo" onClick={() => carga(true)}><Icono n="recargar" t={16} /></button>
      </div>
      <div className="lista">
        {error && <p className="error">{error}</p>}
        {!lista && !error && <p className="suave">Cargando proyectos de Odoo…</p>}
        {lista && !buscando && !visibles.some(p => p.id === actual?.id) && <p className="titulo-lista">Recientes</p>}
        {lista && visibles.length === 0 && <p className="suave">{buscando ? 'Ningún proyecto coincide.' : 'Aún no has abierto ningún proyecto. Búscalo arriba para empezar.'}</p>}
        {buscando ? visibles.map(p => (
          <button key={p.id} className={'proyecto' + (actual?.id === p.id ? ' activo' : '')} onClick={() => { alElegir(p); setFiltro(''); }}>
            <span className="nombre">{p.nombre}</span>
            <span className="detalle">
              {p.cliente?.nombre || 'Sin cliente'}{p.carpetas.length > 0 && <span className="local" title={p.carpetas.join('\n')}> · local</span>}
            </span>
          </button>
        )) : <>
          {visibles.filter(p => p.id === actual?.id).map(grupo)}
          {/* El resto de proyectos, pegados al borde inferior del panel. */}
          {visibles.some(p => p.id !== actual?.id) && (
            <div className={'otros-proyectos' + (visibles.some(p => p.id === actual?.id) ? '' : ' solos')}>
              {visibles.some(p => p.id === actual?.id) && <p className="titulo-lista">Otros proyectos</p>}
              {visibles.filter(p => p.id !== actual?.id).map(grupo)}
            </div>
          )}
        </>}
      </div>
      {propiedades && <PropiedadesProyecto p={propiedades} login={usuario.login} alCerrar={() => setPropiedades(null)}
        alGuardar={a => props.alAjustes(propiedades, a)} />}
    </nav>
  );
}

// General: el directorio principal del proyecto, donde se crean las carpetas de las ramas de las sesiones
// (directorio principal + rama). Por defecto, la carpeta que contiene el repositorio.
function General({ p, a, alCambiar }: { p: Proyecto; a: AjustesProyecto; alCambiar: (c: Partial<AjustesProyecto>) => void }) {
  const [sugerido, setSugerido] = useState('');
  const [error, setError] = useState('');
  useEffect(() => { get<{ principal: string }>(`/api/proyecto/ramas?proyecto=${p.id}`).then(r => setSugerido(r.principal)).catch(() => {}); }, [p.id]);
  const elegir = async () => {
    if (!window.psf?.elegirCarpeta) { setError('El diálogo de carpetas solo está en la aplicación de escritorio'); return; }
    const c = await window.psf.elegirCarpeta(a.directorio || sugerido);
    if (c) alCambiar({ directorio: c });
  };
  const actual = a.directorio || sugerido;
  return <>
    <h3>General</h3>
    <div className="campo">
      <span className="etiqueta-campo">Directorio principal del proyecto</span>
      <div className="carpeta-elegida">
        <code className="recorta" title={actual}>{actual || '—'}</code>
        <button type="button" className="boton-secundario" onClick={elegir}><Icono n="carpeta" t={14} /> Elegir…</button>
      </div>
      <span className="suave pequeno">
        Cada sesión trabaja en la carpeta de su rama: <code>{(actual || '…') + '/<rama>'}</code>.
        {!a.directorio && sugerido && ' Ahora se usa el de por defecto (la carpeta que contiene el repositorio).'}
      </span>
      {a.directorio && <button className="enlace" onClick={() => alCambiar({ directorio: undefined })}>Volver al de por defecto</button>}
      {error && <p className="error">{error}</p>}
    </div>
  </>;
}

// Alojamiento: tipo de hosting y sus datos de despliegue. Sin elegir, vale el que se deduce del repositorio.
function Alojamiento({ a, alCambiar }: { a: AjustesProyecto; alCambiar: (c: Partial<AjustesProyecto>) => void }) {
  const tipo = a.hosting?.tipo;
  const def = TIPOS_HOSTING.find(t => t.id === tipo);
  const pon = (t: TipoHosting | '') => alCambiar({ hosting: t ? { tipo: t, campos: a.hosting?.campos || {} } : undefined });
  const campo = (id: string, v: string) => a.hosting && alCambiar({ hosting: { ...a.hosting, campos: { ...a.hosting.campos, [id]: v } } });
  return <>
    <h3>Alojamiento</h3>
    <p className="suave pequeno">Dónde se ejecuta el programa del proyecto y cómo se despliega.</p>
    <div className="tipos-hosting" role="radiogroup" aria-label="Tipo de alojamiento">
      <button type="button" role="radio" aria-checked={!tipo} className="tipo-hosting" onClick={() => pon('')}>
        <span className="tipo-icono"><Icono n="recargar" t={16} /></span><span><b>Según el repositorio</b><span className="suave pequeno">Se deduce de .psf/proyecto.yml</span></span>
      </button>
      {TIPOS_HOSTING.map(t => (
        <button type="button" key={t.id} role="radio" aria-checked={tipo === t.id} className="tipo-hosting" onClick={() => pon(t.id)}>
          <span className="tipo-icono"><Hosting tipo={t.id} t={20} /></span><span><b>{t.nombre}</b><span className="suave pequeno">{t.descripcion}</span></span>
        </button>
      ))}
    </div>
    {def && def.campos.length > 0 && (
      <div className="campos-hosting">
        {def.campos.map(c => (
          <label key={c.id}>{c.nombre}
            <input value={a.hosting?.campos[c.id] || ''} placeholder={c.ejemplo} onChange={e => campo(c.id, e.target.value)} />
            {c.ayuda && <span className="suave pequeno">{c.ayuda}</span>}
          </label>
        ))}
      </div>
    )}
  </>;
}

// Project properties window (PropertiesModal: sections on the left, their properties on the right).
type ProjectSection = 'general' | 'conversation' | 'skills' | 'hosting';
const PROJECT_SECTIONS: readonly PropertiesSection<ProjectSection>[] = [{ id: 'general', name: 'General', icon: 'carpeta' }, { id: 'conversation', name: 'Conversación', icon: 'comentario' },
  { id: 'skills', name: 'Skills', icon: 'skill' }, { id: 'hosting', name: 'Alojamiento', icon: 'equipo' }];
const SELECCION: { id: SeleccionSkills; nombre: string; ayuda: string }[] = [
  { id: 'agente', nombre: 'Decide el agente', ayuda: 'Como siempre: el modelo lee las descripciones de las skills y decide él. pgocode no interviene.' },
  { id: 'sugerir', nombre: 'pgocode sugiere', ayuda: 'pgocode calcula qué skill encaja y lo muestra junto al mensaje, sin cambiar lo que se envía al agente. Sirve para ver si el agente acierta.' },
  { id: 'elegir', nombre: 'pgocode elige', ayuda: 'Si una skill encaja con claridad (pasa el umbral), pgocode se la indica al agente al enviar (en Claude Code, invocándola directamente con /skill). El agente no tiene que adivinar.' },
];

const DETALLES: { id: Detalle; nombre: string; ayuda: string }[] = [
  { id: 'conciso', nombre: 'Conciso', ayuda: 'Solo la respuesta del agente. Lo que hace (leer, editar, ejecutar…) queda en una línea «N acciones» que se puede desplegar.' },
  { id: 'normal', nombre: 'Normal', ayuda: 'La respuesta y cada acción en una línea (nombre y fichero u orden), plegada.' },
  { id: 'detallado', nombre: 'Detallado', ayuda: 'Todo desplegado: entrada y salida de cada acción y el razonamiento del agente.' },
];

function PropiedadesProyecto({ p, login, alCerrar, alGuardar }: { p: Proyecto; login: string; alCerrar: () => void; alGuardar: (a: AjustesProyecto) => void }) {
  const [a, setA] = useState<AjustesProyecto>(() => leeAjustes(login, p.id));
  const cambia = (c: Partial<AjustesProyecto>) => {
    const n = { ...a, ...c };
    setA(n);
    guardaAjustes(login, p.id, n);
    alGuardar(n);
  };
  return (
    <PropertiesModal title={<><span className="suave">Propiedades ·</span> {p.nombre}</>} label={'Propiedades de ' + p.nombre}
      sections={PROJECT_SECTIONS} onClose={alCerrar}>
      {section => <>
        {section === 'general' && <General p={p} a={a} alCambiar={cambia} />}
        {section === 'hosting' && <Alojamiento a={a} alCambiar={cambia} />}
        {section === 'skills' && <>
          <h3>Skills</h3>
          <fieldset className="opciones-detalle">
            <legend>Cómo se elige la skill de cada petición</legend>
            <p className="suave pequeno">pgocode compara cada petición con las skills del agente de la sesión mediante embeddings, en local (Ollama, embeddinggemma; si no está, Vertex AI): unos 90 ms y sin gastar tokens del agente.</p>
            {SELECCION.map(d => (
              <label key={d.id} className={'opcion-detalle' + (a.skills === d.id ? ' elegida' : '')}>
                <input type="radio" name="skills" checked={a.skills === d.id} onChange={() => cambia({ skills: d.id })} />
                <span><b>{d.nombre}</b><span className="suave pequeno">{d.ayuda}</span></span>
              </label>
            ))}
          </fieldset>
        </>}
        {section === 'conversation' && <>
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
      </>}
    </PropertiesModal>
  );
}

// Sesiones de un proyecto, como en Claude: icono del hosting, título, rama y menú ⋮ (renombrar, borrar, ajustes).
function Sesiones({ hostingDe, proyecto, lista, sesion, alElegirSesion, alCambiarSesion, alBorrarSesion, alAjustesSesion, onDuplicateSession }: Props & { proyecto: Proyecto; lista: Sesion[]; hostingDe: (id: number) => string | undefined }) {
  // Sesión con el menú ⋮ abierto y el botón que lo abrió (para colocar el menú flotante a su lado).
  const [menu, setMenu] = useState<{ id: string; ancla: HTMLElement } | null>(null);
  const [renombrando, setRenombrando] = useState<string | null>(null);
  return (
    <div className="sesiones">
      {lista.length === 0 && <p className="sin-sesiones">Sin sesiones</p>}
      {ordenaConHijas(lista).map(({ s, hija }) => (
        <div key={s.id} className={'sesion' + (sesion?.id === s.id ? ' activa' : '') + (hija ? ' hija' : '')}>
          {renombrando === s.id ? (
            <Renombra s={s} alFin={nombre => { setRenombrando(null); if (nombre !== null) alCambiarSesion({ ...s, nombre }); }} />
          ) : (
            <button className="sesion-boton" onClick={() => alElegirSesion(proyecto, s)} title={s.carpeta + (s.rama ? ' · ' + s.rama : '')}>
              <Hosting tipo={s.hosting || hostingDe(proyecto.id) || s.tipo} t={13} enBlanco />
              <span className={'recorta' + (s.nombre ? '' : ' suave')}>{titulo(s)}</span>
              {hija && <span className="insignia-parte" title="Sesión hija: una parte de un reparto en paralelo">parte {s.parte}</span>}
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
              // Para analizar la sesión (registro de tiempos, depuracion.log, conversación del agente) a partir de sus ids.
              { icono: 'copiar', texto: 'Copiar identificador', accion: () => navigator.clipboard.writeText(identificador(s)).catch(() => {}) },
              // Misma configuración, conversación vacía: el agente empieza de cero.
              { icono: 'nuevo', texto: 'Duplicar sin historial', accion: () => onDuplicateSession(proyecto, s) },
              { icono: 'borrar', texto: 'Borrar', peligro: true, accion: async () => { if (await askConfirm(`¿Borrar la sesión «${titulo(s)}» y su conversación?`, { okText: 'Borrar', danger: true })) alBorrarSesion(s); } },
              null,
              { icono: 'ajustes', texto: 'Ajustes', accion: () => alAjustesSesion(s) },
            ]} />
          )}
        </div>
      ))}
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

export type Opcion = { icono: string; texto: string; accion: () => void; peligro?: boolean } | null;
const ANCHO_MENU = 200, MARGEN = 8;

// Menú flotante del ⋮ (portal sobre document.body, position: fixed), como el de las sesiones de Claude.
export function MenuSesion({ ancla, opciones, alCerrar }: { ancla: HTMLElement; opciones: Opcion[]; alCerrar: () => void }) {
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

