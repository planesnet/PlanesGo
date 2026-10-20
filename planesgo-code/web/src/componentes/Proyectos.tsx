import { useEffect, useMemo, useState } from 'react';
import { get, type Proveedor, type Proyecto, type Usuario } from '../api';
import type { Repo, Sesion } from '../sesiones';
import Icono from './Icono';
import { Agente, Hosting } from './Marca';

interface Props {
  usuario: Usuario; actual: Proyecto | null; inicial: number; alElegir: (p: Proyecto) => void;
  repo: Repo | null; proveedores: Proveedor[];
  sesiones: Sesion[]; sesion: Sesion | null;
  alElegirSesion: (s: Sesion) => void; alNuevaSesion: () => void; alCambiarSesion: (s: Sesion) => void; alBorrarSesion: (s: Sesion) => void;
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
        {visibles.map(p => (
          <div key={p.id}>
            <button className={'proyecto' + (actual?.id === p.id ? ' activo' : '')} onClick={() => alElegir(p)}>
              <span className="nombre">{p.nombre}</span>
              <span className="detalle">
                {p.cliente?.nombre || 'Sin cliente'}{p.carpetas.length > 0 && <span className="local" title={p.carpetas.join('\n')}> · local</span>}
              </span>
            </button>
            {actual?.id === p.id && !buscando && <Sesiones {...props} />}
          </div>
        ))}
      </div>
    </nav>
  );
}

// Sesiones del proyecto activo, como en Claude: icono del hosting, nombre, rama y menú ⋮ con su configuración.
function Sesiones({ repo, proveedores, sesiones, sesion, alElegirSesion, alNuevaSesion, alCambiarSesion, alBorrarSesion }: Props) {
  const [menu, setMenu] = useState<string | null>(null);
  return (
    <div className="sesiones">
      {sesiones.map(s => (
        <div key={s.id} className={'sesion' + (sesion?.id === s.id ? ' activa' : '')}>
          <button className="sesion-boton" onClick={() => alElegirSesion(s)} title={s.carpeta + (s.rama ? ' · ' + s.rama : '')}>
            <Hosting tipo={repo?.tipo} t={13} enBlanco />
            <span className="recorta">{s.nombre}</span>
            {s.rama && <span className="rama-sesion recorta"><Icono n="rama" t={11} />{s.rama}</span>}
          </button>
          <button className="icono puntos" onClick={() => setMenu(m => m === s.id ? null : s.id)} title="Configuración de la sesión" aria-label="Configuración de la sesión" aria-expanded={menu === s.id}>
            <Icono n="puntos" t={15} />
          </button>
          {menu === s.id && (
            <ConfigSesion s={s} repo={repo} proveedores={proveedores} alCerrar={() => setMenu(null)}
              alCambiar={alCambiarSesion} alBorrar={x => { setMenu(null); alBorrarSesion(x); }} />
          )}
        </div>
      ))}
      <button className="sesion-nueva" onClick={alNuevaSesion}><Icono n="nuevo" t={13} /> Nueva sesión</button>
    </div>
  );
}

function ConfigSesion({ s, repo, proveedores, alCerrar, alCambiar, alBorrar }: {
  s: Sesion; repo: Repo | null; proveedores: Proveedor[]; alCerrar: () => void; alCambiar: (s: Sesion) => void; alBorrar: (s: Sesion) => void;
}) {
  const [nombre, setNombre] = useState(s.nombre);
  const cambia = (c: Partial<Sesion>) => alCambiar({ ...s, ...c });
  const prov = proveedores.find(p => p.id === s.agente);
  return (
    <div className="config-sesion" role="dialog" aria-label="Configuración de la sesión" onKeyDown={e => e.key === 'Escape' && alCerrar()}>
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
    </div>
  );
}
