import { useEffect, useState } from 'react';
import { get, post } from '../api';
import Icono from './Icono';
import Texto from './Texto';
import { askConfirm, Modal } from './Modal';

// Planes del modo plan: la tarjeta que aparece en el chat, el editor (el usuario lo cambia a mano antes de
// ejecutarlo) y la lista de planes guardados del proyecto (<directorio del proyecto>/.pgocode/planes/).

export type EstadoPlan = 'borrador' | 'ejecutado' | 'descartado';
export interface Plan { ruta: string; numero?: number; titulo: string; proyecto: number; sesion?: string; agente?: string; estado: EstadoPlan; creado?: string; modificado?: string; texto?: string }

const q = (proyecto: number, directorio?: string) => `proyecto=${proyecto}&directorio=${encodeURIComponent(directorio || '')}`;
export const leePlan = (proyecto: number, directorio: string | undefined, ruta: string) =>
  get<Plan>(`/api/planes/leer?${q(proyecto, directorio)}&ruta=${encodeURIComponent(ruta)}`);
export const listaPlanes = (proyecto: number, directorio?: string) => get<Plan[]>(`/api/planes?${q(proyecto, directorio)}`);
export const guardaPlan = (proyecto: number, directorio: string | undefined, p: Partial<Plan>) =>
  post<Plan>('/api/planes', { ...p, proyecto, directorio: directorio || '' });
// Solo los descartados se pueden borrar.
export const borraPlan = (proyecto: number, directorio: string | undefined, ruta: string) =>
  post('/api/planes/borrar', { proyecto, directorio: directorio || '', ruta });

const ESTADOS: Record<EstadoPlan | 'borrado', string> = { borrador: 'Por decidir', ejecutado: 'Ejecutado', descartado: 'Descartado', borrado: 'Borrado' };
const nombre = (ruta: string) => ruta.split('/').pop() || ruta;

export function Estado({ e }: { e: EstadoPlan | 'borrado' }) {
  return <span className={'plan-estado ' + e}>{ESTADOS[e]}</span>;
}

