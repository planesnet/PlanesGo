import { useEffect, useState } from 'react';
import { PRUNE_MIN_TOKENS } from '../workingMemory';
import { COPIES_CHANGED, get, post, type Proveedor } from '../api';
import type { ProgramDraft } from '../programs';
import { rememberedModel } from '../sessionDefaults';
import { leeAjustes, TIPOS_HOSTING, type TipoHosting } from '../ajustes';
import { identificador, titulo, type Repo, type Sesion } from '../sesiones';
import Icono from './Icono';
import { Agente, Hosting } from './Marca';
import * as CO from '../costes';
import { autoContextOn } from '../contextPolicy';
import { askConfirm, PropertiesModal, type PropertiesSection } from './Modal';
import SqlServerSettings from './SqlServerSettings';
import WinRMSettings from './WinRMSettings';
import AgentDesktopSettings from './AgentDesktopSettings';
import ProgramSettings, { saveProgramChanges } from './ProgramSettings';
import ProjectMemory from './ProjectMemory';
import { MODEL_LABEL_HELP, modelLabel } from '../modelInfo';

// Ventana de la sesión (crear o ajustar), en dos paneles: secciones a la izquierda y sus propiedades a la derecha.
// Lo primero, la rama del repositorio; después el nombre, el alojamiento
// (obligatorio) y la carpeta local donde vive esa rama: por defecto, el directorio principal del proyecto + el
// nombre de la rama (o la carpeta donde ya está descargada), elegible con el diálogo de carpetas del sistema.

interface Ramas { repo: string; repositorio?: string; ramas: string[]; actual?: string; copias: Record<string, string>; principal: string; tipo?: string }

const NUEVA = '\u0000nueva';
// Sections of the window, grouped by topic.
export type SessionSection = 'general' | 'agent' | 'access' | 'conversation' | 'memory' | 'spending' | 'programs' | 'sql' | 'winrm' | 'desktop' | 'ids';
const SESSION_SECTIONS: readonly PropertiesSection<SessionSection>[] = [{ id: 'general', name: 'General', icon: 'rama' }, { id: 'agent', name: 'Agente', icon: 'terminal' },
  { id: 'access', name: 'Acceso a carpetas', icon: 'carpeta' }, { id: 'conversation', name: 'Conversación', icon: 'comentario' },
  { id: 'memory', name: 'Memoria del proyecto', icon: 'plan' },
  { id: 'spending', name: 'Gasto e imputación', icon: 'reloj' }, { id: 'programs', name: 'Programas', icon: 'ejecutar' },
  { id: 'sql', name: 'SQL Server', icon: 'repo' },
  { id: 'winrm', name: 'PowerShell remoto', icon: 'terminal' }, { id: 'desktop', name: 'Escritorio del agente', icon: 'ordenador' },
  { id: 'ids', name: 'Identificador', icon: 'info' }];
