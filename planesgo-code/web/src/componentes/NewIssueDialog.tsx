import { useEffect, useState } from 'react';
import { get, type Proyecto } from '../api';
import { NuevaIssue, type Estado } from './Flujo';
import { Modal } from './Modal';
import { PSF_CHECK } from '../psf';

// Window of the «/issue [title]» command: creates a GitHub issue in the repository of the current project (the same
// form as Flujo, with the repository labels). It only creates it: with the project's automatic resolution on,
// pgocode picks it up from GitHub on its next check; otherwise a person starts it from Flujo («Resolver»).

export default function NewIssueDialog({ project, folder, initialTitle, autoResolve, onClose }: {
  project: Proyecto; folder: string; initialTitle: string; autoResolve: boolean; onClose: () => void;
}) {
  const [state, setState] = useState<Estado | null>(null);
  const [error, setError] = useState('');
  const [created, setCreated] = useState<{ numero: number; url: string; titulo: string } | null>(null);

  useEffect(() => {
    if (!folder) { setError('La sesión no tiene carpeta del proyecto: elige una en sus propiedades.'); return; }
    get<Estado>(`/api/github?proyecto=${project.id}&carpeta=${encodeURIComponent(folder)}`).then(setState).catch(e => setError((e as Error).message));
  }, [project.id, folder]);

  const openOutside = (url: string) => {
    if (window.psf) window.psf.abrirFuera(url).catch(() => {});
    else window.open(url, '_blank', 'noopener');
  };
  return (
    <Modal title="Nueva issue" label="Crear una issue en GitHub" className="new-issue-dialog" onClose={onClose}>
      {error ? <p className="error">{error}</p>
        : !state ? <p className="suave">Leyendo el repositorio…</p>
        : created ? (
          <div className="new-issue-done">
            <p>Issue <button className="enlace" onClick={() => openOutside(created.url)}>#{created.numero}</button> creada en <b>{state.repo}</b>.</p>
            <p className="suave pequeno">{autoResolve
              ? 'La resolución automática del proyecto está activada: pgocode la recoge de GitHub ahora mismo y arranca su sesión automática (si ya hay otra trabajando en el proyecto, cuando acabe).'
              : 'Para resolverla con una sesión automática PSF, pulsa «Resolver» en la pestaña Flujo (o activa la resolución automática en las propiedades del proyecto, sección Issues).'}</p>
            <div className="fila-config"><span className="hueco" /><button className="primario" onClick={onClose}>Cerrar</button></div>
          </div>
        ) : (
          <NuevaIssue estado={state} proyecto={project.id} carpeta={folder} initialTitle={initialTitle} className="gh-nueva new-issue-form"
            alCancelar={onClose} alCrear={r => { setCreated(r); if (autoResolve) window.dispatchEvent(new Event(PSF_CHECK)); }} />
        )}
    </Modal>
  );
}
