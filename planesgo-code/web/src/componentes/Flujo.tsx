import { useEffect, useMemo, useState } from 'react';
import { get, listaCopias, post, quitaCopia, type Proyecto } from '../api';
import * as C from '../copias';
import Icono from './Icono';
import { MenuSesion, type Opcion } from './Proyectos';
import { askConfirm } from './Modal';

// Flujo del repositorio (GitHub): la rama de la sesión y su pull request, las issues por fase (por empezar →
// en desarrollo → en revisión → en test → cerradas) y por clase (peticiones nuevas / mantenimiento), y las PR.
// Con la simbología de GitHub: issue abierta (verde), cerrada (morado); PR abierta, borrador, fusionada, cerrada.

interface Etiqueta { nombre: string; color: string }
interface PRCorta { numero: number; estado: string; borrador: boolean }
interface Issue {
  numero: number; titulo: string; url: string; estado: 'OPEN' | 'CLOSED'; tipo: string;
  clase: 'peticion' | 'mantenimiento' | 'sin-clasificar'; fase: Fase; etiquetas: Etiqueta[]; autor: string; asignados: string[];
  comentarios: number; hito?: string; prs: PRCorta[]; creada: string; actualizada: string; cerrada?: string;
}
interface PR {
  numero: number; titulo: string; url: string; estado: 'OPEN' | 'CLOSED' | 'MERGED'; borrador: boolean; rama: string; base: string;
  autor: string; revision: string; fusionable: string; checks: string; mas: number; menos: number; ficheros: number; commits: number;
  comentarios: number; issues: number[]; etiquetas: Etiqueta[]; creada: string; actualizada: string; fusionada?: string;
}
interface Estado { repo: string; url: string; defecto: string; tipos: string[]; etiquetas: Etiqueta[]; issues: Issue[]; prs: PR[]; abiertas: number; cerradas: number; prs_abiertas: number; leido: string }
type Fase = 'por-empezar' | 'desarrollo' | 'revision' | 'test' | 'cerrada';
type Clase = Issue['clase'] | 'todas';

const FASES: { id: Fase; nombre: string; ayuda: string }[] = [
  { id: 'por-empezar', nombre: 'Por empezar', ayuda: 'Abiertas, sin PR ni etiqueta de trabajo' },
  { id: 'desarrollo', nombre: 'En desarrollo', ayuda: 'Etiqueta «en-desarrollo» o PR en borrador' },
  { id: 'revision', nombre: 'En revisión', ayuda: 'Con una pull request abierta que la cierra' },
  { id: 'test', nombre: 'En test', ayuda: 'Etiqueta «en-test» o «en-dev-alfa»' },
  { id: 'cerrada', nombre: 'Cerradas', ayuda: 'Cerradas hace poco' },
];
const CLASES: { id: Clase; nombre: string }[] = [
  { id: 'todas', nombre: 'Todas' }, { id: 'peticion', nombre: 'Peticiones nuevas' },
  { id: 'mantenimiento', nombre: 'Mantenimiento' }, { id: 'sin-clasificar', nombre: 'Sin clasificar' },
];

// Colores de GitHub (Primer).
const VERDE = '#1a7f37', MORADO = '#8250df', ROJO = '#d1242f', GRIS = '#59636e', AMBAR = '#9a6700';

