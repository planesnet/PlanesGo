import { useEffect, useRef, useState } from 'react';
import { get, post, type Usuario } from '../api';
import Icono from './Icono';
import Imputacion, { type SesionMarcador } from './Imputacion';
import AppProperties, { updateText, type AppSection, type UpdateStatus } from './AppProperties';
import { askConfirm, showMessage } from './Modal';
import { openRemoteDesktop } from '../remoteConnections';

type EstadoAct = UpdateStatus;

const iniciales = (n: string) => n.split(/\s+/).filter(Boolean).slice(0, 2).map(p => p[0]?.toUpperCase()).join('') || '?';

// Barra superior: marca, aviso de actualización pendiente y menú del usuario (avatar).
export default function Barra({ usuario, alSalir, versionImputacion, sesion }: { usuario: Usuario; alSalir: () => void; versionImputacion: number; sesion?: SesionMarcador }) {
  const [act, setAct] = useState<EstadoAct | null>(null);
  const [abierto, setAbierto] = useState(false);
  const [properties, setProperties] = useState<AppSection | null>(null);
  const [sinFoto, setSinFoto] = useState(false);
  const menu = useRef<HTMLDivElement>(null);

  const carga = () => get<EstadoAct>('/api/actualizacion').then(setAct).catch(() => {});
  useEffect(() => {
    carga();
    const id = setInterval(carga, 30000);
    return () => clearInterval(id);
  }, []);

  // El panel se cierra al pulsar fuera o con Escape.
  useEffect(() => {
    if (!abierto) return;
    const fuera = (e: MouseEvent) => { if (!menu.current?.contains(e.target as Node)) setAbierto(false); };
    const tecla = (e: KeyboardEvent) => { if (e.key === 'Escape') setAbierto(false); };
    document.addEventListener('mousedown', fuera);
    document.addEventListener('keydown', tecla);
    return () => { document.removeEventListener('mousedown', fuera); document.removeEventListener('keydown', tecla); };
  }, [abierto]);

  // Pulsar el aviso de actualización pendiente la instala ya; con algo en marcha, nunca (se instalará sola al acabar).
  const aplicar = async () => {
    const no = (motivo: string) => showMessage(`No se puede actualizar ahora: ${motivo}. Se instalará sola cuando acabe y lleves un rato sin usar la app.`, { level: 'info' });
    if (act?.trabajando) { no(act.trabajando); return; }
    if (act?.ocupado && !(await askConfirm('Se cerrará y volverá a abrir pgocode (lo que tengas a medio escribir se conserva). ¿Actualizar ahora?', { okText: 'Actualizar' }))) return;
    setAct(a => a && { ...a, estado: 'aplicando' });
    try { await post('/api/actualizacion/aplicar', {}); } catch (e) {
      const m = (e as Error).message;
      if (m.includes('no se puede actualizar')) no(m.replace(/^.*no se puede actualizar ahora: /, ''));
      carga();
    }
  };
  const pausa = async () => {
    const p = !act?.pausada;
    setAct(a => a && { ...a, pausada: p });
    await post('/api/actualizacion/pausa', { pausada: p }).catch(() => {});
    carga();
  };
  // Buscar actualizaciones: siempre responde algo visible (en Propiedades → Actualizaciones): buscando, el resultado
  // o por qué no se puede (p. ej. «sin servidor de actualizaciones» en desarrollo).
  const [checkResult, setCheckResult] = useState('');
  const buscar = async () => {
    setAbierto(false);
    setProperties('updates');
    setCheckResult('Buscando…');
    try {
      const r = await post<{ ok: boolean; error?: string }>('/api/actualizacion/buscar', {});
      if (!r.ok) { setCheckResult('No se puede buscar: ' + (r.error || 'actualizaciones desactivadas')); return; }
    } catch (e) { setCheckResult('No se puede buscar: ' + (e as Error).message); return; }
    setTimeout(async () => {
      const e = await get<EstadoAct>('/api/actualizacion').catch(() => null);
      if (e) setAct(e);
      setCheckResult(!e ? 'No se ha podido leer el resultado.' : e.error ? 'No se ha podido comprobar: ' + e.error : e.estado === 'al-dia' ? 'Tienes la última versión.' : updateText(e));
    }, 3000);
  };

  const etiqueta = act && ({
    pendiente: { texto: `Actualizar a v${act.nueva}`, titulo: act.pausada ? 'Actualizaciones automáticas en pausa: pulsa para actualizar ahora' : 'Pulsa para actualizar ahora (si no, se instalará sola cuando lleves un rato sin usar la app)' },
    descargando: { texto: `Descargando v${act.nueva}…`, titulo: 'Descargando la nueva versión' },
    aplicando: { texto: 'Actualizando…', titulo: 'Se cierra y se vuelve a abrir con la versión nueva' },
    manual: { texto: `v${act.nueva} · reinstalar`, titulo: act.error || '' },
  } as Record<string, { texto: string; titulo: string }>)[act.estado];

  const foto = sinFoto
    ? <span className="avatar iniciales" aria-hidden="true">{iniciales(usuario.nombre)}</span>
    : <img className="avatar" src="/api/avatar" alt="" onError={() => setSinFoto(true)} />;

  return (
    <header className="barra">
      <div className="menu-usuario" ref={menu}>
        {/* El logo de pgocode abre el menú: arriba el usuario (avatar y nombre) y debajo sus opciones. */}
        <button className="boton-logo" onClick={() => setAbierto(a => !a)} aria-haspopup="menu" aria-expanded={abierto} title={`Planes Go Code · ${usuario.nombre}`}>
          <img src="/logo.svg" alt="Planes Go Code" />
        </button>
        {abierto && (
          <div className="panel-usuario" role="menu">
            <div className="quien">
              {foto}
              <div className="recorta"><b className="recorta">{usuario.nombre}</b><span className="suave pequeno recorta">{usuario.login}</span></div>
            </div>
            <button role="menuitem" onClick={buscar}><Icono n="recargar" t={16} /> Buscar actualizaciones</button>
            {act && act.estado !== 'desactivada' && (
              <button role="menuitemcheckbox" aria-checked={Boolean(act.pausada)} onClick={pausa}>
                <Icono n={act.pausada ? 'ejecutar' : 'parar'} t={16} /> {act.pausada ? 'Reanudar actualizaciones automáticas' : 'Pausar actualizaciones automáticas'}
              </button>
            )}
            <button role="menuitem" onClick={() => { setAbierto(false); setProperties('account'); }}><Icono n="ajustes" t={16} /> Propiedades</button>
            <button role="menuitem" onClick={() => { setAbierto(false); setProperties('about'); }}><Icono n="info" t={16} /> Acerca de Planes Go Code</button>
            <div className="separador" />
            <button role="menuitem" onClick={alSalir}><Icono n="salir" t={16} /> Salir</button>
          </div>
        )}
      </div>
      <span className="marca-barra">Planes <b>Go</b> Code</span>
      <span className="hueco" />
      {/* El reloj de imputación, centrado en la ventana. */}
      <div className="centro-barra"><Imputacion version={versionImputacion} sesion={sesion} /></div>
      <button className="icono boton-barra" onClick={() => openRemoteDesktop()} title="Escritorio remoto (Windows por RDP y AnyDesk)" aria-label="Escritorio remoto">
        <Icono n="ordenador" t={16} />
      </button>
      {etiqueta && (act!.estado === 'pendiente'
        ? <button className={'chip ' + act!.estado} title={etiqueta.titulo} onClick={aplicar}>{etiqueta.texto}</button>
        : <span className={'chip ' + act!.estado} title={etiqueta.titulo}>{etiqueta.texto}</span>)}
      {properties && <AppProperties user={usuario} avatar={foto} status={act} initial={properties} checkResult={checkResult} onClose={() => { setProperties(null); setCheckResult(''); }}
        onCheck={buscar} onTogglePause={pausa} onApply={aplicar} onLogout={alSalir} />}
    </header>
  );
}
