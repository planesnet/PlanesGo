import { useEffect, useState, type PointerEvent as EventoPuntero } from 'react';
import type { Proyecto } from '../api';
import Actividad from './Actividad';
import { Galeria } from './Capturas';
import Navegador, { type PedidoWeb } from './Navegador';
import StudioTV from './StudioTV';
import Pruebas from './Pruebas';
import Programas from './Programas';
import Flujo, { type PeticionResolver } from './Flujo';
import Skills from './Skills';
import type { Sesion } from '../sesiones';
import Icono from './Icono';
import { SHOW_EVIDENCE } from '../psf';

type Pestana = 'actividad' | 'flujo' | 'studio' | 'programas' | 'skills' | 'navegador';
type VistaTV = 'pruebas' | 'directo' | 'capturas';

const ANCHO_MIN = 320;

// Panel derecho: TV (pruebas de la campaña y directo) y navegador. Se redimensiona arrastrando su borde izquierdo.
export default function Artefactos({ proyecto, carpeta, agente, sesiones, sesion, directorio, alAncho, pedidoWeb, versionProcesos, alAbrir, alResolver }: {
  proyecto: Proyecto | null; carpeta: string; agente?: string; sesiones: Sesion[]; alAncho: (px: number) => void;
  sesion?: Sesion | null; directorio?: string;
  pedidoWeb: PedidoWeb | null; versionProcesos: number; alAbrir: (url: string) => void; alResolver: (p: PeticionResolver) => void;
}) {
  const [pestana, setPestana] = useState<Pestana>('actividad');
  // Al pedir abrir una URL (programa que arranca, «pgo abrir»), se pasa al navegador.
  useEffect(() => { if (pedidoWeb) setPestana('navegador'); }, [pedidoWeb?.n]); // eslint-disable-line react-hooks/exhaustive-deps
  const [vista, setVista] = useState<VistaTV>(() => {
    try { return (localStorage.getItem('pgo:vista-tv') as VistaTV) || 'pruebas'; } catch { return 'pruebas'; }
  });
  // An automatic PSF session opens the TV on its pictures (evidence); the others, on the view chosen last time.
  const [vistaAuto, setVistaAuto] = useState<VistaTV>('capturas');
  const vistaTV = sesion?.psf ? vistaAuto : vista;
  const cambiaVista = (v: VistaTV) => (sesion?.psf ? setVistaAuto(v) : eligeVista(v));
  useEffect(() => {
    const ver = () => { setPestana('studio'); setVistaAuto('capturas'); };
    window.addEventListener(SHOW_EVIDENCE, ver);
    return () => window.removeEventListener(SHOW_EVIDENCE, ver);
  }, []);
  const [arrastrando, setArrastrando] = useState(false);

  const eligeVista = (v: VistaTV) => { setVista(v); try { localStorage.setItem('pgo:vista-tv', v); } catch { /* sin almacenamiento */ } };

  // Arrastre del borde: ancho = desde el puntero hasta el borde derecho de la ventana, sin aplastar el chat.
  const empieza = (e: EventoPuntero<HTMLDivElement>) => {
    e.preventDefault();
    setArrastrando(true);
    const mueve = (ev: PointerEvent) => {
      const max = Math.max(ANCHO_MIN, window.innerWidth - 248 - 360);
      alAncho(Math.round(Math.min(max, Math.max(ANCHO_MIN, window.innerWidth - ev.clientX))));
    };
    const suelta = () => {
      setArrastrando(false);
      window.removeEventListener('pointermove', mueve);
      window.removeEventListener('pointerup', suelta);
    };
    window.addEventListener('pointermove', mueve);
    window.addEventListener('pointerup', suelta);
  };

  return (
    <aside className="artefactos" aria-label="Artefactos">
      <div className="asa" onPointerDown={empieza} onDoubleClick={() => alAncho(0)} role="separator" aria-orientation="vertical"
        aria-label="Redimensionar el panel" title="Arrastra para cambiar el ancho (doble clic: ancho por defecto)" />
      {/* Durante el arrastre, una capa encima de todo para que el navegador integrado no se quede los eventos. */}
      {arrastrando && <div className="capa-arrastre" />}
      <div className="pestanas" role="tablist">
        <button role="tab" aria-selected={pestana === 'actividad'} onClick={() => setPestana('actividad')} title="Actividad: agente, subagentes, tareas, programas y planes"><Icono n="actividad" t={16} /> Actividad</button>
        <button role="tab" aria-selected={pestana === 'flujo'} onClick={() => setPestana('flujo')} title="Flujo del repositorio: issues y pull requests"><Icono n="flujo" t={16} /> Flujo</button>
        <button role="tab" aria-selected={pestana === 'studio'} onClick={() => setPestana('studio')} title="Studio TV"><Icono n="tv" t={16} /> TV</button>
        <button role="tab" aria-selected={pestana === 'programas'} onClick={() => setPestana('programas')} title="Programas en local"><Icono n="terminal" t={16} /> Programas</button>
        <button role="tab" aria-selected={pestana === 'skills'} onClick={() => setPestana('skills')} title="Skills instaladas y su uso"><Icono n="skill" t={16} /> Skills</button>
        <button role="tab" aria-selected={pestana === 'navegador'} onClick={() => setPestana('navegador')} title="Navegador"><Icono n="web" t={16} /> Web</button>
      </div>
      <div className="contenido-artefacto">
        {!proyecto
          ? <p className="suave relleno">Elige un proyecto.</p>
          : <>
            {/* The TV views, always visible inside it (in the tab bar they were cut off on a narrow panel). */}
            {pestana === 'studio' && (
              <div className="subpestanas tv-vistas" role="tablist" aria-label="Vistas de Studio TV">
                <button role="tab" aria-selected={vistaTV === 'capturas'} onClick={() => cambiaVista('capturas')} title="Fotos y capturas de evidencia de la sesión (agentes y sesiones automáticas PSF)">Capturas</button>
                <button role="tab" aria-selected={vistaTV === 'pruebas'} onClick={() => cambiaVista('pruebas')}>Pruebas</button>
                <button role="tab" aria-selected={vistaTV === 'directo'} onClick={() => cambiaVista('directo')}>Directo</button>
              </div>
            )}
            {pestana === 'studio' && (vistaTV === 'pruebas'
              ? <Pruebas key={proyecto.id} proyecto={proyecto} alVerDirecto={() => cambiaVista('directo')} />
              : vistaTV === 'capturas' ? <Galeria key={proyecto.id + ':' + (sesion?.id || '')} proyecto={proyecto.id} session={sesion?.id} />
                : <StudioTV key={proyecto.id} proyecto={proyecto} />)}
            {pestana === 'actividad' && <Actividad sesiones={sesiones} key={proyecto.id} proyecto={proyecto} carpeta={carpeta} sesion={sesion} directorio={directorio} versionProcesos={versionProcesos} alAbrir={alAbrir} />}
            {pestana === 'flujo' && <Flujo key={proyecto.id} proyecto={proyecto} carpeta={carpeta} alAbrir={alAbrir} alResolver={alResolver} />}
            {pestana === 'skills' && <Skills key={proyecto.id} proyecto={proyecto} carpeta={carpeta} agente={agente} sesiones={sesiones} />}
            {pestana === 'programas' && <Programas key={proyecto.id} proyecto={proyecto} carpeta={carpeta} version={versionProcesos} alAbrir={alAbrir} />}
            {/* El navegador sigue montado al cambiar de pestaña: no se pierden sus pestañas ni lo que cargaban. */}
            <div className="capa-navegador" hidden={pestana !== 'navegador'}>
              <Navegador key={proyecto.id} proyecto={proyecto} pedido={pedidoWeb} />
            </div>
          </>}
      </div>
    </aside>
  );
}
