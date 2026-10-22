import type { ReactNode } from 'react';
import type { Usuario } from '../api';
import Icono from './Icono';
import { PropertiesModal, type PropertiesSection } from './Modal';

// Application properties (logo menu): account, updates and about.

export interface UpdateStatus { version: string; estado: string; nueva?: string; error?: string; revisado?: string; ocupado?: boolean; pausada?: boolean }

export type AppSection = 'account' | 'updates' | 'about';
const APP_SECTIONS: readonly PropertiesSection<AppSection>[] = [{ id: 'account', name: 'Cuenta', icon: 'equipo' },
  { id: 'updates', name: 'Actualizaciones', icon: 'recargar' }, { id: 'about', name: 'Acerca de', icon: 'info' }];

// Update state as shown to the user.
export function updateText(s: UpdateStatus | null): string {
  switch (s?.estado) {
    case 'al-dia': return 'Al día';
    case 'desactivada': return 'Sin autoactualización (desarrollo)';
    case 'pendiente': return `v${s.nueva} lista para instalar`;
    case 'descargando': return `Descargando v${s.nueva}`;
    case 'aplicando': return 'Actualizando…';
    case 'manual': return s.error || `v${s.nueva}: hay que reinstalar`;
    case 'error': return 'No se pudo actualizar: ' + s.error;
    default: return '—';
  }
}

export default function AppProperties({ user, avatar, status, initial, onClose, onCheck, onTogglePause, onApply, onLogout }: {
  user: Usuario; avatar: ReactNode; status: UpdateStatus | null; initial?: AppSection; onClose: () => void;
  onCheck: () => void; onTogglePause: () => void; onApply: () => void; onLogout: () => void;
}) {
  const version = status?.version || user.version || '—';
  return (
    <PropertiesModal title={<><span className="suave">Propiedades ·</span> Planes Go Code</>} label="Propiedades de Planes Go Code"
      sections={APP_SECTIONS} initial={initial} onClose={onClose}>
      {section => <>
        {section === 'account' && <>
          <h3>Cuenta</h3>
          <div className="account-card">
            {avatar}
            <div className="recorta"><b className="recorta">{user.nombre}</b><span className="suave pequeno recorta">{user.login}</span></div>
          </div>
          <dl className="session-ids">
            <div><dt>Usuario</dt><dd><code className="recorta">{user.login}</code></dd></div>
            {user.odoo && <div><dt>Odoo</dt><dd><code className="recorta">{user.odoo.replace(/^https?:\/\//, '')}</code></dd></div>}
          </dl>
          <div><button type="button" className="boton-secundario" onClick={onLogout}><Icono n="salir" t={13} /> Salir</button></div>
        </>}

        {section === 'updates' && <>
          <h3>Actualizaciones</h3>
          <dl className="session-ids">
            <div><dt>Versión instalada</dt><dd>{version}</dd></div>
            <div><dt>Estado</dt><dd>{updateText(status)}</dd></div>
            {status?.revisado && <div><dt>Última comprobación</dt><dd>{new Date(status.revisado).toLocaleString('es-ES', { dateStyle: 'short', timeStyle: 'short' })}</dd></div>}
          </dl>
          <div className="fila-config app-actions">
            <button type="button" className="boton-secundario" onClick={onCheck}><Icono n="recargar" t={13} /> Buscar actualizaciones</button>
            {status?.estado === 'pendiente' && <button type="button" className="primario" onClick={onApply}>Actualizar a v{status.nueva}</button>}
          </div>
          {status && status.estado !== 'desactivada' && <>
            <label className="casilla"><input type="checkbox" checked={!status.pausada} onChange={onTogglePause} /> Actualizar automáticamente</label>
            <span className="suave pequeno">La versión nueva se instala sola cuando llevas un rato sin usar la aplicación, sin agentes ni programas en marcha.</span>
          </>}
        </>}

        {section === 'about' && <>
          <h3>Acerca de</h3>
          <div className="acerca">
            <img src="/logo.svg" alt="" width="48" height="48" />
            <div>
              <b>Planes Go Code</b>
              <div className="suave pequeno">Versión {version}</div>
              <div className="suave pequeno">Interfaz común sobre los agentes de programación, integrada con Odoo y PlanesGo.</div>
            </div>
          </div>
        </>}
      </>}
    </PropertiesModal>
  );
}
