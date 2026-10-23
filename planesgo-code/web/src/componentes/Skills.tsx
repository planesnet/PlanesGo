import { useEffect, useMemo, useState } from 'react';
import { get, post, type Eleccion, type Proyecto } from '../api';
import { conversacion, type Sesion } from '../sesiones';
import Icono from './Icono';
import { Agente } from './Marca';
import Texto from './Texto';

// Inventario de skills: qué hay instalado, qué agente ve cada una, cuánto cuesta tenerlas (la descripción se
// carga en cada conversación) y cuánto usarlas (el cuerpo de SKILL.md), cuáles se han usado y qué falla.

export interface SkillInfo {
  nombre: string; descripcion: string; ruta: string; ambito: 'personal' | 'proyecto' | 'plugin' | 'sincronizada';
  origen: string; agentes: string[]; tokens_fijo: number; tokens_uso: number; ficheros: number; avisos: string[]; plugin?: string; rota?: boolean;
}

const AGENTES = [{ id: 'claude-code', nombre: 'Claude Code' }, { id: 'opencode', nombre: 'OpenCode' }, { id: 'antigravity', nombre: 'Antigravity' }];
const AMBITOS: Record<string, string> = { proyecto: 'Proyecto', personal: 'Personal', plugin: 'Plugin', sincronizada: 'Sincronizada' };
const casa = (p: string) => p.replace(/^\/home\/[^/]+/, '~');
const k = (n: number) => n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n);

// Cuántas veces se ha usado cada skill en las conversaciones de las sesiones del proyecto.
export function usosDeSkills(sesiones: Sesion[]): Record<string, number> {
  const u: Record<string, number> = {};
  for (const s of sesiones) for (const m of conversacion(s.id)) {
    if (m.skill?.modo === 'elegida') u[m.skill.nombre] = (u[m.skill.nombre] || 0) + 1;
    for (const p of m.partes || []) if (p.tipo === 'herramienta' && p.h.skill) u[p.h.skill] = (u[p.h.skill] || 0) + 1;
  }
  return u;
}

