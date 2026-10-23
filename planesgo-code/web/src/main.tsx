import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
import ErrorBoundary from './componentes/ErrorBoundary';
import './estilos.css';
import { post } from './api';

// Errores de la interfaz al registro del backend (depuracion.log), para poder evaluar fallos que el usuario no ve.
let enviados = 0;
const informa = (mensaje: string, pila?: string) => {
  if (enviados++ >= 20) return; // como mucho 20 por carga
  post('/api/diagnostico', { mensaje: String(mensaje).slice(0, 500), pila: (pila || '').slice(0, 2000), url: location.pathname + location.search }).catch(() => {});
};
window.addEventListener('error', e => informa(e.message, e.error?.stack));
window.addEventListener('unhandledrejection', e => { const r = e.reason as Error | undefined; informa(r?.message || String(e.reason), r?.stack); });

createRoot(document.getElementById('raiz')!).render(<StrictMode><ErrorBoundary area="Planes Go Code" className="pantalla"><App /></ErrorBoundary></StrictMode>);