const unir = (dir: string, rama: string) => dir.replace(/\/+$/, '') + '/' + rama.replace(/\//g, '_');

export default function DialogoSesion({ sesion, nueva, login, proveedores, inicial, alCerrar, alGuardar }: {
  sesion: Sesion; nueva: boolean; login: string; proveedores: Proveedor[]; inicial?: SessionSection;
  alCerrar: () => void; alGuardar: (s: Sesion, repo: Repo | null) => void;
}) {
  const [s, setS] = useState<Sesion>(sesion);
  const [info, setInfo] = useState<Ramas | null>(null);
  const [ramaNueva, setRamaNueva] = useState('');
  const [carpetaElegida, setCarpetaElegida] = useState(!nueva); // la carpeta la ha puesto el usuario: no se recalcula
  const [error, setError] = useState('');
  const [guardando, setGuardando] = useState(false);
  const [tareas, setTareas] = useState<{ id: number; nombre: string }[] | null>(null);
  const [mcps, setMcps] = useState<string[] | null>(null);
  const [programs, setPrograms] = useState<ProgramDraft | null>(null); // borrador de Programas (se guarda con «Guardar»)
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
      .then(i => { if (!vale) return; setLocal(i); if (carpetaElegida && i.git && i.rama && s.carpeta !== sesion.carpeta && !sesion.automatic) cambia({ rama: i.rama }); }) // only when another folder is chosen
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
      if (!(await askConfirm(`Se clonará ${remoto.trim()} en:\n${s.carpeta}\n\ncon la rama «${rama}» (la del repositorio si existe; si no, nueva). ¿Continuar?`))) return;
    } else if (!conRepo && info?.copias[rama] !== s.carpeta) {
      const aviso = s.rama === NUEVA
        ? `Se creará la rama «${rama}» a partir de «${info?.actual || 'la rama actual'}» y una subcarpeta (git worktree) suya en:\n${s.carpeta}\n\n¿Continuar?`
        : `La rama «${rama}» no tiene subcarpeta en local. Se creará una subcarpeta (git worktree) en:\n${s.carpeta}\n\n¿Continuar?`;
      if (!(await askConfirm(aviso))) return;
    }
    setGuardando(true);
    setError('');
    // An automatic PSF session keeps its branch and folder (its worktree belongs to the PSF flow): only the rest
    // of its properties change.
    if (sesion.automatic) {
      try {
        const repo = await get<Repo>(`/api/proyecto/repo?proyecto=${s.proyecto}&carpeta=${encodeURIComponent(sesion.carpeta)}`);
        alGuardar({ ...s, rama: sesion.rama, carpeta: sesion.carpeta, sesionAgente: s.agente !== sesion.agente ? undefined : s.sesionAgente }, repo);
      } catch (e) { setError((e as Error).message); setGuardando(false); }
      return;
    }
    try {
      const repo = await post<Repo>('/api/proyecto/preparar', { proyecto: s.proyecto, sesion: s.id, rama, carpeta: s.carpeta, remoto: pideRemoto ? remoto.trim() : '' });
      window.dispatchEvent(new Event(COPIES_CHANGED));
      if (programs && sesion.carpeta) await saveProgramChanges(s.proyecto, sesion.carpeta, programs);
      const cambioDeContexto = !nueva && (rama !== sesion.rama || s.carpeta !== sesion.carpeta || s.agente !== sesion.agente);
      alGuardar({ ...s, rama, tipo: repo.tipo, sesionAgente: cambioDeContexto ? undefined : s.sesionAgente }, repo);
    } catch (e) { setError((e as Error).message); setGuardando(false); }
  };

  const modelos = prov?.modelos || [];
  // Sections with a required value still empty get a dot in the list on the left.
  const incomplete: Partial<Record<SessionSection, boolean>> = { general: !rama || !s.hosting || !s.carpeta, agent: !s.agente };
  return (
    <PropertiesModal title={nueva ? 'Nueva sesión' : <><span className="suave">Propiedades ·</span> {titulo(sesion)}</>}
      label={nueva ? 'Nueva sesión' : 'Propiedades'} className="ajustes-sesion" onClose={alCerrar}
      sections={SESSION_SECTIONS.filter(x => !nueva || !['programs', 'sql', 'winrm', 'desktop'].includes(x.id)).map(x => ({ ...x, incomplete: incomplete[x.id] }))} initial={inicial}
      footer={<>
        {error && <p className="error footer-error">{error}</p>}
        {!nueva && <span className="suave pequeno">Cambiar la rama, la carpeta o el agente empieza una conversación nueva con el agente.</span>}
        <span className="hueco" />
        <button className="enlace" onClick={alCerrar}>Cancelar</button>
        <button className="primario" onClick={guarda} disabled={Boolean(falta) || guardando} title={falta}>
          {guardando ? 'Preparando…' : nueva ? 'Crear sesión' : 'Guardar'}
        </button>
      </>}>
      {section => <>
        {section === 'general' && <>
          <h3>General</h3>
          <label>Rama del repositorio{(local?.repositorio || info?.repositorio) && <span className="suave"> · {local?.git ? local.repositorio : info?.repositorio}</span>}
            <span className="con-icono"><Icono n="rama" t={15} />
              {sesion.automatic ? <code title="Sesión automática PSF: su rama y su carpeta las lleva el flujo">{sesion.rama}</code> :
              <select value={s.rama} onChange={e => { cambia({ rama: e.target.value }); }} autoFocus={nueva}>
                <option value="" disabled>Elige la rama…</option>
                {s.rama && s.rama !== NUEVA && !ramas.includes(s.rama) && <option value={s.rama}>{s.rama}</option>}
                {ramas.map(r => <option key={r} value={r}>{r}</option>)}
                <option value={NUEVA}>Rama nueva…</option>
              </select>}
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
              {!sesion.automatic && <button type="button" className="boton-secundario" onClick={elegir}><Icono n="carpeta" t={14} /> Elegir…</button>}
            </div>
            {conRepo && <span className="suave pequeno">Repositorio de la carpeta: <b>{local!.repositorio}</b>{local!.rama ? <> · rama <b>{local!.rama}</b></> : null}</span>}
            {pideRemoto && (
              <label className="campo-remoto">Repositorio (SSH) para clonar en esta carpeta
                <input value={remoto} onChange={e => setRemoto(e.target.value)} placeholder="git@github.com:organizacion/repositorio.git" spellCheck={false} />
                <span className="suave pequeno">{info?.repo ? 'La carpeta no tiene repositorio. Vacío: subcarpeta del repositorio del proyecto.' : 'La carpeta no tiene repositorio: se clona en ella (tiene que estar vacía o no existir).'}</span>
              </label>
            )}
            <span className="suave pequeno">
              {!conRepo && !pideRemoto && info?.copias[rama] !== s.carpeta && <>Si no existe, se crea como subcarpeta del repositorio en esa rama.
                {!ajustes.directorio && info?.principal && <> Por defecto: directorio principal del proyecto ({info.principal}) + rama.</>}</>}
            </span>
          </div>
        </>}

        {section === 'agent' && <>
          <h3>Agente</h3>
          <label>Agente
            <span className="con-icono"><Agente id={s.agente} t={15} />
              <select value={s.agente} onChange={e => cambia({ agente: e.target.value, esfuerzo: undefined,
                // El último modelo elegido con ese agente (si no, el suyo por defecto).
                modelo: rememberedModel(login, e.target.value, proveedores.find(p => p.id === e.target.value)?.modelos || []) })}>
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
              <select value={s.modelo || prov?.por_defecto} onChange={e => cambia({ modelo: e.target.value })} title={MODEL_LABEL_HELP}>
                {modelos.map(m => <option key={m} value={m}>{modelLabel(m)}</option>)}
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

        {section === 'access' && <>
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

        {section === 'conversation' && <>
          <h3>Conversación</h3>
          <fieldset className="notificaciones">
            <legend>Preguntas aparte</legend>
            <label className="casilla"><input type="checkbox" checked={s.decideAparte !== false} onChange={e => cambia({ decideAparte: e.target.checked })} /> Decidir automáticamente qué va al contexto</label>
            <span className="suave pequeno">Con el agente trabajando, pgocode responde aparte (sin que entre en la conversación) lo que no es para el agente: «¿cómo va?» en local y sin tokens; las preguntas sueltas, con la IA local del equipo (si no está, un modelo barato). En la duda, va a la conversación. Siempre puedes forzarlo: «?» estado, «//» aparte.</span>
          </fieldset>
          <fieldset className="notificaciones">
            <legend>Órdenes directas</legend>
            <label className="casilla"><input type="checkbox" checked={s.directo !== false} onChange={e => cambia({ directo: e.target.checked ? undefined : false })} /> Lo mecánico lo hace pgocode, sin el agente</label>
            <span className="suave pequeno">Si el mensaje es solo «compila», «pasa los tests», «haz commit», «¿qué cambios hay?», «¿cuánto llevamos gastado?»…, lo hace pgocode en la carpeta de la sesión (0 tokens) y deja una nota en el chat. Con más palabras («compila y arregla los errores»), o con «&gt;» delante, va al agente. Nunca hace push.</span>
          </fieldset>
          <fieldset className="notificaciones">
            <legend>Historial del agente</legend>
            <label className="casilla">
              <input type="checkbox" checked={autoContextOn(s)} onChange={e => cambia({ autoContext: e.target.checked ? undefined : false })} />
              Gestión automática del contexto
            </label>
            <span className="suave pequeno">El agente reenvía todo su historial en cada paso: pgocode lo acota con lo que cada agente permite. OpenCode: al empezar cada turno quita lo que ya no importa (lecturas superadas, salidas largas, lo que no tiene relación con lo último) y lo repone si vuelve a hacer falta. Claude Code: lo mismo, en su historial (entre turnos, con copia de seguridad y teniendo en cuenta la caché); si no se puede, con la conversación parada más que la caché sigue en una sesión nueva con una «memoria de trabajo» de unos 3k tokens. Antigravity no deja tocar su historial: cuando lo que ya no está vigente pasa de {Math.round(PRUNE_MIN_TOKENS / 1000)}k tokens (o la caché ha caducado), sigue en una conversación nueva con la memoria de lo vigente. En todos, el agente resume su historial al llegar a 150k. Desactivado: nada de eso; el agente compacta cuando él decide.</span>
          </fieldset>
        </>}

        {section === 'spending' && <>
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

        {section === 'memory' && <ProjectMemory proyecto={s.proyecto} carpeta={sesion.carpeta || s.carpeta} />}
        {section === 'programs' && <ProgramSettings proyecto={s.proyecto} carpeta={sesion.carpeta} value={programs} onChange={setPrograms} />}
        {section === 'sql' && <SqlServerSettings project={s.proyecto} session={sesion.id} login={login} />}
        {section === 'winrm' && <WinRMSettings project={s.proyecto} session={sesion.id} login={login} />}
        {section === 'desktop' && <AgentDesktopSettings project={s.proyecto} session={sesion.id} login={login} />}
        {section === 'ids' && <>
          <h3>Identificador</h3>
          <span className="suave pequeno">Para localizar la sesión en los registros de pgocode (tiempos y depuracion.log) y su conversación en el agente. Pásaselo a quien vaya a investigarla.</span>
          <dl className="session-ids">
            {/* Con lo elegido en la ventana (agente, carpeta, rama), no con lo guardado: en «Nueva sesión» se ve lo que será. */}
            {([['Sesión de pgocode', s.id], ['Sesión del agente', s.agente === sesion.agente ? s.sesionAgente || '' : ''], ['Agente', s.agente],
              ['Proyecto', String(s.proyecto)], ['Carpeta', s.carpeta], ['Rama', rama]] as const).map(([k, v]) => (
              <div key={k}>
                <dt>{k}</dt>
                <dd>{v ? <><code className="recorta" title={v}>{v}</code><CopyButton text={v} label={k} /></>
                  : <span className="suave">{k === 'Sesión del agente' ? 'Todavía no hay: se crea con el primer mensaje' : '—'}</span>}</dd>
              </div>
            ))}
          </dl>
          <div><CopyButton text={identificador({ ...s, rama, sesionAgente: s.agente === sesion.agente ? s.sesionAgente : undefined })} label="todo" wide /></div>
        </>}
      </>}
    </PropertiesModal>
  );
}

// Clipboard copy button that briefly confirms with «Copiado».
function CopyButton({ text, label, wide }: { text: string; label: string; wide?: boolean }) {
  const [copied, setCopied] = useState(false);
  const copy = () => navigator.clipboard.writeText(text).then(() => { setCopied(true); setTimeout(() => setCopied(false), 1500); }).catch(() => {});
  return wide
    ? <button type="button" className="boton-secundario" onClick={copy}><Icono n={copied ? 'hecho' : 'copiar'} t={13} /> {copied ? 'Copiado' : 'Copiar todo'}</button>
    : <button type="button" className="icono" onClick={copy} title={copied ? 'Copiado' : 'Copiar'} aria-label={'Copiar ' + label}><Icono n={copied ? 'hecho' : 'copiar'} t={13} /></button>;
}
