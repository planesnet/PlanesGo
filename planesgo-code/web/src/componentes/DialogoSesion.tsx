import { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { get, post, type Proveedor } from '../api';
import { leeAjustes, TIPOS_HOSTING, type TipoHosting } from '../ajustes';
import { identificador, titulo, type Repo, type Sesion } from '../sesiones';
import Icono from './Icono';
import { Agente, Hosting } from './Marca';
import * as CO from '../costes';

// Ventana de la sesión (crear o ajustar), en dos paneles: secciones a la izquierda y sus propiedades a la derecha.
// Lo primero, la rama del repositorio; después el nombre, el alojamiento
// (obligatorio) y la carpeta local donde vive esa rama: por defecto, el directorio principal del proyecto + el
// nombre de la rama (o la carpeta donde ya está descargada), elegible con el diálogo de carpetas del sistema.

interface Ramas { repo: string; repositorio?: string; ramas: string[]; actual?: string; copias: Record<string, string>; principal: string; tipo?: string }

const NUEVA = '\u0000nueva';
// Secciones de la ventana, agrupadas por su relación: a la izquierda la lista; a la derecha, sus propiedades.
const SECCIONES = [{ id: 'general', nombre: 'General', icono: 'rama' }, { id: 'agente', nombre: 'Agente', icono: 'terminal' },
  { id: 'acceso', nombre: 'Acceso a carpetas', icono: 'carpeta' }, { id: 'conversacion', nombre: 'Conversación', icono: 'comentario' },
  { id: 'gasto', nombre: 'Gasto e imputación', icono: 'reloj' }, { id: 'identificador', nombre: 'Identificador', icono: 'info' }] as const;
type Seccion = typeof SECCIONES[number]['id'];
const unir = (dir: string, rama: string) => dir.replace(/\/+$/, '') + '/' + rama.replace(/\//g, '_');

export default function DialogoSesion({ sesion, nueva, login, proveedores, alCerrar, alGuardar }: {
  sesion: Sesion; nueva: boolean; login: string; proveedores: Proveedor[];
  alCerrar: () => void; alGuardar: (s: Sesion, repo: Repo | null) => void;
}) {
  const [s, setS] = useState<Sesion>(sesion);
  const [seccion, setSeccion] = useState<Seccion>('general');
  const [info, setInfo] = useState<Ramas | null>(null);
  const [ramaNueva, setRamaNueva] = useState('');
  const [carpetaElegida, setCarpetaElegida] = useState(!nueva); // la carpeta la ha puesto el usuario: no se recalcula
  const [error, setError] = useState('');
  const [guardando, setGuardando] = useState(false);
  const [tareas, setTareas] = useState<{ id: number; nombre: string }[] | null>(null);
  const [mcps, setMcps] = useState<string[] | null>(null);
  useEffect(() => { if (s.carpeta) get<string[]>(`/api/agentes/mcp?proyecto=${s.proyecto}&carpeta=${encodeURIComponent(s.carpeta)}`).then(setMcps).catch(() => setMcps([])); }, [s.proyecto, s.carpeta]);
  useEffect(() => { get<{ id: number; nombre: string }[]>(`/api/proyecto/tareas?proyecto=${sesion.proyecto}`).then(setTareas).catch(() => setTareas([])); }, [sesion.proyecto]);
  const cambia = (c: Partial<Sesion>) => setS(x => ({ ...x, ...c }));
  const prov = proveedores.find(p => p.id === s.agente);
  const ajustes = leeAjustes(login, s.proyecto);
  const directorio = ajustes.directorio || info?.principal || '';

  useEffect(() => {
    get<Ramas>(`/api/proyecto/ramas?proyecto=${s.proyecto}`).then(r => {
      setInfo(r);
      if (nueva && !s.rama && r.actual) setS(x => ({ ...x, rama: r.actual || '' }));
    }).catch(e => setError(e.message));
  }, [s.proyecto]); // eslint-disable-line react-hooks/exhaustive-deps

  // Carpeta por defecto: donde ya está descargada la rama o, si no, directorio principal + rama.
  const rama = s.rama === NUEVA ? ramaNueva.trim() : s.rama;
  const sugerida = rama ? (info?.copias[rama] || (directorio ? unir(directorio, rama) : '')) : '';
  useEffect(() => { if (!carpetaElegida && sugerida) cambia({ carpeta: sugerida }); }, [sugerida, carpetaElegida]); // eslint-disable-line react-hooks/exhaustive-deps

  // El repositorio de la carpeta elegida: la carpeta de la sesión manda (si ya es un repositorio, el suyo; si no
  // lo es, se puede indicar uno en SSH para clonarlo en ella).
  const [local, setLocal] = useState<{ git: boolean; repositorio?: string; remoto?: string; rama?: string; ramas: string[]; existe: boolean } | null>(null);
  const [remoto, setRemoto] = useState('');
  useEffect(() => {
    if (!s.carpeta) { setLocal(null); return; }
    let vale = true;
    get<NonNullable<typeof local>>(`/api/proyecto/info-carpeta?proyecto=${s.proyecto}&carpeta=${encodeURIComponent(s.carpeta)}`)
      .then(i => { if (!vale) return; setLocal(i); if (carpetaElegida && i.git && i.rama) cambia({ rama: i.rama }); })
      .catch(() => { if (vale) setLocal(null); });
    return () => { vale = false; };
  }, [s.carpeta, s.proyecto]); // eslint-disable-line react-hooks/exhaustive-deps
  const conRepo = Boolean(local?.git);
  // Pedir el repositorio: carpeta sin él, elegida a mano (o sin repositorio en el proyecto del que sacar la rama).
  const pideRemoto = Boolean(local && !local.git && (carpetaElegida || !info?.repo));
  const ramas = conRepo ? local!.ramas : info?.ramas || [];

  const elegir = async () => {
    if (!window.psf?.elegirCarpeta) { setError('El diálogo de carpetas solo está en la aplicación de escritorio'); return; }
    const c = await window.psf.elegirCarpeta(s.carpeta || directorio);
    if (c) { cambia({ carpeta: c }); setCarpetaElegida(true); }
  };

  const autoriza = async () => {
    if (!window.psf?.elegirCarpeta) { setError('El diálogo de carpetas solo está en la aplicación de escritorio'); return; }
    const c = await window.psf.elegirCarpeta(s.carpeta || directorio);
    if (!c) return;
    if (c === s.carpeta || s.carpeta.startsWith(c.replace(/\/+$/, '') + '/')) { setError('Esa carpeta ya es (o contiene) la de la sesión'); return; }
    cambia({ autorizadas: [...new Set([...(s.autorizadas || []), c])] });
  };

  const falta = !rama ? 'Elige la rama' : !s.hosting ? 'Elige el alojamiento' : !s.carpeta ? 'Elige la carpeta' : !s.agente ? 'Elige el agente' : '';
  const guarda = async () => {
    if (falta || guardando) return;
    if (pideRemoto && !remoto.trim() && !info?.repo) { setError('Esa carpeta no tiene repositorio: indica el repositorio (SSH) para clonarlo en ella'); return; }
    // Si la rama no está ya en esa carpeta, se pide confirmación antes de crearla, descargarla o clonar.
    if (pideRemoto && remoto.trim()) {
      if (!confirm(`Se clonará ${remoto.trim()} en:\n${s.carpeta}\n\ncon la rama «${rama}» (la del repositorio si existe; si no, nueva). ¿Continuar?`)) return;
    } else if (!conRepo && info?.copias[rama] !== s.carpeta) {
      const aviso = s.rama === NUEVA
        ? `Se creará la rama «${rama}» a partir de «${info?.actual || 'la rama actual'}» y se descargará en:\n${s.carpeta}\n\n¿Continuar?`
        : `La rama «${rama}» no está descargada en local. Se descargará en:\n${s.carpeta}\n\n¿Continuar?`;
      if (!confirm(aviso)) return;
    }
    setGuardando(true);
    setError('');
    try {
      const repo = await post<Repo>('/api/proyecto/preparar', { proyecto: s.proyecto, sesion: s.id, rama, carpeta: s.carpeta, remoto: pideRemoto ? remoto.trim() : '' });
      const cambioDeContexto = !nueva && (rama !== sesion.rama || s.carpeta !== sesion.carpeta || s.agente !== sesion.agente);
      alGuardar({ ...s, rama, tipo: repo.tipo, sesionAgente: cambioDeContexto ? undefined : s.sesionAgente }, repo);
    } catch (e) { setError((e as Error).message); setGuardando(false); }
  };

  useEffect(() => {
    const tecla = (e: KeyboardEvent) => { if (e.key === 'Escape') alCerrar(); };
    document.addEventListener('keydown', tecla);
    return () => document.removeEventListener('keydown', tecla);
  }, [alCerrar]);

  const modelos = prov?.modelos || [];
  // Secciones con algún campo obligatorio sin rellenar (se marcan con un punto en la lista de la izquierda).
  const pendiente: Partial<Record<Seccion, boolean>> = { general: !rama || !s.hosting || !s.carpeta, agente: !s.agente };
  return createPortal(
    <div className="fondo-modal" onMouseDown={e => { if (e.target === e.currentTarget) alCerrar(); }}>
      <div className="modal propiedades ajustes-sesion" role="dialog" aria-modal="true" aria-label={nueva ? 'Nueva sesión' : 'Propiedades de la sesión'}>
        <div className="modal-cabecera">
          <h2>{nueva ? 'Nueva sesión' : <><span className="suave">Propiedades ·</span> {titulo(sesion)}</>}</h2>
          <button className="icono" onClick={alCerrar} title="Cerrar" aria-label="Cerrar"><Icono n="cerrar" /></button>
        </div>
        <div className="propiedades-cuerpo">
          <nav className="propiedades-secciones" aria-label="Secciones">
            {SECCIONES.map(x => (
              <button key={x.id} aria-current={seccion === x.id} onClick={() => setSeccion(x.id)}>
                <Icono n={x.icono} t={15} /> {x.nombre}{pendiente[x.id] && <span className="punto-pendiente" title="Falta un dato obligatorio" />}
              </button>
            ))}
          </nav>
          <section className="propiedades-detalle">
            {seccion === 'general' && <>
              <h3>General</h3>
              <label>Rama del repositorio{(local?.repositorio || info?.repositorio) && <span className="suave"> · {local?.git ? local.repositorio : info?.repositorio}</span>}
                <span className="con-icono"><Icono n="rama" t={15} />
                  <select value={s.rama} onChange={e => { cambia({ rama: e.target.value }); }} autoFocus={nueva}>
                    <option value="" disabled>Elige la rama…</option>
                    {s.rama && s.rama !== NUEVA && !ramas.includes(s.rama) && <option value={s.rama}>{s.rama}</option>}
                    {ramas.map(r => <option key={r} value={r}>{r}</option>)}
                    <option value={NUEVA}>Rama nueva…</option>
                  </select>
                </span>
                {s.rama === NUEVA && <input value={ramaNueva} onChange={e => setRamaNueva(e.target.value)} placeholder="Nombre de la rama nueva (sale de la rama actual del repositorio)" />}
              </label>

              <label>Nombre
                <input value={s.nombre} placeholder="Se toma del primer mensaje" onChange={e => cambia({ nombre: e.target.value })} />
              </label>

              <div className="campo">
                <span className="etiqueta-campo">Alojamiento <span className="obligatorio">obligatorio</span></span>
                <div className="hosting-opciones" role="radiogroup" aria-label="Alojamiento">
                  {TIPOS_HOSTING.map(t => (
                    <button type="button" key={t.id} role="radio" aria-checked={s.hosting === t.id} className="hosting-opcion"
                      onClick={() => cambia({ hosting: t.id as TipoHosting })} title={t.descripcion}>
                      <Hosting tipo={t.id} t={16} /> {t.nombre}
                    </button>
                  ))}
                </div>
              </div>

              <div className="campo">
                <span className="etiqueta-campo">Carpeta local de la rama</span>
                <div className="carpeta-elegida">
                  <code className="recorta" title={s.carpeta}>{s.carpeta || '—'}</code>
                  <button type="button" className="boton-secundario" onClick={elegir}><Icono n="carpeta" t={14} /> Elegir…</button>
                </div>
                {conRepo && <span className="suave pequeno">Repositorio de la carpeta: <b>{local!.repositorio}</b>{local!.rama ? <> · rama <b>{local!.rama}</b></> : null}</span>}
                {pideRemoto && (
                  <label className="campo-remoto">Repositorio (SSH) para clonar en esta carpeta
                    <input value={remoto} onChange={e => setRemoto(e.target.value)} placeholder="git@github.com:organizacion/repositorio.git" spellCheck={false} />
                    <span className="suave pequeno">{info?.repo ? 'La carpeta no tiene repositorio. Vacío: copia de trabajo del repositorio del proyecto.' : 'La carpeta no tiene repositorio: se clona en ella (tiene que estar vacía o no existir).'}</span>
                  </label>
                )}
                <span className="suave pequeno">
                  {!conRepo && !pideRemoto && info?.copias[rama] !== s.carpeta && <>Si no existe, se crea como copia de trabajo del repositorio en esa rama.
                    {!ajustes.directorio && info?.principal && <> Por defecto: directorio principal del proyecto ({info.principal}) + rama.</>}</>}
                </span>
              </div>
            </>}

            {seccion === 'agente' && <>
              <h3>Agente</h3>
              <label>Agente
                <span className="con-icono"><Agente id={s.agente} t={15} />
                  <select value={s.agente} onChange={e => cambia({ agente: e.target.value, modelo: undefined, esfuerzo: undefined })}>
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
              {s.agente === 'claude-code' && mcps && mcps.length > 0 && (
                <fieldset className="notificaciones">
                  <legend>Servidores MCP de Claude</legend>
                  <span className="suave pequeno">Solo arrancan los marcados (y «pgo»): cada servidor es un proceso más y añade sus herramientas a cada turno.</span>
                  <div className="rejilla-mcp">
                    {mcps.map(n => (
                      <label key={n} className="casilla"><input type="checkbox" checked={(s.mcp || []).includes(n)}
                        onChange={e => cambia({ mcp: e.target.checked ? [...(s.mcp || []), n] : (s.mcp || []).filter(x => x !== n) })} /> {n}</label>
                    ))}
                  </div>
                </fieldset>
              )}
              {!nueva && <span className="suave pequeno">Cambiar el agente empieza una conversación nueva con él.</span>}
            </>}

            {seccion === 'acceso' && <>
              <h3>Acceso a carpetas</h3>
              <div className="campo">
                <span className="etiqueta-campo">Carpetas autorizadas fuera del proyecto</span>
                {(s.autorizadas || []).length > 0 && (
                  <ul className="autorizadas">
                    {(s.autorizadas || []).map(c => (
                      <li key={c}><Icono n="carpeta" t={13} /><code className="recorta" title={c}>{c}</code>
                        <button type="button" className="quita-adjunto" onClick={() => cambia({ autorizadas: (s.autorizadas || []).filter(x => x !== c) })} aria-label={'Quitar ' + c}>×</button></li>
                    ))}
                  </ul>
                )}
                <button type="button" className="boton-secundario autorizar" onClick={autoriza}><Icono n="nuevo" t={13} /> Autorizar una carpeta…</button>
                <span className="suave pequeno">El agente solo trabaja en la carpeta de la sesión. Si necesita leer otra (p. ej. un proyecto de referencia), autorízala aquí.</span>
              </div>
            </>}

            {seccion === 'conversacion' && <>
              <h3>Conversación</h3>
              <fieldset className="notificaciones">
                <legend>Preguntas aparte</legend>
                <label className="casilla"><input type="checkbox" checked={s.decideAparte !== false} onChange={e => cambia({ decideAparte: e.target.checked })} /> Decidir automáticamente qué va al contexto</label>
                <span className="suave pequeno">Con el agente trabajando, pgocode responde aparte (sin que entre en la conversación) lo que no es para el agente: «¿cómo va?» en local y sin tokens; las preguntas sueltas, con un modelo barato. En la duda, va a la conversación. Siempre puedes forzarlo: «?» estado, «//» aparte.</span>
              </fieldset>
            </>}

            {seccion === 'gasto' && <>
              <h3>Gasto e imputación</h3>
              <fieldset className="notificaciones">
                <legend>Notificaciones</legend>
                <span className="suave pequeno">{CO.PLANES[s.agente] || 'Avisos de gasto de esta sesión.'} Son avisos sin interrupciones: el gasto del pie del chat cambia de color. Vacío: el valor por defecto del agente (en gris).</span>
                <div className="rejilla-avisos">
                  {([['avisoCoste', 'Aviso a partir de', '$'], ['limiteCoste', 'Límite', '$'], ['avisoTurno', 'Aviso si un turno pasa de', '$'],
                    ['avisoTokens', 'Aviso a partir de', 'tokens'], ['limiteTokens', 'Límite', 'tokens']] as const).map(([k, texto, unidad]) => (
                    <label key={k} className="aviso-campo">{texto}
                      <span className="con-unidad">
                        <input type="number" min={0} step={unidad === '$' ? 0.1 : 100000} value={s.avisos?.[k] ?? ''}
                          placeholder={CO.DEFECTO[s.agente]?.[k] !== undefined ? String(CO.DEFECTO[s.agente]?.[k]) : '—'}
                          onChange={e => cambia({ avisos: { ...s.avisos, [k]: e.target.value === '' ? undefined : Number(e.target.value) } })} />
                        <span className="suave">{unidad}</span>
                      </span>
                    </label>
                  ))}
                </div>
              </fieldset>

              <label>Tarea de Odoo a la que se imputa
                <span className="con-icono"><Icono n="reloj" t={15} />
                  <select value={s.tareaId || 0} onChange={e => {
                    const id = Number(e.target.value);
                    cambia({ tareaId: id || undefined, tarea: id ? tareas?.find(t => t.id === id)?.nombre : undefined });
                  }}>
                    <option value={0}>Desarrollo (por defecto; se crea si no existe)</option>
                    {s.tareaId && !tareas?.some(t => t.id === s.tareaId) && <option value={s.tareaId}>{s.tarea}</option>}
                    {(tareas || []).filter(t => t.nombre !== 'Desarrollo').map(t => <option key={t.id} value={t.id}>{t.nombre}</option>)}
                  </select>
                </span>
                {!tareas && <span className="suave pequeno">Cargando tareas de Odoo…</span>}
              </label>
            </>}

            {seccion === 'identificador' && <>
              <h3>Identificador</h3>
              <span className="suave pequeno">Para localizar la sesión en los registros de pgocode (tiempos y depuracion.log) y su conversación en el agente. Pásaselo a quien vaya a investigarla.</span>
              <dl className="identificadores">
                {([['Sesión de pgocode', sesion.id], ['Sesión del agente', sesion.sesionAgente || ''], ['Agente', sesion.agente],
                  ['Proyecto', String(sesion.proyecto)], ['Carpeta', sesion.carpeta], ['Rama', sesion.rama]] as const).map(([k, v]) => (
                  <div key={k}>
                    <dt>{k}</dt>
                    <dd>{v ? <><code className="recorta" title={v}>{v}</code><Copiar texto={v} etiqueta={k} /></>
                      : <span className="suave">{k === 'Sesión del agente' ? 'Todavía no hay: se crea con el primer mensaje' : '—'}</span>}</dd>
                  </div>
                ))}
              </dl>
              <div><Copiar texto={identificador(sesion)} etiqueta="todo" grande /></div>
            </>}
          </section>
        </div>

        <div className="propiedades-pie">
          {error && <p className="error">{error}</p>}
          <div className="fila-config">
            {!nueva && <span className="suave pequeno">Cambiar la rama, la carpeta o el agente empieza una conversación nueva con el agente.</span>}
            <span className="hueco" />
            <button className="enlace" onClick={alCerrar}>Cancelar</button>
            <button className="primario" onClick={guarda} disabled={Boolean(falta) || guardando} title={falta}>
              {guardando ? 'Preparando…' : nueva ? 'Crear sesión' : 'Guardar'}
            </button>
          </div>
        </div>
      </div>
    </div>,
    document.body,
  );
}

// Botón de copiar al portapapeles que confirma con «Copiado» un momento.
function Copiar({ texto, etiqueta, grande }: { texto: string; etiqueta: string; grande?: boolean }) {
  const [hecho, setHecho] = useState(false);
  const copia = () => navigator.clipboard.writeText(texto).then(() => { setHecho(true); setTimeout(() => setHecho(false), 1500); }).catch(() => {});
  return grande
    ? <button type="button" className="boton-secundario" onClick={copia}><Icono n={hecho ? 'hecho' : 'copiar'} t={13} /> {hecho ? 'Copiado' : 'Copiar todo'}</button>
    : <button type="button" className="icono" onClick={copia} title={hecho ? 'Copiado' : 'Copiar'} aria-label={'Copiar ' + etiqueta}><Icono n={hecho ? 'hecho' : 'copiar'} t={13} /></button>;
}
