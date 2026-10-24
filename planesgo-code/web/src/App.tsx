import { useCallback, useEffect, useRef, useState } from 'react';
import { escuchaAvisos, get, listaCopias, post, quitaCopia, type Proveedor, type Proyecto, type Usuario } from './api';
import * as S from './sesiones';
import * as C from './copias';
import { duplicateSession as duplicateSessionOf } from './duplicate';
import { newSessionDefaults, rememberedModel, rememberModel } from './sessionDefaults';
import type { PedidoWeb } from './componentes/Navegador';
import { leeAjustes, AJUSTES_DEFECTO, type AjustesProyecto } from './ajustes';
import Acceso from './componentes/Acceso';
import ErrorBoundary from './componentes/ErrorBoundary';
import Proyectos from './componentes/Proyectos';
import Chat from './componentes/Chat';
import Artefactos from './componentes/Artefactos';
import Barra from './componentes/Barra';
import DialogoSesion, { type SessionSection } from './componentes/DialogoSesion';
import { OPEN_PROGRAM_SETTINGS } from './componentes/ProgramSettings';
import { askConfirm, Dialogs, showMessage } from './componentes/Modal';
import type { PeticionResolver } from './componentes/Flujo';
import { canStartAutomatic, issuesToResolve, newPsfState, PSF_CHECK, SHOW_EVIDENCE, type WatchedIssue } from './psf';

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

// Al borrar una sesión: si trabajaba en una copia de trabajo que creó pgocode y ninguna otra sesión la usa, se
// ofrece quitarla (solo si no tiene nada sin guardar; si lo tiene, se avisa y se deja).
async function ofreceQuitarCopia(s: S.Sesion, otras: S.Sesion[]) {
  let c: C.Copia | null = null;
  try { c = C.copiaDeSesion(await listaCopias(s.proyecto), s, otras); } catch { return; }
  if (!c) return;
  if (!c.se_puede_quitar) {
    await showMessage(`La sesión trabajaba en una subcarpeta creada por pgocode:\n${c.carpeta}\n\nTiene trabajo sin guardar (${c.pendiente || c.error || 'no se ha podido comprobar'}): no se quita.`, { level: 'warning' });
    return;
  }
  if (!(await askConfirm(C.explicaQuitar(c, true) + '\n\n¿Quitar la subcarpeta de la sesión borrada?', { okText: 'Quitar', danger: true }))) return;
  try {
    const r = await quitaCopia(s.proyecto, c.carpeta, true);
    if (r.aviso) await showMessage(r.aviso, { level: 'warning' });
  } catch (e) { await showMessage('No se ha quitado la subcarpeta: ' + (e as Error).message, { level: 'error' }); }
}

