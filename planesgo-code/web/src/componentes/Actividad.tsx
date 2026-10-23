import { useEffect, useState } from 'react';
import { listaCopias, quitaCopia, type Proyecto } from '../api';
import * as C from '../copias';
import { conversacion, titulo, type Sesion } from '../sesiones';
import * as T from '../tareas';
import Icono from './Icono';
import { EditorPlan, Estado as EstadoPlan, listaPlanes, type Plan } from './Planes';
import Programas from './Programas';
import { askConfirm } from './Modal';

// Actividad: todo lo que corre a la vez en el proyecto, en un sitio. El agente de la sesión (si está trabajando),
// sus subagentes, tareas en segundo plano y lista de tareas; los programas del proyecto (fuera del agente, también
// los que adoptó pgocode), los planes guardados y las copias de trabajo (git worktree) de los repositorios.

export default function Actividad({ proyecto, carpeta, sesion, sesiones = [], directorio, versionProcesos, alAbrir }: {
  proyecto: Proyecto; carpeta: string; sesion?: Sesion | null; sesiones?: Sesion[]; directorio?: string; versionProcesos: number; alAbrir: (url: string) => void;
}) {
  // Tareas de la sesión: las guardadas en su conversación y, en vivo, las que avisa el chat.
  const [tareas, setTareas] = useState<T.Tarea[]>([]);
  const [trabajando, setTrabajando] = useState(false);
  useEffect(() => {
    setTareas(sesion ? conversacion(sesion.id).flatMap(m => m.tareas || []) : []);
    setTrabajando(false);
    const aviso = (e: Event) => {
      const d = (e as CustomEvent<{ sesion: string; tareas: T.Tarea[]; trabajando: boolean }>).detail;
      if (d.sesion !== sesion?.id) return;
      setTareas(d.tareas);
      setTrabajando(d.trabajando);
    };
    window.addEventListener('pgo:actividad', aviso);
    return () => window.removeEventListener('pgo:actividad', aviso);
  }, [sesion?.id]);

  const vivas = T.vivas(tareas);
  const hechas = tareas.filter(t => t.tipo !== 'lista' && t.estado !== 'curso').reverse().slice(0, 15);
  const deTipo = (l: T.Tarea[], sub: boolean) => l.filter(t => (t.tipo === 'subagente') === sub);
  // Subtareas en paralelo: las sesiones hijas de esta (reparto de pgocode).
  const hijas = sesion ? sesiones.filter(s => s.madre === sesion.id).sort((a, b) => (a.parte || 0) - (b.parte || 0)) : [];
  const lista = [...tareas].reverse().find(t => t.tipo === 'lista');

  // Planes del proyecto.
  const [planes, setPlanes] = useState<Plan[] | null>(null);
  const [plan, setPlan] = useState<string | null>(null);
  const cargaPlanes = () => listaPlanes(proyecto.id, directorio).then(setPlanes).catch(() => setPlanes([]));
  useEffect(() => { cargaPlanes(); }, [proyecto.id, directorio]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="actividad">
      <section>
        <h4>Agente{sesion ? <span className="suave"> · {sesion.nombre || 'sesión actual'}</span> : null}</h4>
        {!sesion ? <p className="suave pequeno">Sin sesión abierta.</p> : (
          <p className="act-agente"><span className={'punto-h' + (trabajando ? ' curso' : '')} /> {trabajando ? 'Trabajando' : 'Libre'}
            {vivas.length > 0 && <span className="suave"> · {vivas.length} en marcha</span>}</p>
        )}
      </section>

      <ConceptGuide />

      <section>
        <h4>Subagentes <span className="gh-cuenta">{deTipo(vivas, true).length}</span></h4>
        <p className="suave pequeno">Los lanza el agente dentro de su turno. Cada uno con su propio contexto: lo que lee no entra en tu conversación, solo su resumen. Trabajan en la misma carpeta y rama de la sesión.</p>
        {deTipo(vivas, true).length === 0 && deTipo(hechas, true).length === 0 && <p className="suave pequeno">Ninguno en esta sesión todavía.</p>}
        {deTipo(vivas, true).map(t => <FilaTarea key={t.id} t={t} />)}
        {deTipo(hechas, true).length > 0 && (
          <details className="act-hechas">
            <summary className="suave pequeno">Terminados ({deTipo(hechas, true).length})</summary>
            {deTipo(hechas, true).map(t => <FilaTarea key={t.id} t={t} />)}
          </details>
        )}
      </section>

      <section>
        <h4>Subtareas en paralelo <span className="gh-cuenta">{hijas.length}</span></h4>
        <p className="suave pequeno">Las lanza pgocode al repartir una petición con partes independientes: cada una es una sesión aparte, con su conversación, en su propia copia del repositorio y su rama <code>pgo/…</code>. Al acabar, pgocode une su rama a la tuya (en local: no se sube nada a GitHub).</p>
        {hijas.length === 0 && <p className="suave pequeno">Ninguna: esta sesión no ha repartido nada.</p>}
        {hijas.map(h => (
          <div key={h.id} className="act-tarea">
            <span className="tarea-tipo">Parte {h.parte}</span>
            <span className="act-tarea-texto">
              <span className="recorta">{titulo(h)}</span>
              <span className="suave pequeno recorta" title={h.carpeta}>{h.rama === sesion?.rama
                ? <>unida: sus cambios ya están en <code>{sesion?.rama}</code></>
                : <><code>{h.rama || 'sin rama'}</code> → se unirá a <code>{sesion?.rama || 'tu rama'}</code></>}</span>
            </span>
          </div>
        ))}
      </section>

      <section>
        <h4>En segundo plano <span className="gh-cuenta">{deTipo(vivas, false).length}</span></h4>
        <p className="suave pequeno">Órdenes que el agente dejó en marcha (un servidor, una compilación larga). No ocupan contexto mientras corren; su salida solo entra si el agente la lee.</p>
        {deTipo(vivas, false).length === 0 && deTipo(hechas, false).length === 0 && <p className="suave pequeno">Nada en marcha.</p>}
        {deTipo(vivas, false).map(t => <FilaTarea key={t.id} t={t} />)}
        {deTipo(hechas, false).length > 0 && (
          <details className="act-hechas">
            <summary className="suave pequeno">Terminadas ({deTipo(hechas, false).length})</summary>
            {deTipo(hechas, false).map(t => <FilaTarea key={t.id} t={t} />)}
          </details>
        )}
      </section>

      {lista && (
      <section>
        <h4>Lista de tareas del agente</h4>
          <details className="lista-tareas-agente" open>
            <summary>Lista de tareas · {(lista.elementos || []).filter(e => e.estado === 'hecho').length}/{lista.elementos?.length || 0}</summary>
            <ul>{(lista.elementos || []).map((e, i) => <li key={i} className={e.estado}><span aria-hidden>{e.estado === 'hecho' ? '☑' : e.estado === 'curso' ? '◐' : '☐'}</span> {e.texto}</li>)}</ul>
          </details>
      </section>
      )}

      <section>
        <h4>Programas del proyecto</h4>
        <p className="suave pequeno">Fuera del agente y sin tokens: los guardados, los lanzados aquí y los que el agente dejó en marcha. Se definen en las propiedades de la sesión.</p>
        <Programas proyecto={proyecto} carpeta={carpeta} version={versionProcesos} alAbrir={alAbrir} />
      </section>

      <section>
        <h4>Planes <span className="gh-cuenta">{planes?.length ?? 0}</span>
          <button className="icono" onClick={cargaPlanes} title="Volver a cargar" aria-label="Volver a cargar los planes"><Icono n="recargar" t={13} /></button></h4>
        {planes && planes.length === 0 && <p className="suave pequeno">Sin planes. Elige el modo «Plan» en el chat para crear uno.</p>}
        <ul className="planes">
          {(planes || []).map(p => (
            <li key={p.ruta}><button onClick={() => setPlan(p.ruta)} title={p.ruta}>
              <span className="recorta">{p.numero ? <b className="plan-numero">#{p.numero}</b> : null} {p.titulo}</span><EstadoPlan e={p.estado} />
              <span className="suave pequeno">{p.modificado ? new Date(p.modificado).toLocaleString('es-ES', { dateStyle: 'short', timeStyle: 'short' }) : ''}</span>
            </button></li>
          ))}
        </ul>
      </section>

      <CopiasTrabajo proyecto={proyecto.id} />

      {plan && <EditorPlan proyecto={proyecto.id} directorio={directorio} ruta={plan} ocupado={trabajando}
        alCerrar={() => { setPlan(null); cargaPlanes(); }} alCambio={() => cargaPlanes()} alBorrar={() => cargaPlanes()}
        alEjecutar={p => { window.dispatchEvent(new CustomEvent('pgo:ejecutar-plan', { detail: p })); setPlan(null); cargaPlanes(); }} />}
    </div>
  );
}

// ConceptGuide: qué es cada cosa que corre a la vez y cómo se relaciona con el contexto y con git/GitHub.
function ConceptGuide() {
  return (
    <details className="act-conceptos">
      <summary className="suave pequeno">¿Subagente, subtarea, segundo plano…? Qué es cada cosa</summary>
      <table>
        <thead><tr><th /><th>Quién la lanza</th><th>Contexto</th><th>Carpeta y rama</th><th>Git y GitHub</th></tr></thead>
        <tbody>
          <tr><th>Subagente</th><td>El agente, dentro de su turno (pgocode puede sugerírselo)</td><td>Propio: lo que lee no llena tu conversación; solo vuelve su resumen</td><td>Las de la sesión</td><td>No crea ramas ni commits: sus cambios son los de la sesión</td></tr>
          <tr><th>Subtarea en paralelo</th><td>pgocode, al repartir una petición con partes independientes</td><td>Una sesión aparte, con su conversación</td><td>Su propia copia (git worktree) y rama <code>pgo/…</code></td><td>Al acabar se une a tu rama (merge local). Nada se sube a GitHub</td></tr>
          <tr><th>En segundo plano</th><td>El agente (una orden que sigue en marcha)</td><td>No ocupa contexto; su salida entra si la lee</td><td>Las de la sesión</td><td>—</td></tr>
          <tr><th>Lista de tareas</th><td>El agente, para organizarse</td><td>Va en su contexto (es corta)</td><td>—</td><td>—</td></tr>
          <tr><th>Programa</th><td>Tú o el agente (<code>pgo ejecutar</code>)</td><td>Fuera del agente: no gasta tokens</td><td>La de la sesión</td><td>—</td></tr>
        </tbody>
      </table>
    </details>
  );
}

function FilaTarea({ t }: { t: T.Tarea }) {
  return (
    <div className={'act-tarea ' + (t.estado || '')}>
      <span className="punto-h" aria-label={t.estado} />
      <span className="tarea-tipo">{T.NOMBRES[t.tipo || ''] || 'Tarea'}{t.subtipo ? ' · ' + t.subtipo : ''}</span>
      <span className="act-tarea-texto">
        <span className="recorta">{t.titulo || t.id}</span>
        {(t.progreso || t.detalle) && <span className="suave pequeno recorta" title={t.detalle}>{t.estado === 'curso' ? t.progreso : t.detalle}</span>}
      </span>
    </div>
  );
}

// Copias de trabajo (git worktree) de los repositorios del proyecto: de pgocode, de subagentes o del usuario.
// «Quitar» solo aparece si no se pierde nada; si la rama está fusionada, se borra también (git branch -d).
function CopiasTrabajo({ proyecto }: { proyecto: number }) {
  const [copias, setCopias] = useState<C.Copia[] | null>(null);
  const [error, setError] = useState('');
  const [quitando, setQuitando] = useState('');
  const carga = () => { setError(''); listaCopias(proyecto).then(setCopias).catch(e => { setCopias([]); setError((e as Error).message); }); };
  useEffect(() => { setCopias(null); carga(); }, [proyecto]); // eslint-disable-line react-hooks/exhaustive-deps
  const quita = async (c: C.Copia) => {
    if (!(await askConfirm(C.explicaQuitar(c, true) + '\n\n¿Continuar?', { danger: true }))) return;
    setQuitando(c.carpeta);
    setError('');
    try {
      const r = await quitaCopia(proyecto, c.carpeta, true);
      if (r.aviso) setError(r.aviso);
    } catch (e) { setError((e as Error).message); } finally { setQuitando(''); carga(); }
  };
  return (
    <section>
      <h4>Copias de trabajo <span className="gh-cuenta">{copias?.length ?? 0}</span>
        <button className="icono" onClick={carga} title="Volver a cargar" aria-label="Volver a cargar las copias de trabajo"><Icono n="recargar" t={13} /></button></h4>
      {copias && copias.length === 0 && !error && <p className="suave pequeno">Sin copias de trabajo (git worktree) en los repositorios del proyecto.</p>}
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
            {c.se_puede_quitar && <button className="copia-quitar" onClick={() => quita(c)} disabled={quitando === c.carpeta}
              title={'Quitar la copia de trabajo ' + c.carpeta}>{quitando === c.carpeta ? '…' : 'Quitar'}</button>}
          </div>
        );
      })}
    </section>
  );
}
