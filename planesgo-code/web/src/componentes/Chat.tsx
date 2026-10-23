import { createContext, Fragment, memo, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { chat, get, post, preparaHija, proponeReparto, subeAdjunto, uneHija, type Adjunto, type Eleccion, type Herramienta, type Mensaje, type Parte, type Proveedor, type Proyecto, type SkillPgo, type Uso } from '../api';
import type { Detalle, SeleccionSkills } from '../ajustes';
import { conversacion, guardaConversacion, nombreDe, titulo, type Repo, type Sesion } from '../sesiones';
import * as C from '../cola';
import * as PL from '../plan';
import * as T from '../tareas';
import * as TS from '../todoSync';
import TodoSyncList, { todoCount } from './TodoSyncList';
import * as CO from '../costes';
import { autoContextOn, contextPolicy, lastContext, relayAsBackup } from '../contextPolicy';
import * as CL from '../cleaner';
import * as CS from '../cacheStatus';
import CleanerIndicator from './CleanerIndicator';
import DirectNote from './DirectNote';
import * as AP from '../aparte';
import * as DR from '../direct';
import * as W from '../workingMemory';
import * as PR from '../prune';
import * as RP from '../reparto';
import * as R from '../router';
import { TarjetaPropuesta, TarjetaReparto } from './TarjetasReparto';
import { direccionesEn } from '../lanzador';
import { borraPlan, EditorPlan, guardaPlan, leePlan, ListaPlanes, listaPlanes, TarjetaPlan, type EstadoPlan, type Plan } from './Planes';
import { Miniaturas, ProyectoCapturas, urlCaptura, Visor, VisorConversacion, type FotoVisor } from './Capturas';
import { fotosDe } from '../fotos';
import { withUnanswered } from '../unanswered';
import { actividad, consumoPorAccion, currentAction, silenceLabel, toolStatusTitle } from '../activity';
import * as LA from '../liveActivity';
import Icono from './Icono';
import { Ayuda } from './Ayuda';
import * as IH from '../inputHistory';
import Lanzador, { type AbreWeb } from './Lanzador';
import Texto from './Texto';
import { Agente, Hosting, nombreHosting } from './Marca';

interface Props {
  detalle: Detalle;
  seleccionSkills: SeleccionSkills;
  hosting?: string; // alojamiento elegido en las propiedades del proyecto (si no, el del repositorio)
  directorio?: string; // directorio local del proyecto (sus ajustes); ahí se guardan los planes
  versionProcesos: number; alAbrirWeb: AbreWeb; // barra de prueba: programas del proyecto y abrir la web
  // Mensaje que hay que enviar solo en cuanto la sesión indicada esté abierta (p. ej. «Resuelve la issue…»).
  envioPendiente: { sesion: string; texto: string } | null; alEnviado: () => void;
  proyecto: Proyecto | null; sesion: Sesion | null; repo: Repo | null; proveedores: Proveedor[];
  panelAbierto: boolean; alPanel: () => void;
  alCambiarSesion: (s: Sesion) => void; alNuevaSesion: (rama?: string) => void; alAjustes: () => void;
}

const nombreCarpeta = (c: string) => c.split('/').filter(Boolean).pop() || c;

// umbralVigencia: para probar el relevo por vigencia con poco, un mínimo propio (localStorage «pgo-umbral-vigencia»,
// en tokens); sin él, el de siempre (W.PRUNE_MIN_TOKENS).
function umbralVigencia(): number | undefined {
  try { const n = Number(localStorage.getItem('pgo-umbral-vigencia')); return n > 0 ? n : undefined; } catch { return undefined; }
}

const MODOS = { lectura: 'Solo lectura', editar: 'Editar ficheros', auto: 'Auto: ejecuta comandos' } as const;
// El modo de la sesión («plan» ya no es un modo: los de antes cuentan como el que tenían antes, o solo lectura).
const modoDe = (s: Sesion) => (s.permisos === 'plan' ? s.modoPrevio || 'lectura' : s.permisos || 'lectura');
const ESFUERZOS: Record<string, string> = { low: 'bajo', medium: 'medio', high: 'alto', xhigh: 'muy alto', max: 'máximo' };
const ALTO_MAX = 240;

// Una pregunta aparte: no entra en la conversación (ni en lo que se guarda); desaparece al recargar.
interface Aparte {
  id: string; pregunta: string; destino: Exclude<AP.Destino, 'conversacion'>; texto: string; acciones: number; uso?: Uso;
  curso: boolean; error?: boolean; motivo: string; auto: boolean; incorporada?: boolean; ac?: AbortController;
  via?: string; // quién la ha respondido (lo dice el servidor): copia, modelo o local/<modelo>
  engine?: R.Engine; // quién decidió que fuera aparte (enrutador)
  pos: number;       // cuántos mensajes tenía la conversación al preguntarla: sale ahí (en su sitio cronológico)
  ruta?: R.RouteStamp; // la decisión del enrutador, si no es una pregunta aparte (p. ej. una función directa respondida aparte)
  plegada?: boolean; // respondida y con el turno ya acabado: plegada en una línea (se despliega con un clic)
}
// Quién ha respondido de verdad una pregunta aparte (para el distintivo y el registro del enrutador).
const executorDeVia = (destino: Aparte['destino'], via?: string): R.Executor =>
  destino === 'estado' ? 'pgocode' : via?.startsWith('local/') ? 'local-ai' : via === 'copia' ? 'agent-copy' : via === 'modelo' ? 'cheap-model' : R.asideExecutor(destino as R.AsideKind);
const SITIOS_APARTE: Exclude<AP.Destino, 'conversacion'>[] = ['estado', 'suelta', 'contexto'];
// El distintivo de los mensajes que llegaron al turno en curso (como los demás: «agente · al turno»).
const INJECTED_STAMP: R.RouteStamp = { route: 'agent', executor: 'agent', engine: 'user', reason: 'lo mandaste al turno en curso: el agente lo leyó antes de acabar', modifiers: ['turn'] };
// Lo aprendido de las correcciones, por proyecto; y el gasto aparte, por sesión (se suma al de la conversación).
const leeLS = <T,>(k: string, d: T): T => { try { return JSON.parse(localStorage.getItem(k) || 'null') ?? d; } catch { return d; } };
const ponLS = (k: string, v: unknown) => { try { localStorage.setItem(k, JSON.stringify(v)); } catch { /* sin almacenamiento */ } };
const CLAVE_SIEMPRE_AL_TURNO = 'pgo:siempreAlTurno';
const claveAprendidas = (proyecto: number) => 'pgo:aparte:' + proyecto;
const claveGastoAparte = (sesion: string) => 'pgo:aparte-gasto:' + sesion;

export default function Chat({ directorio, versionProcesos, alAbrirWeb, alAjustes, seleccionSkills, hosting, detalle, envioPendiente, alEnviado, proyecto, sesion, repo, proveedores, panelAbierto, alPanel, alCambiarSesion, alNuevaSesion }: Props) {
  const [texto, setTexto] = useState('');
  const [mensajes, setMensajes] = useState<Mensaje[]>([]);
  // Turnos en curso por sesión: cada sesión lleva el suyo y sigue aunque se cambie de sesión o de proyecto (al
  // volver se ve su progreso). «enCurso» es el de la sesión abierta.
  const [enCursos, setEnCursos] = useState<Record<string, AbortController>>({});
  const enCurso: AbortController | null = (sesion && enCursos[sesion.id]) || null;
  // Último evento del agente en cada sesión, y un pulso cada 15 s mientras trabaja: «sin novedades desde hace N min».
  const ultimoEvento = useRef<Record<string, number>>({});
  const [, setPulso] = useState(0);
  useEffect(() => {
    if (!enCurso) return;
    const t = setInterval(() => setPulso(x => x + 1), 15_000);
    return () => clearInterval(t);
  }, [enCurso]);
  const enCursosRef = useRef(enCursos);
  enCursosRef.current = enCursos;
  const inicioTurno = useRef<Record<string, number>>({});
  // Reparto en sesiones paralelas (reparto.ts): la propuesta con su cuenta atrás (por sesión madre), los repartos en
  // marcha o acabados (se guardan), las sesiones madre de cada uno, las hijas que el usuario ha parado y los avisos
  // para el chat de una madre que está trabajando (se añaden al acabar su turno).
  const [propuestas, setPropuestas] = useState<Record<string, { ses: Sesion; texto: string; prop: RP.Propuesta; hasta: number; decision: R.RouteDecision; ms: number }>>({});
  const propuestasRef = useRef(propuestas);
  propuestasRef.current = propuestas;
  const temporizadores = useRef<Record<string, ReturnType<typeof setTimeout>>>({});
  const [repartos, setRepartos] = useState<RP.Reparto[]>(() => { const l = RP.alCargar(RP.lee(), () => false); RP.guarda(l); return l; });
  const repartosRef = useRef(repartos);
  repartosRef.current = repartos;
  const madres = useRef<Record<string, Sesion>>({});
  const paradas = useRef<Set<string>>(new Set());
  const notasMadre = useRef<Record<string, string[]>>({});
  // Preguntas aparte por sesión, y las que el usuario quiere que el agente conozca (van con su próximo mensaje).
  const [apartes, setApartes] = useState<Record<string, Aparte[]>>({});
  const apartesSesion = (sesion && apartes[sesion.id]) || [];
  const incorporar = useRef<Record<string, { pregunta: string; respuesta: string }[]>>({});
  const [usoAparte, setUsoAparte] = useState<Uso | undefined>(undefined);
  useEffect(() => { setUsoAparte(sesion ? leeLS<Uso | undefined>(claveGastoAparte(sesion.id), undefined) : undefined); }, [sesion?.id]);
  // La conversación de cada turno en curso, para verla al volver a su sesión (se guarda al acabar).
  const vivas = useRef<Record<string, Mensaje[]>>({});
  const [verAgentes, setVerAgentes] = useState(false);
  const fin = useRef<HTMLDivElement>(null);
  const lista = useRef<HTMLDivElement>(null);
  const idActual = useRef(sesion?.id);
  const area = useRef<HTMLTextAreaElement>(null);
  // Ficheros adjuntos al mensaje que se está escribiendo (ya subidos a la carpeta de la sesión).
  const [adjuntos, setAdjuntos] = useState<(Adjunto & { subiendo?: boolean; error?: string })[]>([]);
  const [arrastrando, setArrastrando] = useState(false);
  // Planes (una herramienta del chat, numerados por proyecto): el abierto en el editor y la lista del proyecto.
  const [planAbierto, setPlanAbierto] = useState<string | null>(null);
  const [verPlanes, setVerPlanes] = useState(false);
  // Copiar: el mensaje entero (su texto, sin las herramientas) con su botón, o lo seleccionado con Ctrl+C / el menú,
  // siempre como texto limpio (sin el HTML de las tarjetas de herramientas).
  const [copiado, setCopiado] = useState<number | null>(null);
  const copiaTexto = (t: string, i: number) => {
    navigator.clipboard.writeText(t.trim()).then(() => { setCopiado(i); setTimeout(() => setCopiado(c => (c === i ? null : c)), 1500); }).catch(() => {});
  };
  const copiaSeleccion = (e: React.ClipboardEvent) => {
    const t = window.getSelection()?.toString() || '';
    if (!t.trim()) return;
    e.preventDefault();
    e.clipboardData.setData('text/plain', t.replace(/\n{3,}/g, '\n\n').trim());
  };

  // Dirección local de la última respuesta del agente que la tenga: a la barra de ejecución rápida.
  const propuesta = useMemo(() => {
    for (let i = mensajes.length - 1; i >= 0; i--) {
      const m = mensajes[i];
      if (m.rol !== 'assistant' || m.error || (enCurso && i === mensajes.length - 1)) continue; // la que se escribe, al acabar
      const d = direccionesEn(m.texto || '');
      if (d.length) return d[0];
    }
    return undefined;
  }, [mensajes, enCurso]);
  // La pestaña Actividad puede pedir ejecutar un plan (lo hace el chat, que tiene la sesión).
  useEffect(() => {
    const pide = (e: Event) => { const p = (e as CustomEvent<Plan>).detail; if (p) ejecutaPlan(p); };
    window.addEventListener('pgo:ejecutar-plan', pide);
    return () => window.removeEventListener('pgo:ejecutar-plan', pide);
  });

  // Gasto de la sesión y sus avisos (los de la sesión o, si no tiene, los del plan de su agente).
  const gastoSesion = useMemo(() => {
    const g = CO.gasto(mensajes);
    return usoAparte ? { ...g, coste: g.coste + (usoAparte.coste || 0), entrada: g.entrada + usoAparte.entrada, salida: g.salida + usoAparte.salida } : g;
  }, [mensajes, usoAparte]);
  // Tokens de la conversación hasta cada mensaje (sin las preguntas aparte): el último es el de ahora.
  const acumulados = useMemo(() => { let n = 0; return mensajes.map(m => { const g = CO.gasto([m]); return n += g.entrada + g.salida; }); }, [mensajes]);
  const tokensConversacion = acumulados[acumulados.length - 1] || 0;
  const avisos = sesion ? CO.efectivos(sesion.agente, sesion.avisos) : {};
  const nivelGasto = CO.nivel(gastoSesion, avisos);
  // Avisos de la sesión: solo se anotan (sin interrumpir) y se ven en el indicador del pie, junto al modelo.
  const [alertas, setAlertas] = useState<{ nivel: 'aviso' | 'limite'; texto: string }[]>([]);
  useEffect(() => setAlertas([]), [sesion?.id]);
  const avisa = (l: { nivel: 'aviso' | 'limite'; texto: string }[]) => { if (l.length) setAlertas(x => [...x, ...l].slice(-5)); };

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

  const provAbierto = proveedores.find(p => p.id === sesion?.agente);
  const prov = provAbierto;
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

  // Al cambiar de sesión: su conversación (la del turno en curso, si lo tiene). El turno de la otra sesión no se
  // corta: sigue y guarda su respuesta al acabar.
  useEffect(() => {
    idActual.current = sesion?.id;
    setMensajes(sesion ? vivas.current[sesion.id] || conversacion(sesion.id) : []);
  }, [sesion?.id]);

  // Vigencia (prune.ts): cada mensaje y herramienta se desvanece según la suya, salvo con la gestión automática del
  // contexto desactivada. En segundo plano: al abrir la sesión, al acabar cada turno (cuando pgocode ha marcado lo
  // nuevo) y cada 10 minutos, porque el tiempo también le resta vigencia. OpenCode y Claude Code: la del recolector
  // de su historial (GET); Antigravity: la de esta conversación (POST, contextPolicy «chat»).
  const vigOn = Boolean(sesion && autoContextOn(sesion));
  // Hilo de limpieza (CleanerIndicator): el último informe de cada sesión, con el estado del recolector, y si hay una
  // consulta en curso. Si el historial del agente se puede podar (por su sesión del agente): sin relevo de reserva.
  const [limpieza, setLimpieza] = useState<Record<string, { r?: PR.PruneReport; busy?: boolean }>>({});
  const podables = useRef<Record<string, boolean>>({});
  const ponLimpieza = (id: string, c: { r?: PR.PruneReport; busy?: boolean }) => setLimpieza(x => ({ ...x, [id]: { ...x[id], ...c } }));
  // Actividad (pestaña del panel, liveActivity.ts): el chat publica lo que sabe de su sesión con cada evento del
  // agente (como mucho 4 por segundo) y, mientras trabaja, cada 5 s (para que «pensando» pase a «esperando al
  // modelo»): estado, acción en curso, tiempos, tokens del turno, contexto, cola, preguntas aparte y tareas.
  const publicaActividad = useRef<() => void>(() => {});
  publicaActividad.current = () => {
    if (!sesion) return;
    const u = mensajes[mensajes.length - 1];
    const vivo = enCurso && u?.rol === 'assistant' ? u : undefined;
    const umbral = CL.compactAt(limpieza[sesion.id]?.r);
    const serie = CL.contextSeries(mensajes, umbral, 1);
    const silencio = Date.now() - (ultimoEvento.current[sesion.id] || Date.now());
    const g = vivo?.vivo || vivo?.uso;
    LA.publishSnapshot({
      sesion: sesion.id, tareas: mensajes.flatMap(m => m.tareas || []), trabajando: Boolean(enCurso), todo: TS.sessionTodo(mensajes, Boolean(enCurso)),
      accion: vivo ? currentAction(vivo, silencio) : undefined, inicio: enCurso ? inicioTurno.current[sesion.id] : undefined,
      ultimoEvento: ultimoEvento.current[sesion.id], tokensTurno: g ? g.entrada + g.salida : undefined,
      contexto: serie[serie.length - 1]?.tokens, umbral, enCola: cola.length, colaParada: Boolean(colaSesion.parada),
      apartes: apartesSesion.filter(a => a.curso).map(a => ({ id: a.id, pregunta: a.pregunta })),
    });
  };
  useEffect(() => { publicaActividad.current(); }, [mensajes, enCurso, sesion?.id, colas, apartes, limpieza]);
  useEffect(() => {
    if (!enCurso) return;
    const t = setInterval(() => publicaActividad.current(), 5000);
    return () => clearInterval(t);
  }, [enCurso]);
  const pideVigencia = useCallback(async (s: Sesion, sesionAgente: string | undefined, ms: Mensaje[], mensaje?: string, soloEstado = false): Promise<PR.PruneReport | null> => {
    const pol = contextPolicy(s.agente, s);
    if (pol.vig === 'none' || (pol.vig === 'agent' && !sesionAgente)) return null;
    let r: PR.PruneReport;
    if (pol.vig === 'chat') {
      const xs = CL.chatExchanges(ms);
      if (!xs.length) return null;
      ponLimpieza(s.id, { busy: true });
      try { r = await post<PR.PruneReport>('/api/chat/vigencia', { clave: 'chat-' + s.id, carpeta: s.carpeta, intercambios: xs, mensaje: mensaje || '' }); }
      finally { ponLimpieza(s.id, { busy: false }); }
    } else {
      if (!soloEstado) ponLimpieza(s.id, { busy: true });
      try { r = await get<PR.PruneReport>(`/api/chat/vigencia?sesion_agente=${encodeURIComponent(sesionAgente!)}${soloEstado ? '&estado=1' : ''}`); }
      finally { if (!soloEstado) ponLimpieza(s.id, { busy: false }); }
    }
    if (r.podable !== undefined && sesionAgente && !soloEstado) podables.current[sesionAgente] = Boolean(r.podable);
    ponLimpieza(s.id, { r });
    return r;
  }, []);
  const refrescaVigencia = useCallback(async (s: Sesion, sesionAgente = s.sesionAgente, soloEstado = false) => {
    if (!autoContextOn(s)) return;
    try {
      const r = await pideVigencia(s, sesionAgente, conversacion(s.id), undefined, soloEstado);
      if (!r || (!r.intercambios.length && !r.piezas?.length)) return;
      if (vivas.current[s.id]) return; // un turno en curso: su barrido trae la suya
      const base = conversacion(s.id);
      const n = contextPolicy(s.agente, s).vig === 'chat' ? CL.applyChatVigencia(base, r) : PR.applyPruneReport(base, r);
      if (n.every((m, i) => m === base[i])) return;
      guardaConversacion(s.id, n);
      if (idActual.current === s.id) setMensajes(n);
    } catch { /* sin vigencia: se queda la que había */ }
  }, []);
  useEffect(() => {
    if (!sesion || !vigOn || (!sesion.sesionAgente && contextPolicy(sesion.agente, sesion).vig !== 'chat')) return;
    refrescaVigencia(sesion);
    const t = setInterval(() => refrescaVigencia(sesion), 10 * 60_000);
    return () => clearInterval(t);
  }, [sesion?.id, sesion?.sesionAgente, sesion?.agente, vigOn]);
  // «Barrer ahora» (ventanita del hilo de limpieza): retira ya lo que está fuera de vigencia, sin el mínimo ni la
  // cuenta de la caché. OpenCode, también con un turno en marcha (lo retirado se apunta en ese turno al acabar);
  // Claude Code, si la sesión está abierta, al empezar el próximo turno; Antigravity, con el relevo en el próximo envío.
  const [barriendo, setBarriendo] = useState<Record<string, boolean>>({});
  const [barridoTxt, setBarridoTxt] = useState<Record<string, string>>({});
  const relevoForzado = useRef<Record<string, boolean>>({});
  const [relevoPedido, setRelevoPedido] = useState<Record<string, boolean>>({});
  const barridoPend = useRef<Record<string, number>>({});
  // vaciar: «Vaciar contexto» (el aviso de una conversación grande y parada): todo el historial fuera, no solo lo de
  // vigencia 3+; en Antigravity, el relevo en el próximo envío (no deja tocar su historial).
  async function barreAhora(vaciar = false) {
    if (!sesion || !proyecto) return;
    const s = sesion;
    if (contextPolicy(s.agente, s).vig === 'chat') {
      relevoForzado.current[s.id] = true;
      setRelevoPedido(x => ({ ...x, [s.id]: true }));
      return;
    }
    if (!s.sesionAgente || barriendo[s.id]) return;
    const enMarcha = Boolean(vivas.current[s.id]);
    const ultimoUsuario = [...conversacion(s.id)].reverse().find(m => m.rol === 'user' && m.texto)?.texto || '';
    setBarriendo(x => ({ ...x, [s.id]: true }));
    setBarridoTxt(x => ({ ...x, [s.id]: '' }));
    try {
      const r = await post<PR.PruneReport>('/api/chat/vigencia?barrer=1' + (vaciar ? '&vaciar=1' : ''), { proyecto: proyecto.id, agente: s.agente, sesion_agente: s.sesionAgente,
        carpeta: s.carpeta, mensaje: ultimoUsuario, en_curso: enMarcha });
      ponLimpieza(s.id, { r });
      const quitados = r.retirados || 0;
      setBarridoTxt(x => ({ ...x, [s.id]: quitados > 0 || r.repuestos ? PR.prunedLine({ tokens: quitados, repuestos: r.repuestos }) : r.forzado_pendiente ? '' : 'nada que retirar' }));
      if (quitados > 0 || r.repuestos) {
        if (enMarcha) barridoPend.current[s.id] = (barridoPend.current[s.id] || 0) + quitados;
        else {
          const n = PR.applyPruneReport([...conversacion(s.id), CL.sweepNote(quitados, r.repuestos)], r);
          guardaConversacion(s.id, n);
          if (idActual.current === s.id) setMensajes(n);
        }
      }
    } catch (e) {
      setBarridoTxt(x => ({ ...x, [s.id]: '⚠ ' + (e as Error).message }));
    } finally {
      setBarriendo(x => ({ ...x, [s.id]: false }));
    }
  }
  // Mientras el recolector trabaja (marcando o barriendo), su estado cada 1,5 s sin esperarlo; quieto, nada.
  const faseLimpieza = sesion ? limpieza[sesion.id]?.r?.estado?.fase : undefined;
  useEffect(() => {
    if (!sesion || !vigOn || !faseLimpieza) return;
    const t = setTimeout(() => refrescaVigencia(sesion, sesion.sesionAgente, true), 1500);
    return () => clearTimeout(t);
  }, [sesion?.id, faseLimpieza, limpieza[sesion?.id || '']?.r]);

  // Desplazamiento: al entrar en una sesión, a lo más reciente; mientras llega la respuesta, siempre abajo,
  // salvo que el usuario haya subido a leer (se respeta hasta que vuelva abajo).
  // «pegado» solo se suelta cuando el usuario sube (rueda, tecla, arrastre o toque): un desplazamiento que no es suyo
  // (el contenido que crece o se recoloca) no lo suelta, para que el chat siga bajando hasta el final del turno.
  const pegado = useRef(true);
  const gestoUsuario = useRef(0);
  const marcaGesto = () => { gestoUsuario.current = Date.now(); };
  const alDesplazar = () => {
    const l = lista.current;
    if (!l) return;
    const abajo = l.scrollHeight - l.scrollTop - l.clientHeight < 80;
    if (abajo) pegado.current = true;
    else if (Date.now() - gestoUsuario.current < 800) pegado.current = false;
  };
  const baja = () => { const l = lista.current; if (l) l.scrollTop = l.scrollHeight; };
  useLayoutEffect(() => {
    pegado.current = true;
    baja();
    // Otra vez tras pintar el texto enriquecido y las herramientas (cambian la altura).
    const t = requestAnimationFrame(baja);
    return () => cancelAnimationFrame(t);
  }, [sesion?.id]);
  // Las preguntas aparte y el estado («?») también la siguen.
  useLayoutEffect(() => { if (pegado.current) baja(); }, [mensajes, apartesSesion]);
  // Al acabar el turno de la sesión abierta: abajo (si el usuario no ha subido a leer), y las preguntas aparte ya
  // respondidas se pliegan (no se pierden: un clic las despliega).
  const turnoAbierto = Boolean(enCurso);
  const turnoAntes = useRef(turnoAbierto);
  useEffect(() => {
    const acaba = turnoAntes.current && !turnoAbierto;
    turnoAntes.current = turnoAbierto;
    if (!acaba || !sesion) return;
    if (pegado.current) requestAnimationFrame(baja);
  }, [turnoAbierto, sesion?.id]); // eslint-disable-line react-hooks/exhaustive-deps
  // También al crecer el contenido sin mensajes nuevos (desplegar una herramienta, imágenes, bloques de código).
  useEffect(() => {
    const l = lista.current;
    if (!l) return;
    const ro = new ResizeObserver(() => { if (pegado.current) baja(); });
    Array.from(l.children).forEach(c => ro.observe(c));
    return () => ro.disconnect();
  }, [mensajes.length, apartesSesion.length, sesion?.id]);

  // Envío automático pedido desde fuera, cuando su sesión ya es la abierta y no hay nada en curso.
  useEffect(() => {
    if (!envioPendiente || !sesion || sesion.id !== envioPendiente.sesion || enCurso || !prov?.disponible) return;
    alEnviado();
    envia(envioPendiente.texto);
  }, [envioPendiente, sesion?.id, prov?.disponible]); // eslint-disable-line react-hooks/exhaustive-deps

  // Al acabar un turno sin error, sale la cola: todos sus mensajes juntos (las órdenes, solas).
  useEffect(() => {
    if (!sesion || !prov?.disponible) return;
    // Lo que sale solo (órdenes de pgocode y funciones directas) lo decide el enrutador (router.ts).
    const { enviar, cola: resto, partes, origenes } = C.siguiente(colaSesion, Boolean(enCurso), t => R.standalone(t, sesion.directo !== false));
    if (!enviar) return;
    ponCola(sesion.id, () => resto);
    // Lo encolado con «>» va al agente aunque parezca una orden directa (y sin repartir); el registro conserva quién
    // decidió encolarlo y por qué.
    const q = R.fromQueue(partes || [enviar.texto], origenes);
    envia(q.text, enviar.adjuntos, { desdeCola: true, sinDirecto: q.forced, sinReparto: q.forced, origen: q.origin, generado: q.generated });
  }, [enCurso, sesion?.id, colaSesion, prov?.disponible]); // eslint-disable-line react-hooks/exhaustive-deps

  // Lo último que ha dicho el agente (para decidir si lo que se escribe es su respuesta).
  const ultimoDelAgente = (ms: Mensaje[]) => {
    const m = [...ms].reverse().find(x => x.rol === 'assistant');
    const ts = (m?.partes || []).filter(p => p.tipo === 'texto');
    const u = ts[ts.length - 1];
    return (u && u.tipo === 'texto' ? u.texto : m?.texto || '').slice(-600);
  };

  // Opción «Siempre al turno» (de cada usuario, para todas las sesiones): con el agente trabajando, lo que va a la
  // conversación le llega ya, a mitad de turno, en vez de esperar en la cola.
  // Por defecto, sí.
  const [siempreAlTurno, setSiempreAlTurno] = useState(() => leeLS(CLAVE_SIEMPRE_AL_TURNO, true));
  const [verAyuda, setVerAyuda] = useState(false);
  // Historial con ↑ ↓ (inputHistory.ts): dónde está el usuario y, si ha cargado uno de la cola, cuál (se guarda en su
  // sitio con Intro).
  const [historial, setHistorial] = useState<IH.HistoryState>(IH.HISTORY_START);
  const [editaCola, setEditaCola] = useState<{ index: number; original: string } | null>(null);
  const salHistorial = () => { setHistorial(IH.HISTORY_START); setEditaCola(null); };
  useEffect(salHistorial, [sesion?.id]);
  // Fotos de la conversación (capturas de las herramientas e imágenes de las respuestas), de la más reciente a la
  // más antigua: el visor, que deja pasar de una a otra (se abre al pulsar cualquier miniatura de la conversación).
  const fotos = useMemo<FotoVisor[]>(() => proyecto ? fotosDe(mensajes, r => urlCaptura(proyecto.id, r)).map(f => ({ src: f.src, pie: f.titulo })) : [],
    [mensajes, proyecto?.id]); // eslint-disable-line react-hooks/exhaustive-deps
  const fotosRef = useRef(fotos);
  fotosRef.current = fotos;
  const [fotoAbierta, setFotoAbierta] = useState<{ fotos: FotoVisor[]; i: number } | null>(null);
  const abreFoto = useCallback((src: string) => {
    const l = fotosRef.current;
    const i = l.findIndex(f => f.src === src);
    setFotoAbierta(i >= 0 ? { fotos: l, i } : { fotos: [{ src }], i: 0 });
  }, []);
  const cierraAyuda = useCallback(() => setVerAyuda(false), []);
  // Destino elegido a mano en la etiqueta (o con Alt+Intro): manda sobre la decisión automática.
  const [destinoManual, setDestinoManual] = useState<AP.Destino | null>(null);
  // El texto con el que vale lo elegido: se va moviendo con lo que se escribe; si se vacía o se cambia todo, se olvida.
  const baseManual = useRef('');
  useEffect(() => {
    if (!destinoManual) return;
    if (R.manualStillApplies(baseManual.current, texto)) baseManual.current = texto;
    else setDestinoManual(null);
  }, [texto]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => setDestinoManual(null), [sesion?.id]);
  // ENRUTADOR DE INTENCIONES (router.ts, docs/enrutador.md): lo que necesita saber de la sesión para decidir quién
  // hace qué con un mensaje. La misma decisión sale en la etiqueta encima del cuadro y al enviar.
  const routeInput = (t: string, ses: Sesion | null | undefined, p = prov): R.RouteInput => ({
    text: t, isAgent: p?.tipo === 'agente', busy: Boolean(ses && enCursosRef.current[ses.id]), canInject: Boolean(p?.inyecta), alwaysTurn: siempreAlTurno,
    attachments: adjuntos.filter(a => !a.subiendo && !a.error).length, hasConversation: mensajes.length > 0, lastAgentText: ultimoDelAgente(mensajes),
    learned: proyecto ? leeLS(claveAprendidas(proyecto.id), {}) : {}, autoAside: ses?.decideAparte !== false, directOn: ses?.directo !== false,
    permissions: ses ? modoDe(ses) : undefined, child: Boolean(ses?.madre), cutPending: ses?.corte !== undefined && p?.tipo === 'agente',
    canFork: Boolean(ses?.sesionAgente) && (ses?.agente === 'claude-code' || ses?.agente === 'opencode'),
  });
  // registraRuta: cada decisión, una vez: al registro de tiempos del servidor (suceso «ruta»), al recuento de la
  // sesión (Actividad) y, con t0, cuánto ha tardado en decidirse.
  // ms: lo que tardó en decidirse, si se registra después de hacerlo (la función directa, la pregunta aparte).
  const registraRuta = (sesionId: string, s: R.RouteStamp, t0: number, ms = performance.now() - t0) => {
    post('/api/ruta', R.logPayload(sesionId, s, ms)).catch(() => {});
    R.saveTally(sesionId, R.tally(R.readTally(sesionId), s));
    window.dispatchEvent(new CustomEvent('pgo:ruta', { detail: { sesion: sesionId } }));
  };
  // La decisión mientras se escribe (la etiqueta encima del cuadro). Solo con agentes (con un modelo por API, todo
  // va al modelo).
  const rutaViva = useMemo(() => (texto.trim() && prov?.tipo === 'agente' && sesion ? R.route({ ...routeInput(texto, sesion), busy: Boolean(enCurso), manual: destinoManual || undefined }) : null),
    [texto, enCurso, mensajes, adjuntos, sesion, prov?.tipo, prov?.inyecta, siempreAlTurno, destinoManual]); // eslint-disable-line react-hooks/exhaustive-deps
  // Mensajes al turno en curso (agentes que los admiten, como Claude Code): los de cada sesión durante su turno,
  // que al acabar se guardan con la respuesta.
  const conTurno = Boolean(enCurso && prov?.inyecta);
  // Con adjuntos no se puede mandar al turno en curso (van a la cola): ni Alt+Intro ni la etiqueta lo ofrecen.
  const alTurnoPosible = conTurno && adjuntos.length === 0;
  const [inyectados, setInyectados] = useState<Record<string, string[]>>({});
  const inyectadosRef = useRef(inyectados);
  inyectadosRef.current = inyectados;
  // d y t0: la decisión del enrutador, que se registra si el mensaje llega al turno (si no, la registra el envío).
  async function alTurno(id: string, t: string, adj: Adjunto[] = [], d?: R.RouteDecision, t0 = performance.now()) {
    // Con adjuntos, o si el turno ya acabó o no lo admite: a la cola, como siempre.
    if (adj.length || !enCursosRef.current[id]) { aConversacion(id, t, adj, Boolean(d?.forced), d && R.queueOrigin(d)); return; }
    try {
      await post('/api/chat/inyecta', { sesion: id, texto: t });
      if (d) registraRuta(id, R.stamp(d), t0);
      inyectadosRef.current = { ...inyectadosRef.current, [id]: [...(inyectadosRef.current[id] || []), t] };
      setInyectados(inyectadosRef.current);
    } catch {
      aConversacion(id, t, adj, false, d && R.queueOrigin(d));
    }
  }
  // Antes de enviar: si la conversación es grande (y sin caché por llevar un rato parada), cuánto se reenviará.
  const avisoContexto = useMemo(() => {
    if (enCurso || !sesion) return null;
    let i = mensajes.length - 1;
    while (i >= 0 && !(mensajes[i].rol === 'assistant' && mensajes[i].uso)) i--;
    if (i < 0) return null;
    const u = mensajes[i];
    // Lo retirado después (Barrer ahora / Vaciar contexto) ya no se reenvía.
    const retirado = mensajes.slice(i + 1).reduce((n, m) => n + (m.nota && m.pruned ? m.pruned.tokens : 0), 0);
    if (relevoPedido[sesion.id]) return null;
    return CO.avisoContexto(Math.max((u.uso?.contexto || 0) - retirado, 0), u.fin ? (Date.now() - u.fin) / 60000 : 0, sesion.agente, CS.cacheMinutes(mensajes, sesion.agente));
  }, [mensajes, enCurso, sesion?.agente, Boolean(texto.trim()), relevoPedido]); // eslint-disable-line react-hooks/exhaustive-deps

  // Intro: el ENRUTADOR (router.ts) decide quién hace qué con el mensaje y aquí solo se despacha.
  // forzar: el destino que manda (Alt+Intro → turno en curso); si no, el elegido en la etiqueta o el automático.
  async function decideYEnvia(forzar?: AP.Destino) {
    const t0 = performance.now();
    const t = texto.trim();
    // Uno de la cola cargado con ↑: Intro lo guarda en su sitio (ni se envía ni se duplica) y vuelve el borrador.
    if (editaCola && sesion) {
      const n = C.edita(colaSesion, editaCola.index, editaCola.original, texto);
      salHistorial();
      if (n) { ponCola(sesion.id, () => n); setTexto(historial.draft); return; }
    }
    salHistorial();
    const listos = adjuntos.filter(a => !a.subiendo && !a.error);
    const elegido = forzar || destinoManual || undefined;
    const inp: R.RouteInput = { ...routeInput(t, sesion), busy: Boolean(enCurso), attachments: listos.length };
    let d = R.route({ ...inp, manual: elegido, manualBy: forzar ? 'alt' : 'tag' });
    // 0. «/help»: la ayuda, en la interfaz (sin agente ni tokens).
    if (d.route === 'command' && d.detail === 'help') {
      setTexto(''); setVerAyuda(true);
      if (sesion) registraRuta(sesion.id, R.stamp(d), t0);
      return;
    }
    // Sin texto, solo con adjuntos (una foto): también se envía.
    if ((!t && !listos.length) || !sesion || !proyecto || subiendo) return;
    const ses = sesion;
    // Lo que el usuario elige en contra de lo que habría decidido pgocode se aprende (para los parecidos).
    if (elegido) {
      const auto = R.route(inp);
      const autoDest: AP.Destino = auto.route === 'aside' ? auto.aside! : auto.modifiers.includes('turn') ? 'turno' : 'conversacion';
      if (!auto.forced && !auto.needsServer && elegido !== autoDest) ponLS(claveAprendidas(proyecto.id), AP.aprende(leeLS(claveAprendidas(proyecto.id), {}), t, elegido));
    }
    setDestinoManual(null);
    setTexto(''); setAdjuntos([]);
    // 2'. Duda entre la conversación y aparte (solo con el agente trabajando): el clasificador del servidor (IA local;
    // de respaldo, el modelo barato), con tiempo máximo. Sin respuesta, a la conversación.
    if (d.needsServer === 'classify') {
      const r = await R.withTimeout(post<{ tipo: AP.Destino; modelo?: string }>('/api/chat/clasifica', { texto: t, ultimo: inp.lastAgentText, sesion: ses.id }), R.CLASSIFY_MS);
      d = R.resolveClassify(d, r, inp);
    }
    switch (d.route) {
      case 'direct':
        // Con el agente trabajando: la pregunta de estado se responde ya, aparte (sin tocar su turno); lo mecánico
        // espera en la cola (nunca al turno en curso) y lo hace pgocode cuando acabe.
        if (enCurso) {
          if (d.modifiers.includes('queue')) ponCola(ses.id, c => C.encola(c, t, [], R.queueOrigin(d)));
          else directoAparte(ses, t, d, t0);
          return;
        }
        break;
      case 'aside':
        pregunta(ses, d.text, d.aside!, d.reason, !d.forced, { decision: d, t0 });
        return;
      case 'agent':
        if (d.modifiers.includes('turn')) { await alTurno(ses.id, d.text, listos, d, t0); return; }
        break;
    }
    // Lo demás (agente, órdenes de planes, función directa, posible reparto): ahora o, con el agente trabajando, a la
    // cola (lo que el usuario mandó al agente a propósito se encola con «>», para que al salir siga yendo al agente).
    if (enCursosRef.current[ses.id]) { ponCola(ses.id, c => C.encola(c, d.forced && d.route === 'agent' ? '> ' + d.text : d.text, listos, R.queueOrigin(d))); return; }
    envia(d.text, listos, { decision: d, t0 });
  }

  // A la conversación: a la cola si el agente está ocupado (la cola sale sola al acabar), si no, se envía.
  // sinDirecto: lo ha mandado el usuario a la conversación a propósito (prefijo «>», elegido o «Pásaselo al agente»):
  // al agente, aunque parezca una orden directa (en la cola, con «>» delante para que lo siga siendo).
  // origen: quién lo decidió y por qué (queda en el registro); generated: lo ha redactado pgocode (no da nombre a la sesión).
  function aConversacion(id: string, t: string, adj: Adjunto[] = [], sinDirecto = false, origen?: C.QueueOrigin) {
    if (enCursosRef.current[id] || idActual.current !== id) ponCola(id, c => C.encola(c, sinDirecto ? '> ' + t : t, adj, origen));
    else envia(t, adj, { aCola: true, sinDirecto, sinReparto: sinDirecto, origen, generado: origen?.generated });
  }

  // Responde una pregunta aparte: el estado, en local; el resto, con /api/chat en modo aparte (el servidor dice quién
  // la ha respondido: copia de la sesión, IA local o modelo barato). o.decision: la del enrutador (se registra al
  // saber quién respondió); o.motor «pago»: la suelta, con el modelo de pago aunque haya IA local.
  async function pregunta(ses: Sesion, q: string, destino: Exclude<AP.Destino, 'conversacion'>, motivo: string, auto: boolean,
    o: { sustituye?: string; decision?: R.RouteDecision; t0?: number; motor?: 'pago' } = {}) {
    const idA = Math.random().toString(36).slice(2);
    const pon = (f: (l: Aparte[]) => Aparte[]) => setApartes(x => ({ ...x, [ses.id]: f(x[ses.id] || []) }));
    if (ses.id === sesion?.id) pegado.current = true; // la ha pedido el usuario: se baja hasta ella
    const cambia = (c: Partial<Aparte>) => pon(l => l.map(a => a.id === idA ? { ...a, ...c } : a));
    const ms = vivas.current[ses.id] || (idActual.current === ses.id ? mensajes : conversacion(ses.id));
    const previa = o.sustituye ? (apartes[ses.id] || []).find(a => a.id === o.sustituye) : undefined;
    const base: Aparte = { id: idA, pregunta: q || '¿Cómo va?', destino, texto: '', acciones: 0, curso: destino !== 'estado', motivo, auto, engine: o.decision?.engine ?? 'user', pos: previa?.pos ?? ms.length };
    let via = '';
    const msDecision = o.t0 !== undefined ? performance.now() - o.t0 : 0;
    const registra = () => { if (o.decision) registraRuta(ses.id, { ...R.stamp(o.decision), executor: executorDeVia(destino, via) }, 0, msDecision); };
    if (destino === 'estado') {
      const trabajando = Boolean(enCursosRef.current[ses.id]);
      const ult = [...ms].reverse().find(m => m.rol === 'assistant');
      base.texto = AP.resumenEstado(ult, { trabajando, segundos: (Date.now() - (inicioTurno.current[ses.id] || Date.now())) / 1000, enCola: (colas[ses.id]?.mensajes.length) || 0 });
    }
    pon(l => o.sustituye ? l.map(a => a.id === o.sustituye ? base : a) : [...l, base]);
    if (destino === 'estado' || !proyecto) { registra(); return; }
    const p = proveedores.find(x => x.id === ses.agente);
    if (!p) { cambia({ curso: false, error: true, texto: '⚠ Elige un agente' }); return; }
    const ac = new AbortController();
    cambia({ ac });
    let texto = '', acciones = 0, uso: Uso | undefined;
    // Sin copia de la sesión del agente (Antigravity, o aún sin sesión): un resumen de la conversación.
    const sinCopia = destino === 'contexto' && (ses.agente === 'antigravity' || !ses.sesionAgente || p.tipo !== 'agente');
    try {
      await chat({
        proyecto: proyecto.id, proveedor: p.id, modelo: ses.modelo || p.por_defecto, carpeta: ses.carpeta, rama: ses.rama,
        sesion_agente: ses.sesionAgente, esfuerzo: ses.esfuerzo || '', sesion: ses.id, titulo: ses.nombre, autorizadas: ses.autorizadas || [],
        mcp: ses.mcp || [], tarea_id: ses.tareaId || 0, tarea: ses.tarea || '', mensajes: [{ rol: 'user', texto: q }],
        aparte: destino, aparte_motor: o.motor || '', contexto: sinCopia ? AP.resumenContexto(ms) : '', idioma: navigator.language, compactar: contextPolicy(ses.agente, ses).compactAt,
      }, e => {
        if (e.via) { via = e.via; cambia({ via }); }
        if (e.texto) texto += e.texto;
        if (e.herramienta && e.herramienta.estado === 'curso') acciones++;
        if (e.uso) {
          uso = AP.sumaUso(uso, e.uso);
          const g = AP.sumaUso(leeLS<Uso | undefined>(claveGastoAparte(ses.id), undefined), e.uso);
          ponLS(claveGastoAparte(ses.id), g);
          if (idActual.current === ses.id) setUsoAparte(g);
        }
        if (e.error) texto += (texto ? '\n\n' : '') + '⚠ ' + e.error;
        cambia({ texto, acciones, uso, error: Boolean(e.error) || undefined });
      }, ac.signal);
    } catch (err) {
      if (!ac.signal.aborted) cambia({ texto: (texto ? texto + '\n\n' : '') + '⚠ ' + (err as Error).message, error: true });
    } finally {
      cambia({ curso: false, ac: undefined });
      registra();
    }
  }

  // Correcciones del usuario sobre una pregunta aparte (pgocode aprende de las que decidió él).
  const aprendeDe = (a: { pregunta: string; auto: boolean }, destino: AP.Destino) => {
    if (!proyecto || !a.auto) return;
    ponLS(claveAprendidas(proyecto.id), AP.aprende(leeLS(claveAprendidas(proyecto.id), {}), a.pregunta, destino));
  };
  const quitaAparte = (id: string, a: Aparte) => {
    a.ac?.abort();
    setApartes(x => ({ ...x, [id]: (x[id] || []).filter(y => y.id !== a.id) }));
  };
  // Las respondidas se pliegan (al acabar un turno del agente); plegar/desplegar una a mano.
  const pliegaApartes = (id: string) => setApartes(x => (x[id]?.some(a => !a.curso && !a.plegada) ? { ...x, [id]: x[id].map(a => (a.curso ? a : { ...a, plegada: true })) } : x));
  const despliegaAparte = (id: string, a: Aparte) => setApartes(x => ({ ...x, [id]: (x[id] || []).map(y => (y.id === a.id ? { ...y, plegada: !y.plegada } : y)) }));

  // Parar: el turno se corta y lo que había en la cola vuelve al cuadro de texto, para revisarlo.
  function para() {
    if (!enCurso || !sesion) return;
    if (cola.length) {
      const d = C.devuelve(cola, texto, adjuntos);
      setTexto(d.texto);
      setAdjuntos(d.adjuntos);
      ponCola(sesion.id, c => ({ ...c, mensajes: [] }));
    }
    if (sesion.madre) paradas.current.add(sesion.id);
    enCurso.abort();
  }

  // opc.crudo: el texto va tal cual (sin interpretar órdenes de planes); opc.permisos: los de este turno (no se
  // guardan en la sesión), p. ej. para ejecutar un plan en una sesión de solo lectura.
  // opc.decision y opc.t0: la decisión del enrutador tomada al pulsar Intro (si no, se toma aquí); opc.reason: el
  // porqué, cuando lo sabe quien llama (p. ej. pgocode no sabía hacer la orden directa); opc.desdeCola: sale de la cola.
  async function envia(forzado?: string, adjCola?: Adjunto[], opc: { crudo?: boolean; permisos?: string; visible?: string; aCola?: boolean; sinRelevo?: boolean; sesionAgente?: string; sesion?: Sesion; sinReparto?: boolean; sinDirecto?: boolean;
    decision?: R.RouteDecision; t0?: number; reason?: string; desdeCola?: boolean; origen?: C.QueueOrigin; generado?: boolean } = {}) {
    const t = (forzado ?? texto).trim();
    const listos = adjCola ?? (forzado === undefined ? adjuntos.filter(a => !a.subiendo && !a.error) : []);
    // opc.sesion: el envío es para otra sesión que la abierta (las hijas de un reparto, o la madre tras la cuenta atrás).
    const ses: Sesion | null = opc.sesion ?? sesion;
    const prov = opc.sesion ? proveedores.find(p => p.id === opc.sesion!.agente) : provAbierto;
    if ((!t && !listos.length) || !proyecto || !ses || !ses.hosting || !prov || (opc.sesion ? enCursosRef.current[ses.id] : enCurso) || (forzado === undefined && subiendo)) return;
    if (!opc.sinReparto && propuestasRef.current[ses.id]) return; // hay un reparto propuesto a la espera: primero eso
    // La decisión del enrutador (router.ts): la de Intro o, si el envío no viene de ahí (la cola, un plan, una hija de
    // un reparto, un reenvío), una nueva con el agente libre y sin preguntas aparte (ya va a la conversación).
    const t0 = opc.t0 ?? performance.now();
    let d = opc.decision ?? R.route({ ...routeInput(t, ses, prov), busy: false, attachments: listos.length, noAside: true, raw: opc.crudo,
      noDirect: opc.sinDirecto || Boolean(opc.sesion), noSplit: opc.sinReparto, hasConversation: conversacion(ses.id).length > 0 });
    if (opc.reason) d = { ...d, reason: opc.reason };
    d = R.fromOrigin(d, opc.origen); // lo que salió de la cola: quién decidió encolarlo y por qué
    if (opc.desdeCola && !d.modifiers.includes('queue')) d = { ...d, modifiers: [...d.modifiers, 'queue'] };
    const limpia = () => { if (forzado === undefined) { setTexto(''); setAdjuntos([]); } };
    // 1. Funciones directas (direct.ts): compilar, pruebas, commit, estado de git, gasto… las hace pgocode (0 tokens).
    if (d.route === 'direct' && d.direct) {
      limpia();
      await directo(ses, t, d.direct, d, t0);
      return;
    }
    // 0. Órdenes de planes: las de estado se hacen aquí (sin agente ni tokens); crear, revisar y ejecutar, el agente.
    const orden = !opc.crudo && d.route === 'command' && d.plan ? d.plan : null;
    if (orden && orden.tipo !== 'nuevo' && orden.tipo !== 'revisar') {
      limpia();
      await ordenPlan(orden, t, d, t0);
      return;
    }
    let planRevisado: Plan | undefined;
    if (orden?.tipo === 'revisar') {
      planRevisado = await planNumero(orden.numero);
      if (!planRevisado || planRevisado.estado === 'descartado') {
        limpia();
        nota(t, !planRevisado ? `No hay ningún plan ${orden.numero} en este proyecto.` : `El plan ${orden.numero} está descartado: «reactiva el plan ${orden.numero}» para seguir con él.`, { ...d, executor: 'pgocode' }, t0);
        return;
      }
    }
    // 3. Reparto en sesiones paralelas (reparto.ts): una petición con tres o más partes independientes, en una sesión
    // que edita dentro de un repositorio git. Lo confirma el servidor (reglas y embeddings locales, sin tokens de
    // pago), con tiempo máximo; aquí, la tarjeta con la cuenta atrás: si no se pulsa nada, se reparte.
    if (d.needsServer === 'split') {
      const prop = await R.withTimeout(proponeReparto(proyecto.id, ses.carpeta, ses.rama, t, modoDe(ses)), R.SPLIT_MS);
      d = R.resolveSplit(d, prop);
      if (d.route === 'split' && prop && !propuestasRef.current[ses.id]) {
        limpia();
        propuestasRef.current = { ...propuestasRef.current, [ses.id]: { ses, texto: t, prop, hasta: Date.now() + prop.espera_s * 1000, decision: d, ms: performance.now() - t0 } };
        setPropuestas(propuestasRef.current);
        temporizadores.current[ses.id] = setTimeout(() => lanzaReparto(ses.id), prop.espera_s * 1000);
        return;
      }
      if (d.route === 'split') d = { ...d, route: 'agent', executor: 'agent', reason: 'ya hay un reparto propuesto en esta sesión' };
    }
    // Gasto antes del turno, para anotar los avisos que salte (sin interrumpir).
    const gAntes = CO.gasto(forzado !== undefined || opc.sesion ? conversacion(ses.id) : mensajes);
    const aSes = CO.efectivos(ses.agente, ses.avisos);
    const modoPlan = orden !== null; // crear o revisar un plan
    const id = ses.id;
    ponCola(id, C.empieza);
    let fallo = false;
    const pon = (m: Mensaje[], final = false) => {
      vivas.current[id] = m;
      if (idActual.current === id) setMensajes(m);
      if (final) guardaConversacion(id, m);
    };
    // Envío automático: la sesión acaba de abrirse y el estado aún puede ser el de la anterior; se lee la guardada.
    let previos = forzado !== undefined || opc.sesion ? conversacion(id) : mensajes;
    // «Continuar desde aquí» pendiente: se aplica con este mensaje. Lo de después queda en la vista, fuera del
    // contexto del agente; al agente va el punto (o, si no puede cortar su sesión, el traspaso).
    let corte: AP.CutRequest | undefined;
    if (ses.corte !== undefined && prov.tipo === 'agente' && ses.corte < previos.length) {
      corte = AP.cutRequest(previos, ses.corte);
      previos = AP.markOutside(previos, ses.corte);
    }
    // Gestión del contexto de la sesión (contextPolicy.ts): poda, relevo y compactación según el agente.
    const politica = contextPolicy(ses.agente, ses);
    // Relevo: el mensaje va a una sesión nueva del agente con la memoria de trabajo en vez de reenviar todo el
    // historial, cuando la política del agente lo dice (shouldRelay). El chat no cambia. Nunca con adjuntos, planes
    // ni preguntas aparte (que no pasan por aquí); «Continuar desde aquí» manda sobre él.
    const sesionPrevia = opc.sesionAgente ?? ses.sesionAgente;
    let memoria = '', relevo: Mensaje['relevo'];
    // Con poda (Claude Code), el relevo solo de reserva: si su historial no se puede podar (relayAsBackup).
    const modoRelevo = relayAsBackup(politica, sesionPrevia ? podables.current[sesionPrevia] : undefined);
    if (prov.tipo === 'agente' && modoRelevo !== 'off' && !orden && !opc.crudo && !opc.sinRelevo && sesionPrevia && !corte) {
      // Relevo por vigencia (Antigravity): la vigencia frente al mensaje nuevo; la memoria, solo con lo vigente.
      let fueraVig = 0;
      if (modoRelevo === 'vig') {
        const r = await Promise.race([pideVigencia(ses, sesionPrevia, previos, t).catch(() => null), new Promise<null>(res => setTimeout(() => res(null), 6000))]);
        if (r) { previos = CL.applyChatVigencia(previos, r); fueraVig = r.pendiente || 0; }
      }
      const limpios = previos.filter(m => !m.error);
      const u = [...limpios].reverse().find(m => m.rol === 'assistant' && m.uso?.contexto);
      const mem = W.buildWorkingMemory(modoRelevo === 'vig' ? W.vigentOnly(limpios) : limpios);
      const motivo = W.shouldRelay({ agent: ses.agente, mode: modoRelevo, context: u?.uso?.contexto, idleMinutes: u?.fin ? (Date.now() - u.fin) / 60000 : 0, cacheMinutes: CS.cacheMinutes(limpios, ses.agente),
        hasAgentSession: true, attachments: listos.length, memoryChars: mem.length, outTokens: fueraVig, outMin: umbralVigencia(), forced: relevoForzado.current[ses.id] });
      if (relevoForzado.current[ses.id]) { delete relevoForzado.current[ses.id]; setRelevoPedido(x => ({ ...x, [ses.id]: false })); }
      if (motivo) {
        memoria = mem; relevo = { de: u!.uso!.contexto!, a: W.estimateTokens(mem.length), anterior: sesionPrevia, motivo };
        if (modoRelevo === 'vig') previos = CL.markRelayedOut(previos);
      }
    }
    // La ruta queda en el mensaje (el distintivo); los modificadores que se saben al enviar se le añaden.
    let ruta: R.RouteStamp | undefined = R.stamp(d);
    if (relevo) ruta = R.addModifier(ruta, 'relay');
    const base: Mensaje[] = [...previos.filter(m => !m.error), { rol: 'user', texto: opc.visible ?? t, ...(listos.length ? { adjuntos: listos } : {}), ...(relevo ? { relevo } : {}), ruta }];
    let actual: Mensaje[] = [...base, { rol: 'assistant', texto: '' }];
    pon(actual, true);
    if (forzado === undefined) { setTexto(''); setAdjuntos([]); }
    const ac = new AbortController();
    setEnCursos(x => ({ ...x, [id]: ac }));
    inicioTurno.current[id] = Date.now();
    if (inyectadosRef.current[id]) { const { [id]: _, ...resto } = inyectadosRef.current; inyectadosRef.current = resto; setInyectados(resto); }
    let sesionAgente = sesionPrevia;
    let punto = ''; // el último mensaje del agente en su sesión (para continuar desde esta respuesta más tarde)
    // Skill de la petición por embeddings: «elegir» espera (como mucho 1,5 s) y se la indica al agente;
    // «sugerir» va en paralelo y solo se muestra.
    // Lo que no llegó al agente (turnos cortados antes de que respondiera) va con este mensaje: si no, lo vería el
    // usuario en el chat pero el agente no (p. ej. si el corte fue antes de crear su sesión, este empieza otra).
    const pendiente = prov.tipo === 'agente' && !orden && !opc.crudo ? withUnanswered(previos, t) : { text: t, attachments: [] as Adjunto[] };
    const tAgente = pendiente.text;
    let aEnviar = tAgente;
    const marcaSkill = (k: SkillPgo) => {
      if (k.modo === 'elegida') ruta = R.addModifier(ruta, 'skill');
      actual = actual.map((m, i) => i === actual.length - 2 ? { ...m, skill: k, ...(k.modo === 'elegida' ? { ruta } : {}) } : m);
      pon(actual, true);
    };
    // Crear o revisar un plan: sin skills; el agente recibe la petición de plan (con el plan actual, ya editado).
    if (orden) {
      const actualPlan = planRevisado ? (await leePlan(proyecto.id, directorio, planRevisado.ruta).catch(() => planRevisado)).texto : undefined;
      aEnviar = PL.peticionPlan(orden.peticion, actualPlan);
    } else if (seleccionSkills !== 'agente' && prov.tipo === 'agente' && !opc.crudo && t) {
      const pide = post<Eleccion>('/api/skills/elegir', { proyecto: proyecto.id, carpeta: ses.carpeta, agente: ses.agente, texto: t });
      if (seleccionSkills === 'elegir') {
        const r = await Promise.race([pide.catch(() => null), new Promise<null>(res => setTimeout(() => res(null), 1500))]);
        if (r?.elegida) {
          const n = r.elegida.nombre;
          aEnviar = ses.agente === 'claude-code' ? `/${n} ${tAgente}` : `Usa la skill «${n}».\n\n${tAgente}`;
          marcaSkill({ nombre: n, puntuacion: r.elegida.puntuacion, modo: 'elegida' });
        }
      } else {
        pide.then(r => { if (r.elegida && idActual.current === id) marcaSkill({ nombre: r.elegida.nombre, puntuacion: r.elegida.puntuacion, modo: 'sugerida' }); }).catch(() => {});
      }
    }
    // Lo que ha hecho pgocode sin el agente desde su último turno (commits, pruebas…): una línea, solo si hubo algo.
    if (!orden && !opc.crudo && prov.tipo === 'agente') {
      const hecho = DR.sinceLastTurn(previos, aEnviar);
      if (hecho) aEnviar += '\n\n' + hecho;
    }
    // Preguntas aparte que el usuario ha incorporado: van al final de este mensaje (el agente no gasta otro turno).
    if (!orden && prov.tipo === 'agente' && incorporar.current[id]?.length) {
      aEnviar += AP.incorporacion(incorporar.current[id]);
      incorporar.current[id] = [];
      setApartes(x => ({ ...x, [id]: (x[id] || []).filter(a => !a.incorporada) }));
    }
    if (relevo) aEnviar = W.relayMessage(memoria, aEnviar);
    if (ruta) registraRuta(id, ruta, t0);
    const ultimo = (f: (m: Mensaje) => Mensaje) => { actual = [...actual.slice(0, -1), f(actual[actual.length - 1])]; pon(actual); };
    // Avisos de gasto mientras trabaja el agente (no al final): cada umbral una vez por turno, sin interrumpir.
    let gPrevio = gAntes;
    let avisadoTurno = false;
    const avisosEnVivo = () => {
      const g = CO.gasto(actual);
      const l = CO.nuevos(gPrevio, g, { ...aSes, avisoTurno: undefined });
      const turno = { coste: g.coste - gAntes.coste, tokens: g.entrada + g.salida - gAntes.entrada - gAntes.salida };
      if (!avisadoTurno && ((aSes.avisoTurno && turno.coste >= aSes.avisoTurno) || turno.tokens >= 2_000_000)) {
        avisadoTurno = true;
        l.push({ nivel: 'aviso', texto: `Este turno lleva ${CO.dinero(turno.coste)} y ${CO.tokens(turno.tokens)} tokens (y sigue).` });
      }
      gPrevio = g;
      avisa(l);
    };
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
    ultimoEvento.current[ses.id] = Date.now();
    try {
      await chat({
        proyecto: proyecto.id, proveedor: prov.id, modelo: ses.modelo || prov.por_defecto, carpeta: ses.carpeta, rama: ses.rama,
        // «Continuar desde aquí» manda sobre el relevo: el agente sigue desde el punto elegido (o con su traspaso).
        permisos: modoPlan ? 'lectura' : opc.permisos || modoDe(ses), sesion_agente: corte?.punto ? corte.sesion : relevo ? undefined : sesionPrevia, esfuerzo: ses.esfuerzo || '',
        ...(corte ? { corte: corte.punto || '', traspaso: corte.traspaso } : {}),
        sesion: ses.id, titulo: ses.nombre, adjuntos: [...pendiente.attachments, ...listos].map(a => a.ruta), autorizadas: ses.autorizadas || [], mcp: ses.mcp || [], tarea_id: ses.tareaId || 0, tarea: ses.tarea || '',
        idioma: navigator.language, compactar: politica.compactAt, contexto_previo: lastContext(previos), podar: politica.prune,
        // Lo que ha escrito el usuario (sin memoria de trabajo ni notas de pgocode): de ahí saca la memoria del proyecto.
        texto_usuario: t,
        // Claude Code: el TTL de la caché de la sesión según el ritmo de sus turnos (cacheStatus.ts).
        ...(ses.agente === 'claude-code' ? { cache_ttl: CS.chooseClaudeTTL(previos) } : {}),
        // Los agentes llevan su propio historial: solo el último mensaje (con la skill indicada, si pgocode la eligió).
        mensajes: prov.tipo === 'agente' ? [{ rol: 'user', texto: aEnviar }] : base,
      }, e => {
        ultimoEvento.current[ses.id] = Date.now();
        // Poda del historial del agente al empezar el turno: lo que queda fuera, en los intercambios anteriores; lo
        // retirado, en este turno.
        if (e.poda) {
          const r = e.poda;
          ponLimpieza(ses.id, { r });
          if (r.podable !== undefined && sesionPrevia) podables.current[sesionPrevia] = Boolean(r.podable);
          actual = PR.applyPruneReport(actual, r);
          ultimo(m => (r.retirados > 0 || r.repuestos ? { ...m, pruned: { tokens: r.retirados, ...(r.repuestos ? { repuestos: r.repuestos } : {}) } } : m));
        }
        // Sugerencia de subagentes (modificador que aplica el servidor): al distintivo del mensaje del usuario.
        if (e.subagentes) {
          ruta = R.addModifier(ruta, 'subagents');
          actual = actual.map((m, i) => i === actual.length - 2 && m.rol === 'user' ? { ...m, ruta } : m);
          pon(actual, true);
        }
        // La memoria del proyecto (el agente empieza de cero): también al distintivo.
        if (e.memoria) {
          ruta = R.addModifier(ruta, 'memory');
          actual = actual.map((m, i) => i === actual.length - 2 && m.rol === 'user' ? { ...m, ruta } : m);
          pon(actual, true);
        }
        if (e.texto) parte({ tipo: 'texto', texto: e.texto });
        if (e.pensamiento) parte({ tipo: 'pensamiento', texto: e.pensamiento });
        // Los tokens de cada acción, como los de la conversación hasta ella (los de antes del turno y los de este).
        if (e.herramienta) parte({ tipo: 'herramienta', h: e.herramienta.tokens ? { ...e.herramienta, tokens: gAntes.entrada + gAntes.salida + e.herramienta.tokens } : e.herramienta });
        if (e.vivo) ultimo(m => ({ ...m, vivo: e.vivo }));
        // La lista del agente recuerda por dónde iba el turno al actualizarla (lo de después lo sincroniza todoSync.ts).
        if (e.tarea) ultimo(m => {
          const t = e.tarea!.tipo === 'lista' && !e.tarea!.padre ? { ...e.tarea!, tras: TS.actionsOf(m).length } : e.tarea!;
          return { ...m, tareas: T.fusiona(m.tareas, T.conTiempos(m.tareas, t, Date.now())) };
        });
        // Puede haber varios consumos en un turno (OpenCode da uno por paso; Claude con tareas en segundo plano, más
        // de un «result»): se suman, salvo el contexto, que es el de la última llamada.
        if (e.uso) {
          ultimo(m => ({ ...m, uso: m.uso ? { entrada: m.uso.entrada + e.uso!.entrada, salida: m.uso.salida + e.uso!.salida,
            coste: (m.uso.coste || 0) + (e.uso!.coste || 0) || undefined, segundos: (m.uso.segundos || 0) + (e.uso!.segundos || 0) || undefined,
            contexto: e.uso!.contexto || m.uso.contexto } : e.uso }));
          avisosEnVivo();
        }
        if (e.sesion_agente) sesionAgente = e.sesion_agente;
        if (e.punto) punto = e.punto;
        if (e.error) fallo = true;
        if (e.error) ultimo(m => ({ ...m, texto: (m.texto ? m.texto + '\n\n' : '') + '⚠ ' + e.error, error: true }));
      }, ac.signal);
      // Plan: la respuesta se guarda como plan (el mismo fichero si se revisa uno) y sale su tarjeta, con su número.
      if (orden && !fallo && !ac.signal.aborted) {
        const txt = PL.textoDelPlan(actual[actual.length - 1]);
        if (txt) {
          try {
            const p = await guardaPlan(proyecto.id, directorio, { ruta: planRevisado?.ruta, titulo: PL.tituloPlan(txt, orden.peticion), sesion: ses.id, agente: ses.agente, texto: txt, estado: 'borrador' });
            // Las tarjetas anteriores del mismo plan (si se ha revisado) pasan a su título actual.
            actual = actual.map(x => (x.plan?.ruta === p.ruta ? { ...x, plan: { ...x.plan, titulo: p.titulo, estado: 'borrador' as const } } : x));
            ultimo(m => ({ ...m, plan: { ruta: p.ruta, titulo: p.titulo, estado: 'borrador', numero: p.numero } }));
          } catch (e) {
            ultimo(m => ({ ...m, texto: m.texto + '\n\n⚠ No se ha podido guardar el plan: ' + (e as Error).message }));
          }
        }
      }
    } catch (err) {
      fallo = true;
      if (!ac.signal.aborted) ultimo(m => ({ ...m, texto: '⚠ ' + (err as Error).message, error: true }));
    } finally {
      // Los mensajes que se le mandaron durante el turno quedan con su respuesta.
      const iny = inyectadosRef.current[id];
      if (iny?.length) {
        ultimo(m => ({ ...m, inyectados: iny }));
        const { [id]: _, ...resto } = inyectadosRef.current;
        inyectadosRef.current = resto;
        setInyectados(resto);
      }
      // Un «Barrer ahora» durante el turno: lo retirado, en este turno.
      const barrido = barridoPend.current[id];
      if (barrido) {
        delete barridoPend.current[id];
        ultimo(m => ({ ...m, pruned: { ...m.pruned, tokens: (m.pruned?.tokens || 0) + barrido } }));
      }
      // Lo que quedó «en curso» ya no lo está: el agente ha terminado (o se ha parado).
      if (actual[actual.length - 1]?.tareas) ultimo(m => ({ ...m, tareas: T.alAcabar(m.tareas, ac.signal.aborted, Date.now()) }));
      ultimo(m => { const { vivo: _, ...r } = m; return { ...r, fin: Date.now(), ...(punto && sesionAgente ? { punto: { sesion: sesionAgente, id: punto } } : {}) }; });
      // Relevo: lo que ocupa de verdad el contexto de la sesión nueva (la memoria más lo fijo del agente).
      const ctxNuevo = actual[actual.length - 1].uso?.contexto;
      if (relevo && ctxNuevo) actual = actual.map((m, k) => k === actual.length - 2 && m.relevo ? { ...m, relevo: { ...m.relevo, a: ctxNuevo } } : m);
      pon(actual, true);
      // Avisos del reparto que llegaron mientras esta sesión (madre) trabajaba, y uniones que esperaban a que acabara.
      const notas = notasMadre.current[id];
      if (notas?.length) {
        delete notasMadre.current[id];
        actual = [...actual, ...notas.map(texto => ({ rol: 'assistant' as const, texto, nota: true, reparto: true }))];
        pon(actual, true);
      }
      if (!fallo && !ac.signal.aborted) reintentaUniones(id);
      pliegaApartes(id);
      if (fallo) ponCola(id, C.falla);
      delete vivas.current[id];
      setEnCursos(x => { if (x[id] !== ac) return x; const n = { ...x }; delete n[id]; return n; });
      // El nombre, del primer mensaje del usuario (no de uno que haya redactado pgocode, como «Pásaselo al agente»).
      alCambiarSesion({ ...ses, nombre: ses.nombre || (opc.generado ? '' : nombreDe(t || listos.map(a => a.nombre).join(", "))), sesionAgente, ultima: Date.now(), corte: undefined });
      if (!ac.signal.aborted && sesionAgente) refrescaVigencia(ses, sesionAgente);
    }
  }

  // ---- Órdenes directas (direct.ts): pgocode las hace sin el agente ----
  const peticionDirecta = (ses: Sesion, accion: DR.DirectKind) => ({ proyecto: proyecto?.id, carpeta: ses.carpeta, rama: ses.rama, sesion: ses.id, accion });

  // La respuesta de una pregunta de estado (sin el agente). null: no se sabe responder (que lo haga el agente).
  async function respuestaDirecta(ses: Sesion, kind: DR.DirectKind, ms: Mensaje[], signal?: AbortSignal): Promise<{ texto: string; directo: DR.DirectNote } | null> {
    if (DR.answeredHere(kind)) {
      post('/api/directo', peticionDirecta(ses, kind)).catch(() => {}); // solo se anota (para medir lo ahorrado)
      const texto = kind === 'cost' ? DR.costAnswer(ms, leeLS<Uso | undefined>(claveGastoAparte(ses.id), undefined)) : DR.lastTurnAnswer(ms);
      return { texto, directo: { accion: kind, ok: true } };
    }
    const r = await post<DR.DirectResult>('/api/directo', peticionDirecta(ses, kind), signal);
    if (r.sin_orden) return null;
    if (kind === 'files') return { texto: DR.filesAnswer(ms, r.ok ? r.ficheros || [] : undefined), directo: { accion: kind, ok: true } };
    return DR.resultNote(r);
  }

  // directo: la orden (o la pregunta) y la nota de pgocode con el resultado, en la conversación (el agente no la ve).
  // Mientras se ejecuta, la sesión cuenta como ocupada: lo que se escriba espera en la cola y «Parar» la corta.
  // d y t0: la decisión del enrutador (se registra cuando pgocode lo ha hecho; si no sabe, la registra el envío al agente).
  async function directo(ses: Sesion, t: string, kind: DR.DirectKind, d?: R.RouteDecision, t0 = performance.now()) {
    if (!proyecto) return;
    const id = ses.id;
    const previos = conversacion(id);
    const pon = (m: Mensaje[]) => { if (idActual.current === id) setMensajes(m); guardaConversacion(id, m); };
    const usuario: Mensaje = { rol: 'user', texto: t, ...(d ? { ruta: R.stamp(d) } : {}) };
    const msDecision = performance.now() - t0;
    const ac = new AbortController();
    const largo = !DR.answeredHere(kind);
    if (largo) {
      setEnCursos(x => ({ ...x, [id]: ac }));
      ponCola(id, C.empieza);
      pon([...previos, usuario, { rol: 'assistant', texto: DR.runningText(kind), nota: true, directo: { accion: kind, curso: true } }]);
    }
    let fallo = false;
    try {
      const r = await respuestaDirecta(ses, kind, previos, ac.signal);
      if (!r) {
        // No se sabe con seguridad cómo hacerlo en este proyecto: al agente, como siempre.
        pon(previos);
        setEnCursos(x => { if (x[id] !== ac) return x; const n = { ...x }; delete n[id]; return n; });
        await envia(t, [], { sinDirecto: true, sesion: idActual.current === id ? undefined : ses, t0, reason: 'pgocode no sabe con seguridad cómo hacerlo en este proyecto: al agente' });
        return;
      }
      if (d) registraRuta(id, R.stamp(d), t0, msDecision);
      fallo = r.directo.ok === false;
      pon([...previos, usuario, { rol: 'assistant', texto: r.texto, nota: true, directo: r.directo, fin: Date.now() }]);
    } catch (e) {
      fallo = true;
      const parado = ac.signal.aborted;
      pon([...previos, usuario, { rol: 'assistant', texto: parado ? '■ Parado.' : '⚠ ' + (e as Error).message, nota: true, directo: { accion: kind, ok: false } }]);
    } finally {
      if (largo) {
        // Si ha fallado, la cola se detiene (que decida el usuario antes de seguir).
        if (fallo) ponCola(id, C.falla);
        setEnCursos(x => { if (x[id] !== ac) return x; const n = { ...x }; delete n[id]; return n; });
      }
    }
  }

  // directoAparte: con el agente trabajando, la pregunta de estado se responde ya como pregunta aparte (sin tocar
  // la conversación del turno en curso).
  async function directoAparte(ses: Sesion, t: string, d: R.RouteDecision, t0: number) {
    const kind = d.direct!;
    registraRuta(ses.id, R.stamp(d), t0);
    const idA = Math.random().toString(36).slice(2);
    const pon = (f: (l: Aparte[]) => Aparte[]) => setApartes(x => ({ ...x, [ses.id]: f(x[ses.id] || []) }));
    const cambia = (c: Partial<Aparte>) => pon(l => l.map(a => a.id === idA ? { ...a, ...c } : a));
    const ms = vivas.current[ses.id] || (idActual.current === ses.id ? mensajes : conversacion(ses.id));
    // El distintivo, el de la decisión (función directa respondida aparte), el mismo que queda en el registro.
    pon(l => [...l, { id: idA, pregunta: t, destino: 'estado', texto: '', acciones: 0, curso: true, motivo: 'lo sabe pgocode (función directa)', auto: true, engine: 'rules', pos: ms.length, ruta: R.stamp(d) }]);
    if (ses.id === sesion?.id) pegado.current = true;
    try {
      const r = await respuestaDirecta(ses, kind, ms);
      cambia({ curso: false, texto: r ? r.texto + (r.directo.salida ? '\n\n```\n' + r.directo.salida + '\n```' : '') : 'No lo sé responder sin el agente.' });
    } catch (e) {
      cambia({ curso: false, error: true, texto: '⚠ ' + (e as Error).message });
    }
  }

  // ---- Reparto en sesiones paralelas ----
  const repartoDe = (id: string) => repartosRef.current.find(r => r.id === id);
  function ponReparto(id: string, f: (r: RP.Reparto) => RP.Reparto) {
    const l = repartosRef.current.map(r => (r.id === id ? f(r) : r));
    repartosRef.current = l;
    setRepartos(l);
    RP.guarda(l);
  }

  // Una nota en el chat de la sesión madre (si está trabajando, al acabar su turno: no se pisa su conversación).
  function anotaMadre(id: string, texto: string, usuario?: string, ruta?: R.RouteStamp) {
    if (vivas.current[id] && !usuario) { (notasMadre.current[id] ||= []).push(texto); return; }
    const m: Mensaje[] = [...conversacion(id), ...(usuario ? [{ rol: 'user' as const, texto: usuario, ...(ruta ? { ruta } : {}) }] : []), { rol: 'assistant', texto, nota: true, reparto: true }];
    guardaConversacion(id, m);
    if (idActual.current === id) setMensajes(m);
  }

  // «Hacerlo en esta sesión»: se cancela el reparto y la petición sigue como siempre.
  function enEstaSesion(id: string) {
    const p = propuestasRef.current[id];
    if (!p) return;
    clearTimeout(temporizadores.current[id]);
    delete temporizadores.current[id];
    const { [id]: _, ...resto } = propuestasRef.current;
    propuestasRef.current = resto;
    setPropuestas(resto);
    envia(p.texto, [], { sesion: p.ses, sinReparto: true, reason: 'propuesta de reparto: has preferido hacerlo en esta sesión' });
  }

  // «Repartir» (o la cuenta atrás a cero): las partes pasan a sesiones hijas, de cuatro en cuatro.
  function lanzaReparto(id: string) {
    const p = propuestasRef.current[id];
    if (!p) return;
    clearTimeout(temporizadores.current[id]);
    delete temporizadores.current[id];
    const { [id]: _, ...resto } = propuestasRef.current;
    propuestasRef.current = resto;
    setPropuestas(resto);
    const rep = RP.nuevo(Date.now().toString(36) + Math.random().toString(36).slice(2, 5), { id: p.ses.id, proyecto: p.ses.proyecto, carpeta: p.ses.carpeta }, p.prop, Date.now(), i => `${p.ses.id}-p${i}`, RP.hijasDe(repartosRef.current, p.ses.id));
    madres.current[rep.id] = p.ses;
    repartosRef.current = [rep, ...repartosRef.current];
    setRepartos(repartosRef.current);
    RP.guarda(repartosRef.current);
    const n = rep.hijas.length;
    anotaMadre(p.ses.id, `Repartida en ${n} sesiones paralelas${n > RP.MAX_HIJAS ? ` (${RP.MAX_HIJAS} a la vez, el resto en cola)` : ''}, cada una en su copia del repositorio. Al acabar se unen sus cambios a esta sesión.`, p.texto, R.stamp(p.decision));
    registraRuta(p.ses.id, R.stamp(p.decision), 0, p.ms);
    arrancaHijas(rep.id);
  }

  function arrancaHijas(idR: string) {
    const r = repartoDe(idR);
    if (!r) return;
    for (const h of RP.aLanzar(r)) {
      ponReparto(idR, x => RP.pon(x, h.n, { estado: 'trabajando' }));
      lanzaHija(idR, h.n);
    }
  }

  // Una hija: su copia de trabajo, su sesión (con los ajustes de la madre) y su turno; al acabar, su unión.
  async function lanzaHija(idR: string, n: number) {
    const r = repartoDe(idR);
    const madre = madres.current[idR];
    const h = r?.hijas.find(x => x.n === n);
    if (!r || !h || !madre) { if (r) ponReparto(idR, x => RP.pon(x, n, { estado: 'parada' })); return; }
    try {
      const c = await preparaHija(r.proyecto, r.madre, r.carpeta, h.idx || h.n, h.sesion);
      const base = nuevaSesionHija(madre, c.carpeta, c.rama, n, h.parte, h.sesion);
      ponReparto(idR, x => RP.pon({ ...x, ramaMadre: x.ramaMadre || c.rama_madre }, n, { carpeta: c.carpeta, rama: c.rama }));
      alCambiarSesion(base);
      await envia(RP.mensajeHija({ intro: r.intro, outro: r.outro, partes: r.hijas.map(x => x.parte) }, n), [], { sesion: base, sinReparto: true, reason: `parte ${n} de un reparto` });
      const ms = conversacion(h.sesion);
      const estado = RP.estadoAlAcabar(paradas.current.delete(h.sesion), ms[ms.length - 1]);
      ponReparto(idR, x => RP.pon(x, n, { estado }));
      if (estado === 'hecha') await uneParte(idR, n, false);
    } catch (e) {
      ponReparto(idR, x => RP.pon(x, n, { estado: 'error', motivo: (e as Error).message }));
    }
    arrancaHijas(idR);
    revisaResumen(idR);
  }

  // La sesión de una hija: la de la madre en la carpeta y la rama de su copia.
  function nuevaSesionHija(madre: Sesion, carpeta: string, rama: string, n: number, parte: string, id: string): Sesion {
    const { corte: _c, plan: _p, modoPrevio: _m, sesionAgente: _a, tareaId, tarea, ...ajustes } = madre;
    return { ...ajustes, id, nombre: RP.nombreHija(n, parte), carpeta, rama, permisos: modoDe(madre), tareaId, tarea, madre: madre.id, parte: n, creada: Date.now(), ultima: Date.now() };
  }

  // Unir la rama de una hija a la de la madre (el servidor lo hace solo si la madre está limpia y sin agente trabajando).
  async function uneParte(idR: string, n: number, manual: boolean) {
    const r = repartoDe(idR);
    const h = r?.hijas.find(x => x.n === n);
    if (!r || !h?.carpeta || !h.rama || h.quitada) return;
    try {
      const u = await uneHija(r.proyecto, r.carpeta, r.ramaMadre, h.carpeta, h.rama, n, h.parte);
      // Un conflicto que ha unido el agente de la madre: ya está en su rama («sin cambios» para git), pero está unida.
      const union = h.resolving && u.estado === 'sin_cambios' ? 'unida' : u.estado;
      ponReparto(idR, x => RP.pon(x, n, { union, motivo: u.motivo, ficheros: u.ficheros, aviso: u.aviso, quitada: u.quitada, ...(u.estado !== 'conflicto' ? { resolving: undefined } : {}) }));
      const nueva = { ...h, union, ficheros: u.ficheros, motivo: u.motivo, quitada: u.quitada };
      // Unida y sin copia: la sesión de la parte pasa a la carpeta de la madre y se pliega bajo ella en la lista.
      if (u.quitada) window.dispatchEvent(new CustomEvent('pgo:reubica-sesion', { detail: { id: h.sesion, proyecto: r.proyecto, carpeta: r.carpeta, rama: r.ramaMadre, merged: u.estado === 'unida' || u.estado === 'sin_cambios' } }));
      if (u.estado === 'conflicto') anotaMadre(r.madre, RP.avisoConflicto(nueva));
      else if (manual) anotaMadre(r.madre, RP.fraseManual(nueva));
    } catch (e) {
      ponReparto(idR, x => RP.pon(x, n, { union: 'error', motivo: (e as Error).message }));
    }
    revisaResumen(idR);
  }

  // Al acabar todas (y resuelta cada unión), el resumen en el chat de la madre.
  function revisaResumen(idR: string) {
    const r = repartoDe(idR);
    if (!r || r.resumido || !RP.acabado(r) || r.hijas.some(h => h.estado === 'hecha' && h.union === 'sin')) return;
    ponReparto(idR, x => ({ ...x, resumido: true }));
    const datos: Record<string, RP.DatosHija> = {};
    for (const h of r.hijas) {
      const ms = conversacion(h.sesion);
      const g = CO.gasto(ms);
      datos[h.sesion] = { respuesta: RP.ultimaRespuesta(ms), entrada: g.entrada, salida: g.salida, coste: g.coste };
    }
    anotaMadre(r.madre, RP.resumen(r, datos));
  }

  // La madre ha dejado de trabajar: las uniones que esperaban se reintentan; y las partes en conflicto que se le
  // pidieron al agente, si su último mensaje era ese (si ya las ha unido, salen «sin cambios» y se quita su copia).
  function reintentaUniones(madreId: string) {
    const ultimo = [...conversacion(madreId)].reverse().find(m => m.rol === 'user')?.texto || '';
    for (const r of repartosRef.current) {
      if (r.madre !== madreId) continue;
      for (const h of r.hijas) {
        if (h.estado === 'hecha' && h.union === 'pendiente') uneParte(r.id, h.n, true);
        else if (h.union === 'conflicto' && h.resolving && h.rama && ultimo.includes('git merge ' + h.rama)) uneParte(r.id, h.n, true);
      }
    }
  }

  // «Resolver con el agente»: a la sesión madre (la abierta: la tarjeta sale en su chat), un mensaje para que una la
  // rama de la parte y resuelva el conflicto. No da nombre a la sesión (lo ha redactado pgocode).
  function resuelveConAgente(idR: string, n: number) {
    const r = repartoDe(idR);
    const h = r?.hijas.find(x => x.n === n);
    if (!r || !h?.rama) return;
    ponReparto(idR, x => RP.pon(x, n, { resolving: true }));
    aConversacion(r.madre, RP.mensajeResolver(h, r.ramaMadre), [], true, { engine: 'user', reason: '«Resolver con el agente» (conflicto del reparto)', forced: true, generated: true });
  }

  function paraHija(idR: string, n: number) {
    const h = repartoDe(idR)?.hijas.find(x => x.n === n);
    if (!h) return;
    if (h.estado === 'espera') { ponReparto(idR, x => RP.pon(x, n, { estado: 'parada' })); revisaResumen(idR); return; }
    paradas.current.add(h.sesion);
    enCursosRef.current[h.sesion]?.abort();
  }

  // Deshacer el relevo: el mismo mensaje, a la sesión anterior del agente (con todo su historial). Solo con el último
  // mensaje del usuario y el agente libre; lo que respondió la sesión nueva se quita del chat.
  function deshaceRelevo(i: number) {
    const m = mensajes[i];
    if (!sesion || enCurso || !m?.relevo) return;
    const resto = mensajes.slice(0, i);
    setMensajes(resto);
    guardaConversacion(sesion.id, resto);
    envia(m.texto, m.adjuntos || [], { sinRelevo: true, sesionAgente: m.relevo.anterior });
  }

  // Estado del plan en las tarjetas de la conversación (se guarda con ella).
  // Se parte siempre de la conversación guardada (la más reciente): así un cambio no pisa al anterior.
  function marcaPlan(ruta: string, estado: EstadoPlan | 'borrado', titulo?: string) {
    if (!sesion) return;
    const m = conversacion(sesion.id)
      .map(x => x.plan?.ruta === ruta ? { ...x, plan: { ...x.plan, estado, titulo: titulo || x.plan.titulo } } : x);
    if (idActual.current === sesion.id) setMensajes(m);
    guardaConversacion(sesion.id, m);
  }

  // Una orden y su respuesta de pgocode en el chat (sin agente): «descarta el plan 2» → «Plan #2 descartado».
  // d y t0: la decisión del enrutador (una orden que ha resuelto pgocode): queda en el mensaje y se registra.
  function nota(orden: string, respuesta: string, d?: R.RouteDecision, t0 = performance.now()) {
    if (!sesion) return;
    if (d) registraRuta(sesion.id, R.stamp(d), t0);
    const m: Mensaje[] = [...conversacion(sesion.id), { rol: 'user', texto: orden, ...(d ? { ruta: R.stamp(d) } : {}) }, { rol: 'assistant', texto: respuesta, nota: true }];
    if (idActual.current === sesion.id) setMensajes(m);
    guardaConversacion(sesion.id, m);
  }

  // El plan con ese número en el proyecto.
  async function planNumero(n: number): Promise<Plan | undefined> {
    if (!proyecto) return undefined;
    return (await listaPlanes(proyecto.id, directorio).catch(() => [] as Plan[])).find(p => p.numero === n);
  }

  // Órdenes de estado de un plan, en local: descartar, reactivar, borrar; y ejecutar (que sí va al agente).
  // d y t0: la decisión del enrutador. Lo que responde pgocode se registra con la nota; «ejecutar», con el envío al agente.
  async function ordenPlan(o: PL.OrdenPlan, t: string, d?: R.RouteDecision, t0 = performance.now()) {
    if (!proyecto || o.tipo === 'nuevo' || o.tipo === 'revisar') return;
    const n = (r: string): void => nota(t, r, d && { ...d, executor: 'pgocode' }, t0);
    const p = await planNumero(o.numero);
    if (!p) { n(`No hay ningún plan ${o.numero} en este proyecto.`); return; }
    const nombre = `Plan #${p.numero} «${p.titulo}»`;
    try {
      switch (o.tipo) {
        case 'descartar':
          if (p.estado === 'descartado') { n(`${nombre} ya estaba descartado.`); return; }
          await guardaPlan(proyecto.id, directorio, { ruta: p.ruta, estado: 'descartado' });
          marcaPlan(p.ruta, 'descartado'); n(`${nombre} descartado. Puedes reactivarlo («reactiva el plan ${p.numero}») o borrarlo («borra el plan ${p.numero}»).`);
          return;
        case 'reactivar':
          if (p.estado === 'borrador') { n(`${nombre} ya está por decidir.`); return; }
          await guardaPlan(proyecto.id, directorio, { ruta: p.ruta, estado: 'borrador' });
          marcaPlan(p.ruta, 'borrador'); n(`${nombre} vuelve a estar por decidir.`);
          return;
        case 'borrar':
          if (p.estado !== 'descartado') { n(`Solo se borran los planes descartados: «descarta el plan ${p.numero}» antes.`); return; }
          await borraPlan(proyecto.id, directorio, p.ruta);
          marcaPlan(p.ruta, 'borrado'); n(`${nombre} borrado.`);
          return;
        case 'ejecutar':
          if (p.estado === 'descartado') { n(`${nombre} está descartado: «reactiva el plan ${p.numero}» para ejecutarlo.`); return; }
          await ejecutaPlan(p, t, d, t0);
          return;
      }
    } catch (e) { n('⚠ ' + (e as Error).message); }
  }

  // Ejecutar: el plan pasa a «ejecutado» y el agente lo recibe con permiso para editar (si la sesión es de solo
  // lectura, solo en este turno).
  async function ejecutaPlan(p: Plan, orden?: string, d?: R.RouteDecision, t0?: number) {
    if (!sesion || !proyecto || enCurso) return;
    const texto = p.texto ?? (await leePlan(proyecto.id, directorio, p.ruta)).texto ?? '';
    await guardaPlan(proyecto.id, directorio, { ruta: p.ruta, estado: 'ejecutado' });
    marcaPlan(p.ruta, 'ejecutado', p.titulo);
    setPlanAbierto(null);
    const modo = modoDe(sesion);
    envia(PL.peticionEjecucion(p.ruta, texto), [], { crudo: true, permisos: modo === 'lectura' ? 'editar' : modo, visible: orden || `Ejecuta el plan #${p.numero ?? ''} «${p.titulo}»`,
      decision: d ?? { route: 'command', detail: 'plan:ejecutar', executor: 'agent', engine: 'user', reason: 'botón «Ejecutar» del plan', modifiers: ['plan'], text: orden || '' }, t0 });
  }

  async function descartaPlan(ruta: string) {
    if (!sesion || !proyecto) return;
    await guardaPlan(proyecto.id, directorio, { ruta, estado: 'descartado' }).catch(() => {});
    marcaPlan(ruta, 'descartado');
  }

  // «Continuar desde aquí»: se marca el punto y se aplica al enviar el siguiente mensaje (hasta entonces, se deshace).
  const continuaDesde = (i: number) => { if (sesion && !enCurso) alCambiarSesion({ ...sesion, corte: i }); };
  const deshaceCorte = () => { if (sesion) alCambiarSesion({ ...sesion, corte: undefined }); };
  const veAlCorte = () => { if (sesion?.corte !== undefined) lista.current?.querySelector(`[data-indice="${sesion.corte}"]`)?.scrollIntoView({ block: 'center', behavior: 'smooth' }); };

  const teclas = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    // ↑ ↓: el historial de mensajes (solo en la primera o la última línea); Esc: vuelve lo que se escribía.
    if ((e.key === 'ArrowUp' || e.key === 'ArrowDown') && !e.shiftKey && !e.altKey && !e.ctrlKey && !e.metaKey) {
      const el = e.currentTarget;
      const arriba = e.key === 'ArrowUp';
      if (el.selectionStart !== el.selectionEnd || !(arriba ? IH.onFirstLine(el.value, el.selectionStart) : IH.onLastLine(el.value, el.selectionStart))) return;
      const enviados = mensajes.filter(m => m.rol === 'user' && !m.error && !m.nota).map(m => m.texto);
      const r = IH.moveHistory(IH.historyItems(enviados, cola.map(m => m.texto)), historial, arriba ? 'up' : 'down', texto);
      if (!r) return;
      e.preventDefault();
      setHistorial(r.state);
      setTexto(r.text);
      setEditaCola(r.item?.source === 'queue' ? { index: r.item.queueIndex, original: r.item.text } : null);
      requestAnimationFrame(() => { const a = area.current; if (a) a.setSelectionRange(a.value.length, a.value.length); });
      return;
    }
    if (e.key === 'Escape' && historial.pos >= 0) {
      e.preventDefault();
      e.stopPropagation();
      setTexto(historial.draft);
      salHistorial();
      return;
    }
    // Intro: decide pgocode (o lo elegido en la etiqueta; con «Siempre al turno», al turno en curso); Alt+Intro: al
    // turno en curso (si el agente lo admite; si no, a la cola); Ctrl+Intro y Mayús+Intro: nueva línea.
    if (e.key !== 'Enter' || e.shiftKey || e.nativeEvent.isComposing) return;
    e.preventDefault();
    if (e.ctrlKey || e.metaKey) { insertNewline(e.currentTarget); return; }
    decideYEnvia(e.altKey ? (alTurnoPosible ? 'turno' : 'conversacion') : undefined);
  };

  // Una pregunta aparte, en su sitio cronológico (antes del mensaje que llegó después). Plegada: una línea.
  const verAparte = (a: Aparte) => {
    if (!sesion) return null;
    const s: R.RouteStamp = a.ruta || { route: 'aside', executor: executorDeVia(a.destino, a.via), engine: a.engine || 'user', reason: a.motivo, modifiers: [] };
    const distintivo = (a.ruta || a.via || a.destino === 'estado') && <span className="route-badge" title={R.badgeTitle(s) + (a.via?.startsWith('local/') ? ` (${a.via.slice(6)})` : '')}>{R.badge(s)}</span>;
    if (a.plegada) return (
      <div key={a.id} className="mensaje aparte aside-folded" role="note" aria-label="Pregunta aparte (plegada)">
        <button className="enlace aside-toggle recorta" onClick={() => despliegaAparte(sesion.id, a)} title="Desplegar la respuesta" aria-expanded={false}>
          ▸ Aparte · {AP.NOMBRES[a.destino]}: {a.pregunta}
        </button>
        {distintivo}
        <button className="icono" onClick={() => quitaAparte(sesion.id, a)} title="Quitar" aria-label="Quitar la pregunta aparte"><Icono n="cerrar" t={13} /></button>
      </div>
    );
    return (
      <div key={a.id} className="mensaje aparte" role="note" aria-label="Pregunta aparte">
        <div className="aparte-cabecera">
          <span className="aparte-etiqueta">Aparte · {AP.NOMBRES[a.destino]}</span>
          {distintivo}
          <span className="suave pequeno recorta" title={a.motivo}>no queda en la conversación{a.auto ? ' · lo ha decidido pgocode' : ''}</span>
          <span className="hueco" />
          {!a.curso && <button className="icono" onClick={() => despliegaAparte(sesion.id, a)} title="Plegar" aria-label="Plegar la pregunta aparte" aria-expanded>▾</button>}
          <button className="icono" onClick={() => quitaAparte(sesion.id, a)} title={a.curso ? 'Parar y quitar' : 'Quitar'} aria-label="Quitar la pregunta aparte"><Icono n="cerrar" t={13} /></button>
        </div>
        <div className="aparte-pregunta">{a.pregunta}</div>
        {a.texto ? (a.error ? <p className="con-error">{a.texto}</p> : <Texto md={a.texto} />) : a.curso ? <span className="pensando">Pensando…</span> : null}
        {(a.uso || a.acciones > 0) && <div className="uso">{[a.acciones ? `${a.acciones} ${a.acciones === 1 ? 'acción' : 'acciones'} (solo lectura)` : '', a.uso ? usoTexto(a.uso) : ''].filter(Boolean).join(' · ')}</div>}
        <div className="aparte-acciones">
          <button className="enlace" title="Enviar esta pregunta al agente como un mensaje más: la responde él, con todo su contexto (gasta un turno; a la cola si está trabajando). Esta respuesta aparte se quita."
            onClick={() => { aprendeDe(a, 'conversacion'); quitaAparte(sesion.id, a); aConversacion(sesion.id, a.pregunta, [], false, { engine: 'user', reason: 'pregunta aparte llevada a la conversación' }); }}>Preguntárselo al agente</button>
          {SITIOS_APARTE.filter(x => x !== a.destino && !(x === 'estado' && a.auto === false && a.pregunta === '¿Cómo va?')).map(x => (
            <button key={x} className="enlace" title={x === 'estado' ? 'Responder con el estado del trabajo (sin tokens)' : x === 'suelta' ? 'Responder sin contexto: la IA local del equipo (si no está, un modelo barato)' : 'Responder con el contexto de la conversación (copia de la sesión)'}
              onClick={() => { aprendeDe(a, x); a.ac?.abort(); pregunta(sesion, a.pregunta, x, 'lo has cambiado tú', false, { sustituye: a.id }); }}>{AP.NOMBRES[x]}</button>
          ))}
          {!a.curso && a.destino === 'suelta' && a.via?.startsWith('local/') && (
            <button className="enlace" title="La ha respondido la IA local del equipo (0 tokens): repetirla con el modelo barato de pago"
              onClick={() => pregunta(sesion, a.pregunta, 'suelta', 'lo has pedido tú: con el modelo de pago', false, { sustituye: a.id, motor: 'pago' })}>Con el modelo de pago</button>
          )}
          {!a.curso && !a.error && a.texto && a.destino !== 'estado' && (a.incorporada
            ? <span className="suave pequeno">Irá con tu próximo mensaje al agente</span>
            : <button className="enlace" title="No se envía nada ahora: esta pregunta y su respuesta irán al final de tu próximo mensaje al agente, para que las conozca (sin gastar un turno solo para ella)"
              onClick={() => {
                (incorporar.current[sesion.id] ||= []).push({ pregunta: a.pregunta, respuesta: a.texto });
                setApartes(x => ({ ...x, [sesion.id]: (x[sesion.id] || []).map(y => y.id === a.id ? { ...y, incorporada: true } : y) }));
              }}>Que el agente conozca esta respuesta</button>)}
        </div>
      </div>
    );
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
            <button className="miga-item" onClick={alAjustes} title={nombreHosting(sesion.hosting || hosting || repo?.tipo) + ' · propiedades de la sesión'}><Hosting tipo={sesion.hosting || hosting || repo?.tipo} t={16} /></button>
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
        {proyecto && <button className="icono" onClick={() => setVerPlanes(true)} title="Planes del proyecto" aria-label="Planes del proyecto"><Icono n="plan" /></button>}
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
          <ProyectoCapturas.Provider value={proyecto.id}>
          <VisorConversacion.Provider value={abreFoto}>
          {fotoAbierta && <Visor fotos={fotoAbierta.fotos} inicial={fotoAbierta.i} alCerrar={() => setFotoAbierta(null)} />}
          <div className="mensajes" aria-live="polite" ref={lista} onScroll={alDesplazar} onCopy={copiaSeleccion}
            onWheel={marcaGesto} onTouchMove={marcaGesto} onKeyDown={marcaGesto} onMouseDown={marcaGesto}>
            {mensajes.length === 0 && (
              <div className="vacio pequeno">
                <p><b>{titulo(sesion)}</b></p>
                <p className="suave">{prov?.nombre || 'Elige un agente'}{prov?.tipo === 'agente' ? ` · ${{ lectura: 'solo lectura', editar: 'puede editar', auto: 'auto: ejecuta comandos' }[modoDe(sesion)]}` : ''}</p>
              </div>
            )}
            {mensajes.map((m, i) => {
              const fuera = AP.isOutside(mensajes, i, sesion.corte);
              return (<Fragment key={i}>
              {apartesSesion.filter(a => a.pos === i).map(verAparte)}
              {fuera && !AP.isOutside(mensajes, i - 1, sesion.corte) && (
                <div className="outside-context-label" role="note">
                  {m.fuera ? 'Fuera del contexto del agente: lo de aquí abajo ya no lo ve' : 'Fuera del contexto del agente al enviar el siguiente mensaje'}
                </div>
              )}
              <div data-indice={i} className={'mensaje ' + m.rol + (m.error ? ' con-error' : '') + (m.nota ? ' nota' : '') + (m.reparto ? ' reparto' : '') + (fuera ? ' outside-context' : '') + (sesion.corte === i ? ' cut-point' : '')}
                style={!fuera && vigOn && PR.vigOpacity(m.vig) < 1 ? { opacity: PR.vigOpacity(m.vig) } : undefined}
                title={fuera ? 'Fuera del contexto del agente (no lo verá)' : vigOn && m.vig ? PR.vigTitle(m.vig) : undefined}>
                {!fuera && vigOn && m.vig?.fuera && <span className="vig-out" aria-label="Fuera del contexto del agente">⤓</span>}
                {m.pruned && PR.prunedLine(m.pruned) && <div className="pruned-line" role="note">{PR.prunedLine(m.pruned)}</div>}
                {m.texto && !(enCurso && i === mensajes.length - 1) && (
                  <button className="icono copiar-mensaje" onClick={() => copiaTexto(m.texto, i)} title={copiado === i ? 'Copiado' : 'Copiar el mensaje'} aria-label="Copiar el mensaje">
                    <Icono n={copiado === i ? 'hecho' : 'copiar'} t={13} />
                  </button>
                )}
                {m.partes?.length
                  ? <VigFade.Provider value={!fuera && vigOn ? PR.vigOpacity(m.vig) : 0}><Partes partes={m.partes} tareas={m.tareas} detalle={detalle} inicio={i > 0 ? acumulados[i - 1] : 0} vivo={Boolean(enCurso) && i === mensajes.length - 1} error={m.error} /></VigFade.Provider>
                  : m.texto ? (m.rol === 'assistant' && !m.error ? <Texto md={m.texto} /> : m.texto)
                    : (enCurso && i === mensajes.length - 1 ? <span className="pensando">Pensando…{sesion && ultimoEvento.current[sesion.id] ? <span className="actividad-agente">{silenceLabel(Date.now() - ultimoEvento.current[sesion.id])}</span> : null}</span> : '')}
                {m.partes?.length && m.error ? <p className="con-error">{m.texto.split('⚠').slice(1).map(x => '⚠' + x).join('')}</p> : null}
                {m.rol === 'user' && m.ruta && <span className={'route-badge route-' + m.ruta.route} title={R.badgeTitle(m.ruta)}>{R.badge(m.ruta)}</span>}
                {m.directo && <DirectNote d={m.directo} alPasar={t => aConversacion(sesion.id, t, [], true, { engine: 'user', reason: '«Pásaselo al agente»', forced: true, generated: true })} />}
                {(m.inyectados || (enCurso && i === mensajes.length - 1 ? inyectados[sesion.id] : undefined))?.map((x, j) => (
                  <div key={'iny' + j} className="inyectado" title="Lo mandaste mientras el agente trabajaba: lo leyó durante el turno">↳ Tú, durante el turno: {x}
                    <span className="route-badge route-agent" title={R.badgeTitle(INJECTED_STAMP)}>{R.badge(INJECTED_STAMP)}</span></div>
                ))}
                {enCurso && i === mensajes.length - 1 && m.partes?.length ? <span className="pensando">Trabajando…{actividad(m) ? <span className="actividad-agente"> {actividad(m)}</span> : null}{sesion && ultimoEvento.current[sesion.id] ? <span className="actividad-agente">{silenceLabel(Date.now() - ultimoEvento.current[sesion.id], m)}</span> : null}{m.vivo ? <span title="Tokens consumidos en la conversación hasta ahora (sin las preguntas aparte); al acabar coinciden con los de la conversación"> · {CO.tokens(tokensConversacion)} tokens</span> : null}</span> : null}
                {m.uso && detalle !== 'conciso' && !(enCurso && i === mensajes.length - 1) && <div className="uso">{usoTexto(m.uso)} · <span title="Tokens consumidos en toda la conversación hasta este turno, sumando todos sus turnos (sin las preguntas aparte). No es lo que ocupa el contexto.">{CO.tokens(acumulados[i])} acumulados</span></div>}
                {m.relevo && (
                  <div className="relay-line suave pequeno" title={`Este mensaje fue a una sesión nueva del agente con la memoria de trabajo de la conversación (${m.relevo.motivo}), en vez de reenviar todo el historial.`}>
                    ↻ Contexto renovado: de {CO.tokens(m.relevo.de)} a {CO.tokens(m.relevo.a)} tokens
                    {!enCurso && !mensajes.slice(i + 1).some(x => x.rol === 'user') && <> · <button className="enlace" onClick={() => deshaceRelevo(i)} title="Volver a enviar este mensaje a la sesión anterior del agente, con todo su historial (se quita la respuesta de la sesión nueva)">deshacer</button></>}
                  </div>
                )}
                {m.adjuntos && m.adjuntos.length > 0 && (
                  <div className="adjuntos-mensaje">{m.adjuntos.map(a => <span key={a.ruta} className="ficha-adjunto" title={a.ruta}><Icono n="clip" t={12} /> {a.nombre}</span>)}</div>
                )}
                {m.plan && (
                  <TarjetaPlan numero={m.plan.numero} titulo={m.plan.titulo} ruta={m.plan.ruta} estado={m.plan.estado || 'borrador'} ocupado={Boolean(enCurso)}
                    alAbrir={() => setPlanAbierto(m.plan!.ruta)} alEjecutar={() => ejecutaPlan({ ruta: m.plan!.ruta, titulo: m.plan!.titulo, proyecto: proyecto.id, estado: 'borrador' })}
                    alDescartar={() => descartaPlan(m.plan!.ruta)} />
                )}
                {m.skill && (
                  <span className={'sk-marca sk-pgo ' + m.skill.modo} title={m.skill.modo === 'elegida' ? 'pgocode eligió esta skill (embeddings) y se la indicó al agente' : 'pgocode cree que encaja esta skill (embeddings); el agente decide'}>
                    ✦ {m.skill.modo === 'elegida' ? 'elegida' : 'sugerida'}: {m.skill.nombre} · {m.skill.puntuacion.toFixed(2)}
                  </span>
                )}
                {!enCurso && prov?.tipo === 'agente' && AP.canContinueFrom(mensajes, i, sesion.corte) && (
                  <button className="enlace continue-from-here" onClick={() => continuaDesde(i)}
                    title="El agente sigue desde esta respuesta y olvida todo lo posterior (baja el contexto que reenvía). Se aplica al enviar el siguiente mensaje; hasta entonces se puede deshacer.">
                    Continuar desde aquí
                  </button>
                )}
              </div>
              </Fragment>);
            })}
            {apartesSesion.filter(a => a.pos >= mensajes.length).map(verAparte)}
            {sesion && propuestas[sesion.id] && (
              <TarjetaPropuesta prop={propuestas[sesion.id].prop} hasta={propuestas[sesion.id].hasta}
                alRepartir={() => lanzaReparto(sesion.id)} alEnEstaSesion={() => enEstaSesion(sesion.id)} />
            )}
            {sesion && repartos.filter(r => r.madre === sesion.id).map(r => (
              <TarjetaReparto key={r.id} reparto={r} trabajando={h => Boolean(enCursos[h.sesion])}
                alAbrir={h => window.dispatchEvent(new CustomEvent('pgo:abre-sesion', { detail: h.sesion }))}
                alParar={h => paraHija(r.id, h.n)} alUnir={h => uneParte(r.id, h.n, true)} alResolver={h => resuelveConAgente(r.id, h.n)}
                alQuitar={() => { const l = repartosRef.current.filter(x => x.id !== r.id); repartosRef.current = l; setRepartos(l); RP.guarda(l); }} />
            ))}
            <div ref={fin} />
          </div>
          </VisorConversacion.Provider>
          </ProyectoCapturas.Provider>
          <div className="redactor">
            {!sesion.hosting && (
              <div className="aviso-sesion">
                <Icono n="info" t={15} /> Falta elegir el alojamiento de la sesión.
                <button className="enlace" onClick={alAjustes}>Completar propiedades</button>
              </div>
            )}
            {/* Lo que el agente tiene en marcha (subagentes, órdenes en segundo plano): explica por qué sigue ocupado. */}
            {enCurso && T.vivas(mensajes[mensajes.length - 1]?.tareas).length > 0 && (
              <div className="en-marcha" role="status">
                <span className="punto-h curso" />
                <b>En marcha:</b>
                {T.vivas(mensajes[mensajes.length - 1]?.tareas).map(t => (
                  <span key={t.id} className="en-marcha-tarea recorta" title={(t.progreso || '') + (t.detalle ? '\n' + t.detalle : '')}>
                    {t.tipo === 'subagente' ? 'Subagente' : 'Segundo plano'}{t.subtipo ? ' ' + t.subtipo : ''}: {t.titulo}
                  </span>
                ))}
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
                    {c.adjuntos.length === 0 && conTurno && (
                      <button className="enlace aparte-boton" title="Mandarlo ya al turno en curso: el agente lo lee antes de acabar" onClick={() => {
                        ponCola(sesion.id, k => C.quita(k, i));
                        const x = R.fromQueue([c.texto]).text;
                        alTurno(sesion.id, x, [], R.route({ ...routeInput(x, sesion), busy: true, manual: 'turno' }));
                      }}>Al turno</button>
                    )}
                    {c.adjuntos.length === 0 && (
                      <button className="enlace aparte-boton" title="Preguntarlo aparte, sin que entre en la conversación" onClick={() => {
                        ponCola(sesion.id, k => C.quita(k, i));
                        const x = R.fromQueue([c.texto]).text;
                        if (proyecto) ponLS(claveAprendidas(proyecto.id), AP.aprende(leeLS(claveAprendidas(proyecto.id), {}), x, 'contexto'));
                        pregunta(sesion, x, 'contexto', 'lo has llevado aparte', false, { decision: R.route({ ...routeInput(x, sesion), busy: true, manual: 'contexto' }), t0: performance.now() });
                      }}>Aparte</button>
                    )}
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
            {/* Barra de prueba (debajo de la cola): lanzar lo que se está desarrollando (web o programa) en la rama de la sesión. */}
            <Lanzador proyecto={proyecto} carpeta={sesion.carpeta} version={versionProcesos} alAbrirWeb={alAbrirWeb} propuesta={propuesta} />
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
            {sesion.corte !== undefined && sesion.corte < mensajes.length && (
              <div className="cut-notice" role="status">
                <span>↳ El agente sigue desde <button className="enlace" onClick={veAlCorte} title="Ir al mensaje">este mensaje</button> (lo de después no lo verá).</span>
                <button className="enlace" onClick={deshaceCorte} disabled={Boolean(enCurso)}>Deshacer</button>
              </div>
            )}
            {verAyuda && <Ayuda alTurno={siempreAlTurno} inyecta={Boolean(prov?.inyecta)} alCerrar={cierraAyuda} />}
            {rutaViva && avisoContexto && (
              <div className="aviso-contexto" role="note">
                <span>⚠ {avisoContexto}</span>
                <button className="enlace" onClick={() => barreAhora(true)} disabled={Boolean(barriendo[sesion.id])}
                  title="Saca del contexto del agente todo el historial (lecturas, salidas de órdenes, imágenes); la conversación sigue y el agente vuelve a leer lo que necesite">{barriendo[sesion.id] ? 'Vaciando…' : 'Vaciar contexto'}</button>
              </div>
            )}
            {editaCola && <div className="history-notice suave pequeno" role="status">{IH.queueNotice(editaCola.index)}</div>}
            {rutaViva && !editaCola && (
              <div className="destino-mensaje">
                {/* La etiqueta sale SIEMPRE del enrutador (router.ts): el destino y, al pasar el ratón, quién lo hace,
                    quién lo ha decidido y por qué. Clic: elegir otro destino (una función directa, al agente). */}
                <button className="enlace" onClick={() => {
                  baseManual.current = texto;
                  setDestinoManual(rutaViva.route === 'direct' || rutaViva.route === 'command' ? 'conversacion'
                    : AP.siguiente(destinoManual || (rutaViva.route === 'aside' ? rutaViva.aside! : rutaViva.modifiers.includes('turn') ? 'turno' : 'conversacion'), alTurnoPosible));
                }}
                  // Intro con el foco en la etiqueta envía (como en el cuadro), no cambia el destino.
                  onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); decideYEnvia(e.altKey ? (alTurnoPosible ? 'turno' : 'conversacion') : undefined); } }}
                  title={`${R.tagTitle(rutaViva)}. Clic para cambiarlo.${alTurnoPosible ? ' Alt+Intro: al turno en curso;' : ''} Prefijos «>» directo a la conversación, «//» aparte, «?» estado.`}>
                  {R.tagText(rutaViva, Boolean(enCurso))}
                </button>
                {destinoManual && <span className="suave pequeno">elegido por ti</span>}
                {prov?.inyecta && (
                  <label className="casilla suave pequeno" title="Con el agente trabajando, lo que escribas le llega ya (lo lee antes de acabar su turno, también mientras esperan sus tareas en segundo plano) en vez de esperar en la cola. Con adjuntos u órdenes de planes, a la cola.">
                    <input type="checkbox" checked={siempreAlTurno} onChange={e => { setSiempreAlTurno(e.target.checked); ponLS(CLAVE_SIEMPRE_AL_TURNO, e.target.checked); }} /> Siempre al turno
                  </label>
                )}
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
                placeholder={!sesion.hosting ? 'Completa los propiedades de la sesión' : !prov?.disponible ? 'Elige un agente disponible' : enCurso ? (conTurno && siempreAlTurno ? 'Escribe; le llega ya al agente… (/help)' : 'Escribe; irá a la cola… («>» al turno, «?» estado, «//» aparte · /help)')
                  : 'Escribe… (/plan para crear un plan · /help para la ayuda)'}
                disabled={!prov?.disponible || !sesion.hosting} />
              {enCurso
                ? (texto.trim()
                  ? <button className="redondo primario" onClick={() => decideYEnvia()} disabled={subiendo} title={conTurno && siempreAlTurno ? 'Intro: al turno en curso (el agente lo lee ya)' : 'Intro: a la cola (se enviará cuando termine el agente) o, si no es para el agente, aparte'} aria-label={conTurno && siempreAlTurno ? 'Enviar al turno en curso' : 'Poner en cola'}><Icono n="enviar" t={16} /></button>
                  : <button className="redondo" onClick={para} title={cola.length ? 'Parar (la cola vuelve al cuadro de texto)' : 'Parar'} aria-label="Parar"><Icono n="parar" t={16} /></button>)
                : <button className="redondo primario" onClick={() => decideYEnvia()} disabled={(!texto.trim() && !adjuntos.some(a => !a.subiendo && !a.error)) || !prov?.disponible || subiendo} title="Enviar (Intro)" aria-label="Enviar"><Icono n="enviar" t={16} /></button>}
            </div>
            {/* Debajo del campo, como en Claude Code: el modo a la izquierda; el modelo y su esfuerzo a la derecha. */}
            <div className="pie-redactor">
              {prov?.modos && (
                <select className="pie-select" value={modoDe(sesion)} aria-label="Modo"
                  onChange={e => alCambiarSesion({ ...sesion, permisos: e.target.value as Sesion['permisos'], modoPrevio: undefined })}>
                  {Object.entries(MODOS).map(([v, t]) => <option key={v} value={v}>{t}</option>)}
                </select>
              )}
              {/* Plan: una herramienta, no un modo. Pide como plan lo escrito («/plan …»); sin texto, lo prepara. */}
              {prov?.tipo === 'agente' && (
                <button className="pie-boton" disabled={!sesion.hosting}
                  title="Pedirlo como plan (o escribe «/plan …»). Después: «plan N: cambios», «ejecuta el plan N», «descarta / reactiva / borra el plan N»."
                  onClick={() => {
                    const t = texto.trim();
                    if (!t || /^\/plan\b/i.test(t)) { if (!t) setTexto('/plan '); area.current?.focus(); return; }
                    const listos = adjuntos.filter(a => !a.subiendo && !a.error);
                    if (enCurso) ponCola(sesion.id, c => C.encola(c, '/plan ' + t, listos));
                    else envia('/plan ' + t, listos);
                    setTexto(''); setAdjuntos([]);
                  }}>
                  <Icono n="plan" t={13} /> Plan
                </button>
              )}
              <span className="hueco" />
              {vigOn && prov?.tipo === 'agente' && contextPolicy(sesion.agente, sesion).vig !== 'none' && (
                <CleanerIndicator agent={sesion.agente} model={sesion.modelo || prov.por_defecto} report={limpieza[sesion.id]?.r}
                  busy={limpieza[sesion.id]?.busy} messages={mensajes} onSweep={() => barreAhora()} sweeping={barriendo[sesion.id]}
                  sweepResult={barridoTxt[sesion.id]} relayForced={relevoPedido[sesion.id]} />
              )}
              {gastoSesion.turnos > 0 && (
                <button className={'gasto gasto-' + nivelGasto} onClick={alAjustes}
                  title={[...alertas.map(x => '⚠ ' + x.texto), `Gasto de esta sesión: ${CO.dinero(gastoSesion.coste)} · ${CO.tokens(gastoSesion.entrada)} de entrada y ${CO.tokens(gastoSesion.salida)} de salida en ${gastoSesion.turnos} ${gastoSesion.turnos === 1 ? 'turno' : 'turnos'}.`,
                    CO.PLANES[sesion.agente] || '', `Aviso: ${avisos.avisoCoste ? CO.dinero(avisos.avisoCoste) : '—'} · límite: ${avisos.limiteCoste ? CO.dinero(avisos.limiteCoste) : '—'}${avisos.avisoTokens ? ' · aviso ' + CO.tokens(avisos.avisoTokens) + ' tokens' : ''} (propiedades de la sesión, «Notificaciones»)`].filter(Boolean).join('\n')}>
                  {nivelGasto !== 'ok' && <span aria-hidden>⚠ </span>}{CO.dinero(gastoSesion.coste)} · {CO.tokens(gastoSesion.entrada + gastoSesion.salida)}
                </button>
              )}
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
      {proyecto && planAbierto && <EditorPlan proyecto={proyecto.id} directorio={directorio} ruta={planAbierto} ocupado={Boolean(enCurso)}
        alCerrar={() => setPlanAbierto(null)} alCambio={p => marcaPlan(p.ruta, p.estado, p.titulo)} alEjecutar={ejecutaPlan} />}
      {proyecto && verPlanes && <ListaPlanes proyecto={proyecto.id} directorio={directorio}
        alAbrir={r => { setVerPlanes(false); setPlanAbierto(r); }} alCerrar={() => setVerPlanes(false)} />}
    </main>
  );
}

