import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { chat, get, type Mensaje, type Proveedor, type Proyecto } from '../api';

interface Conversacion { mensajes: Mensaje[]; sesionAgente: string }

interface Props { proyecto: Proyecto | null; panelAbierto: boolean; alPanel: () => void }

const VACIA: Conversacion = { mensajes: [], sesionAgente: '' };

// Conversaciones por proyecto y proveedor: en memoria y en sessionStorage, para que F5 no las pierda.
const conversaciones = new Map<string, Conversacion>();
const claveAlmacen = (k: string) => 'psf:conv:' + k;

function leeConversacion(k: string): Conversacion {
  const c = conversaciones.get(k);
  if (c) return c;
  try {
    const v = sessionStorage.getItem(claveAlmacen(k));
    if (v) {
      const c2 = JSON.parse(v) as Conversacion;
      conversaciones.set(k, c2);
      return c2;
    }
  } catch { /* sin almacenamiento: solo memoria */ }
  return VACIA;
}

function persiste(k: string, c: Conversacion) {
  try {
    if (c.mensajes.length) sessionStorage.setItem(claveAlmacen(k), JSON.stringify(c));
    else sessionStorage.removeItem(claveAlmacen(k));
  } catch { /* lleno o no disponible: se queda en memoria */ }
}

