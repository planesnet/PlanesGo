// Iconos de marca: tipo de hosting del proyecto y agente/proveedor del chat (simplificados, en sus colores).
import Icono from './Icono';

export function Hosting({ tipo, t = 16, enBlanco = false }: { tipo?: string; t?: number; enBlanco?: boolean }) {
  if (tipo === 'odoo.sh') return (
    <svg viewBox="0 0 24 24" width={t} height={t} aria-label="Odoo.sh" role="img">
      <circle cx="12" cy="12" r="10" fill="#714B67" /><circle cx="12" cy="12" r="4.2" fill="none" stroke="#fff" strokeWidth="2.6" />
    </svg>);
  if (tipo === 'autopyme.com') return (
    <svg viewBox="0 0 24 24" width={t} height={t} aria-label="AutoPyme" role="img">
      <rect x="2" y="2" width="20" height="20" rx="6" fill="#0a6fc9" /><path d="M7 17l5-11 5 11M9 13h6" fill="none" stroke="#fff" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>);
  if (tipo === 'gcloud') return (
    <svg viewBox="0 0 24 24" width={t} height={t} aria-label="Google Cloud" role="img">
      <path d="M7 18a4.5 4.5 0 0 1-.6-9 6 6 0 0 1 11.4 1.6A3.8 3.8 0 0 1 17.2 18z" fill="none" stroke="#4285f4" strokeWidth="2.2" strokeLinejoin="round" />
      <path d="M9.5 18h7.7" stroke="#34a853" strokeWidth="2.2" strokeLinecap="round" /><path d="M6.4 9a6 6 0 0 1 4-4" stroke="#ea4335" strokeWidth="2.2" strokeLinecap="round" fill="none" />
      <path d="M17.8 10.6a3.8 3.8 0 0 1 2.9 3.6" stroke="#fbbc04" strokeWidth="2.2" strokeLinecap="round" fill="none" />
    </svg>);
  if (tipo === 'servidor') return (
    <svg viewBox="0 0 24 24" width={t} height={t} aria-label="Servidor propio" role="img">
      <rect x="3" y="3.5" width="18" height="7" rx="2" fill="#3d5a73" /><rect x="3" y="13.5" width="18" height="7" rx="2" fill="#3d5a73" />
      <circle cx="7" cy="7" r="1.2" fill="#7ee08a" /><circle cx="7" cy="17" r="1.2" fill="#7ee08a" /><path d="M11 7h7M11 17h7" stroke="#fff" strokeWidth="1.4" strokeLinecap="round" />
    </svg>);
  if (tipo === 'sage') return (
    <svg viewBox="0 0 24 24" width={t} height={t} aria-label="Sage" role="img">
      <rect x="2" y="2" width="20" height="20" rx="6" fill="#00a443" /><path d="M15.5 8.2c-.8-.9-2-1.4-3.4-1.4-1.9 0-3.2 1-3.2 2.4 0 3.4 6.8 1.8 6.8 5.4 0 1.5-1.4 2.6-3.5 2.6-1.5 0-2.8-.6-3.7-1.6" fill="none" stroke="#fff" strokeWidth="2" strokeLinecap="round" />
    </svg>);
  // Local: lo habitual. En las listas va en blanco; en la cabecera, un ordenador.
  return enBlanco ? <span style={{ width: t, height: t, display: 'inline-block' }} aria-label="Local" /> : <span aria-label="Local"><Icono n="equipo" t={t} /></span>;
}

export const nombreHosting = (tipo?: string) => ({ 'odoo.sh': 'Odoo.sh', 'autopyme.com': 'AutoPyme', gcloud: 'Google Cloud', servidor: 'Servidor propio', sage: 'Servidor Sage' } as Record<string, string>)[tipo || ''] || 'Local';

export function Agente({ id, t = 18 }: { id?: string; t?: number }) {
  const v = { viewBox: '0 0 24 24', width: t, height: t, role: 'img' as const };
  switch (id) {
    case 'claude-code':
    case 'claude':
      return (<svg {...v} aria-label="Claude"><g stroke="#d97757" strokeWidth="2.6" strokeLinecap="round">
        {[0, 30, 60, 90, 120, 150].map(a => <line key={a} x1="12" y1="3.5" x2="12" y2="20.5" transform={`rotate(${a} 12 12)`} />)}</g></svg>);
    case 'gemini-cli':
    case 'gemini':
      return (<svg {...v} aria-label="Gemini"><path d="M12 2c.6 5 4.9 9.4 10 10-5.1.6-9.4 5-10 10-.6-5-4.9-9.4-10-10 5.1-.6 9.4-5 10-10z" fill="#4e86f6" /></svg>);
    case 'openai':
      return (<svg {...v} aria-label="ChatGPT"><circle cx="12" cy="12" r="9" fill="none" stroke="#10a37f" strokeWidth="2.4" /><path d="M8 12h8M12 8v8" stroke="#10a37f" strokeWidth="2.4" strokeLinecap="round" /></svg>);
    case 'opencode':
      return (<svg {...v} aria-label="OpenCode"><rect x="3" y="4" width="18" height="16" rx="3" fill="#1c1c1a" /><path d="M8 10l3 2-3 2M13 15h3" fill="none" stroke="#fff" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" /></svg>);
    case 'antigravity':
      return (<svg {...v} aria-label="Antigravity"><path d="M12 3l8 17H4z" fill="none" stroke="#7b61ff" strokeWidth="2.2" strokeLinejoin="round" /><circle cx="12" cy="14" r="2" fill="#7b61ff" /></svg>);
    default:
      return <Icono n="web" t={t} />;
  }
}
