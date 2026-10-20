import { useState, type FormEvent } from 'react';
import { post, type Usuario } from '../api';

export default function Acceso({ aviso, alEntrar }: { aviso: string; alEntrar: (u: Usuario) => void }) {
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [enviando, setEnviando] = useState(false);
  const [verClave, setVerClave] = useState(false);

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
        <img className="logo" src="/logo.svg" alt="" width="72" height="72" />
        <h1>Planes <span>Go</span> Code</h1>
        <p className="suave">Entra con tu usuario de Odoo de PlanesNet.</p>
        {aviso && !error && <p className="aviso">{aviso}</p>}
        <label>Usuario<input autoFocus autoComplete="username" value={login} onChange={e => setLogin(e.target.value)} required /></label>
        <label>Contraseña
          <span className="clave">
            <input type={verClave ? 'text' : 'password'} autoComplete="current-password" value={password} onChange={e => setPassword(e.target.value)} required />
            {/* Solo se ve mientras se mantiene pulsado (ratón, dedo o Espacio/Intro); al soltar se oculta. */}
            <button type="button" className="ojo" aria-label="Mantén pulsado para ver la contraseña" title="Mantén pulsado para ver la contraseña"
              onPointerDown={e => { e.preventDefault(); setVerClave(true); }}
              onPointerUp={() => setVerClave(false)} onPointerLeave={() => setVerClave(false)} onPointerCancel={() => setVerClave(false)}
              onKeyDown={e => { if (e.key === ' ' || e.key === 'Enter') { e.preventDefault(); setVerClave(true); } }}
              onKeyUp={() => setVerClave(false)} onBlur={() => setVerClave(false)}>
              {verClave
                ? <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" d="M3 3l18 18M10.6 5.1A10.8 10.8 0 0 1 12 5c6 0 10 7 10 7a17.6 17.6 0 0 1-3.1 3.9M6.1 6.1C3.6 7.8 2 12 2 12s4 7 10 7c1.9 0 3.6-.6 5-1.4M9.9 9.9a3 3 0 0 0 4.2 4.2"/></svg>
                : <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path fill="none" stroke="currentColor" strokeWidth="2" d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3" fill="none" stroke="currentColor" strokeWidth="2"/></svg>}
            </button>
          </span>
        </label>
        {error && <p className="error" role="alert">{error}</p>}
        <button className="primario" disabled={enviando}>{enviando ? 'Entrando…' : 'Entrar'}</button>
      </form>
    </main>
  );
}
