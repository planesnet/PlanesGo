import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
import EscritorioRemoto from './componentes/EscritorioRemoto';
import ErrorBoundary from './componentes/ErrorBoundary';
import './estilos.css';
import { ErrorAPI, post } from './api';

// Errores de la interfaz al registro del backend (depuracion.log), para poder evaluar fallos que el usuario no ve; los
// fallos de pgocode acaban además en una issue de GitHub (internal/bugreport). Los errores de la API no: los explica
// la propia pantalla y su causa suele estar fuera (Odoo, GitHub, la red).
let enviados = 0;
const informa = (mensaje: string, pila?: string) => {
  if (enviados++ >= 20) return; // como mucho 20 por carga
  post('/api/diagnostico', { mensaje: String(mensaje).slice(0, 500), pila: (pila || '').slice(0, 2000), url: location.pathname + location.search }).catch(() => {});
};
window.addEventListener('error', e => informa(e.message, e.error?.stack));
window.addEventListener('unhandledrejection', e => { const r = e.reason as Error | undefined; if (!(r instanceof ErrorAPI)) informa(r?.message || String(e.reason), r?.stack); });

// pgocode --rdp: la ventana del escritorio remoto (sin la app entera: sus tareas de fondo irían dos veces).
const modoRdp = new URLSearchParams(location.search).get('modo') === 'rdp';
createRoot(document.getElementById('raiz')!).render(<StrictMode><ErrorBoundary area="Planes Go Code" className="pantalla">{modoRdp ? <EscritorioRemoto /> : <App />}</ErrorBoundary></StrictMode>);
