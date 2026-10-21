import { useEffect, useState } from 'react';
import type { Proyecto } from '../api';
import { conversacion, type Sesion } from '../sesiones';
import * as T from '../tareas';
import Icono from './Icono';
import { EditorPlan, Estado as EstadoPlan, listaPlanes, type Plan } from './Planes';
import Programas from './Programas';

// Actividad: todo lo que corre a la vez en el proyecto, en un sitio. El agente de la sesión (si está trabajando),
// sus subagentes, tareas en segundo plano y lista de tareas; los programas del proyecto (fuera del agente, también
// los que adoptó pgocode), y los planes guardados.

export default function Actividad({ proyecto, carpeta, sesion, directorio, versionProcesos, alAbrir }: {
  proyecto: Proyecto; carpeta: string; sesion?: Sesion | null; directorio?: string; versionProcesos: number; alAbrir: (url: string) => void;
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

      <section>
        <h4>Subagentes y tareas <span className="gh-cuenta">{vivas.length}</span></h4>
        {vivas.length === 0 && hechas.length === 0 && !lista && <p className="suave pequeno">Nada en esta sesión todavía.</p>}
        {vivas.map(t => <FilaTarea key={t.id} t={t} />)}
        {lista && (
          <details className="lista-tareas-agente" open>
            <summary>Lista de tareas · {(lista.elementos || []).filter(e => e.estado === 'hecho').length}/{lista.elementos?.length || 0}</summary>
            <ul>{(lista.elementos || []).map((e, i) => <li key={i} className={e.estado}><span aria-hidden>{e.estado === 'hecho' ? '☑' : e.estado === 'curso' ? '◐' : '☐'}</span> {e.texto}</li>)}</ul>
          </details>
        )}
        {hechas.length > 0 && (
          <details className="act-hechas">
            <summary className="suave pequeno">Terminadas ({hechas.length})</summary>
            {hechas.map(t => <FilaTarea key={t.id} t={t} />)}
          </details>
        )}
      </section>

      <section>
        <h4>Programas del proyecto</h4>
        <p className="suave pequeno">Fuera del agente y sin tokens: los guardados, los lanzados aquí y los que el agente dejó en marcha.</p>
        <Programas proyecto={proyecto} carpeta={carpeta} version={versionProcesos} alAbrir={alAbrir} />
      </section>

      <section>
        <h4>Planes <span className="gh-cuenta">{planes?.length ?? 0}</span>
          <button className="icono" onClick={cargaPlanes} title="Volver a cargar" aria-label="Volver a cargar los planes"><Icono n="recargar" t={13} /></button></h4>
        {planes && planes.length === 0 && <p className="suave pequeno">Sin planes. Elige el modo «Plan» en el chat para crear uno.</p>}
        <ul className="planes">
          {(planes || []).map(p => (
            <li key={p.ruta}><button onClick={() => setPlan(p.ruta)} title={p.ruta}>
              <span className="recorta">{p.titulo}</span><EstadoPlan e={p.estado} />
              <span className="suave pequeno">{p.modificado ? new Date(p.modificado).toLocaleString('es-ES', { dateStyle: 'short', timeStyle: 'short' }) : ''}</span>
            </button></li>
          ))}
        </ul>
      </section>

      {plan && <EditorPlan proyecto={proyecto.id} directorio={directorio} ruta={plan} ocupado={trabajando}
        alCerrar={() => { setPlan(null); cargaPlanes(); }} alCambio={() => cargaPlanes()} alBorrar={() => cargaPlanes()}
        alEjecutar={p => { window.dispatchEvent(new CustomEvent('pgo:ejecutar-plan', { detail: p })); setPlan(null); cargaPlanes(); }} />}
    </div>
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
