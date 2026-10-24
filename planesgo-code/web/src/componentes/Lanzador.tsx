import { useEffect, useRef, useState } from 'react';
import { get, openOutside, post, type ConfigPrograma, type Proceso, type Proyecto } from '../api';
import * as L from '../lanzador';
import Icono from './Icono';
import AgentScreen from './AgentScreen';
import { openRemoteDesktop } from '../remoteConnections';
import type { AgentDesktopView } from '../agentDesktop';

// Barra de prueba encima del cuadro del chat: lanza lo que se está desarrollando en la rama de la sesión.
// A la izquierda, dónde (navegador interno, navegador del sistema o un programa del proyecto; en la misma lista, las
// pantallas que no son una web: el escritorio remoto por RDP o AnyDesk y la del escritorio del agente); después la
// dirección, como un enlace, y el botón de abrir. En el navegador interno, además, el formato de la página
// (ordenador, tablet, móvil) y su orientación. La elección se recuerda por proyecto.

export type AbreWeb = (url: string, formato?: { dispositivo: L.Dispositivo; orientacion: L.Orientacion }) => void;

const clave = (proyecto: number) => 'pgo:prueba:' + proyecto;

const DESTINOS: Record<'interno' | 'sistema', { icono: string; nombre: string }> = {
  interno: { icono: 'web', nombre: 'Navegador interno' },
  sistema: { icono: 'externo', nombre: 'Navegador del sistema' },
};

// Formato de la página en el navegador interno: un solo selector (ordenador; tablet o móvil, en vertical u
// horizontal), con el icono del formato elegido.
const FORMATOS: { d: L.Dispositivo; o: L.Orientacion; nombre: string }[] = [
  { d: 'ordenador', o: 'horizontal', nombre: 'Ordenador' },
  { d: 'tablet', o: 'vertical', nombre: 'Tablet vertical' }, { d: 'tablet', o: 'horizontal', nombre: 'Tablet horizontal' },
  { d: 'movil', o: 'vertical', nombre: 'Móvil vertical' }, { d: 'movil', o: 'horizontal', nombre: 'Móvil horizontal' },
];

export function SelectorFormato({ dispositivo, orientacion, alCambiar }: {
  dispositivo: L.Dispositivo; orientacion: L.Orientacion; alCambiar: (d: L.Dispositivo, o: L.Orientacion) => void;
}) {
  const [abierto, setAbierto] = useState(false);
  const caja = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (!abierto) return;
    const fuera = (e: MouseEvent) => { if (!caja.current?.contains(e.target as Node)) setAbierto(false); };
    document.addEventListener('mousedown', fuera);
    return () => document.removeEventListener('mousedown', fuera);
  }, [abierto]);
  const o = dispositivo === 'ordenador' ? 'horizontal' : orientacion;
  const actual = FORMATOS.find(f => f.d === dispositivo && f.o === o) || FORMATOS[0];
  const icono = (f: { d: L.Dispositivo; o: L.Orientacion }) => <span className={'icono-formato' + (f.d !== 'ordenador' && f.o === 'horizontal' ? ' girado' : '')}><Icono n={f.d} t={15} /></span>;
  return (
    <span className="selector-formato" ref={caja}>
      <button type="button" className="lanzador-elige" onClick={() => setAbierto(a => !a)} title={'Formato: ' + actual.nombre} aria-label={'Formato: ' + actual.nombre} aria-haspopup="listbox" aria-expanded={abierto}>
        {icono(actual)}<Icono n="bajar" t={10} />
      </button>
      {abierto && (
        <ul className="menu-lanzador" role="listbox" aria-label="Formato de la página">
          {FORMATOS.map(f => (
            <li key={f.nombre}><button role="option" aria-selected={f === actual} onClick={() => { alCambiar(f.d, f.o); setAbierto(false); }}>
              {icono(f)} {f.nombre}{f === actual && <Icono n="hecho" t={13} />}
            </button></li>
          ))}
        </ul>
      )}
    </span>
  );
}