export default function App() {
  const [usuario, setUsuario] = useState<Usuario | null | undefined>(undefined);
  const [aviso, setAviso] = useState('');
  const [proyecto, setProyecto] = useState<Proyecto | null>(null);
  const [panel, setPanel] = useState(true);
  // Ancho del panel derecho (0: el de por defecto, un 42 % de la ventana), recordado entre sesiones.
  const [anchoPanel, setAnchoPanel] = useState<number>(() => { try { return Number(localStorage.getItem('pgo:ancho-panel')) || 0; } catch { return 0; } });
  const cambiaAncho = (px: number) => { setAnchoPanel(px); try { localStorage.setItem('pgo:ancho-panel', String(px)); } catch { /* sin almacenamiento */ } };
  // Projects panel: its width (0: fits its content) and whether it is hidden (both remembered).
  const [projectsWidth, setProjectsWidth] = useState<number>(() => { try { return Number(localStorage.getItem('pgo:ancho-proyectos')) || 0; } catch { return 0; } });
  const [projectsHidden, setProjectsHidden] = useState<boolean>(() => { try { return localStorage.getItem('pgo:proyectos-ocultos') === '1'; } catch { return false; } });
  const resizeProjects = (px: number) => { setProjectsWidth(px); try { localStorage.setItem('pgo:ancho-proyectos', String(px)); } catch { /* no storage */ } };
  const toggleProjects = () => setProjectsHidden(h => { try { localStorage.setItem('pgo:proyectos-ocultos', h ? '' : '1'); } catch { /* no storage */ } return !h; });
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
  const [pedidoWeb, setPedidoWeb] = useState<PedidoWeb | null>(null);
  const [versionProcesos, setVersionProcesos] = useState(0);
  const [versionImputacion, setVersionImputacion] = useState(0);
  const proyectoId = useRef(0);
  proyectoId.current = proyecto?.id || 0;
  const abreWeb = useCallback((url: string, formato?: PedidoWeb['formato']) => { setPanel(true); setPedidoWeb({ url, n: Date.now(), formato }); }, []);
  useEffect(() => {
    if (!usuario) return;
    return escuchaAvisos(a => {
      if (a.tipo === 'procesos') setVersionProcesos(v => v + 1);
      else if (a.tipo === 'imputacion') setVersionImputacion(v => v + 1);
      else if (a.tipo === 'abrir' && (!a.proyecto || a.proyecto === proyectoId.current)) abreWeb(a.url);
    });
  }, [usuario, abreWeb]);

  // Uso de la app: mientras el usuario teclea, mueve el ratón o tiene algo a medio escribir (un mensaje, una
  // issue…), el backend no se actualiza solo. Se avisa como mucho cada 20 s.
  // Además, la actividad humana (con la ventana en primer plano) en la sesión abierta se imputa a esa sesión.
  const sesionActividad = useRef<S.Sesion | null>(null);
  sesionActividad.current = sesion;
  useEffect(() => {
    if (!usuario) return;
    let usado = true;
    let humana = false;
    const marca = () => { usado = true; if (document.hasFocus()) humana = true; };
    const borrador = () => Array.from(document.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>('textarea, .gh-nueva input, .nuevo-programa input'))
      .some(e => e.value.trim() !== '');
    const avisa = () => {
      if (!usado && !borrador()) return;
      // Sin sesión abierta, el trabajo con el proyecto (elegirlo, sus propiedades, su panel) va al proyecto.
      const s = sesionActividad.current;
      const pid = proyectoId.current;
      const cuerpo = !humana ? {} : s ? { humana: true, sesion: s.id, titulo: s.nombre, proyecto: s.proyecto, tarea_id: s.tareaId || 0, tarea: s.tarea || '' }
        : pid ? { humana: true, sesion: `proyecto-${pid}`, titulo: 'Proyecto', proyecto: pid } : {};
      usado = false;
      humana = false;
      post('/api/actividad', cuerpo).catch(() => {});
    };
    const eventos = ['keydown', 'pointerdown', 'pointermove', 'wheel', 'focus', 'input', 'change'] as const;
    eventos.forEach(ev => window.addEventListener(ev, marca, { passive: true, capture: true }));
    // El navegador interno (webview) avisa por Electron: sus teclas y su ratón no llegan a esta ventana.
    const fueraWebview = window.psf?.alActividad?.(() => { usado = true; humana = true; });
    avisa();
    const t = setInterval(avisa, 20000);
    return () => { clearInterval(t); fueraWebview?.(); eventos.forEach(ev => window.removeEventListener(ev, marca, { capture: true })); };
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
    // La sesión de otro proyecto se suelta ya: mientras se lee el repositorio del nuevo, el chat no puede seguir
    // con la sesión anterior (sus mensajes irían con este proyecto y la carpeta del otro).
    setSesion(x => (x && x.proyecto === proyecto.id ? x : null));
    let l = S.lista(usuario.login, proyecto.id);
    const quiere = pedida.current;
    pedida.current = '';
    const elegida = l.find(s => s.id === quiere) || l[0];
    const carpeta = elegida?.carpeta || proyecto.carpetas[0] || '';
    carpetaRepo.current = carpeta;
    // Si mientras se lee el repositorio se cambia de proyecto (o de usuario), esta respuesta ya no vale: si no, las
    // sesiones del proyecto anterior se pintarían (y se podrían abrir) bajo el nuevo.
    let vigente = true;
    const sigue = (r: S.Repo | null) => {
      if (!vigente) return;
      l = S.lista(usuario.login, proyecto.id);
      setRepo(r);
      // Sesiones con un agente que ya no está (p. ej. las antiguas por API): al agente por defecto.
      const disponibles = new Set(proveedores.filter(p => p.disponible).map(p => p.id));
      if (proveedores.length) {
        for (const s of l.filter(x => !disponibles.has(x.agente))) {
          l = S.guarda(usuario.login, { ...s, agente: agentePorDefecto(), modelo: undefined, sesionAgente: undefined });
        }
      }
      // La que se pide; si no se pide ninguna y el proyecto tiene una sola sesión (sin contar las archivadas), esa.
      let s = l.find(x => x.id === quiere) || S.onlySession(l, quiere);
      // Nueva sesión pedida desde la lista: se abre su ventana (no se crea hasta confirmar).
      if (quiere === 'nueva' && proveedores.length) {
        setDialogo({ sesion: conDefectos(S.nueva(proyecto.id, '', '', agentePorDefecto()), l), nueva: true });
      }
      setSesiones(l.filter(x => x.proyecto === proyecto.id));
      setSesion(s && s.proyecto === proyecto.id ? s : null);
    };
    if (carpeta) get<S.Repo>(`/api/proyecto/repo?proyecto=${proyecto.id}&carpeta=${encodeURIComponent(carpeta)}`).then(sigue).catch(() => sigue(null));
    else sigue(null);
    return () => { vigente = false; };
  }, [usuario, proyecto, proveedores.length]); // eslint-disable-line react-hooks/exhaustive-deps

  // Si la sesión cambia de carpeta, se carga el repositorio de esa carpeta (y su rama, si la que tenía no está).
  useEffect(() => {
    const c = sesion?.carpeta;
    if (!proyecto || !sesion || sesion.proyecto !== proyecto.id || !c || c === carpetaRepo.current) return;
    carpetaRepo.current = c;
    let vigente = true;
    get<S.Repo>(`/api/proyecto/repo?proyecto=${proyecto.id}&carpeta=${encodeURIComponent(c)}`).then(r => {
      if (!vigente) return;
      setRepo(r);
      setSesion(x => {
        if (!x || x.proyecto !== proyecto.id || x.carpeta !== c || !usuario || x.automatic) return x; // an automatic session keeps its psf/issue-<N> branch
        const rama = r.ramas.includes(x.rama) ? x.rama : (r.rama || '');
        if (rama === x.rama && x.tipo === r.tipo) return x;
        const n = { ...x, rama, tipo: r.tipo, sesionAgente: rama === x.rama ? x.sesionAgente : undefined };
        setSesiones(S.guarda(usuario.login, n));
        return n;
      });
    }).catch(() => { if (vigente) setRepo(null); });
    return () => { vigente = false; };
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
    rememberModel(usuario.login, s.agente, s.modelo);
    const l = S.guarda(usuario.login, s);
    // Con el proyecto activo de ahora (no el de cuando se llamó: un turno que acaba en segundo plano llama a
    // esto con la función de cuando empezó); las de otros proyectos solo se guardan.
    if (s.proyecto === proyectoId.current) setSesiones(l);
    setSesion(x => (x?.id === s.id ? s : x));
  }, [usuario, proyecto?.id]);

  // Ventana de la sesión: crear una (no existe hasta confirmar) o ajustar una existente. «envio»: mensaje que se
  // manda solo al agente en cuanto se cree (p. ej. «Resuelve la issue…»).
  const [dialogo, setDialogo] = useState<{ sesion: S.Sesion; nueva: boolean; envio?: string; seccion?: SessionSection } | null>(null);
  const [envioPendiente, setEnvioPendiente] = useState<{ sesion: string; texto: string } | null>(null);

  // Nueva sesión: se abre su ventana. Con rama (cambiar de rama en la cabecera) va ya elegida; si es la misma
  // rama de la sesión abierta (resolver una issue), también su carpeta y su alojamiento.
  // Una sesión nueva: modo «editar» (no el de la última: podía ser «auto»), el alojamiento de la última del proyecto
  // y el último modelo elegido con su agente (sessionDefaults.ts).
  const conDefectos = (s: S.Sesion, l: S.Sesion[]) => newSessionDefaults(s, { sessions: l, projectHosting: ajustes.hosting?.tipo,
    model: usuario ? rememberedModel(usuario.login, s.agente, proveedores.find(p => p.id === s.agente)?.modelos || []) : undefined });
  const nuevaSesion = useCallback((rama?: string, nombre?: string, envio?: string) => {
    if (!usuario || !proyecto) return;
    const s = S.nueva(proyecto.id, '', rama ?? '', sesion?.agente || agentePorDefecto(), nombre);
    if (sesion && rama && rama === sesion.rama) { s.carpeta = sesion.carpeta; s.hosting = sesion.hosting; }
    setDialogo({ sesion: conDefectos(s, sesiones), nueva: true, envio });
  }, [usuario, proyecto, sesion, sesiones, agentePorDefecto, ajustes, proveedores]); // eslint-disable-line react-hooks/exhaustive-deps

  const guardaDialogo = useCallback((s: S.Sesion, r: S.Repo | null) => {
    if (!usuario || !dialogo) return;
    rememberModel(usuario.login, s.agente, s.modelo);
    const l = S.guarda(usuario.login, s);
    if (s.proyecto === proyecto?.id) {
      setSesiones(l);
      if (dialogo.nueva || sesion?.id === s.id) {
        carpetaRepo.current = s.carpeta;
        setRepo(r);
        setSesion(s);
      }
    }
    if (dialogo.nueva && dialogo.envio) setEnvioPendiente({ sesion: s.id, texto: dialogo.envio });
    setDialogo(null);
  }, [usuario, dialogo, proyecto?.id, sesion?.id]);

  // Resolve an issue: an automatic PSF session (psf.ts), created without opening it, that works on its own in its
  // psf/issue-<N> branch; Chat drives it (psfStarts). If the issue already has one, that one resumes. It can be in
  // another project than the open one (automatic resolution): then it takes that project's settings and the agent
  // of its latest session.
  const [psfStarts, setPsfStarts] = useState<S.Sesion[]>([]);
  const startPsfSession = (projectId: number, p: PeticionResolver, auto = false) => {
    if (!usuario) return;
    const active = projectId === proyectoId.current;
    const list = active ? sesiones : S.lista(usuario.login, projectId);
    const existing = list.find(s => s.automatic && s.psf?.issue === p.number && s.carpeta === p.folder && !s.archivada);
    if (existing) { if (!auto) setPsfStarts(q => [...q, { ...existing }]); return; }
    const settings = leeAjustes(usuario.login, projectId);
    // The agent: the one of the person's latest own session (a change made to one automatic session stays there).
    const byDate = [...list].filter(s => !s.madre).sort((a, b) => (b.creada || 0) - (a.creada || 0));
    const latest = byDate.find(s => !s.automatic) || byDate[0];
    // A person resolving from Flujo: the open session's agent; the automatic resolution: never the open session's.
    const agent = (active && !auto && sesion?.agente) || latest?.agente || agentePorDefecto();
    const name = `⚡ Issue #${p.number} · ${p.title}`;
    const base = S.nueva(projectId, p.folder, '', agent, name.length > 70 ? name.slice(0, 67) + '…' : name);
    const psf = { ...newPsfState(p.number, p.title, p.url), base: settings.issues?.mergeBranch || undefined, autoStarted: auto || undefined };
    const s: S.Sesion = { ...newSessionDefaults(base, { sessions: list, projectHosting: settings.hosting?.tipo,
      model: rememberedModel(usuario.login, agent, proveedores.find(x => x.id === agent)?.modelos || []) }), rama: psf.branch, automatic: true, psf };
    if (active && sesion && sesion.carpeta === p.folder && sesion.hosting) s.hosting = sesion.hosting;
    if (!s.hosting) s.hosting = (active && repo?.tipo) || 'local';
    const l = S.guarda(usuario.login, s);
    if (active) setSesiones(l);
    setPsfStarts(q => [...q, s]);
  };
  const startPsfRef = useRef(startPsfSession);
  startPsfRef.current = startPsfSession;
  const resuelve = useCallback((p: PeticionResolver) => { if (proyecto) startPsfRef.current(proyecto.id, p); }, [proyecto]);

  // Automatic resolution (project «Issues» settings): every 5 minutes (and right away on PSF_CHECK), while pgocode is open, the projects with it
  // turned on are checked for new GitHub issues; the oldest new one starts its automatic session, one working
  // session per project at a time (psf.ts: issuesToResolve, canStartAutomatic).
  useEffect(() => {
    if (!usuario) return;
    let stopped = false;
    const tick = async () => {
      const projects = await get<Proyecto[]>('/api/proyectos').catch(() => [] as Proyecto[]);
      for (const p of projects) {
        const cfg = leeAjustes(usuario.login, p.id).issues;
        const folder = p.carpetas[0];
        if (stopped || !cfg?.autoResolve || !folder) continue;
        const sessions = S.lista(usuario.login, p.id);
        if (!canStartAutomatic(sessions)) continue;
        const st = await get<{ issues: WatchedIssue[] }>(`/api/github?proyecto=${p.id}&carpeta=${encodeURIComponent(folder)}`).catch(() => null);
        const next = st && issuesToResolve(st.issues, sessions, cfg.autoResolveSince || Date.now())[0];
        if (next && !stopped) startPsfRef.current(p.id, { number: next.numero, title: next.titulo, url: next.url, folder }, true);
      }
    };
    const first = setTimeout(tick, 30_000);
    const every = setInterval(tick, 5 * 60_000);
    const now = () => setTimeout(tick, 2000); // GitHub needs a moment to list a just-created issue
    window.addEventListener(PSF_CHECK, now);
    return () => { stopped = true; clearTimeout(first); clearInterval(every); window.removeEventListener(PSF_CHECK, now); };
  }, [usuario?.login]); // eslint-disable-line react-hooks/exhaustive-deps

  const borraSesion = useCallback((s: S.Sesion) => {
    if (!usuario) return;
    const l = S.borra(usuario.login, s);
    ofreceQuitarCopia(s, l);
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

  // El chat solo trabaja con una sesión del proyecto activo: su carpeta es la que manda (repositorio, rama, git).
  const sesionDelProyecto = sesion && proyecto && sesion.proyecto === proyecto.id ? sesion : null;
  // «Definir programas» (Actividad, Programas): abre las propiedades de la sesión en su sección Programas.
  useEffect(() => {
    const abre = () => { if (sesionDelProyecto) setDialogo({ sesion: sesionDelProyecto, nueva: false, seccion: 'programs' }); };
    window.addEventListener(OPEN_PROGRAM_SETTINGS, abre);
    return () => window.removeEventListener(OPEN_PROGRAM_SETTINGS, abre);
  }, [sesionDelProyecto]);
  const elige = useCallback((p: Proyecto) => { setProyecto(p); recuerdaProyecto(usuario?.login || '', p.id); }, [usuario]);

  // Desde la lista lateral: una sesión de cualquier proyecto reciente, o una nueva en él.
  const eligeSesionDe = useCallback((p: Proyecto, s: S.Sesion) => {
    if (proyecto?.id === p.id) { setSesion(s); return; }
    pedida.current = s.id;
    elige(p);
  }, [proyecto, elige]);

  // Abrir una sesión hija desde la tarjeta del reparto (Chat.tsx), por su id.
  useEffect(() => {
    const abre = (e: Event) => {
      const id = (e as CustomEvent<string>).detail;
      const s = usuario && proyecto ? S.lista(usuario.login, proyecto.id).find(x => x.id === id) : undefined;
      if (s) setSesion(s);
    };
    window.addEventListener('pgo:abre-sesion', abre);
    return () => window.removeEventListener('pgo:abre-sesion', abre);
  }, [usuario, proyecto?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // Una hija cuya copia se ha quitado tras unirla sigue en la carpeta y la rama de su madre (su historial se conserva).
  useEffect(() => {
    const reubica = (e: Event) => {
      const d = (e as CustomEvent<{ id: string; proyecto: number; carpeta: string; rama: string; merged?: boolean }>).detail;
      const s = usuario ? S.lista(usuario.login, d.proyecto).find(x => x.id === d.id) : undefined;
      // Unida: se pliega bajo su madre en la lista de sesiones (no una fila por parte terminada).
      if (s) cambiaSesion({ ...s, carpeta: d.carpeta, rama: d.rama, ...(d.merged ? { merged: true } : {}) });
    };
    window.addEventListener('pgo:reubica-sesion', reubica);
    return () => window.removeEventListener('pgo:reubica-sesion', reubica);
  }, [usuario, cambiaSesion]);

  // Archivar sesiones (Actividad, al quitar la copia de trabajo que usaban): salen de la lista principal.
  useEffect(() => {
    const archiva = (e: Event) => {
      const d = (e as CustomEvent<{ proyecto: number; ids: string[] }>).detail;
      if (!usuario) return;
      let l: S.Sesion[] = S.lista(usuario.login, d.proyecto);
      for (const s of l.filter(x => d.ids.includes(x.id))) l = S.guarda(usuario.login, { ...s, archivada: true });
      if (d.proyecto !== proyectoId.current) return;
      setSesiones(l);
      setSesion(x => (x && d.ids.includes(x.id) ? null : x));
    };
    window.addEventListener(S.ARCHIVE_SESSIONS, archiva);
    return () => window.removeEventListener(S.ARCHIVE_SESSIONS, archiva);
  }, [usuario]);

  // The strip's «Fotos»: the side panel opens (Artefactos shows the TV with the session's pictures).
  useEffect(() => {
    const abre = () => setPanel(true);
    window.addEventListener(SHOW_EVIDENCE, abre);
    return () => window.removeEventListener(SHOW_EVIDENCE, abre);
  }, []);

  // A new session on a branch, from elsewhere (the ⋮ menu of a subfolder in Actividad).
  useEffect(() => {
    const nueva = (e: Event) => nuevaSesion((e as CustomEvent<string>).detail);
    window.addEventListener(S.NEW_SESSION_ON_BRANCH, nueva);
    return () => window.removeEventListener(S.NEW_SESSION_ON_BRANCH, nueva);
  }, [nuevaSesion]);

  // Open a session of the open project from elsewhere (e.g. the running issues in Actividad).
  useEffect(() => {
    const abre = (e: Event) => { const s = sesiones.find(x => x.id === (e as CustomEvent<string>).detail); if (s) setSesion(s); };
    window.addEventListener(S.OPEN_SESSION, abre);
    return () => window.removeEventListener(S.OPEN_SESSION, abre);
  }, [sesiones]);

  // «Duplicar sin historial»: la copia se guarda y se abre (si es de otro proyecto, al abrir ese proyecto).
  const duplicateSession = useCallback((p: Proyecto, s: S.Sesion) => {
    if (!usuario) return;
    const c = duplicateSessionOf(s, S.lista(usuario.login, s.proyecto));
    const l = S.guarda(usuario.login, c);
    if (proyecto?.id === p.id) setSesiones(l);
    eligeSesionDe(p, c);
  }, [usuario, proyecto?.id, eligeSesionDe]);
  const nuevaEn = useCallback((p: Proyecto) => {
    if (proyecto?.id === p.id) { nuevaSesion(); return; }
    pedida.current = 'nueva';
    elige(p);
  }, [proyecto, elige, nuevaSesion]);

  if (usuario === undefined) return <div className="cargando">Cargando…</div>;
  if (!usuario) return <Acceso aviso={aviso} alEntrar={u => { setAviso(''); setUsuario(u); }} />;

  return (
    <div className="aplicacion">
    <Barra usuario={usuario} alSalir={salir} versionImputacion={versionImputacion}
      sesion={sesion ? { id: sesion.id, proyecto: sesion.proyecto, nombreProyecto: proyecto?.nombre, tareaId: sesion.tareaId, tarea: sesion.tarea } : undefined} />
    <div className={'arnes' + (panel ? '' : ' sin-panel') + (projectsHidden ? ' sin-proyectos' : '')}
      style={panel && anchoPanel ? { gridTemplateColumns: `${projectsHidden ? '' : 'auto '}minmax(360px, 1fr) ${anchoPanel}px` } : undefined}>
      {!projectsHidden && <ErrorBoundary area="la lista de proyectos"><Proyectos width={projectsWidth} onResize={resizeProjects} onHide={toggleProjects} usuario={usuario} actual={proyecto} inicial={proyectoGuardado(usuario.login)} alElegir={elige}
        repo={repo} proveedores={proveedores} sesiones={sesiones} sesion={sesion}
        alAjustes={(p, a) => { if (p.id === proyecto?.id) setAjustes(a); }}
        alElegirSesion={eligeSesionDe} alNuevaSesion={nuevaEn} alCambiarSesion={cambiaSesion} alBorrarSesion={borraSesion}
        onDuplicateSession={duplicateSession}
        alAjustesSesion={s => setDialogo({ sesion: s, nueva: false })} /></ErrorBoundary>}
      <ErrorBoundary area="el chat" resetKey={sesionDelProyecto?.id}><Chat versionProcesos={versionProcesos} alAbrirWeb={abreWeb} directorio={ajustes.directorio} seleccionSkills={ajustes.skills} hosting={ajustes.hosting?.tipo} detalle={ajustes.detalle} envioPendiente={envioPendiente} alEnviado={() => setEnvioPendiente(null)} onShowProjects={projectsHidden ? toggleProjects : undefined} psfStarts={psfStarts} onPsfStarted={ids => setPsfStarts(q => q.filter(x => !ids.includes(x.id)))} projectSettings={id => (usuario ? leeAjustes(usuario.login, id) : AJUSTES_DEFECTO)} proyecto={proyecto} sesion={sesionDelProyecto} repo={repo} proveedores={proveedores} panelAbierto={panel} alPanel={() => setPanel(p => !p)}
        alCambiarSesion={cambiaSesion} alNuevaSesion={rama => nuevaSesion(rama)} alAjustes={() => sesion && setDialogo({ sesion, nueva: false })} /></ErrorBoundary>
      {panel && <ErrorBoundary area="el panel" resetKey={sesionDelProyecto?.id}><Artefactos proyecto={proyecto} carpeta={sesion?.carpeta || proyecto?.carpetas[0] || ''} agente={sesion?.agente} sesiones={sesiones} sesion={sesionDelProyecto} directorio={ajustes.directorio} alAncho={cambiaAncho}
        pedidoWeb={pedidoWeb} versionProcesos={versionProcesos} alAbrir={abreWeb} alResolver={resuelve} /></ErrorBoundary>}
    {dialogo && <ErrorBoundary area="la ventana de la sesión"><DialogoSesion key={dialogo.sesion.id} sesion={dialogo.sesion} nueva={dialogo.nueva} login={usuario.login} inicial={dialogo.seccion}
      proveedores={proveedores} alCerrar={() => setDialogo(null)} alGuardar={guardaDialogo} /></ErrorBoundary>}
    <Dialogs />
    </div>
    </div>
  );
}
