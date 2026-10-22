import { createContext, useContext, useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { get } from '../api';
import Icono from './Icono';

// Capturas de evidencia: las imágenes que devuelven las herramientas de los agentes (pruebas en el navegador…),
// guardadas por pgocode en la carpeta de la sesión. En el chat, miniaturas junto a cada herramienta (como en
// Claude) y las imágenes de las respuestas también en miniatura; en la TV, la galería del proyecto. Al pulsar, el
// visor a tamaño completo (el mismo en el chat y en la TV); en el chat, con todas las fotos de la conversación.

export interface Captura { ruta: string; t: string; sesion: string; titulo?: string; herramienta?: string; resumen?: string; agente?: string; rama?: string }

// El proyecto del chat (las capturas se piden con él).
export const ProyectoCapturas = createContext(0);

export const urlCaptura = (proyecto: number, ruta: string) => `/api/evidencia?proyecto=${proyecto}&ruta=${encodeURIComponent(ruta)}`;

// Una foto para el visor: su dirección y el pie (qué es).
export interface FotoVisor { src: string; pie?: string }

// El visor de la conversación: abre una foto (por su dirección) con todas las de la conversación alrededor. Sin
// él (fuera del chat), cada miniatura abre su propio visor.
export const VisorConversacion = createContext<((src: string) => void) | null>(null);

// Miniaturas (como mucho `max`; el resto, «+N»).
export function Miniaturas({ rutas, max = 3 }: { rutas?: string[]; max?: number }) {
  const proyecto = useContext(ProyectoCapturas);
  const conversacion = useContext(VisorConversacion);
  const [visor, setVisor] = useState<number | null>(null);
  if (!rutas?.length || !proyecto) return null;
  const abre = (i: number) => conversacion ? conversacion(urlCaptura(proyecto, rutas[Math.min(i, rutas.length - 1)])) : setVisor(i);
  return (
    <span className="miniaturas" onClick={e => e.preventDefault()}>
      {rutas.slice(0, max).map((r, i) => (
        <button key={r} className="miniatura" onClick={e => { e.preventDefault(); e.stopPropagation(); abre(i); }} title="Ver la captura">
          <img src={urlCaptura(proyecto, r)} alt="Captura" loading="lazy" />
        </button>
      ))}
      {rutas.length > max && <span className="mas-miniaturas" onClick={e => { e.preventDefault(); e.stopPropagation(); abre(max); }}>+{rutas.length - max}</span>}
      {visor !== null && <Visor fotos={rutas.map(r => ({ src: urlCaptura(proyecto, r) }))} inicial={visor} alCerrar={() => setVisor(null)} />}
    </span>
  );
}

// Visor a tamaño completo, con anterior y siguiente (flechas del teclado) y cerrar (Escape).
export function Visor({ fotos, inicial, alCerrar }: { fotos: FotoVisor[]; inicial: number; alCerrar: () => void }) {
  const [i, setI] = useState(inicial);
  const c = fotos[i];
  useEffect(() => {
    const tecla = (e: KeyboardEvent) => {
      if (e.key === 'Escape') alCerrar();
      if (e.key === 'ArrowLeft') setI(x => Math.max(0, x - 1));
      if (e.key === 'ArrowRight') setI(x => Math.min(fotos.length - 1, x + 1));
    };
    document.addEventListener('keydown', tecla);
    return () => document.removeEventListener('keydown', tecla);
  }, [alCerrar, fotos.length]);
  if (!c) return null;
  return createPortal(
    <div className="fondo-modal visor-capturas" onMouseDown={e => { if (e.target === e.currentTarget) alCerrar(); }}>
      <figure>
        <img src={c.src} alt={c.pie || 'Foto'} />
        <figcaption>
          <button className="icono" onClick={() => setI(x => Math.max(0, x - 1))} disabled={i === 0} aria-label="Anterior"><Icono n="atras" t={16} /></button>
          <span className="recorta">{i + 1} / {fotos.length}{c.pie ? ' · ' + c.pie : ''}</span>
          <button className="icono" onClick={() => setI(x => Math.min(fotos.length - 1, x + 1))} disabled={i === fotos.length - 1} aria-label="Siguiente"><span className="espejo"><Icono n="atras" t={16} /></span></button>
          <button className="icono" onClick={alCerrar} aria-label="Cerrar"><Icono n="cerrar" t={16} /></button>
        </figcaption>
      </figure>
    </div>,
    document.body,
  );
}

const fecha = (t: string) => new Date(t).toLocaleString('es-ES', { dateStyle: 'short', timeStyle: 'short' });
const dia = (t: string) => new Date(t).toLocaleDateString('es-ES', { weekday: 'long', day: 'numeric', month: 'long' });

// Galería de la TV: las capturas del proyecto, por día y sesión.
export function Galeria({ proyecto }: { proyecto: number }) {
  const [l, setL] = useState<Captura[] | null>(null);
  const [error, setError] = useState('');
  const [visor, setVisor] = useState<number | null>(null);
  const carga = () => get<Captura[]>(`/api/evidencias?proyecto=${proyecto}`).then(setL).catch(e => setError(e.message));
  useEffect(() => { carga(); }, [proyecto]); // eslint-disable-line react-hooks/exhaustive-deps
  if (error) return <p className="error relleno">{error}</p>;
  if (!l) return <p className="suave relleno">Cargando capturas…</p>;
  if (!l.length) return <p className="suave relleno">Aún no hay capturas. Aparecen aquí las imágenes que devuelven los agentes al probar (capturas del navegador…).</p>;
  const grupos: { clave: string; dia: string; titulo: string; capturas: { c: Captura; i: number }[] }[] = [];
  l.forEach((c, i) => {
    const clave = c.t.slice(0, 10) + '|' + c.sesion;
    let g = grupos.find(x => x.clave === clave);
    if (!g) { g = { clave, dia: dia(c.t), titulo: c.titulo || c.sesion, capturas: [] }; grupos.push(g); }
    g.capturas.push({ c, i });
  });
  return (
    <div className="galeria-capturas">
      <div className="galeria-cabecera"><span className="suave pequeno">{l.length} capturas</span>
        <button className="icono" onClick={carga} title="Volver a cargar" aria-label="Volver a cargar"><Icono n="recargar" t={14} /></button></div>
      {grupos.map(g => (
        <section key={g.clave}>
          <h4><span>{g.dia}</span> · {g.titulo}</h4>
          <div className="galeria-rejilla">
            {g.capturas.map(({ c, i }) => (
              <button key={c.ruta} className="galeria-item" onClick={() => setVisor(i)} title={(c.herramienta || '') + (c.resumen ? ' · ' + c.resumen : '') + '\n' + fecha(c.t)}>
                <img src={urlCaptura(proyecto, c.ruta)} alt="Captura" loading="lazy" />
                <span className="recorta">{new Date(c.t).toLocaleTimeString('es-ES', { hour: '2-digit', minute: '2-digit' })} · {c.herramienta?.replace(/^mcp__[^_]+__/, '')}</span>
              </button>
            ))}
          </div>
        </section>
      ))}
      {visor !== null && <Visor fotos={l.map(c => ({ src: urlCaptura(proyecto, c.ruta), pie: [c.titulo, c.herramienta, c.t && fecha(c.t)].filter(Boolean).join(' · ') }))}
        inicial={visor} alCerrar={() => setVisor(null)} />}
    </div>
  );
}

// FotoRespuesta: una imagen que el agente pone en su respuesta (Markdown), en miniatura; al pulsar, el visor. Si no
// se puede cargar (una ruta del equipo, una web caída), su texto.
export function FotoRespuesta({ src, alt }: { src?: string; alt?: string }) {
  const conversacion = useContext(VisorConversacion);
  const [visor, setVisor] = useState(false);
  const [rota, setRota] = useState(false);
  if (!src || rota) return <span className="suave">[{alt || 'imagen'}{src ? ': ' + src : ''}]</span>;
  return (
    <>
      <button className="miniatura-respuesta" title={(alt ? alt + ' · ' : '') + 'Ver la imagen'} onClick={() => conversacion ? conversacion(src) : setVisor(true)}>
        <img src={src} alt={alt || 'Imagen'} loading="lazy" onError={() => setRota(true)} />
      </button>
      {visor && <Visor fotos={[{ src, pie: alt }]} inicial={0} alCerrar={() => setVisor(false)} />}
    </>
  );
}
