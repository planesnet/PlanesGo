import { useEffect, useState } from 'react';
import { get, post } from '../api';
import { askConfirm } from './Modal';
import { memoryState, type MemoryView } from '../projectMemory';

// Memoria del proyecto (internal/memory): lo que pgocode recuerda del proyecto entre sesiones del agente. La
// mantiene sola; aquí se ve, se corrige o se borra (se guarda al pulsar, aparte del «Guardar» de la ventana).
export default function ProjectMemory({ proyecto, carpeta }: { proyecto: number; carpeta: string }) {
  const [view, setView] = useState<MemoryView | null>(null);
  const [text, setText] = useState('');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    if (!carpeta) return;
    let ok = true;
    get<MemoryView>(`/api/memoria?proyecto=${proyecto}&carpeta=${encodeURIComponent(carpeta)}`)
      .then(v => { if (ok) { setView(v); setText(v.texto); } })
      .catch(e => { if (ok) setError((e as Error).message); });
    return () => { ok = false; };
  }, [proyecto, carpeta]);
  const save = async (t: string) => {
    setSaving(true);
    setError('');
    try {
      const v = await post<MemoryView>('/api/memoria', { proyecto, carpeta, texto: t });
      setView(v);
      setText(v.texto);
    } catch (e) { setError((e as Error).message); } finally { setSaving(false); }
  };
  const st = memoryState(view, text);
  return <>
    <h3>Memoria del proyecto</h3>
    <span className="suave pequeno">Lo que sigue valiendo de una sesión a otra: decisiones, convenciones, datos (puertos, bases de datos…) y las órdenes del proyecto que funcionan. pgocode la mantiene sola con lo que dices en la conversación y se la da al agente solo cuando empieza de cero (sesión nueva, relevo, «Vaciar contexto» o compactación), al final del mensaje: no en cada turno. Es del proyecto (la comparten sus sesiones) y no entra en git.</span>
    {!carpeta ? <p className="suave">Elige antes la carpeta de la sesión.</p> : !view && !error ? <p className="suave">Cargando…</p> : <>
      <textarea className="editor-plan-texto project-memory-text" rows={14} value={text} onChange={e => setText(e.target.value)} spellCheck={false}
        placeholder={'Todavía no hay memoria. Puedes escribirla tú, p. ej.:\n## Decisiones y datos\n- El IVA se redondea con ROUND_HALF_UP\n## Órdenes\n- pruebas: `make test`'}
        aria-label="Memoria del proyecto (Markdown)" />
      <div className="project-memory-actions">
        <span className={st.over ? 'error pequeno' : 'suave pequeno'}>{st.line}</span>
        <span className="hueco" />
        <button type="button" className="enlace" disabled={saving || !view?.texto} onClick={async () => {
          if (await askConfirm('¿Borrar la memoria del proyecto? pgocode volverá a llenarla con lo que digas a partir de ahora.')) save('');
        }}>Borrar</button>
        <button type="button" className="boton-secundario" disabled={saving || !st.dirty || st.over} onClick={() => save(text)}>{saving ? 'Guardando…' : 'Guardar memoria'}</button>
      </div>
      {view?.ruta && <span className="suave pequeno"><code className="recorta" title={view.ruta}>{view.ruta}</code></span>}
    </>}
    {error && <p className="error">{error}</p>}
  </>;
}
