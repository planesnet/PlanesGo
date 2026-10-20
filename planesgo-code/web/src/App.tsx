import { useCallback, useEffect, useState } from 'react';
import { get, post, type Proyecto, type Usuario } from './api';
import Acceso from './componentes/Acceso';
import Proyectos from './componentes/Proyectos';
import Chat from './componentes/Chat';
import Artefactos from './componentes/Artefactos';

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
    window.addEventListener('psf:sin-sesion', sinSesion);
    return () => window.removeEventListener('psf:sin-sesion', sinSesion);
  }, []);

  const salir = useCallback(async () => {
    try { await post('/api/logout', {}); } finally { setUsuario(null); setProyecto(null); }
  }, []);

  if (usuario === undefined) return <div className="cargando">Cargando…</div>;
  if (!usuario) return <Acceso aviso={aviso} alEntrar={u => { setAviso(''); setUsuario(u); }} />;

  return (
    <div className={'arnes' + (panel ? '' : ' sin-panel')}>
      <Proyectos usuario={usuario} actual={proyecto} alElegir={setProyecto} alSalir={salir} />
      <Chat proyecto={proyecto} panelAbierto={panel} alPanel={() => setPanel(p => !p)} />
      {panel && <Artefactos proyecto={proyecto} />}
    </div>
  );
}
