import { useEffect, useState } from 'react';
import { get, post } from '../api';
import { clientDate, stopQuestion, type PairedClient } from '../clients';
import Icono from './Icono';
import { askConfirm } from './Modal';

// «Servidor» (Propiedades): the client apps paired with this pgocode server (revocable: a revoked one stops getting
// in at once) and stopping the server, which otherwise keeps running with the window closed.

export default function ServerSettings({ working }: { working?: string }) {
  const [list, setList] = useState<PairedClient[] | null>(null);
  const [me, setMe] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const load = () => get<{ clients: PairedClient[]; me: string }>('/api/clientes')
    .then(r => { setList(r.clients || []); setMe(r.me); }).catch(e => setError(String(e.message || e)));
  useEffect(() => { load(); }, []);

  async function revoke(c: PairedClient) {
    if (!(await askConfirm(`¿Revocar «${c.name || c.id}»? Esa app deja de tener acceso al momento; para volver a usarla en ese equipo se tendrá que emparejar otra vez.`, { danger: true, okText: 'Revocar' }))) return;
    setError('');
    try { await post('/api/clientes/revocar', { id: c.id }); await load(); } catch (e) { setError((e as Error).message); }
  }

  async function stop() {
    if (!(await askConfirm(stopQuestion(working), { danger: true, okText: 'Parar el servidor' }))) return;
    setBusy(true);
    setError('');
    try {
      await post('/api/servidor/parar', {});
      if (window.psf?.salir) await window.psf.salir();
      else setError('El servidor se ha parado. Para volver a usar Planes Go Code, ábrelo otra vez.');
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  }

  return <>
    <h3>Servidor</h3>
    <span className="suave pequeno">El servidor de Planes Go Code sigue en marcha aunque cierres la ventana (los agentes y las sesiones automáticas siguen trabajando). Estas son las apps emparejadas con él.</span>
    {list === null ? <p className="suave pequeno">{error || 'Cargando…'}</p> : (
      <ul className="lista-clientes">
        {list.map(c => (
          <li key={c.id} className="fila-config">
            <Icono n="equipo" t={14} />
            <div className="recorta">
              <b className="recorta">{c.name || c.id}</b>{c.id === me && <span className="suave pequeno"> · esta app</span>}
              <div className="suave pequeno">Emparejada {clientDate(c.created)} · última vez {clientDate(c.last_seen)}</div>
            </div>
            <span className="hueco" />
            {c.id !== me && <button type="button" className="enlace" onClick={() => revoke(c)}>Revocar</button>}
          </li>
        ))}
        {!list.length && <li className="suave pequeno">Ninguna.</li>}
      </ul>
    )}
    <div className="fila-config app-actions">
      <button type="button" className="boton-secundario" disabled={busy} onClick={stop}><Icono n="parar" t={13} /> Parar el servidor</button>
    </div>
    {error && list !== null && <p className="error" role="alert">{error}</p>}
  </>;
}
