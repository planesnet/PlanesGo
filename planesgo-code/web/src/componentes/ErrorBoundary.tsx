import { Component, type ErrorInfo, type ReactNode } from 'react';
import { post } from '../api';

// ErrorBoundary: si un componente falla al pintarse, el error se queda en su sitio («Volver a cargar» lo intenta de
// nuevo) en vez de dejar en blanco la interfaz entera. El error va al registro del backend (depuracion.log).
// resetKey: si cambia (p. ej. otra sesión), se vuelve a intentar sin pulsar nada.
export default class ErrorBoundary extends Component<{ children: ReactNode; area: string; className?: string; resetKey?: string }, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) { return { error }; }

  componentDidUpdate(prev: { resetKey?: string }) {
    if (this.state.error && prev.resetKey !== this.props.resetKey) this.setState({ error: null });
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    post('/api/diagnostico', { mensaje: `${this.props.area}: ${error.message}`.slice(0, 500), pila: ((error.stack || '') + (info.componentStack || '')).slice(0, 2000),
      url: location.pathname + location.search }).catch(() => {});
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    return (
      <div className={'error-limite ' + (this.props.className || '')} role="alert">
        <b>No se ha podido mostrar {this.props.area}.</b>
        <code className="recorta" title={error.stack}>{error.message}</code>
        <button type="button" className="boton-secundario" onClick={() => this.setState({ error: null })}>Volver a cargar</button>
      </div>
    );
  }
}
