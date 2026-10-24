import type { ReactNode } from 'react';

// Switch: an on/off control (role «switch»), with its label beside it. Keyboard: Space or Enter (it is a button).
export default function Switch({ on, onChange, disabled, children, title }: {
  on: boolean; onChange: (on: boolean) => void; disabled?: boolean; children: ReactNode; title?: string;
}) {
  return (
    <button type="button" role="switch" aria-checked={on} className="switch" disabled={disabled} title={title} onClick={() => onChange(!on)}>
      <span className="switch-track" aria-hidden="true"><span className="switch-thumb" /></span>
      <span className="switch-label">{children}</span>
    </button>
  );
}