export default function Skills({ proyecto, carpeta, agente, sesiones }: { proyecto: Proyecto; carpeta: string; agente?: string; sesiones: Sesion[] }) {
  const [l, setL] = useState<SkillInfo[] | null>(null);
  const [error, setError] = useState('');
  const [filtroAgente, setFiltroAgente] = useState<string>(agente && AGENTES.some(a => a.id === agente) ? agente : 'todos');
  const [busca, setBusca] = useState('');
  const [soloAvisos, setSoloAvisos] = useState(false);
  const [abierta, setAbierta] = useState<string | null>(null);
  const [texto, setTexto] = useState<{ ruta: string; md: string } | null>(null);
  // Mapa virtual de skills del agente de la sesión: al cambiar de agente, se ven las suyas.
  useEffect(() => { if (agente && AGENTES.some(a => a.id === agente)) setFiltroAgente(agente); }, [agente]);
  const nombreAgente = AGENTES.find(a => a.id === filtroAgente)?.nombre;

  const carga = () => {
    setError('');
    get<SkillInfo[]>(`/api/skills?proyecto=${proyecto.id}&carpeta=${encodeURIComponent(carpeta)}`).then(setL).catch(e => setError(e.message));
  };
  useEffect(carga, [proyecto.id, carpeta]); // eslint-disable-line react-hooks/exhaustive-deps

  const usos = useMemo(() => usosDeSkills(sesiones), [sesiones, l]);
  const validas = (l || []).filter(s => !s.rota);
  const resumen = AGENTES.map(a => {
    const de = validas.filter(s => s.agentes.includes(a.id));
    return { ...a, n: de.length, fijo: de.reduce((t, s) => t + s.tokens_fijo, 0) };
  });
  const visibles = (l || []).filter(s =>
    (filtroAgente === 'todos' || s.agentes.includes(filtroAgente)) && (!soloAvisos || s.avisos.length > 0) &&
    (!busca.trim() || (s.nombre + ' ' + s.descripcion).toLowerCase().includes(busca.trim().toLowerCase())));
  const grupos = ['proyecto', 'personal', 'plugin', 'sincronizada'].map(a => ({ a, l: visibles.filter(s => s.ambito === a) })).filter(g => g.l.length);

  const verTexto = async (s: SkillInfo) => {
    if (texto?.ruta === s.ruta) { setTexto(null); return; }
    try { const r = await get<{ texto: string }>(`/api/skills/texto?proyecto=${proyecto.id}&carpeta=${encodeURIComponent(carpeta)}&ruta=${encodeURIComponent(s.ruta)}`); setTexto({ ruta: s.ruta, md: r.texto }); }
    catch (e) { setError((e as Error).message); }
  };

  return (
    <div className="skills">
      <details className="sk-como">
        <summary><Icono n="info" t={15} /> Cómo decide el agente qué skill usar</summary>
        <ol>
          <li><b>Al empezar cada conversación</b> el agente solo carga el <b>nombre y la descripción</b> de las skills que ve (el «coste fijo», en tokens, en cada conversación).</li>
          <li>Con cada petición, <b>el propio modelo compara lo que se le pide con esas descripciones</b>. No hay reglas ni palabras clave: si una descripción encaja, decide usarla.</li>
          <li>Al usarla, <b>carga el cuerpo de SKILL.md</b> (el «coste al usarla»); y los demás ficheros de la skill, solo si los necesita.</li>
        </ol>
        <p>Por eso una buena descripción dice <b>qué hace y cuándo usarla</b> («Úsala cuando…»), y las skills que no se usan cuestan igual en cada conversación. Cuando el agente usa una skill, en el chat aparece <span className="sk-marca">✦ nombre</span>.</p>
      </details>

      <Probador proyecto={proyecto.id} carpeta={carpeta} agente={filtroAgente === 'todos' ? '' : filtroAgente} />

      <div className="sk-cabecera">
        {nombreAgente ? <><Agente id={filtroAgente} t={18} /> <b>Skills de {nombreAgente}</b></> : <b>Skills de todos los agentes</b>}
        <span className="hueco" />
        {filtroAgente !== 'todos'
          ? <button className="enlace" onClick={() => setFiltroAgente('todos')}>Ver todas</button>
          : agente && <button className="enlace" onClick={() => setFiltroAgente(agente)}>Solo las del agente de la sesión</button>}
      </div>
      <div className="sk-resumen">
        {resumen.map(a => (
          <button key={a.id} className={'sk-agente' + (filtroAgente === a.id ? ' elegido' : '') + (agente === a.id ? ' actual' : '')}
            onClick={() => setFiltroAgente(f => f === a.id ? 'todos' : a.id)} title={agente === a.id ? 'Agente de la sesión' : ''}>
            <Agente id={a.id} t={16} />
            <span><b>{a.n}</b> skills · <b>{k(a.fijo)}</b> tokens fijos</span>
          </button>
        ))}
      </div>

      <div className="sk-filtros">
        <input placeholder="Buscar skill…" value={busca} onChange={e => setBusca(e.target.value)} />
        <label className="casilla"><input type="checkbox" checked={soloAvisos} onChange={e => setSoloAvisos(e.target.checked)} /> Con avisos ({(l || []).filter(s => s.avisos.length).length})</label>
        <button className="icono" onClick={carga} title="Volver a buscar" aria-label="Recargar"><Icono n="recargar" t={15} /></button>
      </div>
      {error && <p className="error">{error}</p>}
      {!l && !error && <p className="suave">Buscando skills…</p>}

      {grupos.map(g => (
        <section key={g.a} className="sk-grupo">
          <p className="titulo-lista">{AMBITOS[g.a]} <span className="gh-cuenta">{g.l.length}</span></p>
          {g.l.map(s => (
            <div key={s.ruta} className={'sk' + (s.rota ? ' rota' : '') + (abierta === s.ruta ? ' abierta' : '')}>
              <button className="sk-fila" onClick={() => setAbierta(a => a === s.ruta ? null : s.ruta)}>
                <span className="sk-nombre recorta">{s.nombre}</span>
                {usos[s.nombre] ? <span className="sk-usos" title="Veces usada en este proyecto">✦ {usos[s.nombre]}</span> : null}
                {s.avisos.length > 0 && <span className="sk-aviso" title={s.avisos.join('\n')}>⚠ {s.avisos.length}</span>}
                <span className="sk-agentes">{s.agentes.map(a => <Agente key={a} id={a} t={13} />)}</span>
                {!s.rota && <span className="sk-tokens" title="Tokens fijos (descripción) en cada conversación">{k(s.tokens_fijo)}</span>}
              </button>
              {abierta === s.ruta && (
                <div className="sk-detalle">
                  {s.descripcion && <p>{s.descripcion}</p>}
                  {s.avisos.map(a => <p key={a} className="sk-aviso-linea">⚠ {a}</p>)}
                  <dl className="ficha">
                    <dt>Ruta</dt><dd><code>{casa(s.ruta)}</code></dd>
                    {s.plugin && <><dt>Plugin</dt><dd>{s.plugin}</dd></>}
                    <dt>La ven</dt><dd>{s.agentes.map(a => AGENTES.find(x => x.id === a)?.nombre).join(', ')}</dd>
                    {!s.rota && <><dt>Coste fijo</dt><dd>{s.tokens_fijo} tokens en cada conversación</dd>
                      <dt>Al usarla</dt><dd>{k(s.tokens_uso)} tokens{s.ficheros ? ` · ${s.ficheros} ${s.ficheros === 1 ? 'fichero más, solo si hace falta' : 'ficheros más, solo si hacen falta'}` : ''}</dd></>}
                    <dt>Usada</dt><dd>{usos[s.nombre] ? `${usos[s.nombre]} ${usos[s.nombre] === 1 ? 'vez' : 'veces'} en este proyecto` : 'nunca en este proyecto'}</dd>
                  </dl>
                  {!s.rota && <button className="enlace" onClick={() => verTexto(s)}>{texto?.ruta === s.ruta ? 'Ocultar SKILL.md' : 'Ver SKILL.md'}</button>}
                  {texto?.ruta === s.ruta && <div className="sk-texto"><Texto md={texto.md} /></div>}
                </div>
              )}
            </div>
          ))}
        </section>
      ))}
    </div>
  );
}

