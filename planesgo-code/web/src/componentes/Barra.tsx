import { useEffect, useRef, useState } from 'react';
import { get, post, type Usuario } from '../api';
import Icono from './Icono';
import Imputacion, { type SesionMarcador } from './Imputacion';

interface EstadoAct { version: string; estado: string; nueva?: string; error?: string; revisado?: string; ocupado?: boolean; pausada?: boolean }

const iniciales = (n: string) => n.split(/\s+/).filter(Boolean).slice(0, 2).map(p => p[0]?.toUpperCase()).join('') || '?';

// Barra superior: marca, aviso de actualización pendiente y menú del usuario (avatar).
export default function Barra({ usuario, alSalir, versionImputacion, sesion }: { usuario: Usuario; alSalir: () => void; versionImputacion: number; sesion?: SesionMarcador }) {
  const [act, setAct] = useState<EstadoAct | null>(null);
  const [abierto, setAbierto] = useState(false);
  const [acerca, setAcerca] = useState(false);
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

  // Pulsar el aviso de actualización pendiente la instala ya (si hay algo en curso, se avisa antes).
  const aplicar = async () => {
    if (act?.ocupado && !confirm('Se cerrará y volverá a abrir pgocode. Si hay un agente respondiendo, una prueba o un programa en marcha, se detendrá. ¿Actualizar ahora?')) return;
    setAct(a => a && { ...a, estado: 'aplicando' });
    try { await post('/api/actualizacion/aplicar', {}); } catch { carga(); }
  };
  const pausa = async () => {
    const p = !act?.pausada;
    setAct(a => a && { ...a, pausada: p });
    await post('/api/actualizacion/pausa', { pausada: p }).catch(() => {});
    carga();
  };
  const buscar = async () => { await post('/api/actualizacion/buscar', {}).catch(() => {}); setTimeout(carga, 3000); };

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
        <button className="boton-avatar" onClick={() => setAbierto(a => !a)} aria-haspopup="menu" aria-expanded={abierto} title={usuario.nombre}>
          {foto}
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
            <button role="menuitem" onClick={() => setAcerca(a => !a)} aria-expanded={acerca}><Icono n="info" t={16} /> Acerca de Planes Go Code</button>
            {acerca && (
              <div className="acerca">
                <img src="/logo.svg" alt="" width="40" height="40" />
                <div>
                  <b>Planes Go Code</b>
                  <div className="suave pequeno">Versión {act?.version || usuario.version || '—'}</div>
                  <div className="suave pequeno">
                    {act?.estado === 'al-dia' && 'Al día'}
                    {act?.estado === 'desactivada' && 'Sin autoactualización (desarrollo)'}
                    {act?.estado === 'pendiente' && `v${act.nueva} lista para instalar`}
                    {act?.estado === 'descargando' && `Descargando v${act.nueva}`}
                    {act?.estado === 'manual' && act.error}
                    {act?.estado === 'error' && 'No se pudo actualizar: ' + act.error}
                  </div>
                  {usuario.odoo && <div className="suave pequeno recorta">Odoo: {usuario.odoo.replace(/^https?:\/\//, '')}</div>}
                </div>
              </div>
            )}
            <div className="separador" />
            <button role="menuitem" onClick={alSalir}><Icono n="salir" t={16} /> Salir</button>
          </div>
        )}
      </div>
      <span className="marca-barra">Planes <b>Go</b> Code</span>
      <span className="hueco" />
      {/* El reloj de imputación, centrado en la ventana. */}
      <div className="centro-barra"><Imputacion version={versionImputacion} sesion={sesion} /></div>
      {etiqueta && (act!.estado === 'pendiente'
        ? <button className={'chip ' + act!.estado} title={etiqueta.titulo} onClick={aplicar}>{etiqueta.texto}</button>
        : <span className={'chip ' + act!.estado} title={etiqueta.titulo}>{etiqueta.texto}</span>)}
    </header>
  );
}
