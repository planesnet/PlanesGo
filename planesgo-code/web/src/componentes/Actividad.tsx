import { useEffect, useState } from 'react';
import { COPIES_CHANGED, listaCopias, openFolder, openOutside, quitaCopia, subfolderAction, type Proyecto } from '../api';
import * as AP from '../activityPanel';
import * as RP from '../reparto';
import * as C from '../copias';
import { ARCHIVE_SESSIONS, conversacion, NEW_SESSION_ON_BRANCH, OPEN_SESSION, titulo, type Sesion } from '../sesiones';
import * as PSF from '../psf';
import * as LA from '../liveActivity';
import * as TS from '../todoSync';
import TodoSyncList, { todoCount } from './TodoSyncList';
import Icono from './Icono';
import { EditorPlan, Estado as EstadoPlan, listaPlanes, PLANS_CHANGED, type Plan } from './Planes';
import Programas from './Programas';
import { askConfirm, showMessage } from './Modal';
import { MenuSesion, type Opcion } from './Proyectos';

// Actividad: todo lo que corre a la vez en el proyecto, en un sitio y en vivo, como un monitor de procesos: arriba la
// lista de tareas del agente (con la que hace ahora); después una línea por cosa en marcha (el agente y lo que hace,
// sus subagentes, las subtareas en paralelo, las órdenes en segundo plano, las preguntas aparte, los programas); al
// final, solo si hay, los planes y las copias de trabajo (git worktree). Nada de textos fijos: lo que explica cada
// cosa va al pasar el ratón o en «¿qué es cada cosa?».

