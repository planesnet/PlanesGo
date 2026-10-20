import { useEffect, useMemo, useState } from 'react';
import { get, type Proyecto, type Usuario } from '../api';
import Icono from './Icono';

interface Props { usuario: Usuario; actual: Proyecto | null; inicial: number; alElegir: (p: Proyecto) => void }

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

export default function Proyectos({ usuario, actual, inicial, alElegir }: Props) {
  const [lista, setLista] = useState<Proyecto[] | null>(null);
  const [error, setError] = useState('');
  const [filtro, setFiltro] = useState('');
  const [soloLocales, setSoloLocales] = useState(false);
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
    const todos = (lista || []).filter(p => !soloLocales || p.carpetas.length > 0);
    if (!buscando) {
      const porId = new Map(todos.map(p => [p.id, p]));
      return recientes.map(id => porId.get(id)).filter((p): p is Proyecto => Boolean(p));
    }
    const f = sinAcentos(filtro.trim());
    return todos.filter(p => sinAcentos(p.nombre + ' ' + (p.cliente?.nombre || '')).includes(f));
  }, [lista, filtro, soloLocales, recientes, buscando]);

  return (
    <nav className="proyectos" aria-label="Proyectos">
      <div className="busca">
        <input placeholder="Buscar proyecto…" value={filtro} onChange={e => setFiltro(e.target.value)} aria-label="Buscar proyecto" />
        <button className="icono" title="Volver a cargar de Odoo" aria-label="Volver a cargar de Odoo" onClick={() => carga(true)}><Icono n="recargar" t={16} /></button>
      </div>
      <label className="interruptor"><input type="checkbox" checked={soloLocales} onChange={e => setSoloLocales(e.target.checked)} /> Solo locales</label>
      <div className="lista">
        {error && <p className="error">{error}</p>}
        {!lista && !error && <p className="suave">Cargando proyectos de Odoo…</p>}
        {lista && !buscando && <p className="titulo-lista">Recientes</p>}
        {lista && visibles.length === 0 && <p className="suave">{buscando ? 'Ningún proyecto coincide.' : 'Aún no has abierto ningún proyecto. Búscalo arriba para empezar.'}</p>}
        {visibles.map(p => (
          <button key={p.id} className={'proyecto' + (actual?.id === p.id ? ' activo' : '')} onClick={() => alElegir(p)}>
            <span className="nombre">{p.nombre}</span>
            <span className="detalle">
              {p.cliente?.nombre || 'Sin cliente'}{p.carpetas.length > 0 && <span className="local" title={p.carpetas.join('\n')}> · local</span>}
            </span>
          </button>
        ))}
      </div>
    </nav>
  );
}
