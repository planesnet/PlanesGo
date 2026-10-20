import { useEffect, useMemo, useState } from 'react';
import { get, type Proyecto, type Usuario } from '../api';

interface Props { usuario: Usuario; actual: Proyecto | null; inicial: number; alElegir: (p: Proyecto) => void; alSalir: () => void }

const sinAcentos = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();

export default function Proyectos({ usuario, actual, inicial, alElegir, alSalir }: Props) {
  const [lista, setLista] = useState<Proyecto[] | null>(null);
  const [error, setError] = useState('');
  const [filtro, setFiltro] = useState('');
  const [soloLocales, setSoloLocales] = useState(false);

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

  const visibles = useMemo(() => {
    const f = sinAcentos(filtro.trim());
    return (lista || []).filter(p =>
      (!soloLocales || p.carpetas.length > 0) &&
      (!f || sinAcentos(p.nombre + ' ' + (p.cliente?.nombre || '')).includes(f)));
  }, [lista, filtro, soloLocales]);

  return (
    <nav className="proyectos" aria-label="Proyectos">
      <div className="marca">PSF<span>Code</span></div>
      <div className="busca">
        <input placeholder="Buscar proyecto…" value={filtro} onChange={e => setFiltro(e.target.value)} aria-label="Buscar proyecto" />
        <button className="icono" title="Volver a cargar de Odoo" onClick={() => carga(true)}>↻</button>
      </div>
      <label className="interruptor"><input type="checkbox" checked={soloLocales} onChange={e => setSoloLocales(e.target.checked)} /> Solo con carpeta en este equipo</label>
      <div className="lista">
        {error && <p className="error">{error}</p>}
        {!lista && !error && <p className="suave">Cargando proyectos de Odoo…</p>}
        {lista && visibles.length === 0 && <p className="suave">Ningún proyecto coincide.</p>}
        {visibles.map(p => (
          <button key={p.id} className={'proyecto' + (actual?.id === p.id ? ' activo' : '')} onClick={() => alElegir(p)}>
            <span className="nombre">{p.nombre}</span>
            <span className="detalle">
              {p.cliente?.nombre || 'Sin cliente'}{p.carpetas.length > 0 && <span className="local" title={p.carpetas.join('\n')}> · local</span>}
            </span>
          </button>
        ))}
      </div>
      <div className="usuario">
        <span title={usuario.login}>{usuario.nombre}</span>
        <button className="enlace" onClick={alSalir}>Salir</button>
      </div>
    </nav>
  );
}
