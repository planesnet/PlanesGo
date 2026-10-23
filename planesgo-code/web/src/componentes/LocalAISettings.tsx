import { useEffect, useState } from 'react';
import { get, post } from '../api';
import Icono from './Icono';
import { LOCAL_AI_DEFAULT_URL, localAIStatusText, localAITestText, type LocalAIView } from '../localAI';

// «IA local» (Propiedades): dónde está la IA que usa pgocode sin gastar tokens de pago (embeddings del hilo de
// limpieza y del selector de skills). Por defecto el Ollama del equipo; puede ser un servidor de la red (Ollama o
// compatible con OpenAI, como vLLM) con token.

export default function LocalAISettings() {
  const [v, setV] = useState<LocalAIView | null>(null);
  const [url, setUrl] = useState('');
  const [token, setToken] = useState('');
  const [embed, setEmbed] = useState('');
  const [chat, setChat] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const show = (x: LocalAIView) => { setV(x); setUrl(x.url); setEmbed(x.modelo_embeddings); setChat(x.modelo_texto || ''); setToken(''); };
  useEffect(() => { get<LocalAIView>('/api/ia-local').then(show).catch(e => setError(String(e.message || e))); }, []);

  const act = (f: () => Promise<LocalAIView>) => {
    setBusy(true); setError('');
    f().then(show).catch(e => setError(String(e.message || e))).finally(() => setBusy(false));
  };
  const save = (extra: Record<string, unknown> = {}) => act(() => post<LocalAIView>('/api/ia-local',
    { url, token, modelo_embeddings: embed, modelo_texto: chat, ...extra }));

  return <>
    <h3>IA local</h3>
    <span className="suave pequeno">La que usa pgocode sin gastar tokens de pago: el hilo de limpieza y la elección de skills. Puede ser el Ollama de este equipo o un servidor de vuestra red (Ollama o compatible con OpenAI, como vLLM).</span>
    <label>URL del servidor
      <input value={url} placeholder={LOCAL_AI_DEFAULT_URL} onChange={e => setUrl(e.target.value)} spellCheck={false} />
    </label>
    <label>Token {v?.con_token ? <span className="suave pequeno">(guardado; escribe otro para cambiarlo)</span> : <span className="suave pequeno">(opcional)</span>}
      <input type="password" value={token} autoComplete="off" placeholder={v?.con_token ? '••••••••' : ''} onChange={e => setToken(e.target.value)} />
    </label>
    <label>Modelo de embeddings
      <input value={embed} placeholder="embeddinggemma" onChange={e => setEmbed(e.target.value)} spellCheck={false} />
    </label>
    <label>Modelo de texto <span className="suave pequeno">(opcional; para lo que venga: repartir tareas, resúmenes)</span>
      <input value={chat} placeholder="—" onChange={e => setChat(e.target.value)} spellCheck={false} list="ia-local-modelos" />
      <datalist id="ia-local-modelos">{(v?.estado.modelos || []).map(m => <option key={m} value={m} />)}</datalist>
    </label>
    <dl className="session-ids">
      <div><dt>Estado</dt><dd className={v?.estado.listo ? '' : 'aviso-txt'}>{localAIStatusText(v?.estado, v?.modelo_embeddings)}</dd></div>
      {(v?.pruebas || []).map(p => (
        <div key={p.tipo}><dt>{p.tipo === 'texto' ? 'Modelo de texto' : 'Embeddings'}</dt><dd className={p.ok ? '' : 'aviso-txt'}>{localAITestText(p)}</dd></div>
      ))}
    </dl>
    {error && <div className="aviso-txt pequeno" role="alert">{error}</div>}
    <div className="fila-config app-actions">
      <button type="button" className="primario" disabled={busy} onClick={() => save()}>Guardar</button>
      <button type="button" className="boton-secundario" disabled={busy} onClick={() => act(() => get<LocalAIView>('/api/ia-local?probar=1'))}><Icono n="recargar" t={13} /> {busy ? 'Probando…' : 'Probar los dos modelos'}</button>
      {v?.con_token && <button type="button" className="boton-secundario" disabled={busy} onClick={() => save({ quitar_token: true })}>Quitar token</button>}
      {url !== LOCAL_AI_DEFAULT_URL && <button type="button" className="boton-secundario" disabled={busy}
        onClick={() => act(() => post<LocalAIView>('/api/ia-local', { url: LOCAL_AI_DEFAULT_URL, modelo_embeddings: embed, modelo_texto: chat, quitar_token: true }))}>Usar la de este equipo</button>}
    </div>
  </>;
}