export default function Actividad({ proyecto, carpeta, sesion, sesiones = [], directorio, versionProcesos, alAbrir }: {
  proyecto: Proyecto; carpeta: string; sesion?: Sesion | null; sesiones?: Sesion[]; directorio?: string; versionProcesos: number; alAbrir: (url: string) => void;
}) {
  // Lo que pasa en la sesión: la última instantánea del chat (liveActivity.ts) si la hay (también a mitad de turno,
  // aunque el panel se abra ahora) y, si no, las tareas guardadas en su conversación. Se actualiza con cada aviso.
  const [snap, setSnap] = useState<LA.LiveSnapshot | undefined>(undefined);
  useEffect(() => {
    setSnap(sesion ? LA.lastSnapshot(sesion.id) || (c => ({ sesion: sesion.id, tareas: c.flatMap(m => m.tareas || []), trabajando: false, todo: TS.sessionTodo(c, false) }))(conversacion(sesion.id)) : undefined);
    const aviso = (e: Event) => {
      const d = (e as CustomEvent<LA.LiveSnapshot>).detail;
      if (d.sesion === sesion?.id) setSnap(d);
    };
    window.addEventListener(LA.ACTIVITY_EVENT, aviso);
    return () => window.removeEventListener(LA.ACTIVITY_EVENT, aviso);
  }, [sesion?.id]);
  // Pulso de 1 s mientras algo corre: los tiempos avanzan solos.
  const [ahora, setAhora] = useState(Date.now());
  const pulso = LA.needsPulse(snap);
  useEffect(() => {
    setAhora(Date.now());
    if (!pulso) return;
    const t = setInterval(() => setAhora(Date.now()), 1000);
    return () => clearInterval(t);
  }, [pulso]);
  const trabajando = Boolean(snap?.trabajando);

  const todo = snap?.todo;
  const listas = LA.liveLists(snap, ahora);
  // Subtareas en paralelo: las sesiones hijas de esta (reparto de pgocode), con el estado real de cada una y de su unión.
  const repartos = RP.lee();
  const hijas = (sesion ? sesiones.filter(s => s.madre === sesion.id).sort((a, b) => (a.parte || 0) - (b.parte || 0)) : []).map(h => {
    const st = AP.childStatus(h, sesion, repartos);
    const r: LA.Row = { id: h.id, dot: st.kind === 'ok' ? 'ok' : st.kind === 'aviso' ? 'error' : 'curso', kind: 'parte ' + (h.parte || ''),
      name: titulo(h), detail: st.text, time: '', tokens: '', title: st.text + (h.carpeta ? '\n' + h.carpeta : '') };
    return r;
  });
  const terminados = [...listas.subagentsDone, ...listas.backgroundDone, ...hijas.filter(h => h.dot === 'ok')];

  // Planes del proyecto.
  const [todosPlanes, setPlanes] = useState<Plan[] | null>(null);
  // Plans of sessions that are over (archived, or an automatic PSF session already merged) do not show: they are done
  // and would only clutter the panel (they are still in the archived session).
  const terminadas = new Set(sesiones.filter(s => s.archivada || s.psf?.phase === 'merged' || s.psf?.phase === 'discarded').map(s => s.id));
  const planes = todosPlanes && todosPlanes.filter(p => !p.sesion || !terminadas.has(p.sesion));
  const enCurso = sesiones.filter(s => s.psf && !s.archivada && s.psf.phase !== 'merged' && s.psf.phase !== 'discarded').sort((a, b) => a.psf!.issue - b.psf!.issue);
  const [plan, setPlan] = useState<string | null>(null);
  const cargaPlanes = () => listaPlanes(proyecto.id, directorio).then(setPlanes).catch(() => setPlanes([]));
  useEffect(() => {
    cargaPlanes();
    // Un plan guardado o borrado en otro sitio (p. ej. «/plan» en el chat): se recarga la lista.
    const aviso = () => cargaPlanes();
    window.addEventListener(PLANS_CHANGED, aviso);
    return () => window.removeEventListener(PLANS_CHANGED, aviso);
  }, [proyecto.id, directorio]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="actividad">
      {todo && <TodoBoard v={todo} trabajando={trabajando} />}

      <section className="act-vivo" aria-label="Lo que está en marcha" aria-live="off">
        {!sesion ? <p className="suave pequeno">Sin sesión abierta.</p> : <LiveRow r={LA.agentRow(snap, ahora)} principal />}
        {listas.subagents.map(r => <LiveRow key={r.id} r={r} />)}
        {hijas.filter(h => h.dot !== 'ok').map(r => <LiveRow key={r.id} r={r} />)}
        {listas.background.map(r => <LiveRow key={r.id} r={r} />)}
        {listas.asides.map(r => <LiveRow key={r.id} r={r} />)}
        {terminados.length > 0 && (
          <details className="act-hechas">
            <summary className="suave pequeno">{terminados.length} {terminados.length === 1 ? 'terminado' : 'terminados'}</summary>
            {terminados.map(r => <LiveRow key={r.id} r={r} />)}
          </details>
        )}
      </section>

      {/* Automatic PSF sessions still running or waiting for a person (the finished ones are archived). */}
      {enCurso.length > 0 && (
        <section>
          <h4>Issues en curso <span className="gh-cuenta">{enCurso.length}</span></h4>
          <ul className="psf-running">
            {enCurso.map(s => (
              <li key={s.id}><button onClick={() => window.dispatchEvent(new CustomEvent(OPEN_SESSION, { detail: s.id }))} title={PSF.statusText(s.psf!, false)}
                className={sesion?.id === s.id ? 'activa' : ''}>
                <span className="recorta"><b>#{s.psf!.issue}</b> {s.psf!.title}</span>
                <span className={'psf-badge ' + (s.psf!.phase === 'error' || s.psf!.phase === 'blocked' ? 'failed' : PSF.isWaitingPhase(s.psf!.phase) ? 'waiting' : 'working')}>{PSF.PHASE_LABEL[s.psf!.phase]}</span>
              </button></li>
            ))}
          </ul>
        </section>
      )}

      {/* Solo los programas en marcha (los guardados, plegados): se definen en Propiedades de la sesión → Programas. */}
      <Programas proyecto={proyecto} carpeta={carpeta} sesion={sesion?.id} version={versionProcesos} alAbrir={alAbrir} asSection />

      {planes && planes.length > 0 && (
        <section>
          <h4>Planes <span className="gh-cuenta">{planes.length}</span>
            <button className="icono" onClick={cargaPlanes} title="Volver a cargar" aria-label="Volver a cargar los planes"><Icono n="recargar" t={13} /></button></h4>
          <ul className="planes">
            {planes.map(p => (
              <li key={p.ruta}><button onClick={() => setPlan(p.ruta)} title={p.ruta}>
                <span className="recorta">{p.numero ? <b className="plan-numero">#{p.numero}</b> : null} {p.titulo}</span><EstadoPlan e={p.estado} />
                <span className="suave pequeno">{p.modificado ? new Date(p.modificado).toLocaleString('es-ES', { dateStyle: 'short', timeStyle: 'short' }) : ''}</span>
              </button></li>
            ))}
          </ul>
        </section>
      )}

      <CopiasTrabajo proyecto={proyecto.id} sesiones={sesiones} />

      <ConceptGuide />

      {plan && <EditorPlan proyecto={proyecto.id} directorio={directorio} ruta={plan} ocupado={trabajando}
        alCerrar={() => { setPlan(null); cargaPlanes(); }} alCambio={() => cargaPlanes()} alBorrar={() => cargaPlanes()}
        alEjecutar={p => { window.dispatchEvent(new CustomEvent('pgo:ejecutar-plan', { detail: p })); setPlan(null); cargaPlanes(); }} />}
    </div>
  );
}

// TodoBoard: la lista de tareas del agente, arriba del todo: la que está haciendo AHORA, destacada (▷ si es la que
// hace según sus acciones y aún no la ha marcado); debajo lo que falta y, plegadas, las hechas; el progreso. Acabada y
// con el agente libre, plegada en una línea.
function TodoBoard({ v, trabajando }: { v: TS.SyncView; trabajando: boolean }) {
  // Una lista de un turno anterior no es la de ahora: plegada y apagada.
  const viva = !v.old && (trabajando || !v.finished);
  const pct = v.total ? Math.round(((v.done + v.probables) / v.total) * 100) : 100;
  return (
    <details className={'act-todo' + (viva ? ' viva' : '')} open={viva || undefined} title="La lista de tareas que el agente lleva para organizarse (TodoWrite, TaskCreate…)">
      <summary>
        <b>{v.old ? 'Lista de un turno anterior' : 'Tareas del agente'}</b>
        <span className="act-todo-barra" aria-hidden><span style={{ width: pct + '%' }} /></span>
        <span className="act-todo-cuenta" title={v.probables ? `${v.done} marcadas por el agente y ${v.probables} probablemente hechas según lo que ha hecho` : undefined}>{todoCount(v)}</span>
      </summary>
      <TodoSyncList v={v} trabajando={trabajando} />
    </details>
  );
}

// LiveRow: una línea de la lista en vivo: punto de estado, qué es, nombre y lo que hace, tiempo y tokens.
function LiveRow({ r, principal }: { r: LA.Row; principal?: boolean }) {
  return (
    <div className={'act-fila ' + r.dot + (principal ? ' principal' : '')} title={r.title}>
      <span className={'act-punto ' + r.dot} aria-label={r.dot} />
      <span className="act-tipo">{r.kind}</span>
      <span className="act-que"><b>{r.name}</b>{r.detail && !principal ? <span className="suave"> · {r.detail}</span> : null}</span>
      <span className="act-num">{r.time}</span>
      <span className="act-num suave">{r.tokens}</span>
      {principal && r.detail ? <span className="act-ahora">{r.detail}</span> : null}
    </div>
  );
}

// ConceptGuide: qué es cada cosa que corre a la vez y cómo se relaciona con el contexto y con git/GitHub.
function ConceptGuide() {
  return (
    <details className="act-conceptos">
      <summary className="suave pequeno">¿Qué es cada cosa?</summary>
      <table>
        <thead><tr><th /><th>Quién la lanza</th><th>Contexto</th><th>Carpeta y rama</th><th>Git y GitHub</th></tr></thead>
        <tbody>
          <tr><th>Agente</th><td>Tú, con cada mensaje. Debajo, lo que hace ahora; a la derecha, tiempo del turno, tokens del turno y % de contexto (sobre donde compacta)</td><td>La conversación de la sesión</td><td>Las de la sesión</td><td>Hace commits si se lo pides</td></tr>
          <tr><th>Subagente</th><td>El agente, dentro de su turno (pgocode puede sugerírselo)</td><td>Propio: lo que lee no llena tu conversación; solo vuelve su resumen</td><td>Las de la sesión</td><td>No crea ramas ni commits: sus cambios son los de la sesión</td></tr>
          <tr><th>Subtarea en paralelo</th><td>pgocode, al repartir una petición con partes independientes</td><td>Una sesión aparte, con su conversación</td><td>Su propia copia (git worktree) y rama <code>pgo/…</code></td><td>Al acabar se une a tu rama (merge local). Nada se sube a GitHub</td></tr>
          <tr><th>En segundo plano</th><td>El agente (una orden que sigue en marcha)</td><td>No ocupa contexto; su salida entra si la lee</td><td>Las de la sesión</td><td>—</td></tr>
          <tr><th>Lista de tareas</th><td>El agente, para organizarse</td><td>Va en su contexto (es corta)</td><td>—</td><td>—</td></tr>
          <tr><th>Pregunta aparte</th><td>Tú (con «//») o el enrutador</td><td>No entra en la conversación</td><td>Las de la sesión</td><td>—</td></tr>
          <tr><th>Programa</th><td>Tú (desde Actividad o el lanzador) o el agente</td><td>Fuera del agente: no gasta tokens</td><td>La de la sesión</td><td>—</td></tr>
        </tbody>
      </table>
    </details>
  );
}

// Subfolders (git worktree) of the project repositories: pgocode's, subagents' or the user's (also the ones whose
// folder was deleted by hand). Each one has its ⋮ menu (copias.ts: subfolderActions); removing only when nothing is
// lost, discarding (losing the uncommitted work) after a confirmation that lists it.
function CopiasTrabajo({ proyecto, sesiones }: { proyecto: number; sesiones: Sesion[] }) {
  const [copias, setCopias] = useState<C.Copia[] | null>(null);
  const [error, setError] = useState('');
  const [ocupada, setOcupada] = useState('');
  const [menu, setMenu] = useState<{ c: C.Copia; ancla: HTMLElement } | null>(null);
  // Reloading keeps the message of the last action (it is cleared when another action starts or by hand).
  const carga = () => { listaCopias(proyecto).then(setCopias).catch(e => { setCopias([]); setError((e as Error).message); }); };
  const recarga = () => { setError(''); carga(); };
  useEffect(() => {
    setCopias(null);
    recarga();
    // Se recarga sola al acabar un turno de la sesión (puede haber creado o quitado subcarpetas) y cuando un reparto
    // crea o une las de sus hijas. Varios avisos seguidos, una sola carga.
    let t: ReturnType<typeof setTimeout> | undefined;
    const pronto = () => { clearTimeout(t); t = setTimeout(carga, 800); };
    let trabajando = false;
    const actividad = (e: Event) => {
      const d = (e as CustomEvent<{ trabajando: boolean }>).detail;
      if (trabajando && !d.trabajando) pronto();
      trabajando = d.trabajando;
    };
    window.addEventListener(COPIES_CHANGED, pronto);
    window.addEventListener('pgo:actividad', actividad);
    return () => { clearTimeout(t); window.removeEventListener(COPIES_CHANGED, pronto); window.removeEventListener('pgo:actividad', actividad); };
  }, [proyecto]); // eslint-disable-line react-hooks/exhaustive-deps

  // Runs an action with the subfolder marked as busy; errors go to the list.
  const conCopia = async (c: C.Copia, f: () => Promise<void>) => {
    setOcupada(c.carpeta);
    setError('');
    try { await f(); } catch (e) { setError((e as Error).message); } finally { setOcupada(''); carga(); }
  };
  const archiva = (usuarios: Sesion[]) => {
    if (usuarios.length) window.dispatchEvent(new CustomEvent(ARCHIVE_SESSIONS, { detail: { proyecto, ids: usuarios.map(s => s.id) } }));
  };
  const haz = async (c: C.Copia, a: C.SubfolderAction) => {
    const usuarios = C.copyUsers(c, sesiones);
    switch (a) {
      case 'sesion':
        // The session already working there, if any; otherwise a new one on its branch.
        if (usuarios.length) window.dispatchEvent(new CustomEvent(OPEN_SESSION, { detail: usuarios[0].id }));
        else window.dispatchEvent(new CustomEvent(NEW_SESSION_ON_BRANCH, { detail: c.rama }));
        return;
      case 'abrir':
        return conCopia(c, () => openFolder(c.carpeta));
      case 'cambios':
        return conCopia(c, async () => {
          const r = await subfolderAction<{ texto: string }>(proyecto, c.carpeta, 'cambios');
          await showMessage(<pre className="texto-git">{r.texto}</pre>, { title: 'Cambios de la subcarpeta' });
        });
      case 'subir':
        if (!(await askConfirm(`¿Subir la rama «${c.rama}» a origin (git push)? Tiene ${c.sin_subir} ${c.sin_subir === 1 ? 'commit' : 'commits'} sin subir.`, { okText: 'Subir' }))) return;
        return conCopia(c, async () => { await subfolderAction(proyecto, c.carpeta, 'subir'); });
      case 'pr':
        return conCopia(c, async () => {
          const r = await subfolderAction<{ url: string }>(proyecto, c.carpeta, 'pr');
          await openOutside(r.url);
        });
      case 'quitar':
        if (!(await askConfirm(C.explicaQuitar(c, true, usuarios) + '\n\n¿Continuar?', { danger: true, okText: usuarios.length ? 'Quitar y archivar' : 'Quitar' }))) return;
        return conCopia(c, async () => {
          const r = await quitaCopia(proyecto, c.carpeta, true);
          if (r.aviso) setError(r.aviso);
          archiva(usuarios);
        });
      case 'descartar':
        if (!(await askConfirm(C.explicaDescartar(c, usuarios) + '\n\n¿Descartar?', { danger: true, okText: 'Descartar y quitar' }))) return;
        return conCopia(c, async () => { await subfolderAction(proyecto, c.carpeta, 'descartar'); archiva(usuarios); });
      case 'olvidar':
        return conCopia(c, async () => { await subfolderAction(proyecto, c.carpeta, 'olvidar'); });
    }
  };
  const opciones = (c: C.Copia): Opcion[] => C.subfolderActions(c).map(a => {
    const o = C.SUBFOLDER_ACTION[a];
    return { icono: o.icono, texto: o.texto, peligro: o.peligro, accion: () => { setMenu(null); haz(c, a); } };
  });

  if (!error && !copias?.length) return null;
  return (
    <section>
      <h4>Subcarpetas <span className="gh-cuenta">{copias?.length ?? 0}</span>
        <button className="icono" onClick={recarga} title="Volver a cargar" aria-label="Volver a cargar las subcarpetas"><Icono n="recargar" t={13} /></button></h4>
      {error && <p className="error pequeno">{error}</p>}
      {(copias || []).map(c => {
        const e = C.estado(c);
        return (
          <div key={c.carpeta} className="copia">
            <span className="copia-texto">
              <span className="recorta" title={c.carpeta}>{c.carpeta.split('/').pop()}</span>
              <span className="suave pequeno recorta" title={c.pendiente || c.error || c.carpeta}>
                {c.rama ? <code>{c.rama}</code> : 'sin rama'} · {C.ORIGENES[c.origen]}{c.pendiente ? ' · ' + c.pendiente : ''}
              </span>
            </span>
            <span className={'copia-estado ' + e}>{C.NOMBRE_ESTADO[e]}</span>
            <button className="icono puntos" disabled={ocupada === c.carpeta} aria-haspopup="menu" aria-expanded={menu?.c.carpeta === c.carpeta}
              onClick={ev => { const ancla = ev.currentTarget; setMenu(m => (m?.c.carpeta === c.carpeta ? null : { c, ancla })); }}
              title="Opciones de la subcarpeta" aria-label={'Opciones de la subcarpeta ' + c.carpeta}>
              {ocupada === c.carpeta ? '…' : <Icono n="puntos" t={15} />}
            </button>
          </div>
        );
      })}
      {menu && <MenuSesion ancla={menu.ancla} opciones={opciones(menu.c)} alCerrar={() => setMenu(null)} />}
    </section>
  );
}
