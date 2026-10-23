import { useEffect, useState, type FormEvent } from 'react';
import { get, post, type ConfigPrograma } from '../api';
import Icono from './Icono';
import { programChanges, replaceProgram, type ProgramDraft } from '../programs';

// Propiedades de la sesión → Programas: cómo se arrancan los programas del proyecto (un Odoo, un servicio, lo que se
// está desarrollando), guardados en .psf/ejecutar.json de la carpeta de la sesión. Aquí solo se definen; se
// ejecutan y se paran desde Actividad o desde el lanzador. Lo que se edita es un borrador: se guarda con «Guardar».

export const OPEN_PROGRAM_SETTINGS = 'pgo:ajustes-programas';

export default function ProgramSettings({ proyecto, carpeta, value, onChange }: {
  proyecto: number; carpeta: string; value: ProgramDraft | null; onChange: (d: ProgramDraft) => void;
}) {
  // Borrador: se carga una vez y lo que se cambia aquí se guarda solo con «Guardar» de la ventana (programs.ts).
  const [edit, setEdit] = useState<ConfigPrograma | 'new' | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    if (!carpeta || value) return;
    get<{ configuraciones: ConfigPrograma[] }>(`/api/procesos?proyecto=${proyecto}&carpeta=${encodeURIComponent(carpeta)}`)
      .then(r => onChange({ original: r.configuraciones || [], current: r.configuraciones || [] })).catch(e => setError((e as Error).message));
  }, [proyecto, carpeta]); // eslint-disable-line react-hooks/exhaustive-deps
  const cfg = value?.current || null;
  const save = (c: ConfigPrograma, previous?: string) => {
    if (!value) return;
    onChange({ ...value, current: replaceProgram(value.current, c, previous) });
    setEdit(null);
  };
  const remove = (c: ConfigPrograma) => value && onChange({ ...value, current: value.current.filter(x => x.nombre !== c.nombre) });
  const pending = value ? programChanges(value) : { save: [], remove: [] };

  return <>
    <h3>Programas</h3>
    <p className="suave pequeno">Cómo se arrancan los programas de este proyecto en local (un Odoo, un servicio…). Se guardan en <code>.psf/ejecutar.json</code> de la carpeta de la sesión y se ejecutan desde <b>Actividad</b> o el lanzador, sin el agente ni tokens. Si un programa dice su dirección al arrancar, pgocode la abre en el navegador integrado.</p>
    <p className="suave pequeno">El agente también puede arrancarlos con <code>pgo ejecutar &lt;nombre&gt; &lt;orden&gt;</code> (y pararlos con <code>pgo parar &lt;nombre&gt;</code>); los que deja en marcha salen en Actividad y se pueden guardar aquí.</p>
    {!carpeta && <p className="suave">La sesión no tiene carpeta local.</p>}
    {error && <p className="error pequeno">{error}</p>}
    {carpeta && cfg && cfg.length === 0 && edit !== 'new' && <p className="suave pequeno">Aún no hay programas guardados.</p>}
    {(cfg || []).map(c => edit !== 'new' && edit?.nombre === c.nombre
      ? <ProgramForm key={c.nombre} initial={c} onCancel={() => setEdit(null)} onSave={n => save(n, c.nombre)} />
      : (
        <div key={c.nombre} className="programa-def">
          <span className="programa-texto">
            <b className="recorta">{c.nombre}</b>
            <code className="recorta" title={c.orden}>{c.orden}</code>
            {c.url && <span className="suave pequeno recorta">{c.url}</span>}
          </span>
          <button className="icono" onClick={() => setEdit(c)} title="Editar" aria-label={'Editar ' + c.nombre}><Icono n="lapiz" t={14} /></button>
          <button className="icono" onClick={() => remove(c)} title="Quitar (si está en marcha, sigue funcionando)" aria-label={'Quitar ' + c.nombre}><Icono n="borrar" t={14} /></button>
        </div>
      ))}
    {edit === 'new'
      ? <ProgramForm onCancel={() => setEdit(null)} onSave={c => save(c)} />
      : carpeta && cfg && <div><button type="button" className="boton-secundario" onClick={() => setEdit('new')}><Icono n="nuevo" t={13} /> Añadir programa</button></div>}
    {(pending.save.length > 0 || pending.remove.length > 0) && <p className="suave pequeno">Cambios sin guardar: se aplican al pulsar «Guardar».</p>}
  </>;
}

// saveProgramChanges: aplica el borrador (al pulsar «Guardar» en la ventana de la sesión).
export async function saveProgramChanges(proyecto: number, carpeta: string, d: ProgramDraft) {
  const { save, remove } = programChanges(d);
  for (const c of save) await post('/api/procesos/guardar', { proyecto, carpeta, nombre: c.nombre, orden: c.orden, url: c.url || '' });
  for (const nombre of remove) await post('/api/procesos/borrar-config', { proyecto, carpeta, nombre });
}

function ProgramForm({ initial, onSave, onCancel }: { initial?: ConfigPrograma; onSave: (c: ConfigPrograma) => void; onCancel: () => void }) {
  const [nombre, setNombre] = useState(initial?.nombre || '');
  const [orden, setOrden] = useState(initial?.orden || '');
  const [url, setUrl] = useState(initial?.url || '');
  const send = (e: FormEvent) => { e.preventDefault(); if (nombre.trim() && orden.trim()) onSave({ nombre: nombre.trim(), orden: orden.trim(), url: url.trim() || undefined }); };
  return (
    <form className="nuevo-programa" onSubmit={send}>
      <input placeholder="Nombre (p. ej. odoo)" value={nombre} onChange={e => setNombre(e.target.value)} pattern="[A-Za-z0-9][A-Za-z0-9._\-]{0,63}" required autoFocus />
      <input placeholder="Orden (p. ej. ./odoo-bin -c odoo.conf)" value={orden} onChange={e => setOrden(e.target.value)} required />
      <input placeholder="URL (opcional, si no la dice al arrancar)" value={url} onChange={e => setUrl(e.target.value)} />
      <div className="fila-config"><span className="hueco" /><button type="button" className="enlace" onClick={onCancel}>Cancelar</button><button className="primario">Guardar</button></div>
    </form>
  );
}