// Probar la elección: escribe una petición y mira qué skill elegiría pgocode (y con qué parecido).
function Probador({ proyecto, carpeta, agente }: { proyecto: number; carpeta: string; agente: string }) {
  const [texto, setTexto] = useState('');
  const [r, setR] = useState<Eleccion | null>(null);
  const [error, setError] = useState('');
  const [cargando, setCargando] = useState(false);
  const prueba = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!texto.trim()) return;
    setCargando(true); setError('');
    try { setR(await post<Eleccion>('/api/skills/elegir', { proyecto, carpeta, agente, texto })); }
    catch (x) { setError((x as Error).message); } finally { setCargando(false); }
  };
  return (
    <form className="sk-probador" onSubmit={prueba}>
      <div className="sk-probar">
        <input value={texto} onChange={e => setTexto(e.target.value)} placeholder="Prueba una petición: ¿qué skill se usaría?" />
        <button className="primario" disabled={cargando || !texto.trim()}>{cargando ? '…' : 'Probar'}</button>
      </div>
      {error && <p className="error">{error}</p>}
      {r && <div className="sk-ranking">
        <p className="suave pequeno">{r.elegida ? <>Se usaría <b>✦ {r.elegida.nombre}</b></> : 'Ninguna skill encaja con claridad'} · {r.ms} ms · {r.motor === 'ollama/embeddinggemma' ? 'en local (Ollama)' : 'Vertex AI'} · umbral {r.umbral.toFixed(2)}</p>
        {r.candidatos.map(c => (
          <div key={c.ruta} className={'sk-barra' + (c.puntuacion >= r.umbral ? ' pasa' : '')}>
            <span className="recorta">{c.nombre}</span>
            {/* Escala relativa al umbral (cada motor tiene la suya): el umbral queda en la mitad de la barra. */}
            <span className="sk-barra-fondo"><span style={{ width: `${Math.max(0, Math.min(1, c.puntuacion / (2 * r.umbral))) * 100}%` }} />
              <i style={{ left: '50%' }} /></span>
            <span className="sk-tokens">{c.puntuacion.toFixed(3)}</span>
          </div>
        ))}
      </div>}
    </form>
  );
}