const miles = (n: number) => n >= 1000 ? (n / 1000).toFixed(n >= 10000 ? 0 : 1) + 'k' : String(n);
const usoTexto = (u: { entrada: number; salida: number; coste?: number; segundos?: number }) =>
  [`${miles(u.entrada)} ↑ · ${miles(u.salida)} ↓ tokens`, u.coste ? `$${u.coste.toFixed(3)}` : '', u.segundos ? `${u.segundos.toFixed(1)} s` : ''].filter(Boolean).join(' · ');

// Una parte de la respuesta: texto, razonamiento (plegado) o una herramienta (una línea que se despliega), como en Claude Code.
// Las partes según el nivel de detalle del proyecto: conciso (solo texto y una línea con las acciones), normal
// (cada acción plegada) o detallado (todo desplegado).
// memo (también las de abajo): las partes de los mensajes anteriores no cambian mientras llega la respuesta.
// Lo que costó cada acción del turno (por id de herramienta), para la fila de cada una.
const ConsumoAccion = createContext<Map<string, number>>(new Map());

const Partes = memo(function Partes({ partes, tareas, detalle, inicio = 0, vivo = false, error }: { partes: Parte[]; tareas?: T.Tarea[]; detalle: Detalle; inicio?: number; vivo?: boolean; error?: boolean }) {
  const consumo = useMemo(() => consumoPorAccion(partes, inicio), [partes, inicio]);
  return <ConsumoAccion.Provider value={consumo}><PartesDentro partes={partes} tareas={tareas} detalle={detalle} vivo={vivo} error={error} /></ConsumoAccion.Provider>;
});

