// The project of a remote desktop (RDP or AnyDesk), mandatory: the chosen one with «Cambiar», or the project finder
// of the projects bar.
import { useState } from 'react';
import type { Proyecto } from '../api';
import { BuscaProyecto } from './Proyectos';

export default function ProjectField({ login, projects, value, onChange, label = 'Proyecto' }: {
  login: string; projects: Proyecto[]; value: number; onChange: (p: Proyecto) => void; label?: string;
}) {
  const [changing, setChanging] = useState(false);
  const chosen = projects.find(p => p.id === value);
  return (
    <div className="rdp-proyecto">
      <span className="rdp-etiqueta">{label}</span>
      {chosen && !changing
        ? <p className="rdp-elegido"><b>{chosen.nombre}</b><button type="button" className="enlace" onClick={() => setChanging(true)}>Cambiar</button></p>
        : <BuscaProyecto login={login} lista={projects} elegido={value} alElegir={p => { onChange(p); setChanging(false); }} />}
    </div>
  );
}
