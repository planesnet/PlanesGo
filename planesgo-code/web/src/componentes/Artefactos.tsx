import { useState } from 'react';
import type { Proyecto } from '../api';
import Navegador from './Navegador';
import StudioTV from './StudioTV';
import Icono from './Icono';

type Pestana = 'studio' | 'navegador';

export default function Artefactos({ proyecto }: { proyecto: Proyecto | null }) {
  const [pestana, setPestana] = useState<Pestana>('studio');
  return (
    <aside className="artefactos" aria-label="Artefactos">
      <div className="pestanas" role="tablist">
        <button role="tab" aria-selected={pestana === 'studio'} onClick={() => setPestana('studio')} title="Studio TV"><Icono n="tv" t={16} /> TV</button>
        <button role="tab" aria-selected={pestana === 'navegador'} onClick={() => setPestana('navegador')} title="Navegador"><Icono n="web" t={16} /> Web</button>
      </div>
      <div className="contenido-artefacto">
        {!proyecto
          ? <p className="suave relleno">Elige un proyecto.</p>
          : pestana === 'studio'
            ? <StudioTV key={proyecto.id} proyecto={proyecto} />
            : <Navegador key={proyecto.id} proyecto={proyecto} />}
      </div>
    </aside>
  );
}