const PartesDentro = memo(function PartesDentro({ partes, tareas, detalle, vivo, error }: { partes: Parte[]; tareas?: T.Tarea[]; detalle: Detalle; vivo: boolean; error?: boolean }) {
  // Las herramientas de un subagente van dentro de su tarea; la herramienta que crea una tarea se pinta como tarea.
  const dentro = (id: string) => partes.filter(p => p.tipo === 'herramienta' && p.h.padre === id);
  const sueltas = partes.filter(p => p.tipo !== 'herramienta' || !p.h.padre);
  const pinta = (p: Parte, j: number | string, abierta: boolean) => {
    const t = p.tipo === 'herramienta' ? T.deHerramienta(tareas, p.h.id) : undefined;
    return t ? <BloqueTarea key={j} t={t} h={p.tipo === 'herramienta' ? p.h : undefined} internas={dentro(t.herramienta!)} abierta={abierta} />
      : <ParteMensaje key={j} p={p} abierta={abierta} />;
  };
  // La lista de tareas del agente principal, sincronizada con lo que hace en el turno (todoSync.ts).
  const todo = useMemo(() => TS.turnTodo({ rol: 'assistant', texto: '', partes, tareas, error }, vivo), [partes, tareas, error, vivo]);
  const lista = todo && <ListaTareas v={todo} vivo={vivo} />;
  // Tareas sin herramienta en la conversación (p. ej. lanzadas por un subagente): al final.
  const huerfanas = (tareas || []).filter(t => t.tipo !== 'lista' && !partes.some(p => p.tipo === 'herramienta' && p.h.id === t.herramienta));
  const extra = <>
    {lista}
    {huerfanas.map(t => <BloqueTarea key={t.id} t={t} internas={t.herramienta ? dentro(t.herramienta) : []} abierta={false} />)}
  </>;
  if (detalle !== 'conciso') return <>{lista}{sueltas.map((p, j) => pinta(p, j, detalle === 'detallado'))}{huerfanas.map(t => <BloqueTarea key={t.id} t={t} internas={t.herramienta ? dentro(t.herramienta) : []} abierta={false} />)}</>;
  const acciones = sueltas.filter(p => p.tipo === 'herramienta' && !p.h.skill && !T.deHerramienta(tareas, p.h.id));
  const conTarea = sueltas.filter(p => p.tipo === 'herramienta' && T.deHerramienta(tareas, p.h.id));
  const skills = sueltas.filter(p => p.tipo === 'herramienta' && p.h.skill);
  const enCurso = acciones.some(p => p.tipo === 'herramienta' && p.h.estado === 'curso');
  return <>
    {skills.map((p, j) => <ParteMensaje key={'s' + j} p={p} abierta={false} />)}
    {acciones.length > 0 && (
      <details className="acciones">
        <summary><span className={'punto-h' + (enCurso ? ' curso' : '')} /> {acciones.length} {acciones.length === 1 ? 'acción' : 'acciones'}
          <Miniaturas rutas={acciones.flatMap(p => (p.tipo === 'herramienta' && p.h.capturas) || [])} /></summary>
        {acciones.map((p, j) => <ParteMensaje key={j} p={p} abierta={false} />)}
      </details>
    )}
    {conTarea.map((p, j) => pinta(p, 't' + j, false))}
    {extra}
    {sueltas.filter(p => p.tipo === 'texto').map((p, j) => <ParteMensaje key={j} p={p} abierta={false} />)}
  </>;
});

