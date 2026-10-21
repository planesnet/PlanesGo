import { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { get, post, type Proveedor } from '../api';
import { leeAjustes, TIPOS_HOSTING, type TipoHosting } from '../ajustes';
import type { Repo, Sesion } from '../sesiones';
import Icono from './Icono';
import { Agente, Hosting } from './Marca';

// Ventana de la sesión (crear o ajustar). Lo primero, la rama del repositorio; después el nombre, el alojamiento
// (obligatorio) y la carpeta local donde vive esa rama: por defecto, el directorio principal del proyecto + el
// nombre de la rama (o la carpeta donde ya está abierta), elegible con el diálogo de carpetas del sistema.

interface Ramas { repo: string; repositorio?: string; ramas: string[]; actual?: string; copias: Record<string, string>; principal: string; tipo?: string }

const NUEVA = '\u0000nueva';
const unir = (dir: string, rama: string) => dir.replace(/\/+$/, '') + '/' + rama.replace(/\//g, '_');

export default function DialogoSesion({ sesion, nueva, login, proveedores, alCerrar, alGuardar }: {
  sesion: Sesion; nueva: boolean; login: string; proveedores: Proveedor[];
  alCerrar: () => void; alGuardar: (s: Sesion, repo: Repo | null) => void;
}) {
  const [s, setS] = useState<Sesion>(sesion);
  const [info, setInfo] = useState<Ramas | null>(null);
  const [ramaNueva, setRamaNueva] = useState('');
  const [carpetaElegida, setCarpetaElegida] = useState(!nueva); // la carpeta la ha puesto el usuario: no se recalcula
  const [error, setError] = useState('');
  const [guardando, setGuardando] = useState(false);
  const [tareas, setTareas] = useState<{ id: number; nombre: string }[] | null>(null);
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

  // Carpeta por defecto: donde ya está abierta la rama o, si no, directorio principal + rama.
  const rama = s.rama === NUEVA ? ramaNueva.trim() : s.rama;
  const sugerida = rama ? (info?.copias[rama] || (directorio ? unir(directorio, rama) : '')) : '';
  useEffect(() => { if (!carpetaElegida && sugerida) cambia({ carpeta: sugerida }); }, [sugerida, carpetaElegida]); // eslint-disable-line react-hooks/exhaustive-deps

  const elegir = async () => {
    if (!window.psf?.elegirCarpeta) { setError('El diálogo de carpetas solo está en la aplicación de escritorio'); return; }
    const c = await window.psf.elegirCarpeta(s.carpeta || directorio);
    if (c) { cambia({ carpeta: c }); setCarpetaElegida(true); }
  };

  const falta = !rama ? 'Elige la rama' : !s.hosting ? 'Elige el alojamiento' : !s.carpeta ? 'Elige la carpeta' : !s.agente ? 'Elige el agente' : '';
  const guarda = async () => {
    if (falta || guardando) return;
    setGuardando(true);
    setError('');
    try {
      const repo = await post<Repo>('/api/proyecto/preparar', { proyecto: s.proyecto, rama, carpeta: s.carpeta });
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
  return createPortal(
    <div className="fondo-modal" onMouseDown={e => { if (e.target === e.currentTarget) alCerrar(); }}>
      <div className="modal ajustes-sesion" role="dialog" aria-modal="true" aria-label={nueva ? 'Nueva sesión' : 'Ajustes de la sesión'}>
        <div className="modal-cabecera">
          <h2>{nueva ? 'Nueva sesión' : 'Ajustes de la sesión'}</h2>
          <button className="icono" onClick={alCerrar} title="Cerrar" aria-label="Cerrar"><Icono n="cerrar" /></button>
        </div>

        <label>Rama del repositorio{info?.repositorio && <span className="suave"> · {info.repositorio}</span>}
          <span className="con-icono"><Icono n="rama" t={15} />
            <select value={s.rama} onChange={e => { cambia({ rama: e.target.value }); }} autoFocus={nueva}>
              <option value="" disabled>Elige la rama…</option>
              {(info?.ramas || []).map(r => <option key={r} value={r}>{r}{info?.copias[r] ? ' · abierta' : ''}</option>)}
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
          <span className="suave pequeno">
            {info?.copias[rama] && s.carpeta === info.copias[rama] ? 'La rama ya está abierta en esta carpeta.'
              : 'Si no existe, se crea como copia de trabajo del repositorio en esa rama.'}
            {!ajustes.directorio && info?.principal && <> Por defecto: directorio principal del proyecto ({info.principal}) + rama.</>}
          </span>
        </div>

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
    </div>,
    document.body,
  );
}
