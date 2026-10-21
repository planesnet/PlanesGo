import { useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { chat, post, subeAdjunto, type Adjunto, type Eleccion, type Herramienta, type Mensaje, type Parte, type Proveedor, type Proyecto, type SkillPgo } from '../api';
import type { Detalle, SeleccionSkills } from '../ajustes';
import { conversacion, guardaConversacion, nombreDe, titulo, type Repo, type Sesion } from '../sesiones';
import * as C from '../cola';
import Icono from './Icono';
import Texto from './Texto';
import { Agente, Hosting, nombreHosting } from './Marca';

interface Props {
  detalle: Detalle;
  seleccionSkills: SeleccionSkills;
  hosting?: string; // alojamiento elegido en las propiedades del proyecto (si no, el del repositorio)
  // Mensaje que hay que enviar solo en cuanto la sesión indicada esté abierta (p. ej. «Resuelve la issue…»).
  envioPendiente: { sesion: string; texto: string } | null; alEnviado: () => void;
  proyecto: Proyecto | null; sesion: Sesion | null; repo: Repo | null; proveedores: Proveedor[];
  panelAbierto: boolean; alPanel: () => void;
  alCambiarSesion: (s: Sesion) => void; alNuevaSesion: (rama?: string) => void; alAjustes: () => void;
}

const nombreCarpeta = (c: string) => c.split('/').filter(Boolean).pop() || c;

const MODOS = { lectura: 'Solo lectura', editar: 'Editar ficheros', auto: 'Auto: ejecuta comandos' } as const;
const ESFUERZOS: Record<string, string> = { low: 'bajo', medium: 'medio', high: 'alto', xhigh: 'muy alto', max: 'máximo' };
const ALTO_MAX = 240;

export default function Chat({ alAjustes, seleccionSkills, hosting, detalle, envioPendiente, alEnviado, proyecto, sesion, repo, proveedores, panelAbierto, alPanel, alCambiarSesion, alNuevaSesion }: Props) {
  const [texto, setTexto] = useState('');
  const [mensajes, setMensajes] = useState<Mensaje[]>([]);
  const [enCurso, setEnCurso] = useState<AbortController | null>(null);
  const [verAgentes, setVerAgentes] = useState(false);
  const fin = useRef<HTMLDivElement>(null);
  const lista = useRef<HTMLDivElement>(null);
  const idActual = useRef(sesion?.id);
  const area = useRef<HTMLTextAreaElement>(null);
  // Ficheros adjuntos al mensaje que se está escribiendo (ya subidos a la carpeta de la sesión).
  const [adjuntos, setAdjuntos] = useState<(Adjunto & { subiendo?: boolean; error?: string })[]>([]);
  const [arrastrando, setArrastrando] = useState(false);
  // Cola de mensajes (como en Claude Code), una por sesión: ver cola.ts.
  const [colas, setColas] = useState<Record<string, C.Cola<Adjunto>>>({});
  const colaSesion = (sesion && colas[sesion.id]) || C.vacia<Adjunto>();
  const cola = colaSesion.mensajes;
  const ponCola = (id: string, f: (c: C.Cola<Adjunto>) => C.Cola<Adjunto>) => setColas(x => {
    const n = f(x[id] || C.vacia<Adjunto>());
    return n === x[id] ? x : { ...x, [id]: n };
  });
  const selector = useRef<HTMLInputElement>(null);
  useEffect(() => { setAdjuntos([]); }, [sesion?.id]);
  const adjunta = (fs: File[]) => {
    if (!proyecto || !sesion) return;
    for (const f of fs) {
      const clave = Math.random().toString(36).slice(2);
      setAdjuntos(l => [...l, { nombre: f.name || 'imagen pegada', ruta: clave, tamano: f.size, subiendo: true }]);
      subeAdjunto(proyecto.id, sesion.carpeta, sesion.id, f)
        .then(a => setAdjuntos(l => l.map(x => x.ruta === clave ? a : x)))
        .catch(e => setAdjuntos(l => l.map(x => x.ruta === clave ? { ...x, subiendo: false, error: (e as Error).message } : x)));
    }
  };
  const subiendo = adjuntos.some(a => a.subiendo);

  // El campo empieza en una línea y crece hacia arriba según lo escrito (hasta un máximo, luego scroll).
  useLayoutEffect(() => {
    const a = area.current;
    if (!a) return;
    a.style.height = 'auto';
    a.style.height = Math.min(a.scrollHeight, ALTO_MAX) + 'px';
    a.style.overflowY = a.scrollHeight > ALTO_MAX ? 'auto' : 'hidden';
  }, [texto, sesion?.id]);

  const prov = proveedores.find(p => p.id === sesion?.agente);
  // Skills usadas en la sesión (en orden) y la que se está usando ahora (la última del turno en curso).
  const skillsSesion = useMemo(() => {
    const l: string[] = [];
    for (const m of mensajes) {
      if (m.skill?.modo === 'elegida' && !l.includes(m.skill.nombre)) l.push(m.skill.nombre);
      for (const p of m.partes || []) if (p.tipo === 'herramienta' && p.h.skill && !l.includes(p.h.skill)) l.push(p.h.skill);
    }
    return l;
  }, [mensajes]);
  const skillActiva = useMemo(() => {
    const u = mensajes[mensajes.length - 1];
    if (!u || u.rol !== 'assistant') return '';
    const ss = (u.partes || []).filter(p => p.tipo === 'herramienta' && p.h.skill);
    const x = ss[ss.length - 1];
    return x && x.tipo === 'herramienta' ? x.h.skill || '' : '';
  }, [mensajes]);
  const modelos = useMemo(() => prov?.modelos?.length ? prov.modelos : (prov?.por_defecto ? [prov.por_defecto] : []), [prov]);

  // Al cambiar de sesión: su conversación; y se corta la respuesta en curso (no se escribe en otra sesión).
  useEffect(() => {
    idActual.current = sesion?.id;
    setMensajes(sesion ? conversacion(sesion.id) : []);
    return () => { setEnCurso(ac => { ac?.abort(); return null; }); };
  }, [sesion?.id]);

  // Desplazamiento: al entrar en una sesión, a lo más reciente; mientras llega la respuesta, siempre abajo,
  // salvo que el usuario haya subido a leer (se respeta hasta que vuelva abajo).
  const pegado = useRef(true);
  const alDesplazar = () => {
    const l = lista.current;
    if (l) pegado.current = l.scrollHeight - l.scrollTop - l.clientHeight < 80;
  };
  const baja = () => { const l = lista.current; if (l) l.scrollTop = l.scrollHeight; };
  useLayoutEffect(() => {
    pegado.current = true;
    baja();
    // Otra vez tras pintar el texto enriquecido y las herramientas (cambian la altura).
    const t = requestAnimationFrame(baja);
    return () => cancelAnimationFrame(t);
  }, [sesion?.id]);
  useLayoutEffect(() => { if (pegado.current) baja(); }, [mensajes]);
  // También al crecer el contenido sin mensajes nuevos (desplegar una herramienta, imágenes, bloques de código).
  useEffect(() => {
    const l = lista.current;
    if (!l) return;
    const ro = new ResizeObserver(() => { if (pegado.current) baja(); });
    Array.from(l.children).forEach(c => ro.observe(c));
    return () => ro.disconnect();
  }, [mensajes.length, sesion?.id]);

  // Envío automático pedido desde fuera, cuando su sesión ya es la abierta y no hay nada en curso.
  useEffect(() => {
    if (!envioPendiente || !sesion || sesion.id !== envioPendiente.sesion || enCurso || !prov?.disponible) return;
    alEnviado();
    envia(envioPendiente.texto);
  }, [envioPendiente, sesion?.id, prov?.disponible]); // eslint-disable-line react-hooks/exhaustive-deps

  // Al acabar un turno sin error, sale el primero de la cola.
  useEffect(() => {
    if (!sesion || !prov?.disponible) return;
    const { enviar, cola: resto } = C.siguiente(colaSesion, Boolean(enCurso));
    if (!enviar) return;
    ponCola(sesion.id, () => resto);
    envia(enviar.texto, enviar.adjuntos);
  }, [enCurso, sesion?.id, colaSesion, prov?.disponible]); // eslint-disable-line react-hooks/exhaustive-deps

  // Con el agente trabajando, Intro deja el mensaje (y sus adjuntos) en la cola.
  function encola() {
    if (!texto.trim() || !sesion || subiendo) return;
    const listos = adjuntos.filter(a => !a.subiendo && !a.error);
    ponCola(sesion.id, c => C.encola(c, texto, listos));
    setTexto('');
    setAdjuntos([]);
  }

  // Parar: el turno se corta y lo que había en la cola vuelve al cuadro de texto, para revisarlo.
  function para() {
    if (!enCurso || !sesion) return;
    if (cola.length) {
      const d = C.devuelve(cola, texto, adjuntos);
      setTexto(d.texto);
      setAdjuntos(d.adjuntos);
      ponCola(sesion.id, c => ({ ...c, mensajes: [] }));
    }
    enCurso.abort();
  }

  async function envia(forzado?: string, adjCola?: Adjunto[]) {
    const t = (forzado ?? texto).trim();
    if (!t || !proyecto || !sesion || !sesion.hosting || !prov || enCurso || (forzado === undefined && subiendo)) return;
    const id = sesion.id;
    ponCola(id, C.empieza);
    let fallo = false;
    const pon = (m: Mensaje[], final = false) => {
      if (idActual.current === id) setMensajes(m);
      if (final) guardaConversacion(id, m);
    };
    // Envío automático: la sesión acaba de abrirse y el estado aún puede ser el de la anterior; se lee la guardada.
    const previos = forzado !== undefined ? conversacion(id) : mensajes;
    const listos = adjCola ?? (forzado === undefined ? adjuntos.filter(a => !a.subiendo && !a.error) : []);
    const base: Mensaje[] = [...previos.filter(m => !m.error), { rol: 'user', texto: t, ...(listos.length ? { adjuntos: listos } : {}) }];
    let actual: Mensaje[] = [...base, { rol: 'assistant', texto: '' }];
    pon(actual, true);
    if (forzado === undefined) { setTexto(''); setAdjuntos([]); }
    const ac = new AbortController();
    setEnCurso(ac);
    let sesionAgente = sesion.sesionAgente;
    // Skill de la petición por embeddings: «elegir» espera (como mucho 1,5 s) y se la indica al agente;
    // «sugerir» va en paralelo y solo se muestra.
    let aEnviar = t;
    const marcaSkill = (k: SkillPgo) => {
      actual = actual.map((m, i) => i === actual.length - 2 ? { ...m, skill: k } : m);
      pon(actual, true);
    };
    if (seleccionSkills !== 'agente' && prov.tipo === 'agente') {
      const pide = post<Eleccion>('/api/skills/elegir', { proyecto: proyecto.id, carpeta: sesion.carpeta, agente: sesion.agente, texto: t });
      if (seleccionSkills === 'elegir') {
        const r = await Promise.race([pide.catch(() => null), new Promise<null>(res => setTimeout(() => res(null), 1500))]);
        if (r?.elegida) {
          const n = r.elegida.nombre;
          aEnviar = sesion.agente === 'claude-code' ? `/${n} ${t}` : `Usa la skill «${n}».\n\n${t}`;
          marcaSkill({ nombre: n, puntuacion: r.elegida.puntuacion, modo: 'elegida' });
        }
      } else {
        pide.then(r => { if (r.elegida && idActual.current === id) marcaSkill({ nombre: r.elegida.nombre, puntuacion: r.elegida.puntuacion, modo: 'sugerida' }); }).catch(() => {});
      }
    }
    const ultimo = (f: (m: Mensaje) => Mensaje) => { actual = [...actual.slice(0, -1), f(actual[actual.length - 1])]; pon(actual); };
    // Añade un trozo a las partes del mensaje: el texto seguido se junta; la herramienta se actualiza por su id.
    const parte = (p: Parte) => ultimo(m => {
      const ps = [...(m.partes || [])];
      const u = ps[ps.length - 1];
      if (p.tipo !== 'herramienta' && u && u.tipo === p.tipo) ps[ps.length - 1] = { ...u, texto: u.texto + p.texto };
      else if (p.tipo === 'herramienta') {
        const i = ps.findIndex(x => x.tipo === 'herramienta' && x.h.id === p.h.id);
        if (i >= 0) { const v = (ps[i] as { h: Herramienta }).h; ps[i] = { tipo: 'herramienta', h: { ...v, ...p.h, nombre: p.h.nombre || v.nombre, resumen: v.resumen || p.h.resumen, entrada: v.entrada || p.h.entrada } }; }
        else ps.push(p);
      } else ps.push(p);
      return { ...m, partes: ps, texto: p.tipo === 'texto' ? m.texto + p.texto : m.texto };
    });
    try {
      await chat({
        proyecto: proyecto.id, proveedor: prov.id, modelo: sesion.modelo || prov.por_defecto, carpeta: sesion.carpeta, rama: sesion.rama,
        permisos: sesion.permisos || 'lectura', sesion_agente: sesion.sesionAgente, esfuerzo: sesion.esfuerzo || '',
        sesion: sesion.id, titulo: sesion.nombre, adjuntos: listos.map(a => a.ruta), tarea_id: sesion.tareaId || 0, tarea: sesion.tarea || '',
        // Los agentes llevan su propio historial: solo el último mensaje (con la skill indicada, si pgocode la eligió).
        mensajes: prov.tipo === 'agente' ? [{ rol: 'user', texto: aEnviar }] : base,
      }, e => {
        if (e.texto) parte({ tipo: 'texto', texto: e.texto });
        if (e.pensamiento) parte({ tipo: 'pensamiento', texto: e.pensamiento });
        if (e.herramienta) parte({ tipo: 'herramienta', h: e.herramienta });
        if (e.uso) ultimo(m => ({ ...m, uso: e.uso }));
        if (e.sesion_agente) sesionAgente = e.sesion_agente;
        if (e.error) fallo = true;
        if (e.error) ultimo(m => ({ ...m, texto: (m.texto ? m.texto + '\n\n' : '') + '⚠ ' + e.error, error: true }));
      }, ac.signal);
    } catch (err) {
      fallo = true;
      if (!ac.signal.aborted) ultimo(m => ({ ...m, texto: '⚠ ' + (err as Error).message, error: true }));
    } finally {
      pon(actual, true);
      if (fallo) ponCola(id, C.falla);
      setEnCurso(c => (c === ac ? null : c));
      alCambiarSesion({ ...sesion, nombre: sesion.nombre || nombreDe(t), sesionAgente, ultima: Date.now() });
    }
  }

  const teclas = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); if (enCurso) encola(); else envia(); }
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
            <button className="miga-item" onClick={alAjustes} title={nombreHosting(sesion.hosting || hosting || repo?.tipo) + ' · ajustes de la sesión'}><Hosting tipo={sesion.hosting || hosting || repo?.tipo} t={16} /></button>
            {repo?.repositorio && <><span className="sep">/</span><span className="miga-item" title={repo.remoto}><Icono n="repo" t={14} /> {repo.repositorio}</span></>}
            {sesion.carpeta && <><span className="sep">/</span><span className="miga-item" title={sesion.carpeta}><Icono n="carpeta" t={14} /> {nombreCarpeta(sesion.carpeta)}</span></>}
            <span className="sep">·</span>
            <button className="miga-item imputa" onClick={alAjustes} title="Tarea de Odoo a la que se imputa el tiempo de esta sesión (cámbiala en los ajustes)">
              <Icono n="reloj" t={13} /> {sesion.tarea || 'Desarrollo'}
            </button>
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
        {skillsSesion.length > 0 && (
          <span className="skills-sesion" title="Skills usadas en esta sesión">
            {skillsSesion.map(n => <span key={n} className={'sk-marca' + (enCurso && n === skillActiva ? ' en-uso' : '')} title={enCurso && n === skillActiva ? 'En uso ahora' : 'Usada en esta sesión'}>✦ {n}</span>)}
          </span>
        )}
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
        <div className="vacio">
          <p>Elige una sesión a la izquierda o empieza una nueva.</p>
          <button className="primario boton-nueva-sesion" onClick={() => alNuevaSesion()}><Icono n="nuevo" t={15} /> Nueva sesión</button>
        </div>
      ) : (
        <>
          <div className="mensajes" aria-live="polite" ref={lista} onScroll={alDesplazar}>
            {mensajes.length === 0 && (
              <div className="vacio pequeno">
                <p><b>{titulo(sesion)}</b></p>
                <p className="suave">{prov?.nombre || 'Elige un agente'}{prov?.tipo === 'agente' ? ` · ${{ lectura: 'solo lectura', editar: 'puede editar', auto: 'auto: ejecuta comandos' }[sesion.permisos || 'lectura']}` : ''}</p>
              </div>
            )}
            {mensajes.map((m, i) => (
              <div key={i} className={'mensaje ' + m.rol + (m.error ? ' con-error' : '')}>
                {m.partes?.length
                  ? <Partes partes={m.partes} detalle={detalle} />
                  : m.texto ? (m.rol === 'assistant' && !m.error ? <Texto md={m.texto} /> : m.texto)
                    : (enCurso && i === mensajes.length - 1 ? <span className="pensando">Pensando…</span> : '')}
                {m.partes?.length && m.error ? <p className="con-error">{m.texto.split('⚠').slice(1).map(x => '⚠' + x).join('')}</p> : null}
                {enCurso && i === mensajes.length - 1 && m.partes?.length ? <span className="pensando">Trabajando…</span> : null}
                {m.uso && detalle !== 'conciso' && <div className="uso">{usoTexto(m.uso)}</div>}
                {m.adjuntos && m.adjuntos.length > 0 && (
                  <div className="adjuntos-mensaje">{m.adjuntos.map(a => <span key={a.ruta} className="ficha-adjunto" title={a.ruta}><Icono n="clip" t={12} /> {a.nombre}</span>)}</div>
                )}
                {m.skill && (
                  <span className={'sk-marca sk-pgo ' + m.skill.modo} title={m.skill.modo === 'elegida' ? 'pgocode eligió esta skill (embeddings) y se la indicó al agente' : 'pgocode cree que encaja esta skill (embeddings); el agente decide'}>
                    ✦ {m.skill.modo === 'elegida' ? 'elegida' : 'sugerida'}: {m.skill.nombre} · {m.skill.puntuacion.toFixed(2)}
                  </span>
                )}
              </div>
            ))}
            <div ref={fin} />
          </div>
          <div className="redactor">
            {!sesion.hosting && (
              <div className="aviso-sesion">
                <Icono n="info" t={15} /> Falta elegir el alojamiento de la sesión.
                <button className="enlace" onClick={alAjustes}>Completar ajustes</button>
              </div>
            )}
            {cola.length > 0 && (
              <ol className="cola" aria-label="Mensajes en cola">
                {cola.map((c, i) => (
                  <li key={i} className="en-cola">
                    <span className="cola-num">{i + 1}</span>
                    <span className="cola-texto recorta" title={c.texto}>{c.texto}</span>
                    {c.adjuntos.length > 0 && <span className="suave pequeno"><Icono n="clip" t={11} /> {c.adjuntos.length}</span>}
                    <button className="icono" title="Editar (vuelve al cuadro de texto)" aria-label="Editar mensaje en cola" onClick={() => {
                      ponCola(sesion.id, k => C.quita(k, i));
                      const d = C.devuelve([c], texto, adjuntos, false);
                      setTexto(d.texto);
                      setAdjuntos(d.adjuntos);
                      area.current?.focus();
                    }}><Icono n="lapiz" t={13} /></button>
                    <button className="quita-adjunto" title="Quitar de la cola" aria-label="Quitar mensaje de la cola" onClick={() => ponCola(sesion.id, k => C.quita(k, i))}>×</button>
                  </li>
                ))}
                {!enCurso && colaSesion.parada && (
                  <li className="cola-parada suave pequeno">
                    Cola detenida: el último turno se paró o falló.
                    <button className="enlace" onClick={() => ponCola(sesion.id, C.empieza)}>Seguir con la cola</button>
                  </li>
                )}
              </ol>
            )}
            {adjuntos.length > 0 && (
              <div className="adjuntos-pendientes">
                {adjuntos.map(a => (
                  <span key={a.ruta} className={'ficha-adjunto' + (a.error ? ' error' : '')} title={a.error || a.ruta}>
                    <Icono n="clip" t={12} /> {a.nombre}{a.subiendo ? ' · subiendo…' : a.error ? ' · ⚠ ' + a.error : ` · ${a.tamano < 1024 * 1024 ? Math.ceil(a.tamano / 1024) + ' KB' : (a.tamano / 1048576).toFixed(1) + ' MB'}`}
                    <button className="quita-adjunto" onClick={() => setAdjuntos(l => l.filter(x => x.ruta !== a.ruta))} aria-label={'Quitar ' + a.nombre}>×</button>
                  </span>
                ))}
              </div>
            )}
            <div className={'entrada' + (arrastrando ? ' soltando' : '')}
              onDragOver={e => { if (e.dataTransfer.types.includes('Files')) { e.preventDefault(); setArrastrando(true); } }}
              onDragLeave={() => setArrastrando(false)}
              onDrop={e => { e.preventDefault(); setArrastrando(false); adjunta(Array.from(e.dataTransfer.files)); }}>
              {/* El agente, separado por una línea vertical de lo que es el mensaje (adjuntos y texto). */}
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
              <input ref={selector} type="file" multiple hidden onChange={e => { adjunta(Array.from(e.target.files || [])); e.target.value = ''; }} />
              <button className="icono adjuntar" onClick={() => selector.current?.click()} title="Adjuntar ficheros (también puedes arrastrarlos o pegar una imagen)" aria-label="Adjuntar ficheros" disabled={!sesion.hosting}>
                <Icono n="clip" t={17} />
              </button>
              <textarea ref={area} rows={1} value={texto} onChange={e => setTexto(e.target.value)} onKeyDown={teclas}
                onPaste={e => { const fs = Array.from(e.clipboardData.files); if (fs.length) { e.preventDefault(); adjunta(fs); } }}
                placeholder={!sesion.hosting ? 'Completa los ajustes de la sesión' : !prov?.disponible ? 'Elige un agente disponible' : enCurso ? 'Escribe; se enviará cuando termine el agente…' : 'Escribe…'}
                disabled={!prov?.disponible || !sesion.hosting} />
              {enCurso
                ? (texto.trim()
                  ? <button className="redondo primario" onClick={encola} disabled={subiendo} title="Poner en cola (Intro): se enviará cuando termine el agente" aria-label="Poner en cola"><Icono n="enviar" t={16} /></button>
                  : <button className="redondo" onClick={para} title={cola.length ? 'Parar (la cola vuelve al cuadro de texto)' : 'Parar'} aria-label="Parar"><Icono n="parar" t={16} /></button>)
                : <button className="redondo primario" onClick={() => envia()} disabled={!texto.trim() || !prov?.disponible || subiendo} title="Enviar (Intro)" aria-label="Enviar"><Icono n="enviar" t={16} /></button>}
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

const miles = (n: number) => n >= 1000 ? (n / 1000).toFixed(n >= 10000 ? 0 : 1) + 'k' : String(n);
const usoTexto = (u: { entrada: number; salida: number; coste?: number; segundos?: number }) =>
  [`${miles(u.entrada)} ↑ · ${miles(u.salida)} ↓ tokens`, u.coste ? `$${u.coste.toFixed(3)}` : '', u.segundos ? `${u.segundos.toFixed(1)} s` : ''].filter(Boolean).join(' · ');

// Una parte de la respuesta: texto, razonamiento (plegado) o una herramienta (una línea que se despliega), como en Claude Code.
// Las partes según el nivel de detalle del proyecto: conciso (solo texto y una línea con las acciones), normal
// (cada acción plegada) o detallado (todo desplegado).
function Partes({ partes, detalle }: { partes: Parte[]; detalle: Detalle }) {
  if (detalle !== 'conciso') return <>{partes.map((p, j) => <ParteMensaje key={j} p={p} abierta={detalle === 'detallado'} />)}</>;
  const acciones = partes.filter(p => p.tipo === 'herramienta' && !p.h.skill);
  const skills = partes.filter(p => p.tipo === 'herramienta' && p.h.skill);
  const enCurso = acciones.some(p => p.tipo === 'herramienta' && p.h.estado === 'curso');
  return <>
    {skills.map((p, j) => <ParteMensaje key={'s' + j} p={p} abierta={false} />)}
    {acciones.length > 0 && (
      <details className="acciones">
        <summary><span className={'punto-h' + (enCurso ? ' curso' : '')} /> {acciones.length} {acciones.length === 1 ? 'acción' : 'acciones'}</summary>
        {acciones.map((p, j) => <ParteMensaje key={j} p={p} abierta={false} />)}
      </details>
    )}
    {partes.filter(p => p.tipo === 'texto').map((p, j) => <ParteMensaje key={j} p={p} abierta={false} />)}
  </>;
}

function ParteMensaje({ p, abierta }: { p: Parte; abierta: boolean }) {
  if (p.tipo === 'texto') return <Texto md={p.texto} />;
  if (p.tipo === 'pensamiento') return <details className="pensamiento" open={abierta}><summary>Razonamiento</summary><div>{p.texto}</div></details>;
  const h = p.h;
  if (h.skill) return (
    <details className={'herramienta skill ' + h.estado} open={abierta}>
      <summary><span className="sk-marca">✦ Skill · {h.skill}</span>{h.estado === 'curso' && <span className="suave pequeno">cargando…</span>}</summary>
      {h.salida && <pre className="salida-h">{h.salida}</pre>}
    </details>
  );
  return (
    <details className={'herramienta ' + h.estado} open={abierta}>
      <summary>
        <span className="punto-h" aria-label={h.estado} />
        <b>{h.nombre}</b>{h.resumen && <span className="resumen-h recorta">{h.resumen}</span>}
      </summary>
      {h.entrada && <pre className="entrada-h">{h.entrada}</pre>}
      {h.salida && <pre className="salida-h">{h.salida}</pre>}
    </details>
  );
}