// Iconos al estilo de los Octicons (16×16, relleno).
function Oct({ n, color, t = 16 }: { n: 'issue' | 'issue-cerrada' | 'pr' | 'pr-borrador' | 'pr-fusionada' | 'pr-cerrada' | 'check' | 'x' | 'punto' | 'rama' | 'comentario' | 'github'; color?: string; t?: number }) {
  const p: Record<string, JSX.Element> = {
    issue: <><circle cx="8" cy="8" r="6.25" fill="none" stroke="currentColor" strokeWidth="1.5" /><circle cx="8" cy="8" r="1.5" /></>,
    'issue-cerrada': <><circle cx="8" cy="8" r="6.25" fill="none" stroke="currentColor" strokeWidth="1.5" /><path d="M5.3 8.2l1.8 1.8 3.6-3.8" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" /></>,
    pr: <g fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"><circle cx="4" cy="3.5" r="1.6" /><circle cx="4" cy="12.5" r="1.6" /><circle cx="12" cy="12.5" r="1.6" /><path d="M4 5.1v5.8M12 10.9V6.5a2 2 0 0 0-2-2H7.5" /><path d="M9 3l-1.6 1.5L9 6" /></g>,
    'pr-borrador': <g fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"><circle cx="4" cy="3.5" r="1.6" /><circle cx="4" cy="12.5" r="1.6" /><circle cx="12" cy="12.5" r="1.6" /><path d="M4 5.1v5.8M12 4v1M12 7.5v1" /></g>,
    'pr-fusionada': <g fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"><circle cx="4.5" cy="3.5" r="1.6" /><circle cx="4.5" cy="12.5" r="1.6" /><circle cx="12" cy="9" r="1.6" /><path d="M4.5 5.1v5.8M4.5 5.5c0 2.5 2.5 3.5 5.9 3.5" /></g>,
    'pr-cerrada': <g fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"><circle cx="4" cy="3.5" r="1.6" /><circle cx="4" cy="12.5" r="1.6" /><circle cx="12" cy="12.5" r="1.6" /><path d="M4 5.1v5.8M10.3 3.3l3.4 3.4M13.7 3.3l-3.4 3.4" /></g>,
    check: <path d="M3.5 8.5l3 3 6-7" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />,
    x: <path d="M4.5 4.5l7 7M11.5 4.5l-7 7" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />,
    punto: <circle cx="8" cy="8" r="3.5" />,
    rama: <g fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"><circle cx="4.5" cy="3.5" r="1.5" /><circle cx="4.5" cy="12.5" r="1.5" /><circle cx="11.5" cy="5" r="1.5" /><path d="M4.5 5v6M11.5 6.5c0 3-7 2.5-7 4.5" /></g>,
    comentario: <path d="M2.5 3.5h11v7h-6l-3 2.5v-2.5h-2z" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" />,
    github: <path d="M8 .7a7.3 7.3 0 0 0-2.3 14.2c.4.1.5-.2.5-.4v-1.3c-2 .4-2.5-.9-2.5-.9-.3-.9-.8-1.1-.8-1.1-.7-.5 0-.5 0-.5.7.1 1.1.8 1.1.8.7 1.1 1.7.8 2.1.6.1-.5.3-.8.5-1-1.6-.2-3.3-.8-3.3-3.6 0-.8.3-1.4.8-1.9-.1-.2-.3-.9.1-1.9 0 0 .6-.2 2 .7a6.9 6.9 0 0 1 3.7 0c1.4-.9 2-.7 2-.7.4 1 .2 1.7.1 1.9.5.5.8 1.1.8 1.9 0 2.8-1.7 3.4-3.3 3.6.3.2.5.7.5 1.3v2c0 .2.1.5.6.4A7.3 7.3 0 0 0 8 .7z" />,
  };
  return <svg width={t} height={t} viewBox="0 0 16 16" fill="currentColor" style={{ color, flex: 'none' }} aria-hidden="true">{p[n]}</svg>;
}

const hace = (iso?: string) => {
  if (!iso) return '';
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (s < 3600) return `hace ${Math.max(1, Math.round(s / 60))} min`;
  if (s < 86400) return `hace ${Math.round(s / 3600)} h`;
  const d = Math.round(s / 86400);
  return d < 60 ? `hace ${d} d` : `hace ${Math.round(d / 30)} meses`;
};