// Tarjeta del plan bajo la respuesta del agente: abrirlo para revisarlo o editarlo, ejecutarlo o descartarlo.
export function TarjetaPlan({ numero, titulo, ruta, estado, ocupado, alAbrir, alEjecutar, alDescartar }: {
  numero?: number; titulo: string; ruta: string; estado: EstadoPlan | 'borrado'; ocupado: boolean;
  alAbrir: () => void; alEjecutar: () => void; alDescartar: () => void;
}) {
  return (
    <div className="tarjeta-plan">
      <div className="tarjeta-plan-cabecera">
        <Icono n="lapiz" t={14} />
        {numero ? <span className="plan-numero">Plan #{numero}</span> : null}
        <b className="recorta">{titulo}</b>
        <Estado e={estado} />
      </div>
      <code className="recorta suave pequeno" title={ruta}>{nombre(ruta)}</code>
      <div className="tarjeta-plan-acciones">
        {estado !== 'borrado' && <button className="boton-secundario" onClick={alAbrir}>Revisar y editar</button>}
        {estado === 'borrador' && <>
          <button className="primario" onClick={alEjecutar} disabled={ocupado} title={ocupado ? 'Espera a que termine el agente' : 'Pasa la sesión al modo de ejecución y le pide al agente que siga el plan'}>Ejecutar</button>
          <button className="enlace" onClick={alDescartar}>Descartar</button>
        </>}
      </div>
      {estado === 'borrador' && numero ? <span className="suave pequeno">En el chat: «plan {numero}: cambios» lo revisa (con tus ediciones), «ejecuta el plan {numero}», «descarta el plan {numero}».</span> : null}
    </div>
  );
}

// Editor del plan: Markdown a la izquierda (o arriba) y la vista previa.
export function EditorPlan({ proyecto, directorio, ruta, ocupado, alCerrar, alCambio, alEjecutar, alBorrar }: {
  proyecto: number; directorio?: string; ruta: string; ocupado: boolean;
  alCerrar: () => void; alCambio: (p: Plan) => void; alEjecutar: (p: Plan) => void; alBorrar?: (ruta: string) => void;
}) {
  const [plan, setPlan] = useState<Plan | null>(null);
  const [texto, setTexto] = useState('');
  const [vista, setVista] = useState(true); // se abre para leerlo; «Editar» a un clic
  const [error, setError] = useState('');
  const [guardando, setGuardando] = useState(false);
  useEffect(() => {
    leePlan(proyecto, directorio, ruta).then(p => { setPlan(p); setTexto(p.texto || ''); }).catch(e => setError(e.message));
  }, [proyecto, directorio, ruta]);
  const cambiado = plan !== null && texto !== (plan.texto || '');

  const guarda = async (estado?: EstadoPlan) => {
    if (!plan) return null;
    setGuardando(true);
    setError('');
    try {
      const p = await guardaPlan(proyecto, directorio, { ruta: plan.ruta, texto: cambiado ? texto : undefined, estado });
      const n = { ...p, texto };
      setPlan(n);
      alCambio(n);
      return n;
    } catch (e) { setError((e as Error).message); return null; } finally { setGuardando(false); }
  };

  return (
    <Modal title={<>{plan?.numero ? `Plan #${plan.numero} · ` : ''}{plan?.titulo || 'Plan'} {plan && <Estado e={plan.estado} />}</>} label="Plan"
      className="editor-plan" onClose={alCerrar}>
      <code className="recorta suave pequeno" title={ruta}>{ruta}</code>
      {error && <p className="error">{error}</p>}
      <div className="editor-plan-pestanas" role="tablist">
        <button role="tab" aria-selected={vista} onClick={() => setVista(true)}>Vista previa</button>
        <button role="tab" aria-selected={!vista} onClick={() => setVista(false)}>Editar</button>
        {cambiado && <span className="suave pequeno">Cambios sin guardar</span>}
      </div>
      {vista
        ? <div className="editor-plan-vista"><Texto md={texto} /></div>
        : <textarea className="editor-plan-texto" value={texto} onChange={e => setTexto(e.target.value)} spellCheck={false} aria-label="Texto del plan (Markdown)" disabled={!plan} />}
      <div className="editor-plan-acciones">
        {plan?.estado === 'borrador' && <button className="enlace" onClick={() => guarda('descartado')} disabled={guardando}>Descartar plan</button>}
        {plan && plan.estado !== 'borrador' && <button className="enlace" onClick={() => guarda('borrador')} disabled={guardando}>Volver a «Por decidir»</button>}
        {plan?.estado === 'descartado' && (
          <button className="enlace peligro" disabled={guardando} onClick={async () => {
            if (!(await askConfirm(`¿Borrar el plan «${plan.titulo}»? Se borra su fichero y no se puede deshacer.`, { okText: 'Borrar', danger: true }))) return;
            try { await borraPlan(proyecto, directorio, plan.ruta); alBorrar?.(plan.ruta); alCerrar(); } catch (e) { setError((e as Error).message); }
          }}>Borrar el plan</button>
        )}
        <span className="hueco" />
        <button className="boton-secundario" onClick={() => guarda()} disabled={!cambiado || guardando}>Guardar</button>
        {plan?.estado === 'borrador' && (
          <button className="primario" disabled={guardando || ocupado || !texto.trim()} title={ocupado ? 'Espera a que termine el agente' : ''}
            onClick={async () => { const p = await guarda(); if (p) alEjecutar(p); }}>Guardar y ejecutar</button>
        )}
      </div>
    </Modal>
  );
}

// Lista de planes guardados del proyecto.
export function ListaPlanes({ proyecto, directorio, alAbrir, alCerrar }: {
  proyecto: number; directorio?: string; alAbrir: (ruta: string) => void; alCerrar: () => void;
}) {
  const [l, setL] = useState<Plan[] | null>(null);
  const [error, setError] = useState('');
  useEffect(() => { listaPlanes(proyecto, directorio).then(setL).catch(e => setError(e.message)); }, [proyecto, directorio]);
  return (
    <Modal title="Planes del proyecto" className="lista-planes" onClose={alCerrar}>
      {error && <p className="error">{error}</p>}
      {!l && !error && <p className="suave">Cargando…</p>}
      {l && l.length === 0 && <p className="suave">Aún no hay planes. Elige el modo «Plan» en el chat y pide lo que quieras planificar.</p>}
      {l && l.length > 0 && (
        <ul className="planes">
          {l.map(p => (
            <li key={p.ruta}>
              <button onClick={() => alAbrir(p.ruta)} title={p.ruta}>
                <span className="recorta">{p.numero ? <b className="plan-numero">#{p.numero}</b> : null} {p.titulo}</span>
                <Estado e={p.estado} />
                <span className="suave pequeno">{p.modificado ? new Date(p.modificado).toLocaleString('es-ES', { dateStyle: 'short', timeStyle: 'short' }) : ''}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}
