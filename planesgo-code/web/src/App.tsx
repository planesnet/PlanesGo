import { useCallback, useEffect, useState } from 'react';
import { get, post, type Proveedor, type Proyecto, type Usuario } from './api';
import * as S from './sesiones';
import Acceso from './componentes/Acceso';
import Proyectos from './componentes/Proyectos';
import Chat from './componentes/Chat';
import Artefactos from './componentes/Artefactos';
import Barra from './componentes/Barra';

// El último proyecto de cada usuario se guarda en la app (y en la URL, ?proyecto=ID): al volver a entrar,
// tras F5, cerrar la app o pulsar «Salir», se abre directamente.
const claveProyecto = (login: string) => 'pgo:proyecto:' + login.toLowerCase();

function proyectoGuardado(login: string): number {
  const deUrl = Number(new URLSearchParams(location.search).get('proyecto'));
  if (deUrl > 0) return deUrl;
  try { return Number(localStorage.getItem(claveProyecto(login))) || 0; } catch { return 0; }
}

function recuerdaProyecto(login: string, id: number) {
  const u = new URL(location.href);
  if (id) u.searchParams.set('proyecto', String(id)); else u.searchParams.delete('proyecto');
  history.replaceState(null, '', u);
  if (!id) return; // al salir se quita de la URL, pero el último proyecto del usuario se conserva
  try { localStorage.setItem(claveProyecto(login), String(id)); } catch { /* sin almacenamiento */ }
}

export default function App() {
  const [usuario, setUsuario] = useState<Usuario | null | undefined>(undefined);
  const [aviso, setAviso] = useState('');
  const [proyecto, setProyecto] = useState<Proyecto | null>(null);
  const [panel, setPanel] = useState(true);
  const [proveedores, setProveedores] = useState<Proveedor[]>([]);
  const [repo, setRepo] = useState<S.Repo | null>(null);
  const [sesiones, setSesiones] = useState<S.Sesion[]>([]);
  const [sesion, setSesion] = useState<S.Sesion | null>(null);

  // Título de la ventana con la versión.
  useEffect(() => {
    get<{ version: string }>('/api/version').then(v => { document.title = 'Planes Go Code ' + v.version; }).catch(() => {});
  }, []);

  useEffect(() => { if (usuario) get<Proveedor[]>('/api/proveedores').then(setProveedores).catch(() => {}); }, [usuario]);

  // Agente por defecto de una sesión nueva: el último elegido, si está disponible; si no, el primero disponible.
  const agentePorDefecto = useCallback(() => {
    let pref = '';
    try { pref = localStorage.getItem('pgo:proveedor') || ''; } catch { /* sin almacenamiento */ }
    return (proveedores.find(p => p.id === pref && p.disponible) || proveedores.find(p => p.disponible) || proveedores[0])?.id || '';
  }, [proveedores]);

  // Al elegir proyecto: su repositorio (si tiene carpeta local) y sus sesiones; si no hay ninguna, se crea una.
  useEffect(() => {
    if (!usuario || !proyecto) { setRepo(null); setSesiones([]); setSesion(null); return; }
    const carpeta = proyecto.carpetas[0] || '';
    const sigue = (r: S.Repo | null) => {
      setRepo(r);
      let l = S.lista(usuario.login, proyecto.id);
      if (!l.length && proveedores.length) {
        l = S.guarda(usuario.login, S.nueva(proyecto.id, carpeta, r?.rama || '', agentePorDefecto()));
      }
      setSesiones(l);
      setSesion(l[0] || null);
    };
    if (carpeta) get<S.Repo>(`/api/proyecto/repo?proyecto=${proyecto.id}&carpeta=${encodeURIComponent(carpeta)}`).then(sigue).catch(() => sigue(null));
    else sigue(null);
  }, [usuario, proyecto, proveedores.length]); // eslint-disable-line react-hooks/exhaustive-deps

  const cambiaSesion = useCallback((s: S.Sesion) => {
    if (!usuario) return;
    setSesiones(S.guarda(usuario.login, s));
    setSesion(x => (x?.id === s.id ? s : x));
  }, [usuario]);

  // Nueva sesión (cambiar de rama abre una sesión nueva en esa rama).
  const nuevaSesion = useCallback((rama?: string) => {
    if (!usuario || !proyecto) return;
    const s = S.nueva(proyecto.id, proyecto.carpetas[0] || '', rama ?? (repo?.rama || ''), sesion?.agente || agentePorDefecto());
    s.permisos = sesion?.permisos;
    setSesiones(S.guarda(usuario.login, s));
    setSesion(s);
  }, [usuario, proyecto, repo, sesion, agentePorDefecto]);

  const borraSesion = useCallback((s: S.Sesion) => {
    if (!usuario) return;
    const l = S.borra(usuario.login, s);
    setSesiones(l);
    setSesion(x => (x?.id === s.id ? l[0] || null : x));
  }, [usuario]);

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
    try { await post('/api/logout', {}); } finally { setUsuario(null); setProyecto(null); recuerdaProyecto('', 0); }
  }, []);

  const elige = useCallback((p: Proyecto) => { setProyecto(p); recuerdaProyecto(usuario?.login || '', p.id); }, [usuario]);

  if (usuario === undefined) return <div className="cargando">Cargando…</div>;
  if (!usuario) return <Acceso aviso={aviso} alEntrar={u => { setAviso(''); setUsuario(u); }} />;

  return (
    <div className="aplicacion">
    <Barra usuario={usuario} alSalir={salir} />
    <div className={'arnes' + (panel ? '' : ' sin-panel')}>
      <Proyectos usuario={usuario} actual={proyecto} inicial={proyectoGuardado(usuario.login)} alElegir={elige}
        repo={repo} proveedores={proveedores} sesiones={sesiones} sesion={sesion}
        alElegirSesion={setSesion} alNuevaSesion={() => nuevaSesion()} alCambiarSesion={cambiaSesion} alBorrarSesion={borraSesion} />
      <Chat proyecto={proyecto} sesion={sesion} repo={repo} proveedores={proveedores} panelAbierto={panel} alPanel={() => setPanel(p => !p)}
        alCambiarSesion={cambiaSesion} alNuevaSesion={nuevaSesion} />
      {panel && <Artefactos proyecto={proyecto} />}
    </div>
    </div>
  );
}