// Texto legible sobre el color de fondo de una etiqueta de GitHub.
const sobre = (hex: string) => {
  const n = parseInt(hex || 'cccccc', 16);
  return ((n >> 16) * 299 + ((n >> 8) & 255) * 587 + (n & 255) * 114) / 1000 > 150 ? '#1f2328' : '#fff';
};

const iconoPR = (p: { estado: string; borrador: boolean }) =>
  p.estado === 'MERGED' ? <Oct n="pr-fusionada" color={MORADO} /> : p.estado === 'CLOSED' ? <Oct n="pr-cerrada" color={ROJO} />
    : p.borrador ? <Oct n="pr-borrador" color={GRIS} /> : <Oct n="pr" color={VERDE} />;

function Checks({ estado }: { estado: string }) {
  if (!estado) return null;
  const [n, c, t] = estado === 'SUCCESS' ? ['check', VERDE, 'Checks correctos'] as const
    : estado === 'FAILURE' || estado === 'ERROR' ? ['x', ROJO, 'Checks fallidos'] as const : ['punto', AMBAR, 'Checks en curso'] as const;
  return <span className="gh-chip" title={t}><Oct n={n} color={c} t={13} /> {t.replace('Checks ', '')}</span>;
}

function Revision({ r }: { r: string }) {
  if (r === 'APPROVED') return <span className="gh-chip" style={{ color: VERDE }}><Oct n="check" t={13} /> Aprobada</span>;
  if (r === 'CHANGES_REQUESTED') return <span className="gh-chip" style={{ color: ROJO }}><Oct n="x" t={13} /> Cambios pedidos</span>;
  if (r === 'REVIEW_REQUIRED') return <span className="gh-chip" style={{ color: AMBAR }}><Oct n="punto" t={13} /> Revisión pendiente</span>;
  return <span className="gh-chip suave">Sin revisión</span>;
}

const Etiquetas = ({ l }: { l: Etiqueta[] }) => <>{l.map(e => (
  <span key={e.nombre} className="gh-etiqueta" style={{ background: '#' + e.color, color: sobre(e.color) }}>{e.nombre}</span>
))}</>;

// Lo que se le pasa al chat para resolver una issue.
export interface PeticionResolver { nombre: string; texto: string }

