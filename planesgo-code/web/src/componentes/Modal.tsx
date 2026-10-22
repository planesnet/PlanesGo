import { useEffect, useId, useRef, useState, useSyncExternalStore, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import Icono from './Icono';

// pgocode windows. Every one is built on <Modal> (portal on document.body, backdrop that closes on click, Escape,
// header with title and ×, optional footer); the others compose it:
//  - <PropertiesModal>: section list on the left, the chosen section's properties on the right
//    (session, project and application properties).
//  - <MessageDialog>: a text and «Aceptar», with a level (info, warning, error).
//  - <ConfirmDialog>: a text with «Aceptar» and «Cancelar».
// showMessage() and askConfirm() open the last two from anywhere and return a promise (like alert/confirm, but
// with the app's look); <Dialogs /> renders them and is mounted once in App.

// Open modals, topmost last: Escape only closes the one on top (e.g. a confirmation over the session window).
const stack: string[] = [];

export function Modal({ title, label, className, onClose, footer, children, backdropClass }: {
  title: ReactNode; label?: string; className?: string; onClose: () => void; footer?: ReactNode; children: ReactNode; backdropClass?: string;
}) {
  const id = useId();
  const close = useRef(onClose);
  close.current = onClose;
  useEffect(() => {
    stack.push(id);
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape' && stack[stack.length - 1] === id) { e.stopPropagation(); close.current(); } };
    document.addEventListener('keydown', onKey);
    return () => { document.removeEventListener('keydown', onKey); stack.splice(stack.indexOf(id), 1); };
  }, [id]);
  return createPortal(
    <div className={'fondo-modal' + (backdropClass ? ' ' + backdropClass : '')} onMouseDown={e => { if (e.target === e.currentTarget) onClose(); }}>
      <div className={'modal' + (className ? ' ' + className : '')} role="dialog" aria-modal="true" aria-label={label || (typeof title === 'string' ? title : undefined)}>
        <div className="modal-cabecera">
          <h2 className="recorta">{title}</h2>
          <button className="icono" onClick={onClose} title="Cerrar" aria-label="Cerrar"><Icono n="cerrar" /></button>
        </div>
        {children}
        {footer && <div className="modal-footer">{footer}</div>}
      </div>
    </div>,
    document.body,
  );
}

export interface PropertiesSection<T extends string> { id: T; name: string; icon: Parameters<typeof Icono>[0]['n']; incomplete?: boolean }

// Properties window: sections grouped by topic on the left; on the right, the properties of the selected one
// (children receives it). An «incomplete» section (missing a required value) shows a dot, visible from any other.
export function PropertiesModal<T extends string>({ title, label, sections, initial, onClose, footer, children, className }: {
  title: ReactNode; label?: string; sections: readonly PropertiesSection<T>[]; initial?: T; onClose: () => void;
  footer?: ReactNode; children: (section: T) => ReactNode; className?: string;
}) {
  const [section, setSection] = useState<T>(initial ?? sections[0].id);
  return (
    <Modal title={title} label={label} className={'propiedades' + (footer ? ' with-footer' : '') + (className ? ' ' + className : '')} onClose={onClose} footer={footer}>
      <div className="propiedades-cuerpo">
        <nav className="propiedades-secciones" aria-label="Secciones">
          {sections.map(x => (
            <button key={x.id} aria-current={section === x.id} onClick={() => setSection(x.id)}>
              <Icono n={x.icon} t={15} /> {x.name}{x.incomplete && <span className="incomplete-dot" title="Falta un dato obligatorio" />}
            </button>
          ))}
        </nav>
        <section className="propiedades-detalle">{children(section)}</section>
      </div>
    </Modal>
  );
}

export type Level = 'info' | 'warning' | 'error';
const LEVELS: Record<Level, { title: string; icon: 'info' | 'warning' | 'error' }> = {
  info: { title: 'Información', icon: 'info' },
  warning: { title: 'Aviso', icon: 'warning' },
  error: { title: 'Error', icon: 'error' },
};

function DialogBody({ text, level }: { text: ReactNode; level?: Level }) {
  return (
    <div className={'dialog-body' + (level ? ' ' + level : '')}>
      {level && <span className="dialog-icon"><Icono n={LEVELS[level].icon} t={22} /></span>}
      <div className="dialog-text">{text}</div>
    </div>
  );
}

// Message with «Aceptar». Without a level, plain text; with one, its icon and colour.
export function MessageDialog({ text, level, title, onClose }: { text: ReactNode; level?: Level; title?: string; onClose: () => void }) {
  return (
    <Modal title={title || (level ? LEVELS[level].title : 'Planes Go Code')} className="dialog" backdropClass="on-top" onClose={onClose}
      footer={<><span className="hueco" /><button className="primario" onClick={onClose} autoFocus>Aceptar</button></>}>
      <DialogBody text={text} level={level} />
    </Modal>
  );
}

// Confirmation with «Aceptar» and «Cancelar» (Escape or clicking outside cancels). «danger»: cannot be undone.
export function ConfirmDialog({ text, level, title, okText = 'Aceptar', cancelText = 'Cancelar', danger, onAnswer }: {
  text: ReactNode; level?: Level; title?: string; okText?: string; cancelText?: string; danger?: boolean; onAnswer: (ok: boolean) => void;
}) {
  return (
    <Modal title={title || 'Confirmar'} className="dialog" backdropClass="on-top" onClose={() => onAnswer(false)}
      footer={<><span className="hueco" /><button className="enlace" onClick={() => onAnswer(false)}>{cancelText}</button>
        <button className={'primario' + (danger ? ' danger' : '')} onClick={() => onAnswer(true)} autoFocus>{okText}</button></>}>
      <DialogBody text={text} level={level ?? (danger ? 'warning' : undefined)} />
    </Modal>
  );
}

// showMessage() and askConfirm(): dialogs from anywhere, queued (one after another).
type MessageOptions = { level?: Level; title?: string };
type ConfirmOptions = MessageOptions & { okText?: string; cancelText?: string; danger?: boolean };
type Pending = { n: number; text: ReactNode } & (
  | ({ kind: 'message'; done: () => void } & MessageOptions)
  | ({ kind: 'confirm'; done: (ok: boolean) => void } & ConfirmOptions));
type DistributiveOmit<T, K extends keyof T> = T extends unknown ? Omit<T, K> : never;

let queue: Pending[] = [];
let counter = 0;
const listeners = new Set<() => void>();
const push = (d: DistributiveOmit<Pending, 'n'>) => { queue = [...queue, { ...d, n: ++counter } as Pending]; listeners.forEach(f => f()); };
const shift = () => { queue = queue.slice(1); listeners.forEach(f => f()); };

export function showMessage(text: ReactNode, options: MessageOptions = {}): Promise<void> {
  return new Promise(done => push({ kind: 'message', text, ...options, done }));
}

export function askConfirm(text: ReactNode, options: ConfirmOptions = {}): Promise<boolean> {
  return new Promise(done => push({ kind: 'confirm', text, ...options, done }));
}

export function Dialogs() {
  const d = useSyncExternalStore(f => { listeners.add(f); return () => { listeners.delete(f); }; }, () => queue[0]);
  if (!d) return null;
  if (d.kind === 'message') return <MessageDialog key={d.n} text={d.text} level={d.level} title={d.title} onClose={() => { shift(); d.done(); }} />;
  return <ConfirmDialog key={d.n} text={d.text} level={d.level} title={d.title} okText={d.okText} cancelText={d.cancelText} danger={d.danger}
    onAnswer={ok => { shift(); d.done(ok); }} />;
}