// Un subagente o una orden en segundo plano: estado, título, progreso y, dentro, sus herramientas y su resultado.
const BloqueTarea = memo(function BloqueTarea({ t, h, internas, abierta }: { t: T.Tarea; h?: Herramienta; internas: Parte[]; abierta: boolean }) {
  const estado = t.estado || h?.estado || 'curso';
  return (
    <details className={'tarea-agente ' + estado} open={abierta}>
      <summary>
        <span className="punto-h" aria-label={estado} />
        <span className="tarea-tipo">{T.NOMBRES[t.tipo || ''] || 'Tarea'}{t.subtipo ? ' · ' + t.subtipo : ''}{t.segundo_plano && t.tipo === 'subagente' ? ' · en paralelo' : ''}</span>
        <span className="recorta">{t.titulo || h?.resumen || h?.nombre}</span>
        {estado === 'curso' && t.progreso && <span className="tarea-progreso recorta suave pequeno">{t.progreso}</span>}
        {estado === 'parada' && <span className="suave pequeno">parada</span>}
        <Miniaturas rutas={[...(h?.capturas || []), ...internas.flatMap(p => (p.tipo === 'herramienta' && p.h.capturas) || [])]} />
      </summary>
      {internas.length > 0 && <div className="tarea-internas">{internas.map((p, j) => <ParteMensaje key={j} p={p} abierta={false} />)}</div>}
      {(t.salida || h?.salida) && <pre className="salida-h">{t.salida || h?.salida}</pre>}
      {(t.detalle || t.sesion_hija) && <p className="suave pequeno tarea-detalle" title={t.detalle}>{t.detalle ? 'Detalle: ' + t.detalle : 'Sesión: ' + t.sesion_hija}</p>}
    </details>
  );
});

