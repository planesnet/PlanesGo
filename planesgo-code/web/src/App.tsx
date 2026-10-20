import { useCallback, useEffect, useState } from 'react';
import { get, post, type Proyecto, type Usuario } from './api';
import Acceso from './componentes/Acceso';
import Proyectos from './componentes/Proyectos';
import Chat from './componentes/Chat';
import Artefactos from './componentes/Artefactos';

// El proyecto elegido se guarda en la URL (?proyecto=ID) y en el navegador: F5 no lo pierde.
function proyectoGuardado(): number {
  const deUrl = Number(new URLSearchParams(location.search).get('proyecto'));
  if (deUrl > 0) return deUrl;
  try { return Number(localStorage.getItem('pgo:proyecto')) || 0; } catch { return 0; }
}

function recuerdaProyecto(id: number) {
  const u = new URL(location.href);
  if (id) u.searchParams.set('proyecto', String(id)); else u.searchParams.delete('proyecto');
  history.replaceState(null, '', u);
  try { if (id) localStorage.setItem('pgo:proyecto', String(id)); else localStorage.removeItem('pgo:proyecto'); } catch { /* sin almacenamiento */ }
}

export default function App() {
  const [usuario, setUsuario] = useState<Usuario | null | undefined>(undefined);
  const [aviso, setAviso] = useState('');
  const [proyecto, setProyecto] = useState<Proyecto | null>(null);
  const [panel, setPanel] = useState(true);

  useEffect(() => {
    get<Usuario>('/api/yo').then(setUsuario).catch(() => setUsuario(null));
    // Solo se avisa si había sesión (caducó); al abrir la app sin sesión no hay nada que avisar.
    const sinSesion = (e: Event) => setUsuario(prev => {
      if (prev) setAviso((e as CustomEvent).detail || '');
      return null;
    });
    window.addEventListener('pgo:sin-sesion', sinSesion);
    return () => window.removeEventListener('pgo:sin-sesion', sinSesion);
  }, []);

  const salir = useCallback(async () => {
    try { await post('/api/logout', {}); } finally { setUsuario(null); setProyecto(null); recuerdaProyecto(0); }
  }, []);

  const elige = useCallback((p: Proyecto) => { setProyecto(p); recuerdaProyecto(p.id); }, []);

  if (usuario === undefined) return <div className="cargando">Cargando…</div>;
  if (!usuario) return <Acceso aviso={aviso} alEntrar={u => { setAviso(''); setUsuario(u); }} />;

  return (
    <div className={'arnes' + (panel ? '' : ' sin-panel')}>
      <Proyectos usuario={usuario} actual={proyecto} inicial={proyectoGuardado()} alElegir={elige} alSalir={salir} />
      <Chat proyecto={proyecto} panelAbierto={panel} alPanel={() => setPanel(p => !p)} />
      {panel && <Artefactos proyecto={proyecto} />}
    </div>
  );
}
