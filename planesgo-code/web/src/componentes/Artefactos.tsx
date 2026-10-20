import { useState } from 'react';
import type { Proyecto } from '../api';
import Navegador from './Navegador';
import StudioTV from './StudioTV';

type Pestana = 'studio' | 'navegador';

export default function Artefactos({ proyecto }: { proyecto: Proyecto | null }) {
  const [pestana, setPestana] = useState<Pestana>('studio');
  return (
    <aside className="artefactos" aria-label="Artefactos">
      <div className="pestanas" role="tablist">
        <button role="tab" aria-selected={pestana === 'studio'} onClick={() => setPestana('studio')}>Studio TV</button>
        <button role="tab" aria-selected={pestana === 'navegador'} onClick={() => setPestana('navegador')}>Navegador</button>
      </div>
      <div className="contenido-artefacto">
        {!proyecto
          ? <p className="suave relleno">Los artefactos trabajan dentro de un proyecto de Odoo: elige uno.</p>
          : pestana === 'studio'
            ? <StudioTV key={proyecto.id} proyecto={proyecto} />
            : <Navegador key={proyecto.id} proyecto={proyecto} />}
      </div>
    </aside>
  );
}