// La lista de tareas del agente en el turno: lo que hace ahora, lo que falta y, plegado, lo hecho (TodoSyncList).
const ListaTareas = memo(function ListaTareas({ v, vivo }: { v: TS.SyncView; vivo: boolean }) {
  return (
    <details className="lista-tareas-agente" open>
      <summary title={v.inferred ? 'Con ▷ y ≈, lo que pgocode deduce de lo que hace el agente (no lo ha marcado él)' : undefined}>Lista de tareas · {todoCount(v)}</summary>
      <TodoSyncList v={v} trabajando={vivo} />
    </details>
  );
});

// VigFade: la opacidad del mensaje (por su vigencia) para que cada herramienta se desvanezca según la suya; 0: sin
// desvanecido (gestión automática del contexto desactivada o fuera del contexto por «Continuar desde aquí»).
const VigFade = createContext(0);

// El consumo de una acción y lo acumulado de la conversación hasta ella («+3 k · 120 k»), a la vista sin ratón.
function ConsumoDeAccion({ id, total }: { id: string; total: number }) {
  const d = useContext(ConsumoAccion).get(id);
  return <span className="suave pequeno" style={{ marginLeft: 'auto', whiteSpace: 'nowrap' }}
    title={(d ? `Esta acción: ${CO.tokens(d)} tokens (la llamada al modelo que la lanzó). ` : 'Lanzada en la misma llamada que la anterior. ') + `Conversación hasta aquí: ${CO.tokens(total)} tokens.`}>
    {d ? <><b style={{ fontWeight: 600 }}>+{CO.tokens(d)}</b> · </> : null}{CO.tokens(total)}</span>;
}

