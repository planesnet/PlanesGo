import { useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { chat, type Mensaje, type Proveedor, type Proyecto } from '../api';
import { conversacion, guardaConversacion, nombreDe, titulo, type Repo, type Sesion } from '../sesiones';
import Icono from './Icono';
import { Agente, Hosting, nombreHosting } from './Marca';

interface Props {
  proyecto: Proyecto | null; sesion: Sesion | null; repo: Repo | null; proveedores: Proveedor[];
  panelAbierto: boolean; alPanel: () => void;
  alCambiarSesion: (s: Sesion) => void; alNuevaSesion: (rama?: string) => void;
}

const nombreCarpeta = (c: string) => c.split('/').filter(Boolean).pop() || c;

const MODOS = { lectura: 'Solo lectura', editar: 'Editar ficheros', auto: 'Auto: ejecuta comandos' } as const;
const ESFUERZOS: Record<string, string> = { low: 'bajo', medium: 'medio', high: 'alto', xhigh: 'muy alto', max: 'máximo' };
const ALTO_MAX = 240;

export default function Chat({ proyecto, sesion, repo, proveedores, panelAbierto, alPanel, alCambiarSesion, alNuevaSesion }: Props) {
  const [texto, setTexto] = useState('');
  const [mensajes, setMensajes] = useState<Mensaje[]>([]);
  const [enCurso, setEnCurso] = useState<AbortController | null>(null);
  const [verAgentes, setVerAgentes] = useState(false);
  const fin = useRef<HTMLDivElement>(null);
  const lista = useRef<HTMLDivElement>(null);
  const idActual = useRef(sesion?.id);
  const area = useRef<HTMLTextAreaElement>(null);

  // El campo empieza en una línea y crece hacia arriba según lo escrito (hasta un máximo, luego scroll).
  useLayoutEffect(() => {
    const a = area.current;
    if (!a) return;
    a.style.height = 'auto';
    a.style.height = Math.min(a.scrollHeight, ALTO_MAX) + 'px';
    a.style.overflowY = a.scrollHeight > ALTO_MAX ? 'auto' : 'hidden';
  }, [texto, sesion?.id]);

  const prov = proveedores.find(p => p.id === sesion?.agente);
  const modelos = useMemo(() => prov?.modelos?.length ? prov.modelos : (prov?.por_defecto ? [prov.por_defecto] : []), [prov]);

  // Al cambiar de sesión: su conversación; y se corta la respuesta en curso (no se escribe en otra sesión).
  useEffect(() => {
    idActual.current = sesion?.id;
    setMensajes(sesion ? conversacion(sesion.id) : []);
    return () => { setEnCurso(ac => { ac?.abort(); return null; }); };
  }, [sesion?.id]);

  // Solo se baja al final si el usuario ya estaba abajo.
  useEffect(() => {
    const l = lista.current;
    if (!l || l.scrollHeight - l.scrollTop - l.clientHeight < 120) fin.current?.scrollIntoView({ block: 'end' });
  }, [mensajes]);

  async function envia() {
    const t = texto.trim();
    if (!t || !proyecto || !sesion || !prov || enCurso) return;
    const id = sesion.id;
    const pon = (m: Mensaje[], final = false) => {
      if (idActual.current === id) setMensajes(m);
      if (final) guardaConversacion(id, m);
    };
    const base: Mensaje[] = [...mensajes.filter(m => !m.error), { rol: 'user', texto: t }];
    let actual: Mensaje[] = [...base, { rol: 'assistant', texto: '' }];
    pon(actual, true);
    setTexto('');
    const ac = new AbortController();
    setEnCurso(ac);
    let sesionAgente = sesion.sesionAgente;
    const ultimo = (f: (m: Mensaje) => Mensaje) => { actual = [...actual.slice(0, -1), f(actual[actual.length - 1])]; pon(actual); };
    try {
      await chat({
        proyecto: proyecto.id, proveedor: prov.id, modelo: sesion.modelo || prov.por_defecto, carpeta: sesion.carpeta, rama: sesion.rama,
        permisos: sesion.permisos || 'lectura', sesion_agente: sesion.sesionAgente, esfuerzo: sesion.esfuerzo || '',
        mensajes: prov.tipo === 'agente' ? base.slice(-1) : base, // los agentes llevan su propio historial
      }, e => {
        if (e.texto) ultimo(m => ({ ...m, texto: m.texto + e.texto }));
        if (e.sesion_agente) sesionAgente = e.sesion_agente;
        if (e.error) ultimo(m => ({ ...m, texto: (m.texto ? m.texto + '\n\n' : '') + '⚠ ' + e.error, error: true }));
      }, ac.signal);
    } catch (err) {
      if (!ac.signal.aborted) ultimo(m => ({ ...m, texto: '⚠ ' + (err as Error).message, error: true }));
    } finally {
      pon(actual, true);
      setEnCurso(c => (c === ac ? null : c));
      alCambiarSesion({ ...sesion, nombre: sesion.nombre || nombreDe(t), sesionAgente, ultima: Date.now() });
    }
  }

  const teclas = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); envia(); }
  };

  const eligeAgente = (p: Proveedor) => {
    setVerAgentes(false);
    if (sesion && p.id !== sesion.agente) alCambiarSesion({ ...sesion, agente: p.id, modelo: undefined, sesionAgente: undefined });
    try { localStorage.setItem('pgo:proveedor', p.id); } catch { /* sin almacenamiento */ }
  };

  return (
    <main className="chat">
      <header className="cabecera">
        {proyecto && sesion ? (
          <nav className="miga" aria-label="Ubicación de la sesión">
            <span className="miga-item" title={nombreHosting(repo?.tipo)}><Hosting tipo={repo?.tipo} t={16} /></span>
            {repo?.repositorio && <><span className="sep">/</span><span className="miga-item" title={repo.remoto}><Icono n="repo" t={14} /> {repo.repositorio}</span></>}
            {sesion.carpeta && <><span className="sep">/</span><span className="miga-item" title={sesion.carpeta}><Icono n="carpeta" t={14} /> {nombreCarpeta(sesion.carpeta)}</span></>}
            {repo && repo.ramas.length > 0 && (
              <><span className="sep">/</span>
                <label className="miga-item rama" title="Cambiar de rama abre una sesión nueva">
                  <Icono n="rama" t={14} />
                  <select value={sesion.rama} onChange={e => alNuevaSesion(e.target.value)} disabled={Boolean(enCurso)}>
                    {repo.ramas.map(r => <option key={r}>{r}</option>)}
                  </select>
                </label></>
            )}
          </nav>
        ) : <span className="suave titulo">{proyecto ? proyecto.nombre : 'Elige un proyecto de Odoo'}</span>}
        <span className="hueco" />
        {sesion && mensajes.length > 0 && !enCurso && (
          <button className="icono" onClick={() => alNuevaSesion(sesion.rama)} title="Nueva sesión" aria-label="Nueva sesión"><Icono n="nuevo" /></button>
        )}
        <button className="icono" onClick={alPanel} title={panelAbierto ? 'Ocultar artefactos' : 'Mostrar artefactos'} aria-label="Artefactos" aria-pressed={panelAbierto}><Icono n="panel" /></button>
      </header>

      {!proyecto ? (
        <div className="vacio">
          <h2>Sin proyecto, no hay trabajo</h2>
          <p>Todo en Planes Go Code va ligado a un proyecto de Odoo. Elige uno a la izquierda.</p>
        </div>
      ) : !sesion ? (
        <div className="vacio"><p>Preparando la sesión…</p></div>
      ) : (
        <>
          <div className="mensajes" aria-live="polite" ref={lista}>
            {mensajes.length === 0 && (
              <div className="vacio pequeno">
                <p><b>{titulo(sesion)}</b></p>
                <p className="suave">{prov?.nombre || 'Elige un agente'}{prov?.tipo === 'agente' ? ` · ${{ lectura: 'solo lectura', editar: 'puede editar', auto: 'auto: ejecuta comandos' }[sesion.permisos || 'lectura']}` : ''}</p>
              </div>
            )}
            {mensajes.map((m, i) => (
              <div key={i} className={'mensaje ' + m.rol + (m.error ? ' con-error' : '')}>
                {m.texto || (enCurso && i === mensajes.length - 1 ? <span className="pensando">Pensando…</span> : '')}
              </div>
            ))}
            <div ref={fin} />
          </div>
          <div className="redactor">
            <div className="entrada">
              <div className="selector-agente">
                <button className="agente-actual" onClick={() => setVerAgentes(v => !v)} title={prov ? prov.nombre : 'Elegir agente'} aria-label="Agente" aria-haspopup="listbox" aria-expanded={verAgentes}>
                  <Agente id={sesion.agente} t={18} /><Icono n="bajar" t={12} />
                </button>
                {verAgentes && (
                  <ul className="lista-agentes" role="listbox" aria-label="Agentes">
                    {proveedores.map(p => (
                      <li key={p.id}>
                        <button role="option" aria-selected={p.id === sesion.agente} disabled={!p.disponible} onClick={() => eligeAgente(p)} title={p.disponible ? '' : p.motivo}>
                          <Agente id={p.id} t={16} /> <span className="recorta">{p.nombre}</span>
                          {p.id === sesion.agente && <Icono n="hecho" t={14} />}
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
              <textarea ref={area} rows={1} value={texto} onChange={e => setTexto(e.target.value)} onKeyDown={teclas}
                placeholder={prov?.disponible ? 'Escribe…' : 'Elige un agente disponible'} disabled={!prov?.disponible} />
              {enCurso
                ? <button className="redondo" onClick={() => enCurso.abort()} title="Parar" aria-label="Parar"><Icono n="parar" t={16} /></button>
                : <button className="redondo primario" onClick={envia} disabled={!texto.trim() || !prov?.disponible} title="Enviar (Intro)" aria-label="Enviar"><Icono n="enviar" t={16} /></button>}
            </div>
            {/* Debajo del campo, como en Claude Code: el modo a la izquierda; el modelo y su esfuerzo a la derecha. */}
            <div className="pie-redactor">
              {prov?.modos && (
                <select className="pie-select" value={sesion.permisos || 'lectura'} aria-label="Modo"
                  onChange={e => alCambiarSesion({ ...sesion, permisos: e.target.value as Sesion['permisos'] })}>
                  {Object.entries(MODOS).map(([v, t]) => <option key={v} value={v}>{t}</option>)}
                </select>
              )}
              <span className="hueco" />
              {modelos.length > 1 && (
                <select className="pie-select" value={sesion.modelo || prov?.por_defecto} aria-label="Modelo"
                  onChange={e => alCambiarSesion({ ...sesion, modelo: e.target.value })}>
                  {modelos.map(m => <option key={m} value={m}>{m === 'por defecto' ? 'Modelo por defecto' : m}</option>)}
                </select>
              )}
              {modelos.length <= 1 && prov && <span className="pie-texto">{prov.nombre}</span>}
              {prov?.esfuerzos?.length ? (
                <select className="pie-select" value={sesion.esfuerzo || ''} aria-label="Esfuerzo"
                  onChange={e => alCambiarSesion({ ...sesion, esfuerzo: e.target.value || undefined })}>
                  <option value="">Esfuerzo por defecto</option>
                  {prov.esfuerzos.map(x => <option key={x} value={x}>Esfuerzo {ESFUERZOS[x] || x}</option>)}
                </select>
              ) : null}
            </div>
          </div>
        </>
      )}
    </main>
  );
}