export default function Chat({ proyecto, panelAbierto, alPanel }: Props) {
  const [proveedores, setProveedores] = useState<Proveedor[]>([]);
  const [provId, setProvId] = useState('');
  const [modelo, setModelo] = useState('');
  const [carpeta, setCarpeta] = useState('');
  const [texto, setTexto] = useState('');
  const [conv, setConv] = useState<Conversacion>(VACIA);
  const [enCurso, setEnCurso] = useState<AbortController | null>(null);
  const fin = useRef<HTMLDivElement>(null);
  const lista = useRef<HTMLDivElement>(null);

  useEffect(() => {
    get<Proveedor[]>('/api/proveedores').then(l => {
      setProveedores(l);
      let preferido = '';
      try { preferido = localStorage.getItem('psf:proveedor') || ''; } catch { /* sin almacenamiento */ }
      const p = l.find(x => x.id === preferido && x.disponible) || l.find(x => x.disponible) || l[0];
      if (p) { setProvId(p.id); setModelo(p.por_defecto); }
    }).catch(() => {});
  }, []);

  const prov = proveedores.find(p => p.id === provId);
  const clave = proyecto ? proyecto.id + '|' + provId : '';
  const claveActual = useRef(clave);

  // Al cambiar de proyecto o proveedor se corta la respuesta en curso: así no se escribe en otra conversación.
  useEffect(() => {
    claveActual.current = clave;
    setConv(clave ? leeConversacion(clave) : VACIA);
    setCarpeta(proyecto?.carpetas[0] || '');
    return () => { setEnCurso(ac => { ac?.abort(); return null; }); };
  }, [clave, proyecto]);

  // Solo se baja al final si el usuario ya estaba abajo (si está leyendo arriba, no se le mueve).
  useEffect(() => {
    const l = lista.current;
    if (!l || l.scrollHeight - l.scrollTop - l.clientHeight < 120) fin.current?.scrollIntoView({ block: 'end' });
  }, [conv]);

  const modelos = useMemo(() => prov?.modelos?.length ? prov.modelos : (prov?.por_defecto ? [prov.por_defecto] : []), [prov]);

  async function envia() {
    const t = texto.trim();
    if (!t || !proyecto || !prov || enCurso) return;
    const k = clave;
    const guarda = (c: Conversacion, final = false) => {
      conversaciones.set(k, c);
      if (claveActual.current === k) setConv(c);
      if (final) persiste(k, c);
    };
    const base: Mensaje[] = [...conv.mensajes.filter(m => !m.error), { rol: 'user', texto: t }];
    let actual: Conversacion = { mensajes: [...base, { rol: 'assistant', texto: '' }], sesionAgente: conv.sesionAgente };
    guarda(actual, true);
    setTexto('');
    const ac = new AbortController();
    setEnCurso(ac);
    const pon = (f: (m: Mensaje) => Mensaje, sesion?: string) => {
      const ms = actual.mensajes.slice();
      ms[ms.length - 1] = f(ms[ms.length - 1]);
      actual = { mensajes: ms, sesionAgente: sesion ?? actual.sesionAgente };
      guarda(actual);
    };
    // Los agentes llevan su propio historial: solo se les manda el último mensaje.
    const mensajes = prov.tipo === 'agente' ? base.slice(-1) : base;
    try {
      await chat({ proyecto: proyecto.id, proveedor: prov.id, modelo, carpeta, sesion_agente: conv.sesionAgente, mensajes },
        e => {
          if (e.texto) pon(m => ({ ...m, texto: m.texto + e.texto }));
          if (e.sesion_agente) pon(m => m, e.sesion_agente);
          if (e.error) pon(m => ({ ...m, texto: (m.texto ? m.texto + '\n\n' : '') + '⚠ ' + e.error, error: true }));
        }, ac.signal);
    } catch (err) {
      if (!ac.signal.aborted) pon(m => ({ ...m, texto: '⚠ ' + (err as Error).message, error: true }));
    } finally {
      guarda(actual, true);
      setEnCurso(c => (c === ac ? null : c));
    }
  }

  const eligeProveedor = (id: string) => {
    const p = proveedores.find(x => x.id === id);
    setProvId(id);
    setModelo(p?.por_defecto || '');
    try { localStorage.setItem('psf:proveedor', id); } catch { /* sin almacenamiento */ }
  };

  const nueva = () => {
    conversaciones.set(clave, VACIA);
    persiste(clave, VACIA);
    setConv(VACIA);
  };

  const teclas = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); envia(); }
  };

  return (
    <main className="chat">
      <header className="cabecera">
        <div className="titulo">
          {proyecto ? <><b>{proyecto.nombre}</b><span className="suave"> · Odoo #{proyecto.id}</span></> : <span className="suave">Elige un proyecto de Odoo</span>}
        </div>
        <button className="icono" onClick={alPanel} title={panelAbierto ? 'Ocultar artefactos' : 'Mostrar artefactos'} aria-pressed={panelAbierto}>▤</button>
      </header>

      {!proyecto ? (
        <div className="vacio">
          <h2>Sin proyecto, no hay trabajo</h2>
          <p>Todo en PSFCode va ligado a un proyecto de Odoo. Elige uno a la izquierda para hablar con un modelo o un agente, abrir enlaces o ver las pruebas en directo.</p>
        </div>
      ) : (
        <>
          <div className="mensajes" aria-live="polite" ref={lista}>
            {conv.mensajes.length === 0 && (
              <div className="vacio pequeno">
                <p>Habla con {prov?.nombre || 'un modelo'} sobre <b>{proyecto.nombre}</b>.</p>
                {prov?.tipo === 'agente' && <p className="suave">El agente trabaja en la carpeta del proyecto y lleva su propio historial.</p>}
              </div>
            )}
            {conv.mensajes.map((m, i) => (
              <div key={i} className={'mensaje ' + m.rol + (m.error ? ' con-error' : '')}>
                {m.texto || (enCurso && i === conv.mensajes.length - 1 ? <span className="pensando">Pensando…</span> : '')}
              </div>
            ))}
            <div ref={fin} />
          </div>
          <div className="redactor">
            <div className="opciones">
              <select value={provId} onChange={e => eligeProveedor(e.target.value)} aria-label="Proveedor">
                {proveedores.map(p => <option key={p.id} value={p.id} disabled={!p.disponible}>{p.nombre}{p.disponible ? '' : ' — ' + p.motivo}</option>)}
              </select>
              {prov?.tipo === 'api' && (
                <select value={modelo} onChange={e => setModelo(e.target.value)} aria-label="Modelo">
                  {modelos.map(m => <option key={m}>{m}</option>)}
                </select>
              )}
              {prov?.necesita_carpeta && (
                proyecto.carpetas.length
                  ? <select value={carpeta} onChange={e => setCarpeta(e.target.value)} aria-label="Carpeta">{proyecto.carpetas.map(c => <option key={c}>{c}</option>)}</select>
                  : <span className="error pequeno">Sin carpeta local (.planesgo.json)</span>
              )}
              {conv.mensajes.length > 0 && !enCurso && <button className="enlace" onClick={nueva}>Nueva conversación</button>}
            </div>
            <div className="entrada">
              <textarea rows={3} value={texto} onChange={e => setTexto(e.target.value)} onKeyDown={teclas}
                placeholder={prov?.disponible ? 'Escribe… (Intro envía, Mayús+Intro salto de línea)' : 'Este proveedor no está disponible'} disabled={!prov?.disponible} />
              {enCurso
                ? <button onClick={() => enCurso.abort()}>Parar</button>
                : <button className="primario" onClick={envia} disabled={!texto.trim() || !prov?.disponible}>Enviar</button>}
            </div>
          </div>
        </>
      )}
    </main>
  );
}