const ParteMensaje = memo(function ParteMensaje({ p, abierta }: { p: Parte; abierta: boolean }) {
  const padre = useContext(VigFade);
  if (p.tipo === 'texto') return <Texto md={p.texto} />;
  if (p.tipo === 'pensamiento') return <details className="pensamiento" open={abierta}><summary>Razonamiento</summary><div>{p.texto}</div></details>;
  const h = p.h;
  if (h.skill) return (
    <details className={'herramienta skill ' + h.estado} open={abierta}>
      <summary><span className="sk-marca">✦ Skill · {h.skill}</span>{h.estado === 'curso' && <span className="suave pequeno">cargando…</span>}</summary>
      {h.salida && <pre className="salida-h">{h.salida}</pre>}
    </details>
  );
  const vig = padre > 0 && h.vig ? h.vig : undefined;
  const op = vig ? PR.relativeOpacity(PR.vigOpacity(vig), padre) : 1;
  return (
    <details className={'herramienta ' + h.estado} open={abierta} style={op < 1 ? { opacity: op } : undefined}>
      <summary title={vig ? PR.vigTitle(vig) : undefined}>
        <span className="punto-h" aria-label={toolStatusTitle(h)} title={toolStatusTitle(h)} />
        {vig?.fuera && <span className="vig-out" aria-label="Fuera del contexto del agente">⤓</span>}
        <b>{h.nombre}</b>{h.resumen && <span className="resumen-h recorta">{h.resumen}</span>}
        <Miniaturas rutas={h.capturas} />
        {h.tokens ? <ConsumoDeAccion id={h.id} total={h.tokens} /> : null}
      </summary>
      {h.entrada && <pre className="entrada-h">{h.entrada}</pre>}
      {h.salida && <pre className="salida-h">{h.salida}</pre>}
    </details>
  );
});

// Inserts a line break at the cursor (Ctrl+Intro). execCommand keeps the editor's undo history and fires the
// input event React listens to; if it is not available, the value is set by hand.
function insertNewline(area: HTMLTextAreaElement) {
  if (document.execCommand?.('insertText', false, '\n')) return;
  const { selectionStart: start, selectionEnd: end, value } = area;
  const set = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')?.set;
  set?.call(area, value.slice(0, start) + '\n' + value.slice(end));
  area.selectionStart = area.selectionEnd = start + 1;
  area.dispatchEvent(new Event('input', { bubbles: true }));
}
