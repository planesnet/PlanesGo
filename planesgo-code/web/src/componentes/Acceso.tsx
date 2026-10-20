import { useState, type FormEvent } from 'react';
import { post, type Usuario } from '../api';

export default function Acceso({ aviso, alEntrar }: { aviso: string; alEntrar: (u: Usuario) => void }) {
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [enviando, setEnviando] = useState(false);

  async function entrar(e: FormEvent) {
    e.preventDefault();
    setEnviando(true);
    setError('');
    try {
      alEntrar(await post<Usuario>('/api/login', { login, password }));
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setEnviando(false);
      setPassword('');
    }
  }

  return (
    <main className="acceso">
      <form onSubmit={entrar} className="tarjeta-acceso">
        <h1>PSF<span>Code</span></h1>
        <p className="suave">Entra con tu usuario de Odoo de PlanesNet.</p>
        {aviso && !error && <p className="aviso">{aviso}</p>}
        <label>Usuario<input autoFocus autoComplete="username" value={login} onChange={e => setLogin(e.target.value)} required /></label>
        <label>Contraseña<input type="password" autoComplete="current-password" value={password} onChange={e => setPassword(e.target.value)} required /></label>
        {error && <p className="error" role="alert">{error}</p>}
        <button className="primario" disabled={enviando}>{enviando ? 'Entrando…' : 'Entrar'}</button>
      </form>
    </main>
  );
}
