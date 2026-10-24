import type { ReactNode } from 'react';
import type { Usuario } from '../api';
import Icono from './Icono';
import ErrorReportSettings from './ErrorReportSettings';
import LocalAISettings from './LocalAISettings';
import { PropertiesModal, type PropertiesSection } from './Modal';

// Application properties (logo menu): account, updates, local AI, error reports and about.

export interface UpdateStatus { version: string; estado: string; nueva?: string; error?: string; revisado?: string; ocupado?: boolean; trabajando?: string; pausada?: boolean } // trabajando: qué hay en marcha (entonces no se actualiza)

export type AppSection = 'account' | 'updates' | 'localai' | 'errors' | 'about';
const APP_SECTIONS: readonly PropertiesSection<AppSection>[] = [{ id: 'account', name: 'Cuenta', icon: 'equipo' },
  { id: 'updates', name: 'Actualizaciones', icon: 'recargar' }, { id: 'localai', name: 'IA local', icon: 'ordenador' },
  { id: 'errors', name: 'Errores', icon: 'warning' },   { id: 'about', name: 'Acerca de', icon: 'info' }];

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

// checkedText: cuándo se buscó la última vez («nunca» si no consta: antes salía la fecha cero, «31/12/01»).
export function checkedText(revisado?: string): string {
  const d = revisado ? new Date(revisado) : null;
  if (!d || isNaN(d.getTime()) || d.getFullYear() < 2000) return 'nunca';
  return d.toLocaleString('es-ES', { dateStyle: 'short', timeStyle: 'short' });
}

export default function AppProperties({ user, avatar, status, initial, checkResult, onClose, onCheck, onTogglePause, onApply, onLogout }: {
  user: Usuario; avatar: ReactNode; status: UpdateStatus | null; initial?: AppSection; onClose: () => void;
  checkResult?: string; // lo que ha dado «Buscar actualizaciones» (buscando…, al día, o por qué no se puede)
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
            <div><dt>Última comprobación</dt><dd>{checkedText(status?.revisado)}</dd></div>
          </dl>
          <div className="fila-config app-actions">
            <button type="button" className="boton-secundario" onClick={onCheck}><Icono n="recargar" t={13} /> Buscar actualizaciones</button>
            {status?.estado === 'pendiente' && <button type="button" className="primario" onClick={onApply}>Actualizar a v{status.nueva}</button>}
          </div>
          {checkResult && <p className="suave pequeno app-check" role="status">{checkResult}</p>}
          {status && status.estado !== 'desactivada' && <>
            <label className="casilla"><input type="checkbox" checked={!status.pausada} onChange={onTogglePause} /> Actualizar automáticamente</label>
            <span className="suave pequeno">La versión nueva se instala sola cuando llevas un rato sin usar la aplicación, sin agentes ni programas en marcha.</span>
          </>}
        </>}

        {section === 'localai' && <LocalAISettings />}

        {section === 'errors' && <ErrorReportSettings />}

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