export default function Lanzador({ proyecto, carpeta, version, propuesta, alAbrirWeb, sesion }: {
  proyecto: Proyecto; carpeta: string; version: number; alAbrirWeb: AbreWeb;
  sesion?: string; // the work session: its programs get its access (Programas)
  propuesta?: string; // dirección local que acaba de dar el agente: se rellena (sin abrirla) para que el usuario la abra
}) {
  const [p, setP] = useState<L.Prueba>(() => { try { return L.leePrueba(localStorage.getItem(clave(proyecto.id))); } catch { return { ...L.PRUEBA_DEFECTO }; } });
  useEffect(() => { try { setP(L.leePrueba(localStorage.getItem(clave(proyecto.id)))); } catch { /* sin almacenamiento */ } }, [proyecto.id]);
  const cambia = (c: Partial<L.Prueba>) => setP(x => {
    const n = { ...x, ...c };
    try { localStorage.setItem(clave(proyecto.id), JSON.stringify(n)); } catch { /* sin almacenamiento */ }
    return n;
  });
  const [url, setUrl] = useState(p.url);
  useEffect(() => setUrl(p.url), [p.url]);
  // Dirección que da el agente: rellena la barra (se resalta un momento); la abre el usuario.
  const [detectada, setDetectada] = useState(false);
  useEffect(() => {
    if (!propuesta || propuesta === p.url) return;
    cambia({ url: propuesta, ...(p.destino === 'programa' ? { destino: 'interno' as const } : {}) });
    setDetectada(true);
    const t = setTimeout(() => setDetectada(false), 6000);
    return () => clearTimeout(t);
  }, [propuesta]); // eslint-disable-line react-hooks/exhaustive-deps

  // Programas del proyecto (los guardados y los que están en marcha) para proponer direcciones y ejecutarlos.
  const [procs, setProcs] = useState<Proceso[]>([]);
  const [cfg, setCfg] = useState<ConfigPrograma[]>([]);
  const [error, setError] = useState('');
  const carga = () => {
    if (!carpeta) return;
    get<{ procesos: Proceso[]; configuraciones: ConfigPrograma[] }>(`/api/procesos?proyecto=${proyecto.id}&carpeta=${encodeURIComponent(carpeta)}`)
      .then(r => { setProcs(r.procesos); setCfg(r.configuraciones); }).catch(() => {});
  };
  useEffect(carga, [proyecto.id, carpeta, version]); // eslint-disable-line react-hooks/exhaustive-deps

  const [menu, setMenu] = useState(false);
  const caja = useRef<HTMLDivElement>(null);
  // The session's agent desktop, if it is running: its screen is offered in the list (looked at when it opens).
  const [agentScreen, setAgentScreen] = useState<'no' | 'si' | 'abierta'>('no');
  useEffect(() => {
    if (!menu || !sesion) return;
    get<AgentDesktopView>(`/api/agent-desktop?project=${proyecto.id}&session=${encodeURIComponent(sesion)}`)
      .then(v => setAgentScreen(a => a === 'abierta' ? a : v.running ? 'si' : 'no')).catch(() => {});
  }, [menu, sesion, proyecto.id]);


  const enMarcha = procs.filter(x => x.estado === 'curso');
  const sug = L.sugerencias(enMarcha.map(x => x.url), cfg.map(c => c.url), p.url);
  const destino = p.destino === 'sistema' ? 'sistema' : 'interno';

  // La ejecución del proyecto: sus programas (los guardados y los que están en marcha, también los que dejó el
  // agente y adoptó pgocode), que pgocode arranca y para fuera del agente, sin tokens; y la dirección que se abre.
  const programas = [...cfg.map(c => ({ ...c, guardado: true })),
    ...procs.filter(x => !cfg.some(c => c.nombre === x.nombre)).map(x => ({ nombre: x.nombre, orden: x.orden, url: x.url, guardado: false }))];
  const estadoDe = (n: string) => procs.find(x => x.nombre === n);
  const vivos = programas.filter(c => estadoDe(c.nombre)?.estado === 'curso').length;
  const [verProgramas, setVerProgramas] = useState(false);
  useEffect(() => {
    if (!menu && !verProgramas) return;
    const fuera = (e: MouseEvent) => { if (!caja.current?.contains(e.target as Node)) { setMenu(false); setVerProgramas(false); } };
    document.addEventListener('mousedown', fuera);
    return () => document.removeEventListener('mousedown', fuera);
  }, [menu, verProgramas]);

  const abre = async (u = url) => {
    const n = L.normalizaUrl(u);
    if (!n) { setError('Dirección no válida'); return; }
    setError('');
    if (n !== p.url) cambia({ url: n });
    setUrl(n);
    if (destino === 'sistema') {
      try {
        await openOutside(n);
      } catch (e) { setError((e as Error).message); }
    } else {
      alAbrirWeb(n, { dispositivo: p.dispositivo, orientacion: p.orientacion });
    }
  };

  const lanza = async (c: { nombre: string; orden: string; url?: string }) => {
    await post('/api/procesos/lanzar', { proyecto: proyecto.id, carpeta, nombre: c.nombre, orden: c.orden, url: c.url, sesion });
  };
  const para = async (nombre: string) => { await post('/api/procesos/parar', { nombre }); };
  const haz = async (f: () => Promise<unknown>) => {
    setError('');
    try { await f(); } catch (e) { setError((e as Error).message); }
    carga();
  };
  // Ejecutar: arranca los programas guardados que no estén en marcha (cada uno abre su dirección si la tiene).
  const ejecutaTodo = () => haz(async () => { for (const c of cfg) if (estadoDe(c.nombre)?.estado !== 'curso') await lanza(c); });
  const paraTodo = () => haz(async () => { for (const c of programas) if (estadoDe(c.nombre)?.estado === 'curso') await para(c.nombre); });
  const guarda = (c: { nombre: string; orden: string; url?: string }) =>
    haz(() => post('/api/procesos/guardar', { proyecto: proyecto.id, carpeta, nombre: c.nombre.replace(/^agente-/, ''), orden: c.orden, url: c.url || '' }));

  return (
    <div className="lanzador" ref={caja}>
      {programas.length > 0 && <>
        <div className="lanzador-programas">
          {vivos > 0
            ? <button className="icono" onClick={paraTodo} title={`Parar los programas en marcha (${vivos})`} aria-label="Parar los programas"><Icono n="parar" t={13} /></button>
            : <button className="icono" onClick={ejecutaTodo} disabled={!cfg.length || !carpeta} title={cfg.length ? 'Ejecutar los programas del proyecto' : 'No hay programas guardados'} aria-label="Ejecutar los programas"><Icono n="ejecutar" t={13} /></button>}
          <button className="lanzador-cuenta" onClick={() => setVerProgramas(v => !v)} aria-expanded={verProgramas} title="Programas del proyecto">
            <span className={'punto ' + (vivos ? 'ok' : '')} /> {vivos}/{programas.length}<Icono n="bajar" t={10} />
          </button>
          {verProgramas && (
            <ul className="menu-lanzador menu-programas" aria-label="Programas del proyecto">
              <li className="menu-titulo">Programas del proyecto (fuera del agente, sin tokens)</li>
              {programas.map(c => {
                const x = estadoDe(c.nombre);
                const vivo = x?.estado === 'curso';
                return (
                  <li key={c.nombre} className="programa-lanzador">
                    <span className={'punto ' + (vivo ? 'ok' : x?.estado === 'error' ? 'fallo' : '')} title={x?.estado || 'parado'} />
                    <span className="programa-lanzador-texto">
                      <b className="recorta">{c.nombre}{x?.lanzado === 'agente' && <span className="etiqueta">agente</span>}</b>
                      <code className="recorta" title={c.orden}>{c.orden}</code>
                    </span>
                    {vivo && x?.url && <button className="icono" onClick={() => abre(x.url)} title={'Abrir ' + x.url} aria-label="Abrir su dirección"><Icono n="web" t={13} /></button>}
                    {!c.guardado && <button className="enlace pequeno" onClick={() => guarda(c)} title="Guardarlo en el proyecto (.psf/ejecutar.json): la próxima vez lo arranca pgocode, sin el agente">Guardar</button>}
                    {vivo
                      ? <button className="icono" onClick={() => haz(() => para(c.nombre))} title="Parar" aria-label={'Parar ' + c.nombre}><Icono n="parar" t={12} /></button>
                      : <button className="icono" onClick={() => haz(() => lanza(c))} disabled={!carpeta} title="Ejecutar" aria-label={'Ejecutar ' + c.nombre}><Icono n="ejecutar" t={12} /></button>}
                  </li>
                );
              })}
            </ul>
          )}
        </div>
        <span className="lanzador-sep" />
      </>}
      {destino === 'interno' && <SelectorFormato dispositivo={p.dispositivo} orientacion={p.orientacion}
        alCambiar={(d, o) => { cambia({ dispositivo: d, orientacion: o }); if (L.normalizaUrl(url)) alAbrirWeb(L.normalizaUrl(url), { dispositivo: d, orientacion: o }); }} />}
      <div className="lanzador-destino">
        <button type="button" className="lanzador-elige" onClick={() => setMenu(m => !m)} title={DESTINOS[destino].nombre + ' (cambiar)'} aria-label={'Abrir en: ' + DESTINOS[destino].nombre} aria-haspopup="listbox" aria-expanded={menu}>
          <Icono n={DESTINOS[destino].icono} t={15} /><Icono n="bajar" t={10} />
        </button>
        {menu && (
          <ul className="menu-lanzador" role="listbox" aria-label="Dónde abrir">
            {(['interno', 'sistema'] as const).map(d => (
              <li key={d}><button role="option" aria-selected={destino === d} onClick={() => { cambia({ destino: d }); setMenu(false); }}>
                <Icono n={DESTINOS[d].icono} t={15} /> {DESTINOS[d].nombre}{destino === d && <Icono n="hecho" t={13} />}
              </button></li>
            ))}
            <li className="menu-sep" role="separator" />
            <li><button onClick={() => { openRemoteDesktop('rdp'); setMenu(false); }} title="La ventana del escritorio remoto, en Windows (RDP)">
              <Icono n="equipo" t={15} /> Escritorio remoto (RDP)
            </button></li>
            <li><button onClick={() => { openRemoteDesktop('anydesk'); setMenu(false); }} title="La ventana del escritorio remoto, en AnyDesk">
              <Icono n="conectar" t={15} /> AnyDesk
            </button></li>
            {agentScreen !== 'no' && sesion && <li><button onClick={() => { setAgentScreen('abierta'); setMenu(false); }} title="Ver y usar la pantalla del escritorio del agente de esta sesión">
              <Icono n="ordenador" t={15} /> Pantalla del agente
            </button></li>}
          </ul>
        )}
        {agentScreen === 'abierta' && sesion && <AgentScreen project={proyecto.id} session={sesion} onClose={() => setAgentScreen('si')} />}
      </div>
      <form className={'lanzador-url' + (detectada ? ' detectada' : '')} title={detectada ? 'Dirección que acaba de dar el agente: pulsa abrir' : undefined} onSubmit={e => { e.preventDefault(); abre(); }}>
        {/* Sugerencias solo si hay otra dirección que elegir (si no, sin la flecha del desplegable). */}
        <input value={url} onChange={e => setUrl(e.target.value)} list={sug.some(x => x !== L.normalizaUrl(url)) ? 'sug-prueba-' + proyecto.id : undefined} spellCheck={false}
          size={Math.min(48, Math.max(url ? url.length + 2 : 30, 12))}
          placeholder="Dirección de la prueba (p. ej. localhost:8888)" aria-label="Dirección de la prueba" />
        <datalist id={'sug-prueba-' + proyecto.id}>{sug.map(s => <option key={s} value={s} />)}</datalist>
        <button type="submit" className="icono" disabled={!url.trim()} title={destino === 'sistema' ? 'Abrir en el navegador del sistema' : 'Abrir en el navegador interno'} aria-label="Abrir">
          <Icono n="ejecutar" t={14} />
        </button>
      </form>
      {error && <span className="error pequeno">{error}</span>}
    </div>
  );
}