export default function Flujo({ proyecto, carpeta, rama, alAbrir, alResolver }: {
  proyecto: Proyecto; carpeta: string; rama: string; alAbrir: (url: string) => void; alResolver: (p: PeticionResolver) => void;
}) {
  const [e, setE] = useState<Estado | null>(null);
  const [error, setError] = useState('');
  const [cargando, setCargando] = useState(false);
  const [clase, setClase] = useState<Clase>('todas');
  const [creando, setCreando] = useState(false);
  const [creada, setCreada] = useState<{ numero: number; url: string } | null>(null);
  const [resolviendo, setResolviendo] = useState<number | null>(null);
  const [errorAccion, setErrorAccion] = useState('');

  // Resolver una issue: se lee su descripción y se pasa al chat (sesión nueva) para que el agente la resuelva.
  const resolver = async (i: Issue) => {
    setResolviendo(i.numero);
    setError('');
    try {
      const r = await get<{ repo: string; cuerpo: string }>(`/api/github/issue?proyecto=${proyecto.id}&carpeta=${encodeURIComponent(carpeta)}&numero=${i.numero}`);
      const texto = `Resuelve la issue #${i.numero} de ${r.repo}: ${i.titulo}\n\n${r.cuerpo || '(sin descripción)'}\n\n${i.url}`;
      alResolver({ nombre: `#${i.numero} ${i.titulo}`, texto });
    } catch (x) { setError((x as Error).message); } finally { setResolviendo(null); }
  };

  const carga = (recargar = false) => {
    if (!carpeta) return;
    setCargando(true);
    setError('');
    get<Estado>(`/api/github?proyecto=${proyecto.id}&carpeta=${encodeURIComponent(carpeta)}${recargar ? '&recargar=1' : ''}`)
      .then(setE).catch(x => setError(x.message)).finally(() => setCargando(false));
  };
  useEffect(() => { setE(null); carga(); }, [proyecto.id, carpeta]); // eslint-disable-line react-hooks/exhaustive-deps

  // Copias de trabajo locales por rama: una PR fusionada cuya rama tiene copia ofrece limpiar rama y copia.
  const [copias, setCopias] = useState<C.Copia[]>([]);
  const cargaCopias = () => { listaCopias(proyecto.id).then(setCopias).catch(() => setCopias([])); };
  useEffect(() => { if (e) cargaCopias(); }, [e]); // eslint-disable-line react-hooks/exhaustive-deps
  const copiaDe = (rama: string) => copias.find(c => c.rama === rama && c.worktree);
  const limpia = async (c: C.Copia) => {
    if (!(await askConfirm(C.explicaQuitar(c, true) + '\n\n¿Limpiar rama y copia?', { okText: 'Limpiar', danger: true }))) return;
    setErrorAccion('');
    try {
      const r = await quitaCopia(proyecto.id, c.carpeta, true);
      if (r.aviso) setErrorAccion(r.aviso);
    } catch (err) { setErrorAccion((err as Error).message); } finally { cargaCopias(); }
  };

  // Acciones de GitHub sobre una issue o PR (menú ⋮ de cada fila): cerrar, «no se hará», reabrir, borrar.
  const [menu, setMenu] = useState<{ tipo: 'issue' | 'pr'; numero: number; estado: string; ancla: HTMLElement } | null>(null);
  const accion = async (tipo: 'issue' | 'pr', numero: number, que: string) => {
    setErrorAccion('');
    try { await post('/api/github/accion', { proyecto: proyecto.id, carpeta, tipo, numero, accion: que }); carga(true); }
    catch (err) { setErrorAccion((err as Error).message); }
  };
  const abreMenu = (tipo: 'issue' | 'pr', numero: number, estado: string) => (ancla: HTMLElement) =>
    setMenu(m => (m && m.tipo === tipo && m.numero === numero ? null : { tipo, numero, estado, ancla }));
  const opciones = (m: NonNullable<typeof menu>): Opcion[] => m.tipo === 'issue'
    ? (m.estado === 'OPEN'
      ? [{ icono: 'hecho', texto: 'Cerrar como hecha', accion: () => accion('issue', m.numero, 'cerrar') },
        { icono: 'cerrar', texto: 'Cerrar como «no se hará»', accion: () => accion('issue', m.numero, 'descartar') },
        null,
        { icono: 'borrar', texto: 'Borrar la issue', peligro: true, accion: async () => { if (await askConfirm(`¿Borrar la issue #${m.numero}? No se puede deshacer (solo pueden los administradores del repositorio).`, { okText: 'Borrar', danger: true })) accion('issue', m.numero, 'borrar'); } }]
      : [{ icono: 'recargar', texto: 'Reabrir', accion: () => accion('issue', m.numero, 'reabrir') },
        null,
        { icono: 'borrar', texto: 'Borrar la issue', peligro: true, accion: async () => { if (await askConfirm(`¿Borrar la issue #${m.numero}? No se puede deshacer.`, { okText: 'Borrar', danger: true })) accion('issue', m.numero, 'borrar'); } }])
    : (m.estado === 'OPEN'
      ? [{ icono: 'cerrar', texto: 'Cerrar sin fusionar', accion: async () => { if (await askConfirm(`¿Cerrar la pull request #${m.numero} sin fusionarla?`, { okText: 'Cerrar' })) accion('pr', m.numero, 'cerrar'); } }]
      : [{ icono: 'recargar', texto: 'Reabrir', accion: () => accion('pr', m.numero, 'reabrir') }]);

  const abiertas = useMemo(() => (e?.issues || []).filter(i => i.estado === 'OPEN'), [e]);
  const cuenta = (c: Clase) => c === 'todas' ? abiertas.length : abiertas.filter(i => i.clase === c).length;
  const visibles = (e?.issues || []).filter(i => clase === 'todas' || i.clase === clase);
  const deRama = (e?.prs || []).filter(p => p.rama === rama && p.estado === 'OPEN');
  const aRama = (e?.prs || []).filter(p => p.base === rama && p.estado === 'OPEN');
  const prsAbiertas = (e?.prs || []).filter(p => p.estado === 'OPEN' && p.rama !== rama);
  const prsHechas = (e?.prs || []).filter(p => p.estado !== 'OPEN');
  const porNumero = new Map((e?.issues || []).map(i => [i.numero, i]));

  if (!carpeta) return <p className="suave relleno">La sesión no tiene carpeta local con repositorio.</p>;

  return (
    <div className="flujo">
      {errorAccion && <p className="error">{errorAccion}</p>}
      {menu && <MenuSesion ancla={menu.ancla} alCerrar={() => setMenu(null)} opciones={opciones(menu)} />}
      <div className="flujo-cabecera">
        <Oct n="github" t={18} />
        {e ? <button className="enlace gh-repo" onClick={() => alAbrir(e.url)} title="Abrir en GitHub">{e.repo}</button> : <span className="suave">GitHub</span>}
        <span className="hueco" />
        {e && <span className="t suave" title={new Date(e.leido).toLocaleString()}>{hace(e.leido)}</span>}
        {e && <button className="boton-nueva" onClick={() => { setCreando(c => !c); setCreada(null); }} aria-expanded={creando}><Oct n="issue" color={VERDE} t={14} /> Nueva issue</button>}
        <button className="icono" onClick={() => carga(true)} disabled={cargando} title="Volver a leer de GitHub" aria-label="Recargar"><Icono n="recargar" t={15} /></button>
      </div>
      {error && <p className="error">{error}</p>}
      {!e && !error && <p className="suave">Leyendo GitHub…</p>}
      {creada && <p className="gh-creada"><Oct n="issue" color={VERDE} t={14} /> Creada la issue <button className="enlace" onClick={() => alAbrir(creada.url)}>#{creada.numero}</button></p>}
      {e && creando && <NuevaIssue estado={e} proyecto={proyecto.id} carpeta={carpeta} alCancelar={() => setCreando(false)}
        alCrear={r => { setCreando(false); setCreada(r); carga(true); }} />}
      {e && <>
        {/* La rama de la sesión y su pull request */}
        <section className="gh-tarjeta">
          <div className="gh-titulo"><Oct n="rama" color={GRIS} /> <code className="gh-rama">{rama || '—'}</code>
            {rama === e.defecto && <span className="gh-chip suave">rama por defecto</span>}</div>
          {deRama.length === 0 && aRama.length === 0 && <p className="suave pequeno">Ninguna pull request abierta desde esta rama ni hacia ella.</p>}
          {deRama.map(p => <TarjetaPR key={p.numero} p={p} porNumero={porNumero} alAbrir={alAbrir} />)}
          {aRama.length > 0 && <>
            <p className="titulo-lista">Entran en esta rama</p>
            {aRama.map(p => <FilaPR key={p.numero} p={p} alAbrir={alAbrir} alMenu={p.estado === 'MERGED' ? undefined : abreMenu('pr', p.numero, p.estado)} />)}
          </>}
        </section>

        {/* Resumen y barra del flujo */}
        <div className="gh-resumen">
          <span><Oct n="issue" color={VERDE} t={14} /> <b>{e.abiertas}</b> abiertas</span>
          <span><Oct n="issue-cerrada" color={MORADO} t={14} /> <b>{e.cerradas}</b> cerradas</span>
          <span><Oct n="pr" color={VERDE} t={14} /> <b>{e.prs_abiertas}</b> PR abiertas</span>
        </div>
        <div className="gh-barra" role="img" aria-label="Issues abiertas por fase">
          {FASES.filter(f => f.id !== 'cerrada').map(f => {
            const n = abiertas.filter(i => i.fase === f.id).length;
            return n ? <span key={f.id} className={'gh-tramo ' + f.id} style={{ flexGrow: n }} title={`${f.nombre}: ${n}`}>{n}</span> : null;
          })}
        </div>
        <div className="filtros" role="tablist" aria-label="Clase de issue">
          {CLASES.map(c => (
            <button key={c.id} role="tab" aria-selected={clase === c.id} className="filtro" onClick={() => setClase(c.id)}>{c.nombre} <b>{cuenta(c.id)}</b></button>
          ))}
        </div>
        {clase !== 'todas' || cuenta('sin-clasificar') === 0 ? null : (
          <p className="suave pequeno">«Sin clasificar»: issues sin etiqueta de petición («mejora») ni de mantenimiento («corrección»).</p>
        )}

        {/* Issues por fase */}
        {FASES.map(f => {
          const l = visibles.filter(i => i.fase === f.id);
          if (!l.length) return null;
          return (
            <details key={f.id} className="gh-fase" open={f.id !== 'cerrada'}>
              <summary><span className={'gh-marca ' + f.id} /> {f.nombre} <span className="gh-cuenta">{l.length}</span><span className="suave pequeno"> · {f.ayuda}</span></summary>
              {l.map(i => <FilaIssue key={i.numero} i={i} alAbrir={alAbrir} alMenu={abreMenu('issue', i.numero, i.estado)} alResolver={i.estado === 'OPEN' ? () => resolver(i) : undefined} resolviendo={resolviendo === i.numero} />)}
            </details>
          );
        })}

        {/* Pull requests */}
        {prsAbiertas.length > 0 && <details className="gh-fase" open>
          <summary><Oct n="pr" color={VERDE} t={14} /> Pull requests abiertas <span className="gh-cuenta">{prsAbiertas.length}</span></summary>
          {prsAbiertas.map(p => <FilaPR key={p.numero} p={p} alAbrir={alAbrir} alMenu={p.estado === 'MERGED' ? undefined : abreMenu('pr', p.numero, p.estado)} />)}
        </details>}
        {prsHechas.length > 0 && <details className="gh-fase">
          <summary><Oct n="pr-fusionada" color={MORADO} t={14} /> Pull requests recientes <span className="gh-cuenta">{prsHechas.length}</span></summary>
          {prsHechas.map(p => {
            const c = p.estado === 'MERGED' ? copiaDe(p.rama) : undefined;
            return <FilaPR key={p.numero} p={p} alAbrir={alAbrir} alMenu={p.estado === 'MERGED' ? undefined : abreMenu('pr', p.numero, p.estado)}
              copia={c} alLimpiar={c ? () => limpia(c) : undefined} />;
          })}
        </details>}
      </>}
    </div>
  );
}

function FilaIssue({ i, alAbrir, alResolver, resolviendo, alMenu }: { i: Issue; alAbrir: (u: string) => void; alResolver?: () => void; resolviendo: boolean; alMenu?: (ancla: HTMLElement) => void }) {
  return (
    <div className="gh-fila-caja">
    <button className="gh-fila" onClick={() => alAbrir(i.url)} title={i.titulo}>
      {i.estado === 'OPEN' ? <Oct n="issue" color={VERDE} /> : <Oct n="issue-cerrada" color={MORADO} />}
      <span className="gh-cuerpo">
        <span className="gh-texto">{i.titulo} <Etiquetas l={i.etiquetas} /></span>
        <span className="gh-meta">
          #{i.numero} · {i.estado === 'OPEN' ? 'abierta ' + hace(i.creada) : 'cerrada ' + hace(i.cerrada)} por {i.autor}
          {i.hito && <> · {i.hito}</>}
          {i.prs.map(p => <span key={p.numero} className="gh-mini">{iconoPR(p)}#{p.numero}</span>)}
        </span>
      </span>
      {i.asignados.length > 0 && <span className="gh-asignados">{i.asignados.map(a => <span key={a} className="gh-avatar" title={a}>{a.slice(0, 2).toUpperCase()}</span>)}</span>}
      {i.comentarios > 0 && <span className="gh-coment" title={`${i.comentarios} comentarios`}><Oct n="comentario" t={13} /> {i.comentarios}</span>}
    </button>
    {alResolver && (
      <button className="gh-resolver" onClick={alResolver} disabled={resolviendo} title="Resolver: abre una sesión nueva y se la pide al agente" aria-label={'Resolver la issue #' + i.numero}>
        <Icono n="ejecutar" t={13} /> {resolviendo ? '…' : 'Resolver'}
      </button>
    )}
    {alMenu && <BotonMenu titulo={'Más acciones de la issue #' + i.numero} alAbrir={alMenu} />}
    </div>
  );
}

function FilaPR({ p, alAbrir, alMenu, copia, alLimpiar }: { p: PR; alAbrir: (u: string) => void; alMenu?: (ancla: HTMLElement) => void; copia?: C.Copia; alLimpiar?: () => void }) {
  return (
    <div className="gh-fila-caja">
    <button className="gh-fila" onClick={() => alAbrir(p.url)} title={p.titulo}>
      {iconoPR(p)}
      <span className="gh-cuerpo">
        <span className="gh-texto">{p.titulo} <Etiquetas l={p.etiquetas} /></span>
        <span className="gh-meta">#{p.numero} · <code>{p.rama}</code> → <code>{p.base}</code> · {p.estado === 'MERGED' ? 'fusionada ' + hace(p.fusionada) : hace(p.actualizada)} · {p.autor}</span>
      </span>
      {p.estado === 'OPEN' && p.checks && <Checks estado={p.checks} />}
      <span className="gh-diff"><b style={{ color: VERDE }}>+{p.mas}</b> <b style={{ color: ROJO }}>−{p.menos}</b></span>
    </button>
    {alLimpiar && copia && (
      <button className="gh-limpiar" onClick={alLimpiar} disabled={!copia.se_puede_quitar}
        title={copia.se_puede_quitar ? `Quita la copia local ${copia.carpeta} y la rama «${p.rama}» (ya fusionada)` : `La copia ${copia.carpeta} tiene trabajo sin guardar: ${copia.pendiente || copia.error || ''}`}>
        Limpiar rama y copia
      </button>
    )}
    {alMenu && <BotonMenu titulo={'Más acciones de la pull request #' + p.numero} alAbrir={alMenu} />}
    </div>
  );
}

// Botón ⋮ de una fila: abre el menú de acciones de GitHub.
function BotonMenu({ titulo, alAbrir }: { titulo: string; alAbrir: (ancla: HTMLElement) => void }) {
  return (
    <button className="icono puntos gh-menu" onClick={e => alAbrir(e.currentTarget)} title={titulo} aria-label={titulo} aria-haspopup="menu">
      <Icono n="puntos" t={15} />
    </button>
  );
}

function TarjetaPR({ p, porNumero, alAbrir }: { p: PR; porNumero: Map<number, Issue>; alAbrir: (u: string) => void }) {
  const total = p.mas + p.menos || 1;
  return (
    <div className="gh-pr">
      <button className="gh-fila" onClick={() => alAbrir(p.url)}>
        {iconoPR(p)}
        <span className="gh-cuerpo">
          <span className="gh-texto"><b>{p.titulo}</b> <span className="suave">#{p.numero}</span></span>
          <span className="gh-meta">{p.borrador ? 'Borrador' : 'Abierta'} · quiere fusionar {p.commits} commits en <code>{p.base}</code> desde <code>{p.rama}</code></span>
        </span>
      </button>
      <div className="gh-chips">
        <Checks estado={p.checks} />
        <Revision r={p.revision} />
        {p.fusionable === 'CONFLICTING' ? <span className="gh-chip" style={{ color: ROJO }}><Oct n="x" t={13} /> Conflictos</span>
          : p.fusionable === 'MERGEABLE' ? <span className="gh-chip" style={{ color: VERDE }}><Oct n="check" t={13} /> Sin conflictos</span> : null}
        <span className="gh-chip"><b style={{ color: VERDE }}>+{p.mas.toLocaleString()}</b>&nbsp;<b style={{ color: ROJO }}>−{p.menos.toLocaleString()}</b>&nbsp;· {p.ficheros} ficheros</span>
      </div>
      <div className="gh-diffbarra" title={`+${p.mas} −${p.menos}`}><span style={{ width: `${(p.mas / total) * 100}%` }} /></div>
      {p.issues.length > 0 && <div className="gh-cierra">Cierra: {p.issues.map(n => {
        const i = porNumero.get(n);
        return <span key={n} className="gh-mini">{i?.estado === 'CLOSED' ? <Oct n="issue-cerrada" color={MORADO} t={13} /> : <Oct n="issue" color={VERDE} t={13} />}#{n}{i ? ' ' + i.titulo : ''}</span>;
      })}</div>}
    </div>
  );
}

// Nueva issue: título, descripción (Markdown) y etiquetas del repositorio (que dicen si es petición o mantenimiento).
function NuevaIssue({ estado, proyecto, carpeta, alCrear, alCancelar }: {
  estado: Estado; proyecto: number; carpeta: string; alCrear: (r: { numero: number; url: string }) => void; alCancelar: () => void;
}) {
  const [titulo, setTitulo] = useState('');
  const [cuerpo, setCuerpo] = useState('');
  const [etiquetas, setEtiquetas] = useState<string[]>([]);
  const [enviando, setEnviando] = useState(false);
  const [error, setError] = useState('');
  const alterna = (n: string) => setEtiquetas(l => l.includes(n) ? l.filter(x => x !== n) : [...l, n]);
  const envia = async (ev: React.FormEvent) => {
    ev.preventDefault();
    if (!titulo.trim() || enviando) return;
    setEnviando(true);
    setError('');
    try { alCrear(await post<{ numero: number; url: string }>('/api/github/issue', { proyecto, carpeta, titulo, cuerpo, etiquetas })); }
    catch (x) { setError((x as Error).message); setEnviando(false); }
  };
  return (
    <form className="gh-nueva" onSubmit={envia}>
      <div className="gh-titulo"><Oct n="issue" color={VERDE} /> Nueva issue en <b>{estado.repo}</b></div>
      <input value={titulo} onChange={e => setTitulo(e.target.value)} placeholder="Título" autoFocus required maxLength={256} />
      <textarea value={cuerpo} onChange={e => setCuerpo(e.target.value)} placeholder="Descripción (Markdown): qué se pide, por qué, cómo comprobarlo…" rows={6} />
      {estado.etiquetas.length > 0 && (
        <div className="gh-opciones" aria-label="Etiquetas">
          <span className="suave pequeno">Etiquetas</span>
          {estado.etiquetas.map(et => (
            <button type="button" key={et.nombre} aria-pressed={etiquetas.includes(et.nombre)} className="gh-etiqueta gh-etiqueta-boton" onClick={() => alterna(et.nombre)}
              style={etiquetas.includes(et.nombre) ? { background: '#' + et.color, color: sobre(et.color) } : { borderColor: '#' + et.color }}>{et.nombre}</button>
          ))}
        </div>
      )}
      {error && <p className="error">{error}</p>}
      <div className="fila-config"><span className="hueco" /><button type="button" className="enlace" onClick={alCancelar}>Cancelar</button>
        <button className="primario" disabled={!titulo.trim() || enviando}>{enviando ? 'Creando…' : 'Crear issue'}</button></div>
    </form>
  );
}
