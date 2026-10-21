import { useEffect, useState, type PointerEvent as EventoPuntero } from 'react';
import type { Proyecto } from '../api';
import { Galeria } from './Capturas';
import Navegador, { type PedidoWeb } from './Navegador';
import StudioTV from './StudioTV';
import Pruebas from './Pruebas';
import Programas from './Programas';
import Flujo, { type PeticionResolver } from './Flujo';
import Skills from './Skills';
import type { Sesion } from '../sesiones';
import Icono from './Icono';

type Pestana = 'flujo' | 'studio' | 'programas' | 'skills' | 'navegador';
type VistaTV = 'pruebas' | 'directo' | 'capturas';

const ANCHO_MIN = 320;

// Panel derecho: TV (pruebas de la campaña y directo) y navegador. Se redimensiona arrastrando su borde izquierdo.
export default function Artefactos({ proyecto, carpeta, rama, agente, sesiones, alAncho, pedidoWeb, versionProcesos, alAbrir, alResolver }: {
  proyecto: Proyecto | null; carpeta: string; rama: string; agente?: string; sesiones: Sesion[]; alAncho: (px: number) => void;
  pedidoWeb: PedidoWeb | null; versionProcesos: number; alAbrir: (url: string) => void; alResolver: (p: PeticionResolver) => void;
}) {
  const [pestana, setPestana] = useState<Pestana>('flujo');
  // Al pedir abrir una URL (programa que arranca, «pgo abrir»), se pasa al navegador.
  useEffect(() => { if (pedidoWeb) setPestana('navegador'); }, [pedidoWeb?.n]); // eslint-disable-line react-hooks/exhaustive-deps
  const [vista, setVista] = useState<VistaTV>(() => {
    try { return (localStorage.getItem('pgo:vista-tv') as VistaTV) || 'pruebas'; } catch { return 'pruebas'; }
  });
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
        <button role="tab" aria-selected={pestana === 'flujo'} onClick={() => setPestana('flujo')} title="Flujo del repositorio: issues y pull requests"><Icono n="flujo" t={16} /> Flujo</button>
        <button role="tab" aria-selected={pestana === 'studio'} onClick={() => setPestana('studio')} title="Studio TV"><Icono n="tv" t={16} /> TV</button>
        <button role="tab" aria-selected={pestana === 'programas'} onClick={() => setPestana('programas')} title="Programas en local"><Icono n="terminal" t={16} /> Programas</button>
        <button role="tab" aria-selected={pestana === 'skills'} onClick={() => setPestana('skills')} title="Skills instaladas y su uso"><Icono n="skill" t={16} /> Skills</button>
        <button role="tab" aria-selected={pestana === 'navegador'} onClick={() => setPestana('navegador')} title="Navegador"><Icono n="web" t={16} /> Web</button>
        {pestana === 'studio' && proyecto && (
          <span className="subpestanas">
            <button aria-pressed={vista === 'pruebas'} onClick={() => eligeVista('pruebas')}>Pruebas</button>
            <button aria-pressed={vista === 'directo'} onClick={() => eligeVista('directo')}>Directo</button>
            <button aria-pressed={vista === 'capturas'} onClick={() => eligeVista('capturas')} title="Capturas de evidencia que devuelven los agentes al probar">Capturas</button>
          </span>
        )}
      </div>
      <div className="contenido-artefacto">
        {!proyecto
          ? <p className="suave relleno">Elige un proyecto.</p>
          : <>
            {pestana === 'studio' && (vista === 'pruebas'
              ? <Pruebas key={proyecto.id} proyecto={proyecto} alVerDirecto={() => eligeVista('directo')} />
              : vista === 'capturas' ? <Galeria key={proyecto.id} proyecto={proyecto.id} />
                : <StudioTV key={proyecto.id} proyecto={proyecto} />)}
            {pestana === 'flujo' && <Flujo key={proyecto.id} proyecto={proyecto} carpeta={carpeta} rama={rama} alAbrir={alAbrir} alResolver={alResolver} />}
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
