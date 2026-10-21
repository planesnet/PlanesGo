import { memo, useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { chat, post, subeAdjunto, type Adjunto, type Eleccion, type Herramienta, type Mensaje, type Parte, type Proveedor, type Proyecto, type SkillPgo, type Uso } from '../api';
import type { Detalle, SeleccionSkills } from '../ajustes';
import { conversacion, guardaConversacion, nombreDe, titulo, type Repo, type Sesion } from '../sesiones';
import * as C from '../cola';
import * as PL from '../plan';
import * as T from '../tareas';
import * as CO from '../costes';
import * as AP from '../aparte';
import { direccionesEn } from '../lanzador';
import { borraPlan, EditorPlan, guardaPlan, leePlan, ListaPlanes, listaPlanes, TarjetaPlan, type EstadoPlan, type Plan } from './Planes';
import { Miniaturas, ProyectoCapturas } from './Capturas';
import Icono from './Icono';
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

const MODOS = { lectura: 'Solo lectura', editar: 'Editar ficheros', auto: 'Auto: ejecuta comandos' } as const;
// El modo de la sesión («plan» ya no es un modo: los de antes cuentan como el que tenían antes, o solo lectura).
const modoDe = (s: Sesion) => (s.permisos === 'plan' ? s.modoPrevio || 'lectura' : s.permisos || 'lectura');
const ESFUERZOS: Record<string, string> = { low: 'bajo', medium: 'medio', high: 'alto', xhigh: 'muy alto', max: 'máximo' };
const ALTO_MAX = 240;

// Una pregunta aparte: no entra en la conversación (ni en lo que se guarda); desaparece al recargar.
interface Aparte {
  id: string; pregunta: string; destino: Exclude<AP.Destino, 'conversacion'>; texto: string; acciones: number; uso?: Uso;
  curso: boolean; error?: boolean; motivo: string; auto: boolean; incorporada?: boolean; ac?: AbortController;
}
const SITIOS_APARTE: Exclude<AP.Destino, 'conversacion'>[] = ['estado', 'suelta', 'contexto'];
// Lo aprendido de las correcciones, por proyecto; y el gasto aparte, por sesión (se suma al de la conversación).
const leeLS = <T,>(k: string, d: T): T => { try { return JSON.parse(localStorage.getItem(k) || 'null') ?? d; } catch { return d; } };
const ponLS = (k: string, v: unknown) => { try { localStorage.setItem(k, JSON.stringify(v)); } catch { /* sin almacenamiento */ } };
const claveAprendidas = (proyecto: number) => 'pgo:aparte:' + proyecto;
const claveGastoAparte = (sesion: string) => 'pgo:aparte-gasto:' + sesion;

export default function Chat({ directorio, versionProcesos, alAbrirWeb, alAjustes, seleccionSkills, hosting, detalle, envioPendiente, alEnviado, proyecto, sesion, repo, proveedores, panelAbierto, alPanel, alCambiarSesion, alNuevaSesion }: Props) {
  const [texto, setTexto] = useState('');
  const [mensajes, setMensajes] = useState<Mensaje[]>([]);
  // Turnos en curso por sesión: cada sesión lleva el suyo y sigue aunque se cambie de sesión o de proyecto (al
  // volver se ve su progreso). «enCurso» es el de la sesión abierta.
  const [enCursos, setEnCursos] = useState<Record<string, AbortController>>({});
  const enCurso: AbortController | null = (sesion && enCursos[sesion.id]) || null;
  const enCursosRef = useRef(enCursos);
  enCursosRef.current = enCursos;
  const inicioTurno = useRef<Record<string, number>>({});
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
  // Actividad (pestaña del panel): el chat avisa de las tareas de su sesión y de si el agente está trabajando.
  useEffect(() => {
    if (!sesion) return;
    const tareas = mensajes.flatMap(m => m.tareas || []);
    window.dispatchEvent(new CustomEvent('pgo:actividad', { detail: { sesion: sesion.id, tareas, trabajando: Boolean(enCurso) } }));
  }, [mensajes, enCurso, sesion?.id]); // eslint-disable-line react-hooks/exhaustive-deps
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

  // Al cambiar de sesión: su conversación (la del turno en curso, si lo tiene). El turno de la otra sesión no se
  // corta: sigue y guarda su respuesta al acabar.
  useEffect(() => {
    idActual.current = sesion?.id;
    setMensajes(sesion ? vivas.current[sesion.id] || conversacion(sesion.id) : []);
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

  // Lo último que ha dicho el agente (para decidir si lo que se escribe es su respuesta).
  const ultimoDelAgente = (ms: Mensaje[]) => {
    const m = [...ms].reverse().find(x => x.rol === 'assistant');
    const ts = (m?.partes || []).filter(p => p.tipo === 'texto');
    const u = ts[ts.length - 1];
    return (u && u.tipo === 'texto' ? u.texto : m?.texto || '').slice(-600);
  };

  // La decisión de adónde irá lo que se escribe (se ve en la etiqueta encima del cuadro, antes de enviar).
  const situacion = (t: string): AP.Situacion => ({ ocupado: Boolean(enCurso), ultimo: ultimoDelAgente(mensajes), hayConversacion: mensajes.length > 0,
    adjuntos: adjuntos.filter(a => !a.subiendo && !a.error).length, aprendidas: proyecto ? leeLS(claveAprendidas(proyecto.id), {}) : {},
    activo: sesion?.decideAparte !== false, ordenPlan: PL.interpreta(t) !== null });
  const decisionViva = useMemo(() => (texto.trim() && prov?.tipo === 'agente' ? AP.decide(texto, situacion(texto)) : null),
    [texto, enCurso, mensajes, adjuntos, sesion?.decideAparte, prov?.tipo]); // eslint-disable-line react-hooks/exhaustive-deps
  // Mensajes al turno en curso (agentes que los admiten, como Claude Code): los de cada sesión durante su turno,
  // que al acabar se guardan con la respuesta.
  const conTurno = Boolean(enCurso && prov?.inyecta);
  const [inyectados, setInyectados] = useState<Record<string, string[]>>({});
  const inyectadosRef = useRef(inyectados);
  inyectadosRef.current = inyectados;
  async function alTurno(id: string, t: string, adj: Adjunto[] = []) {
    // Con adjuntos, o si el turno ya acabó o no lo admite: a la cola, como siempre.
    if (adj.length || !enCursosRef.current[id]) { aConversacion(id, t, adj); return; }
    try {
      await post('/api/chat/inyecta', { sesion: id, texto: t });
      inyectadosRef.current = { ...inyectadosRef.current, [id]: [...(inyectadosRef.current[id] || []), t] };
      setInyectados(inyectadosRef.current);
    } catch {
      aConversacion(id, t, adj);
    }
  }
  // Destino elegido a mano en la etiqueta (o con Ctrl+Intro): manda sobre la decisión automática.
  const [destinoManual, setDestinoManual] = useState<AP.Destino | null>(null);
  useEffect(() => { if (!texto.trim()) setDestinoManual(null); }, [texto]);
  useEffect(() => setDestinoManual(null), [sesion?.id]);

  // Intro: pgocode decide antes de enviar si va a la conversación (ahora o a la cola) o aparte (ver aparte.ts).
  // forzar: el destino que manda (Ctrl+Intro → conversación); si no, el elegido en la etiqueta o el automático.
  async function decideYEnvia(forzar?: AP.Destino) {
    const t = texto.trim();
    if (!t || !sesion || !proyecto || subiendo) return;
    const ses = sesion;
    const listos = adjuntos.filter(a => !a.subiendo && !a.error);
    const ultimo = ultimoDelAgente(mensajes);
    const auto = AP.decide(t, situacion(t));
    const elegido = forzar || destinoManual;
    // Lo que el usuario elige en contra de lo que habría decidido pgocode se aprende (para los parecidos).
    if (elegido && !auto.forzado && !auto.duda && elegido !== auto.destino) {
      ponLS(claveAprendidas(proyecto.id), AP.aprende(leeLS(claveAprendidas(proyecto.id), {}), t, elegido));
    }
    const d: AP.Decision = elegido ? { destino: elegido, texto: auto.forzado ? auto.texto : t, forzado: true, motivo: forzar ? 'Ctrl+Intro' : 'elegido por ti' } : auto;
    setDestinoManual(null);
    if (d.destino === 'turno') {
      setTexto(''); setAdjuntos([]);
      await alTurno(ses.id, d.texto, listos);
      return;
    }
    if (d.destino === 'conversacion' && !d.duda) {
      if (!d.forzado) { if (enCurso) encola(); else envia(); return; }
      setTexto(''); setAdjuntos([]);
      aConversacion(ses.id, d.texto, listos);
      return;
    }
    setTexto(''); setAdjuntos([]);
    let destino: AP.Destino = d.destino, motivo = d.motivo;
    if (d.duda) {
      // Solo con el agente ocupado: un modelo barato con el mensaje y lo último del agente (unos 200 tokens).
      const r = await Promise.race([post<{ tipo: AP.Destino }>('/api/chat/clasifica', { texto: t, ultimo, sesion: ses.id }).catch(() => null),
        new Promise<null>(res => setTimeout(() => res(null), 5000))]);
      destino = r?.tipo || 'conversacion';
      motivo = r ? 'lo ha decidido el clasificador (modelo barato)' : 'el clasificador no ha respondido';
    }
    if (destino === 'conversacion') { aConversacion(ses.id, t, listos); return; }
    pregunta(ses, d.texto || t, destino, motivo, !d.forzado);
  }

  // A la conversación: a la cola si el agente está ocupado (la cola sale sola al acabar), si no, se envía.
  function aConversacion(id: string, t: string, adj: Adjunto[] = []) {
    if (enCursosRef.current[id] || idActual.current !== id) ponCola(id, c => C.encola(c, t, adj));
    else envia(t, adj, { aCola: true });
  }

  // Responde una pregunta aparte: el estado, en local; el resto, con /api/chat en modo aparte.
  async function pregunta(ses: Sesion, q: string, destino: Exclude<AP.Destino, 'conversacion'>, motivo: string, auto: boolean, sustituye?: string) {
    const idA = Math.random().toString(36).slice(2);
    const pon = (f: (l: Aparte[]) => Aparte[]) => setApartes(x => ({ ...x, [ses.id]: f(x[ses.id] || []) }));
    const cambia = (c: Partial<Aparte>) => pon(l => l.map(a => a.id === idA ? { ...a, ...c } : a));
    const base: Aparte = { id: idA, pregunta: q || '¿Cómo va?', destino, texto: '', acciones: 0, curso: destino !== 'estado', motivo, auto };
    const ms = vivas.current[ses.id] || (idActual.current === ses.id ? mensajes : conversacion(ses.id));
    if (destino === 'estado') {
      const trabajando = Boolean(enCursosRef.current[ses.id]);
      const ult = [...ms].reverse().find(m => m.rol === 'assistant');
      base.texto = AP.resumenEstado(ult, { trabajando, segundos: (Date.now() - (inicioTurno.current[ses.id] || Date.now())) / 1000, enCola: (colas[ses.id]?.mensajes.length) || 0 });
    }
    pon(l => sustituye ? l.map(a => a.id === sustituye ? base : a) : [...l, base]);
    if (destino === 'estado' || !proyecto) return;
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
        aparte: destino, contexto: sinCopia ? AP.resumenContexto(ms) : '', idioma: navigator.language,
      }, e => {
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

  // opc.crudo: el texto va tal cual (sin interpretar órdenes de planes); opc.permisos: los de este turno (no se
  // guardan en la sesión), p. ej. para ejecutar un plan en una sesión de solo lectura.
  async function envia(forzado?: string, adjCola?: Adjunto[], opc: { crudo?: boolean; permisos?: string; visible?: string; aCola?: boolean } = {}) {
    const t = (forzado ?? texto).trim();
    if (!t || !proyecto || !sesion || !sesion.hosting || !prov || enCurso || (forzado === undefined && subiendo)) return;
    const ses: Sesion = sesion;
    // Órdenes de planes: las de estado se hacen aquí (sin agente ni tokens); crear y revisar van al agente.
    const orden = opc.crudo ? null : PL.interpreta(t);
    if (orden && orden.tipo !== 'nuevo' && orden.tipo !== 'revisar') {
      if (forzado === undefined) { setTexto(''); setAdjuntos([]); }
      await ordenPlan(orden, t);
      return;
    }
    let planRevisado: Plan | undefined;
    if (orden?.tipo === 'revisar') {
      planRevisado = await planNumero(orden.numero);
      if (!planRevisado || planRevisado.estado === 'descartado') {
        if (forzado === undefined) { setTexto(''); setAdjuntos([]); }
        nota(t, !planRevisado ? `No hay ningún plan ${orden.numero} en este proyecto.` : `El plan ${orden.numero} está descartado: «reactiva el plan ${orden.numero}» para seguir con él.`);
        return;
      }
    }
    // Gasto antes del turno, para anotar los avisos que salte (sin interrumpir).
    const gAntes = CO.gasto(forzado !== undefined ? conversacion(sesion.id) : mensajes);
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
    const previos = forzado !== undefined ? conversacion(id) : mensajes;
    const listos = adjCola ?? (forzado === undefined ? adjuntos.filter(a => !a.subiendo && !a.error) : []);
    const base: Mensaje[] = [...previos.filter(m => !m.error), { rol: 'user', texto: opc.visible ?? t, ...(listos.length ? { adjuntos: listos } : {}) }];
    let actual: Mensaje[] = [...base, { rol: 'assistant', texto: '' }];
    pon(actual, true);
    if (forzado === undefined) { setTexto(''); setAdjuntos([]); }
    const ac = new AbortController();
    setEnCursos(x => ({ ...x, [id]: ac }));
    inicioTurno.current[id] = Date.now();
    if (inyectadosRef.current[id]) { const { [id]: _, ...resto } = inyectadosRef.current; inyectadosRef.current = resto; setInyectados(resto); }
    let sesionAgente = ses.sesionAgente;
    // Skill de la petición por embeddings: «elegir» espera (como mucho 1,5 s) y se la indica al agente;
    // «sugerir» va en paralelo y solo se muestra.
    let aEnviar = t;
    const marcaSkill = (k: SkillPgo) => {
      actual = actual.map((m, i) => i === actual.length - 2 ? { ...m, skill: k } : m);
      pon(actual, true);
    };
    // Crear o revisar un plan: sin skills; el agente recibe la petición de plan (con el plan actual, ya editado).
    if (orden) {
      const actualPlan = planRevisado ? (await leePlan(proyecto.id, directorio, planRevisado.ruta).catch(() => planRevisado)).texto : undefined;
      aEnviar = PL.peticionPlan(orden.peticion, actualPlan);
    } else if (seleccionSkills !== 'agente' && prov.tipo === 'agente' && !opc.crudo) {
      const pide = post<Eleccion>('/api/skills/elegir', { proyecto: proyecto.id, carpeta: ses.carpeta, agente: ses.agente, texto: t });
      if (seleccionSkills === 'elegir') {
        const r = await Promise.race([pide.catch(() => null), new Promise<null>(res => setTimeout(() => res(null), 1500))]);
        if (r?.elegida) {
          const n = r.elegida.nombre;
          aEnviar = ses.agente === 'claude-code' ? `/${n} ${t}` : `Usa la skill «${n}».\n\n${t}`;
          marcaSkill({ nombre: n, puntuacion: r.elegida.puntuacion, modo: 'elegida' });
        }
      } else {
        pide.then(r => { if (r.elegida && idActual.current === id) marcaSkill({ nombre: r.elegida.nombre, puntuacion: r.elegida.puntuacion, modo: 'sugerida' }); }).catch(() => {});
      }
    }
    // Preguntas aparte que el usuario ha incorporado: van al final de este mensaje (el agente no gasta otro turno).
    if (!orden && prov.tipo === 'agente' && incorporar.current[id]?.length) {
      aEnviar += AP.incorporacion(incorporar.current[id]);
      incorporar.current[id] = [];
      setApartes(x => ({ ...x, [id]: (x[id] || []).filter(a => !a.incorporada) }));
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
        proyecto: proyecto.id, proveedor: prov.id, modelo: ses.modelo || prov.por_defecto, carpeta: ses.carpeta, rama: ses.rama,
        permisos: modoPlan ? 'lectura' : opc.permisos || modoDe(ses), sesion_agente: ses.sesionAgente, esfuerzo: ses.esfuerzo || '',
        sesion: ses.id, titulo: ses.nombre, adjuntos: listos.map(a => a.ruta), autorizadas: ses.autorizadas || [], mcp: ses.mcp || [], tarea_id: ses.tareaId || 0, tarea: ses.tarea || '',
        idioma: navigator.language,
        // Los agentes llevan su propio historial: solo el último mensaje (con la skill indicada, si pgocode la eligió).
        mensajes: prov.tipo === 'agente' ? [{ rol: 'user', texto: aEnviar }] : base,
      }, e => {
        if (e.texto) parte({ tipo: 'texto', texto: e.texto });
        if (e.pensamiento) parte({ tipo: 'pensamiento', texto: e.pensamiento });
        if (e.herramienta) parte({ tipo: 'herramienta', h: e.herramienta });
        if (e.tarea) ultimo(m => ({ ...m, tareas: T.fusiona(m.tareas, e.tarea!) }));
        // Puede haber varios consumos en un turno (Claude con tareas en segundo plano hace más de un «result»).
        if (e.uso) ultimo(m => ({ ...m, uso: m.uso ? { entrada: m.uso.entrada + e.uso!.entrada, salida: m.uso.salida + e.uso!.salida,
          coste: (m.uso.coste || 0) + (e.uso!.coste || 0) || undefined, segundos: (m.uso.segundos || 0) + (e.uso!.segundos || 0) || undefined } : e.uso }));
        if (e.sesion_agente) sesionAgente = e.sesion_agente;
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
      // Lo que quedó «en curso» ya no lo está: el agente ha terminado (o se ha parado).
      if (actual[actual.length - 1]?.tareas) ultimo(m => ({ ...m, tareas: T.alAcabar(m.tareas, ac.signal.aborted) }));
      pon(actual, true);
      if (fallo) ponCola(id, C.falla);
      avisa(CO.nuevos(gAntes, CO.gasto(actual), aSes));
      delete vivas.current[id];
      setEnCursos(x => { if (x[id] !== ac) return x; const n = { ...x }; delete n[id]; return n; });
      alCambiarSesion({ ...ses, nombre: ses.nombre || nombreDe(t), sesionAgente, ultima: Date.now() });
    }
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
  function nota(orden: string, respuesta: string) {
    if (!sesion) return;
    const m: Mensaje[] = [...conversacion(sesion.id), { rol: 'user', texto: orden }, { rol: 'assistant', texto: respuesta, nota: true }];
    if (idActual.current === sesion.id) setMensajes(m);
    guardaConversacion(sesion.id, m);
  }

  // El plan con ese número en el proyecto.
  async function planNumero(n: number): Promise<Plan | undefined> {
    if (!proyecto) return undefined;
    return (await listaPlanes(proyecto.id, directorio).catch(() => [] as Plan[])).find(p => p.numero === n);
  }

  // Órdenes de estado de un plan, en local: descartar, reactivar, borrar; y ejecutar (que sí va al agente).
  async function ordenPlan(o: PL.OrdenPlan, t: string) {
    if (!proyecto || o.tipo === 'nuevo' || o.tipo === 'revisar') return;
    const p = await planNumero(o.numero);
    if (!p) { nota(t, `No hay ningún plan ${o.numero} en este proyecto.`); return; }
    const nombre = `Plan #${p.numero} «${p.titulo}»`;
    try {
      switch (o.tipo) {
        case 'descartar':
          if (p.estado === 'descartado') { nota(t, `${nombre} ya estaba descartado.`); return; }
          await guardaPlan(proyecto.id, directorio, { ruta: p.ruta, estado: 'descartado' });
          marcaPlan(p.ruta, 'descartado'); nota(t, `${nombre} descartado. Puedes reactivarlo («reactiva el plan ${p.numero}») o borrarlo («borra el plan ${p.numero}»).`);
          return;
        case 'reactivar':
          if (p.estado === 'borrador') { nota(t, `${nombre} ya está por decidir.`); return; }
          await guardaPlan(proyecto.id, directorio, { ruta: p.ruta, estado: 'borrador' });
          marcaPlan(p.ruta, 'borrador'); nota(t, `${nombre} vuelve a estar por decidir.`);
          return;
        case 'borrar':
          if (p.estado !== 'descartado') { nota(t, `Solo se borran los planes descartados: «descarta el plan ${p.numero}» antes.`); return; }
          await borraPlan(proyecto.id, directorio, p.ruta);
          marcaPlan(p.ruta, 'borrado'); nota(t, `${nombre} borrado.`);
          return;
        case 'ejecutar':
          if (p.estado === 'descartado') { nota(t, `${nombre} está descartado: «reactiva el plan ${p.numero}» para ejecutarlo.`); return; }
          await ejecutaPlan(p, t);
          return;
      }
    } catch (e) { nota(t, '⚠ ' + (e as Error).message); }
  }

  // Ejecutar: el plan pasa a «ejecutado» y el agente lo recibe con permiso para editar (si la sesión es de solo
  // lectura, solo en este turno).
  async function ejecutaPlan(p: Plan, orden?: string) {
    if (!sesion || !proyecto || enCurso) return;
    const texto = p.texto ?? (await leePlan(proyecto.id, directorio, p.ruta)).texto ?? '';
    await guardaPlan(proyecto.id, directorio, { ruta: p.ruta, estado: 'ejecutado' });
    marcaPlan(p.ruta, 'ejecutado', p.titulo);
    setPlanAbierto(null);
    const modo = modoDe(sesion);
    envia(PL.peticionEjecucion(p.ruta, texto), [], { crudo: true, permisos: modo === 'lectura' ? 'editar' : modo, visible: orden || `Ejecuta el plan #${p.numero ?? ''} «${p.titulo}»` });
  }

  async function descartaPlan(ruta: string) {
    if (!sesion || !proyecto) return;
    await guardaPlan(proyecto.id, directorio, { ruta, estado: 'descartado' }).catch(() => {});
    marcaPlan(ruta, 'descartado');
  }

  const teclas = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    // Intro: decide pgocode (o lo elegido en la etiqueta); Ctrl+Intro: siempre a la conversación; Alt+Intro: al turno
    // en curso (si el agente lo admite; si no, a la cola).
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      decideYEnvia(e.altKey ? (conTurno ? 'turno' : 'conversacion') : e.ctrlKey || e.metaKey ? 'conversacion' : undefined);
    }
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
          <div className="mensajes" aria-live="polite" ref={lista} onScroll={alDesplazar} onCopy={copiaSeleccion}>
            {mensajes.length === 0 && (
              <div className="vacio pequeno">
                <p><b>{titulo(sesion)}</b></p>
                <p className="suave">{prov?.nombre || 'Elige un agente'}{prov?.tipo === 'agente' ? ` · ${{ lectura: 'solo lectura', editar: 'puede editar', auto: 'auto: ejecuta comandos' }[modoDe(sesion)]}` : ''}</p>
              </div>
            )}
            {mensajes.map((m, i) => (
              <div key={i} className={'mensaje ' + m.rol + (m.error ? ' con-error' : '') + (m.nota ? ' nota' : '')}>
                {m.texto && !(enCurso && i === mensajes.length - 1) && (
                  <button className="icono copiar-mensaje" onClick={() => copiaTexto(m.texto, i)} title={copiado === i ? 'Copiado' : 'Copiar el mensaje'} aria-label="Copiar el mensaje">
                    <Icono n={copiado === i ? 'hecho' : 'copiar'} t={13} />
                  </button>
                )}
                {m.partes?.length
                  ? <Partes partes={m.partes} tareas={m.tareas} detalle={detalle} />
                  : m.texto ? (m.rol === 'assistant' && !m.error ? <Texto md={m.texto} /> : m.texto)
                    : (enCurso && i === mensajes.length - 1 ? <span className="pensando">Pensando…</span> : '')}
                {m.partes?.length && m.error ? <p className="con-error">{m.texto.split('⚠').slice(1).map(x => '⚠' + x).join('')}</p> : null}
                {(m.inyectados || (enCurso && i === mensajes.length - 1 ? inyectados[sesion.id] : undefined))?.map((x, j) => (
                  <div key={'iny' + j} className="inyectado" title="Lo mandaste mientras el agente trabajaba: lo leyó durante el turno">↳ Tú, durante el turno: {x}</div>
                ))}
                {enCurso && i === mensajes.length - 1 && m.partes?.length ? <span className="pensando">Trabajando…</span> : null}
                {m.uso && detalle !== 'conciso' && <div className="uso">{usoTexto(m.uso)}</div>}
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
              </div>
            ))}
            {apartesSesion.map(a => (
              <div key={a.id} className="mensaje aparte" role="note" aria-label="Pregunta aparte">
                <div className="aparte-cabecera">
                  <span className="aparte-etiqueta">Aparte · {AP.NOMBRES[a.destino]}</span>
                  <span className="suave pequeno recorta" title={a.motivo}>no queda en la conversación{a.auto ? ' · lo ha decidido pgocode' : ''}</span>
                  <span className="hueco" />
                  <button className="icono" onClick={() => quitaAparte(sesion.id, a)} title={a.curso ? 'Parar y quitar' : 'Quitar'} aria-label="Quitar la pregunta aparte"><Icono n="cerrar" t={13} /></button>
                </div>
                <div className="aparte-pregunta">{a.pregunta}</div>
                {a.texto ? (a.error ? <p className="con-error">{a.texto}</p> : <Texto md={a.texto} />) : a.curso ? <span className="pensando">Pensando…</span> : null}
                {(a.uso || a.acciones > 0) && <div className="uso">{[a.acciones ? `${a.acciones} ${a.acciones === 1 ? 'acción' : 'acciones'} (solo lectura)` : '', a.uso ? usoTexto(a.uso) : ''].filter(Boolean).join(' · ')}</div>}
                <div className="aparte-acciones">
                  <button className="enlace" title="Enviarla al agente como un mensaje más (a la cola si está trabajando)"
                    onClick={() => { aprendeDe(a, 'conversacion'); quitaAparte(sesion.id, a); aConversacion(sesion.id, a.pregunta); }}>Llevar a la conversación</button>
                  {SITIOS_APARTE.filter(x => x !== a.destino && !(x === 'estado' && a.auto === false && a.pregunta === '¿Cómo va?')).map(x => (
                    <button key={x} className="enlace" title={x === 'estado' ? 'Responder con el estado del trabajo (sin tokens)' : x === 'suelta' ? 'Responder sin contexto, con un modelo barato' : 'Responder con el contexto de la conversación (copia de la sesión)'}
                      onClick={() => { aprendeDe(a, x); a.ac?.abort(); pregunta(sesion, a.pregunta, x, 'lo has cambiado tú', false, a.id); }}>{AP.NOMBRES[x]}</button>
                  ))}
                  {!a.curso && !a.error && a.texto && a.destino !== 'estado' && (a.incorporada
                    ? <span className="suave pequeno">Irá con tu próximo mensaje al agente</span>
                    : <button className="enlace" title="El agente la conocerá: se añade a tu próximo mensaje (sin gastar un turno solo para ella)"
                      onClick={() => {
                        (incorporar.current[sesion.id] ||= []).push({ pregunta: a.pregunta, respuesta: a.texto });
                        setApartes(x => ({ ...x, [sesion.id]: (x[sesion.id] || []).map(y => y.id === a.id ? { ...y, incorporada: true } : y) }));
                      }}>Incorporar</button>)}
                </div>
              </div>
            ))}
            <div ref={fin} />
          </div>
          </ProyectoCapturas.Provider>
          <div className="redactor">
            {!sesion.hosting && (
              <div className="aviso-sesion">
                <Icono n="info" t={15} /> Falta elegir el alojamiento de la sesión.
                <button className="enlace" onClick={alAjustes}>Completar ajustes</button>
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
            {/* Barra de prueba: lanzar lo que se está desarrollando (web o programa) en la rama de la sesión. */}
            <Lanzador proyecto={proyecto} carpeta={sesion.carpeta} version={versionProcesos} alAbrirWeb={alAbrirWeb} propuesta={propuesta} />
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
                        alTurno(sesion.id, c.texto);
                      }}>Al turno</button>
                    )}
                    {c.adjuntos.length === 0 && (
                      <button className="enlace aparte-boton" title="Preguntarlo aparte, sin que entre en la conversación" onClick={() => {
                        ponCola(sesion.id, k => C.quita(k, i));
                        if (proyecto) ponLS(claveAprendidas(proyecto.id), AP.aprende(leeLS(claveAprendidas(proyecto.id), {}), c.texto, 'contexto'));
                        pregunta(sesion, c.texto, 'contexto', 'lo has llevado aparte', false);
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
            {decisionViva && (
              <div className="destino-mensaje">
                <button className="enlace" onClick={() => setDestinoManual(AP.siguiente(destinoManual || decisionViva.destino, conTurno))}
                  title={`${destinoManual ? 'Elegido por ti' : 'Lo decide pgocode: ' + decisionViva.motivo}. Clic para cambiarlo. Ctrl+Intro: siempre a la conversación${conTurno ? '; Alt+Intro: al turno en curso' : ''}; prefijos «!» conversación, «//» aparte, «?» estado.`}>
                  {destinoManual ? AP.etiqueta({ destino: destinoManual === 'turno' && !conTurno ? 'conversacion' : destinoManual, texto: '', motivo: '' }, Boolean(enCurso)) : AP.etiqueta(decisionViva, Boolean(enCurso))}
                </button>
                {destinoManual && <span className="suave pequeno">elegido por ti</span>}
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
                placeholder={!sesion.hosting ? 'Completa los ajustes de la sesión' : !prov?.disponible ? 'Elige un agente disponible' : enCurso ? 'Escribe; irá a la cola… («?» estado, «//» aparte, «!» o Ctrl+Intro a la conversación)'
                  : 'Escribe… (/plan para crear un plan)'}
                disabled={!prov?.disponible || !sesion.hosting} />
              {enCurso
                ? (texto.trim()
                  ? <button className="redondo primario" onClick={() => decideYEnvia()} disabled={subiendo} title="Intro: a la cola (se enviará cuando termine el agente) o, si no es para el agente, aparte" aria-label="Poner en cola"><Icono n="enviar" t={16} /></button>
                  : <button className="redondo" onClick={para} title={cola.length ? 'Parar (la cola vuelve al cuadro de texto)' : 'Parar'} aria-label="Parar"><Icono n="parar" t={16} /></button>)
                : <button className="redondo primario" onClick={() => decideYEnvia()} disabled={!texto.trim() || !prov?.disponible || subiendo} title="Enviar (Intro)" aria-label="Enviar"><Icono n="enviar" t={16} /></button>}
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
              {gastoSesion.turnos > 0 && (
                <button className={'gasto gasto-' + nivelGasto} onClick={alAjustes}
                  title={[...alertas.map(x => '⚠ ' + x.texto), `Gasto de esta sesión: ${CO.dinero(gastoSesion.coste)} · ${CO.tokens(gastoSesion.entrada)} de entrada y ${CO.tokens(gastoSesion.salida)} de salida en ${gastoSesion.turnos} turnos.`,
                    CO.PLANES[sesion.agente] || '', `Aviso: ${avisos.avisoCoste ? CO.dinero(avisos.avisoCoste) : '—'} · límite: ${avisos.limiteCoste ? CO.dinero(avisos.limiteCoste) : '—'}${avisos.avisoTokens ? ' · aviso ' + CO.tokens(avisos.avisoTokens) + ' tokens' : ''} (ajustes de la sesión, «Notificaciones»)`].filter(Boolean).join('\n')}>
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
const Partes = memo(function Partes({ partes, tareas, detalle }: { partes: Parte[]; tareas?: T.Tarea[]; detalle: Detalle }) {
  // Las herramientas de un subagente van dentro de su tarea; la herramienta que crea una tarea se pinta como tarea.
  const dentro = (id: string) => partes.filter(p => p.tipo === 'herramienta' && p.h.padre === id);
  const sueltas = partes.filter(p => p.tipo !== 'herramienta' || !p.h.padre);
  const pinta = (p: Parte, j: number | string, abierta: boolean) => {
    const t = p.tipo === 'herramienta' ? T.deHerramienta(tareas, p.h.id) : undefined;
    return t ? <BloqueTarea key={j} t={t} h={p.tipo === 'herramienta' ? p.h : undefined} internas={dentro(t.herramienta!)} abierta={abierta} />
      : <ParteMensaje key={j} p={p} abierta={abierta} />;
  };
  const lista = tareas?.find(t => t.tipo === 'lista');
  // Tareas sin herramienta en la conversación (p. ej. lanzadas por un subagente): al final.
  const huerfanas = (tareas || []).filter(t => t.tipo !== 'lista' && !partes.some(p => p.tipo === 'herramienta' && p.h.id === t.herramienta));
  const extra = <>
    {lista && <ListaTareas t={lista} />}
    {huerfanas.map(t => <BloqueTarea key={t.id} t={t} internas={t.herramienta ? dentro(t.herramienta) : []} abierta={false} />)}
  </>;
  if (detalle !== 'conciso') return <>{lista && <ListaTareas t={lista} />}{sueltas.map((p, j) => pinta(p, j, detalle === 'detallado'))}{huerfanas.map(t => <BloqueTarea key={t.id} t={t} internas={t.herramienta ? dentro(t.herramienta) : []} abierta={false} />)}</>;
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
        <span className="tarea-tipo">{T.NOMBRES[t.tipo || ''] || 'Tarea'}{t.subtipo ? ' · ' + t.subtipo : ''}{t.segundo_plano && t.tipo === 'subagente' ? ' · en segundo plano' : ''}</span>
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

// La lista de tareas del agente, con casillas.
const ListaTareas = memo(function ListaTareas({ t }: { t: T.Tarea }) {
  const hechas = (t.elementos || []).filter(e => e.estado === 'hecho').length;
  return (
    <details className="lista-tareas-agente" open>
      <summary>Lista de tareas · {hechas}/{t.elementos?.length || 0}</summary>
      <ul>{(t.elementos || []).map((e, i) => (
        <li key={i} className={e.estado}><span className="casilla" aria-hidden>{e.estado === 'hecho' ? '☑' : e.estado === 'curso' ? '◐' : '☐'}</span> {e.texto}</li>
      ))}</ul>
    </details>
  );
});

const ParteMensaje = memo(function ParteMensaje({ p, abierta }: { p: Parte; abierta: boolean }) {
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
        <Miniaturas rutas={h.capturas} />
      </summary>
      {h.entrada && <pre className="entrada-h">{h.entrada}</pre>}
      {h.salida && <pre className="salida-h">{h.salida}</pre>}
    </details>
  );
});
