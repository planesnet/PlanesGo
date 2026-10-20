import { useCallback, useEffect, useRef, useState } from 'react';
import { escuchaAvisos, get, post, type Proveedor, type Proyecto, type Usuario } from './api';
import * as S from './sesiones';
import { leeAjustes, AJUSTES_DEFECTO, type AjustesProyecto } from './ajustes';
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
  // Ancho del panel derecho (0: el de por defecto, un 42 % de la ventana), recordado entre sesiones.
  const [anchoPanel, setAnchoPanel] = useState<number>(() => { try { return Number(localStorage.getItem('pgo:ancho-panel')) || 0; } catch { return 0; } });
  const cambiaAncho = (px: number) => { setAnchoPanel(px); try { localStorage.setItem('pgo:ancho-panel', String(px)); } catch { /* sin almacenamiento */ } };
  const [proveedores, setProveedores] = useState<Proveedor[]>([]);
  const [repo, setRepo] = useState<S.Repo | null>(null);
  const [sesiones, setSesiones] = useState<S.Sesion[]>([]);
  const [sesion, setSesion] = useState<S.Sesion | null>(null);
  const [ajustes, setAjustes] = useState<AjustesProyecto>(AJUSTES_DEFECTO);
  useEffect(() => { setAjustes(usuario && proyecto ? leeAjustes(usuario.login, proyecto.id) : AJUSTES_DEFECTO); }, [usuario, proyecto?.id]);
  // Sesión pedida al cambiar de proyecto desde la lista (su id, o 'nueva' para abrir una sesión nueva).
  const pedida = useRef('');
  // Carpeta cuyo repositorio está cargado (para recargarlo si la sesión cambia de carpeta).
  const carpetaRepo = useRef('');

  // Avisos del backend: programas que cambian y URL que abrir en el navegador integrado.
  const [pedidoWeb, setPedidoWeb] = useState<{ url: string; n: number } | null>(null);
  const [versionProcesos, setVersionProcesos] = useState(0);
  const proyectoId = useRef(0);
  proyectoId.current = proyecto?.id || 0;
  const abreWeb = useCallback((url: string) => { setPanel(true); setPedidoWeb({ url, n: Date.now() }); }, []);
  useEffect(() => {
    if (!usuario) return;
    return escuchaAvisos(a => {
      if (a.tipo === 'procesos') setVersionProcesos(v => v + 1);
      else if (a.tipo === 'abrir' && (!a.proyecto || a.proyecto === proyectoId.current)) abreWeb(a.url);
    });
  }, [usuario, abreWeb]);

  // Uso de la app: mientras el usuario teclea, mueve el ratón o tiene algo a medio escribir (un mensaje, una
  // issue…), el backend no se actualiza solo. Se avisa como mucho cada 20 s.
  useEffect(() => {
    if (!usuario) return;
    let usado = true;
    const marca = () => { usado = true; };
    const borrador = () => Array.from(document.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>('textarea, .gh-nueva input, .nuevo-programa input'))
      .some(e => e.value.trim() !== '');
    const avisa = () => {
      if (!usado && !borrador()) return;
      usado = false;
      post('/api/actividad', {}).catch(() => {});
    };
    const eventos = ['keydown', 'pointerdown', 'pointermove', 'wheel', 'focus'] as const;
    eventos.forEach(ev => window.addEventListener(ev, marca, { passive: true }));
    avisa();
    const t = setInterval(avisa, 20000);
    return () => { clearInterval(t); eventos.forEach(ev => window.removeEventListener(ev, marca)); };
  }, [usuario]);

  // Título de la ventana con la versión.
  useEffect(() => {
    get<{ version: string }>('/api/version').then(v => { document.title = 'Planes Go Code ' + v.version; }).catch(() => {});
  }, []);

  useEffect(() => { if (usuario) get<Proveedor[]>('/api/proveedores').then(setProveedores).catch(() => {}); }, [usuario]);

  // Agente por defecto de una sesión nueva: el último elegido, si está disponible; si no, el primero disponible.
  const agentePorDefecto = useCallback(() => {
    let pref = '';
    try { pref = localStorage.getItem('pgo:proveedor') || ''; } catch { /* sin almacenamiento */ }
    return (proveedores.find(p => p.id === pref && p.disponible) || proveedores.find(p => p.id === 'claude-code' && p.disponible)
      || proveedores.find(p => p.disponible) || proveedores[0])?.id || '';
  }, [proveedores]);

  // Al elegir proyecto: su repositorio (si tiene carpeta local) y sus sesiones; si no hay ninguna, se crea una.
  useEffect(() => {
    if (!usuario || !proyecto) { setRepo(null); setSesiones([]); setSesion(null); return; }
    let l = S.lista(usuario.login, proyecto.id);
    const quiere = pedida.current;
    pedida.current = '';
    const elegida = l.find(s => s.id === quiere) || l[0];
    const carpeta = elegida?.carpeta || proyecto.carpetas[0] || '';
    carpetaRepo.current = carpeta;
    const sigue = (r: S.Repo | null) => {
      setRepo(r);
      // Sesiones con un agente que ya no está (p. ej. las antiguas por API): al agente por defecto.
      const disponibles = new Set(proveedores.filter(p => p.disponible).map(p => p.id));
      if (proveedores.length) {
        for (const s of l.filter(x => !disponibles.has(x.agente))) {
          l = S.guarda(usuario.login, { ...s, agente: agentePorDefecto(), modelo: undefined, sesionAgente: undefined });
        }
      }
      // Sin sesión por defecto: solo la que se pide (o una nueva, si se pide crearla).
      let s = l.find(x => x.id === quiere) || null;
      if (quiere === 'nueva' && proveedores.length) {
        s = { ...S.nueva(proyecto.id, carpeta, r?.rama || '', agentePorDefecto()), tipo: r?.tipo };
        l = S.guarda(usuario.login, s);
      }
      setSesiones(l);
      setSesion(s);
    };
    if (carpeta) get<S.Repo>(`/api/proyecto/repo?proyecto=${proyecto.id}&carpeta=${encodeURIComponent(carpeta)}`).then(sigue).catch(() => sigue(null));
    else sigue(null);
  }, [usuario, proyecto, proveedores.length]); // eslint-disable-line react-hooks/exhaustive-deps

  // Si la sesión cambia de carpeta, se carga el repositorio de esa carpeta (y su rama, si la que tenía no está).
  useEffect(() => {
    const c = sesion?.carpeta;
    if (!proyecto || !c || c === carpetaRepo.current) return;
    carpetaRepo.current = c;
    get<S.Repo>(`/api/proyecto/repo?proyecto=${proyecto.id}&carpeta=${encodeURIComponent(c)}`).then(r => {
      setRepo(r);
      setSesion(x => {
        if (!x || x.carpeta !== c || !usuario) return x;
        const rama = r.ramas.includes(x.rama) ? x.rama : (r.rama || '');
        if (rama === x.rama && x.tipo === r.tipo) return x;
        const n = { ...x, rama, tipo: r.tipo, sesionAgente: rama === x.rama ? x.sesionAgente : undefined };
        setSesiones(S.guarda(usuario.login, n));
        return n;
      });
    }).catch(() => setRepo(null));
  }, [sesion?.carpeta, proyecto?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // El icono del hosting se guarda en la sesión para pintarlo en la lista sin consultar el repositorio.
  useEffect(() => {
    if (usuario && sesion && repo && repo.carpeta === sesion.carpeta && sesion.tipo !== repo.tipo) {
      const n = { ...sesion, tipo: repo.tipo };
      setSesiones(S.guarda(usuario.login, n));
      setSesion(n);
    }
  }, [repo, sesion?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const cambiaSesion = useCallback((s: S.Sesion) => {
    if (!usuario) return;
    const l = S.guarda(usuario.login, s);
    if (s.proyecto === proyecto?.id) setSesiones(l); // las de otros proyectos solo se guardan
    setSesion(x => (x?.id === s.id ? s : x));
  }, [usuario, proyecto?.id]);

  // Nueva sesión (cambiar de rama abre una sesión nueva en esa rama).
  const nuevaSesion = useCallback((rama?: string, nombre?: string) => {
    if (!usuario || !proyecto) return null;
    const carpeta = sesion?.carpeta || proyecto.carpetas[0] || '';
    const s = S.nueva(proyecto.id, carpeta, rama ?? (repo?.rama || ''), sesion?.agente || agentePorDefecto(), nombre);
    s.permisos = sesion?.permisos;
    s.tipo = repo?.tipo;
    setSesiones(S.guarda(usuario.login, s));
    setSesion(s);
    return s;
  }, [usuario, proyecto, repo, sesion, agentePorDefecto]);

  // Resolver una issue desde Flujo: sesión nueva (en la misma carpeta y rama) y su petición se envía sola al agente.
  const [envioPendiente, setEnvioPendiente] = useState<{ sesion: string; texto: string } | null>(null);
  const resuelve = useCallback((p: { nombre: string; texto: string }) => {
    const s = nuevaSesion(sesion?.rama, p.nombre.length > 70 ? p.nombre.slice(0, 67) + '…' : p.nombre);
    if (s) setEnvioPendiente({ sesion: s.id, texto: p.texto });
  }, [nuevaSesion, sesion?.rama]);

  const borraSesion = useCallback((s: S.Sesion) => {
    if (!usuario) return;
    const l = S.borra(usuario.login, s);
    if (s.proyecto !== proyecto?.id) return;
    setSesiones(l);
    setSesion(x => (x?.id === s.id ? l[0] || null : x));
  }, [usuario, proyecto?.id]);

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

  // Desde la lista lateral: una sesión de cualquier proyecto reciente, o una nueva en él.
  const eligeSesionDe = useCallback((p: Proyecto, s: S.Sesion) => {
    if (proyecto?.id === p.id) { setSesion(s); return; }
    pedida.current = s.id;
    elige(p);
  }, [proyecto, elige]);
  const nuevaEn = useCallback((p: Proyecto) => {
    if (proyecto?.id === p.id) { nuevaSesion(); return; }
    pedida.current = 'nueva';
    elige(p);
  }, [proyecto, elige, nuevaSesion]);

  if (usuario === undefined) return <div className="cargando">Cargando…</div>;
  if (!usuario) return <Acceso aviso={aviso} alEntrar={u => { setAviso(''); setUsuario(u); }} />;

  return (
    <div className="aplicacion">
    <Barra usuario={usuario} alSalir={salir} />
    <div className={'arnes' + (panel ? '' : ' sin-panel')}
      style={panel && anchoPanel ? { gridTemplateColumns: `248px minmax(360px, 1fr) ${anchoPanel}px` } : undefined}>
      <Proyectos usuario={usuario} actual={proyecto} inicial={proyectoGuardado(usuario.login)} alElegir={elige}
        repo={repo} proveedores={proveedores} sesiones={sesiones} sesion={sesion}
        alAjustes={(p, a) => { if (p.id === proyecto?.id) setAjustes(a); }}
        alElegirSesion={eligeSesionDe} alNuevaSesion={nuevaEn} alCambiarSesion={cambiaSesion} alBorrarSesion={borraSesion} />
      <Chat seleccionSkills={ajustes.skills} hosting={ajustes.hosting?.tipo} detalle={ajustes.detalle} envioPendiente={envioPendiente} alEnviado={() => setEnvioPendiente(null)} proyecto={proyecto} sesion={sesion} repo={repo} proveedores={proveedores} panelAbierto={panel} alPanel={() => setPanel(p => !p)}
        alCambiarSesion={cambiaSesion} alNuevaSesion={nuevaSesion} />
      {panel && <Artefactos proyecto={proyecto} carpeta={sesion?.carpeta || proyecto?.carpetas[0] || ''} rama={sesion?.rama || ''} agente={sesion?.agente} sesiones={sesiones} alAncho={cambiaAncho}
        pedidoWeb={pedidoWeb} versionProcesos={versionProcesos} alAbrir={abreWeb} alResolver={resuelve} />}
    </div>
    </div>
  );
}
