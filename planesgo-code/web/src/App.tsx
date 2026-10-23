import { useCallback, useEffect, useRef, useState } from 'react';
import { escuchaAvisos, get, listaCopias, post, quitaCopia, type Proveedor, type Proyecto, type Usuario } from './api';
import * as S from './sesiones';
import * as C from './copias';
import { duplicateSession as duplicateSessionOf } from './duplicate';
import type { PedidoWeb } from './componentes/Navegador';
import { leeAjustes, AJUSTES_DEFECTO, type AjustesProyecto } from './ajustes';
import Acceso from './componentes/Acceso';
import Proyectos from './componentes/Proyectos';
import Chat from './componentes/Chat';
import Artefactos from './componentes/Artefactos';
import Barra from './componentes/Barra';
import DialogoSesion, { type SessionSection } from './componentes/DialogoSesion';
import { OPEN_PROGRAM_SETTINGS } from './componentes/ProgramSettings';
import { askConfirm, Dialogs, showMessage } from './componentes/Modal';

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
    await showMessage(`La sesión trabajaba en una copia de trabajo creada por pgocode:\n${c.carpeta}\n\nTiene trabajo sin guardar (${c.pendiente || c.error || 'no se ha podido comprobar'}): no se quita.`, { level: 'warning' });
    return;
  }
  if (!(await askConfirm(C.explicaQuitar(c, true) + '\n\n¿Quitar la copia de trabajo de la sesión borrada?', { okText: 'Quitar', danger: true }))) return;
  try {
    const r = await quitaCopia(s.proyecto, c.carpeta, true);
    if (r.aviso) await showMessage(r.aviso, { level: 'warning' });
  } catch (e) { await showMessage('No se ha quitado la copia: ' + (e as Error).message, { level: 'error' }); }
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
      // Nueva sesión pedida desde la lista: se abre su ventana (no se crea hasta confirmar).
      if (quiere === 'nueva' && proveedores.length) {
        setDialogo({ sesion: S.nueva(proyecto.id, '', '', agentePorDefecto()), nueva: true });
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
  const nuevaSesion = useCallback((rama?: string, nombre?: string, envio?: string) => {
    if (!usuario || !proyecto) return;
    const s = S.nueva(proyecto.id, '', rama ?? '', sesion?.agente || agentePorDefecto(), nombre);
    s.permisos = sesion?.permisos;
    if (sesion && rama && rama === sesion.rama) { s.carpeta = sesion.carpeta; s.hosting = sesion.hosting; }
    setDialogo({ sesion: s, nueva: true, envio });
  }, [usuario, proyecto, sesion, agentePorDefecto]);

  const guardaDialogo = useCallback((s: S.Sesion, r: S.Repo | null) => {
    if (!usuario || !dialogo) return;
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

  // Resolver una issue desde Flujo: sesión nueva en la misma rama; su petición se envía sola al crearla.
  const resuelve = useCallback((p: { nombre: string; texto: string }) => {
    nuevaSesion(sesion?.rama, p.nombre.length > 70 ? p.nombre.slice(0, 67) + '…' : p.nombre, p.texto);
  }, [nuevaSesion, sesion?.rama]);

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
    <div className={'arnes' + (panel ? '' : ' sin-panel')}
      style={panel && anchoPanel ? { gridTemplateColumns: `auto minmax(360px, 1fr) ${anchoPanel}px` } : undefined}>
      <Proyectos usuario={usuario} actual={proyecto} inicial={proyectoGuardado(usuario.login)} alElegir={elige}
        repo={repo} proveedores={proveedores} sesiones={sesiones} sesion={sesion}
        alAjustes={(p, a) => { if (p.id === proyecto?.id) setAjustes(a); }}
        alElegirSesion={eligeSesionDe} alNuevaSesion={nuevaEn} alCambiarSesion={cambiaSesion} alBorrarSesion={borraSesion}
        onDuplicateSession={duplicateSession}
        alAjustesSesion={s => setDialogo({ sesion: s, nueva: false })} />
      <Chat versionProcesos={versionProcesos} alAbrirWeb={abreWeb} directorio={ajustes.directorio} seleccionSkills={ajustes.skills} hosting={ajustes.hosting?.tipo} detalle={ajustes.detalle} envioPendiente={envioPendiente} alEnviado={() => setEnvioPendiente(null)} proyecto={proyecto} sesion={sesionDelProyecto} repo={repo} proveedores={proveedores} panelAbierto={panel} alPanel={() => setPanel(p => !p)}
        alCambiarSesion={cambiaSesion} alNuevaSesion={rama => nuevaSesion(rama)} alAjustes={() => sesion && setDialogo({ sesion, nueva: false })} />
      {panel && <Artefactos proyecto={proyecto} carpeta={sesion?.carpeta || proyecto?.carpetas[0] || ''} rama={sesion?.rama || ''} agente={sesion?.agente} sesiones={sesiones} sesion={sesionDelProyecto} directorio={ajustes.directorio} alAncho={cambiaAncho}
        pedidoWeb={pedidoWeb} versionProcesos={versionProcesos} alAbrir={abreWeb} alResolver={resuelve} />}
    {dialogo && <DialogoSesion key={dialogo.sesion.id} sesion={dialogo.sesion} nueva={dialogo.nueva} login={usuario.login} inicial={dialogo.seccion}
      proveedores={proveedores} alCerrar={() => setDialogo(null)} alGuardar={guardaDialogo} />}
    <Dialogs />
    </div>
    </div>
  );
}
