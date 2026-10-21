// Preguntas aparte: lo que el usuario pregunta sin que entre en la conversación con el agente (una interrupción
// que una persona olvidaría). pgocode decide antes de enviar si un mensaje va a la conversación o aparte:
//   · estado: «¿cómo va?» con el agente trabajando; lo responde pgocode con lo que sabe del turno (0 tokens).
//   · suelta: nada que ver con el trabajo; un modelo barato sin historial.
//   · contexto: sobre el trabajo, sin pedir nada; una copia de la sesión del agente que después se tira.
// Solo se manda aparte lo que está claro: en la duda, a la conversación (que el agente no se pierda una orden es
// más importante que algo de ruido en su contexto). Prefijos: «?» estado, «//» aparte, «!» a la conversación
// (también Ctrl+Intro).
// Funciones puras para poder probarlas: node --test src/aparte.test.ts
import type { Herramienta, Mensaje, Uso } from './api';
import type { Tarea } from './tareas';

// turno: a la conversación, pero ya, al turno en curso (solo agentes que lo admiten: Claude Code); nunca automático.
export type Destino = 'conversacion' | 'turno' | 'estado' | 'suelta' | 'contexto';
export interface Decision {
  destino: Destino;
  texto: string;       // sin el prefijo
  duda?: boolean;      // las reglas no lo tienen claro: que decida el clasificador (si el agente está ocupado)
  forzado?: boolean;   // lo ha pedido el usuario con un prefijo
  motivo: string;
}
// Lo aprendido de las correcciones del usuario: la firma del mensaje → adónde lo llevó.
export type Aprendidas = Record<string, Destino>;

const norm = (t: string) => t.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '');

// firma: las dos primeras palabras con significado (sin signos ni tildes), para reconocer mensajes parecidos.
const VACIAS = new Set(['el', 'la', 'los', 'las', 'un', 'una', 'de', 'del', 'que', 'y', 'a', 'en', 'me', 'te', 'se', 'lo', 'es', 'por', 'para', 'con', 'oye', 'pues', 'vale', 'bueno', 'cosa', 'otra', 'tal', 'va', 'hay', 'esta', 'son', 'su', 'mi', 'tu', 'al', 'o']);
export function firma(t: string): string {
  return norm(t).replace(/[^a-z0-9ñ\s]/g, ' ').split(/\s+/).filter(p => p && !VACIAS.has(p)).slice(0, 2).join(' ');
}

const ESTADO = /^(?:y\s+)?(?:c[oó]mo\s+(?:va|vas|vamos|lo\s+llevas|sigue|va\s+(?:eso|esto|la\s+cosa|el\s+trabajo|todo))|qu[eé]\s+(?:est[aá]s\s+haciendo|haces|tal\s+va|te\s+queda|falta)|en\s+qu[eé]\s+(?:est[aá]s|vas|punto)|por\s+d[oó]nde\s+vas|cu[aá]nto\s+(?:te\s+)?(?:falta|queda|llevas)|(?:actualiza(?:me)?|dame|dime|ens[eé][ñn]ame|mu[eé]strame|resume(?:me)?)\s+(?:el\s+|un\s+)?(?:estado|progreso|avance|resumen)|(?:estado|progreso|avance)(?:\s+(?:del?|de\s+la)\s+.*)?|has\s+terminado|ya\s+(?:has\s+)?(?:terminado|acabado)|va\s+todo\s+bien|sigues?\s+(?:ah[ií]|trabajando|vivo))(?=[\s?!.,]|$)/i;
// Órdenes y avisos para el agente: siempre a la conversación.
const ORDEN = /^(?:haz|hazlo|hagas|cambia|cambialo|añade|anade|agrega|quita|elimina|borra|crea|pon|usa|utiliza|prueba|pruebalo|ejecuta|lanza|sigue|continua|contin[uú]a|para(?=\s*$|\s+(?:ya|ah[ií]|de\s+\w+ar))|detente|det[eé]n|no\s|nunca|siempre|arregla|corrige|implementa|refactoriza|mueve|renombra|actualiza|revisa|comprueba|verifica|aseg[uú]rate|recuerda|ten\s+en\s+cuenta|tambi[eé]n|adem[aá]s|ojo|cuidado|mejor|en\s+vez|en\s+lugar|olvida|deja|vuelve|repite|sube|publica|commit|push|instala|escribe|documenta|explica\s+en|a[ñn]ade|(?:s[ií]|vale|ok|bien|de\s+acuerdo|adelante|perfecto|genial|gracias)(?=[\s,.!]|$))/i;
// Preguntas que no tienen que ver con el trabajo (sin nombrar nada del proyecto ni de la conversación).
const SUELTA = /^(?:qu[eé]\s+(?:hora|d[ií]a|tiempo)\s+(?:es|hace)|c[oó]mo\s+se\s+dice|qu[eé]\s+significa|cu[aá]l\s+es\s+la\s+capital|qui[eé]n\s+(?:es|fue|invent[oó]))/i;
// Algo de la conversación o del proyecto: «esto», «lo que has hecho», un fichero, código…
const REFERENCIA = /\b(?:esto|eso|esta|ese|aquello|lo\s+que\s+(?:has|est[aá]s|vas)|has\s+\w+|est[aá]s\s+\w+ndo|tu\s+(?:plan|cambio|propuesta|respuesta)|el\s+(?:c[oó]digo|fichero|archivo|test|commit|plan|cambio|error|fallo|proyecto|repo))\b|`|\w+\.(?:go|ts|tsx|js|jsx|py|md|json|css|html|sql|xml|yml|yaml|sh|toml)\b|\/\w+\//i;
const PREGUNTA = (t: string) => /^¿/.test(t) || /\?\s*$/.test(t) || /^(?:qu[eé]|c[oó]mo|cu[aá]l|cu[aá]ndo|d[oó]nde|por\s+qu[eé]|qui[eé]n|cu[aá]nto|sabes|hay)\b/i.test(t);

export interface Situacion {
  ocupado: boolean;      // el agente está trabajando (el mensaje se quedaría en la cola)
  ultimo?: string;       // lo último que ha dicho el agente
  hayConversacion?: boolean; // la sesión ya tiene mensajes (si no, no hay contexto que dar)
  adjuntos?: number;
  aprendidas?: Aprendidas;
  activo?: boolean;      // decidir automáticamente (ajuste de la sesión; por defecto, sí)
  ordenPlan?: boolean;   // es una orden de planes («/plan …», «ejecuta el plan 2»…)
}

export function decide(texto: string, s: Situacion): Decision {
  const t = texto.trim();
  // Prefijos: los pide el usuario, se respetan siempre.
  if (/^!(\s|$)/.test(t)) return { destino: 'conversacion', texto: t.slice(1).trim(), forzado: true, motivo: 'prefijo «!»' };
  if (/^\?(\s|$)/.test(t) || t === '?') return { destino: 'estado', texto: t.slice(1).trim(), forzado: true, motivo: 'prefijo «?»' };
  if (t.startsWith('//')) {
    const r = t.slice(2).trim();
    // Con contexto salvo que sea claramente ajena o que aún no haya conversación (en la duda, que sepa de qué va).
    return { destino: SUELTA.test(r.replace(/^[¿¡]/, '')) || !s.hayConversacion ? 'suelta' : 'contexto', texto: r, forzado: true, motivo: 'prefijo «//»' };
  }
  const conv = (motivo: string, duda = false): Decision => ({ destino: 'conversacion', texto: t, motivo, duda });
  if (s.activo === false) return conv('decisión automática desactivada en la sesión');
  if (s.ordenPlan) return conv('orden de planes');
  if (s.adjuntos) return conv('lleva adjuntos');
  const aprendida = s.aprendidas?.[firma(t)];
  if (aprendida) return { destino: aprendida === 'estado' && !s.ocupado ? 'conversacion' : aprendida, texto: t, motivo: 'como lo corregiste antes' };
  // Con el agente libre no hay cola ni nada que interrumpir: a la conversación (sin gastar en clasificar).
  if (!s.ocupado) return conv('el agente está libre');
  if (ESTADO.test(t.replace(/^[¿¡]/, ''))) return { destino: 'estado', texto: t, motivo: 'pregunta por el estado del trabajo' };
  if (ORDEN.test(t.replace(/^[¿¡]/, ''))) return conv('es una orden o una respuesta al agente');
  if (s.ultimo && /\?\s*$/.test(s.ultimo.trim())) return conv('el agente había hecho una pregunta');
  if (!PREGUNTA(t)) return conv('no es una pregunta');
  if (SUELTA.test(t.replace(/^[¿¡]/, '')) && !REFERENCIA.test(t)) return { destino: 'suelta', texto: t, motivo: 'pregunta que no tiene que ver con el trabajo' };
  return conv('pregunta: no está claro', true);
}

// aprende: la corrección del usuario (llevar un mensaje a otro sitio) se recuerda para los parecidos.
export function aprende(a: Aprendidas, texto: string, destino: Destino): Aprendidas {
  const f = firma(texto);
  if (!f) return a;
  const n = { ...a, [f]: destino };
  // Como mucho 200 (las últimas).
  const ks = Object.keys(n);
  return ks.length > 200 ? Object.fromEntries(ks.slice(-200).map(k => [k, n[k]])) : n;
}

// Estado del trabajo, de lo que pgocode ya sabe del turno en curso (sin preguntar al agente).
const EDITA = /^(write|edit|multiedit|notebookedit|patch|apply_patch|create_file|replace|write_file|edit_file|str_replace)/i;
const ORDENES = /^(bash|shell|run|exec|run_command|terminal)/i;
export function resumenEstado(m: Mensaje | undefined, o: { trabajando: boolean; segundos?: number; enCola?: number }): string {
  if (!m || m.rol !== 'assistant') return o.trabajando ? 'El agente acaba de empezar: aún no ha hecho nada.' : 'No hay ningún turno en curso ni anterior en esta sesión.';
  const l: string[] = [];
  const cab = o.trabajando ? `**Trabajando** desde hace ${duracion(o.segundos || 0)}` : '**Último turno** (ya terminado)';
  const u = m.uso ? ` · ${tokens(m.uso.entrada + m.uso.salida)} tokens${m.uso.coste ? ' · $' + m.uso.coste.toFixed(2) : ''}` : '';
  l.push(cab + u + (o.enCola ? ` · ${o.enCola} en cola` : '') + '.');
  const lista = m.tareas?.find(t => t.tipo === 'lista');
  if (lista?.elementos?.length) {
    const el = lista.elementos;
    const hechas = el.filter(e => e.estado === 'hecho').length;
    const curso = el.filter(e => e.estado === 'curso');
    const pend = el.filter(e => e.estado === 'pendiente');
    l.push(`\n**Lista de tareas: ${hechas} de ${el.length} hechas.**`);
    for (const e of curso) l.push(`- ◐ Ahora: ${e.texto}`);
    if (pend.length) l.push(`- Pendientes: ${pend.map(e => e.texto).join(' · ')}`);
  }
  const vivas = (m.tareas || []).filter((t: Tarea) => t.tipo !== 'lista' && t.estado === 'curso');
  if (vivas.length) l.push(`\n**En marcha:** ${vivas.map(t => `${t.tipo === 'subagente' ? 'subagente' : 'segundo plano'} «${t.titulo || t.subtipo || t.id}»${t.progreso ? ` (${t.progreso})` : ''}`).join(' · ')}`);
  const hs = (m.partes || []).filter(p => p.tipo === 'herramienta').map(p => (p as { h: Herramienta }).h);
  const editados = [...new Set(hs.filter(h => EDITA.test(h.nombre) && h.resumen).map(h => corto(h.resumen!)))];
  if (editados.length) l.push(`\n**Ficheros cambiados (${editados.length}):** ${editados.slice(-8).map(f => '`' + f + '`').join(', ')}${editados.length > 8 ? '…' : ''}`);
  const ordenes = hs.filter(h => ORDENES.test(h.nombre) && h.resumen);
  if (ordenes.length) l.push(`\n**Última orden:** \`${recorta(ordenes[ordenes.length - 1].resumen!, 120)}\`${ordenes[ordenes.length - 1].estado === 'curso' ? ' (en curso)' : ''}`);
  const ahora = hs.filter(h => h.estado === 'curso');
  if (o.trabajando && ahora.length) l.push(`\n**Ahora mismo:** ${ahora.map(h => h.nombre + (h.resumen ? ' ' + recorta(h.resumen, 80) : '')).join(' · ')}`);
  else if (hs.length) l.push(`\n${hs.length} acciones en el turno (${hs.filter(h => h.estado === 'error').length} con error).`);
  const ultimo = ultimoTexto(m);
  if (ultimo) l.push(`\n**Lo último que ha dicho:** «${recorta(ultimo, 300)}»`);
  return l.join('\n');
}

// resumenContexto: lo que se le pasa al modelo barato cuando el agente no permite copiar su sesión (Antigravity).
export function resumenContexto(ms: Mensaje[], max = 12000): string {
  const l: string[] = [];
  const primera = ms.find(m => m.rol === 'user');
  if (primera) l.push('Primera petición del usuario: ' + recorta(primera.texto, 1500));
  const ultimos = ms.slice(-8).filter(m => m !== primera && (m.texto || '').trim());
  for (const m of ultimos) l.push((m.rol === 'user' ? 'Usuario: ' : 'Agente: ') + recorta(m.rol === 'assistant' ? ultimoTexto(m) || m.texto : m.texto, 1500));
  const u = [...ms].reverse().find(m => m.rol === 'assistant');
  if (u) l.push('Estado: ' + resumenEstado(u, { trabajando: false }).replace(/\*\*/g, ''));
  const t = l.join('\n\n');
  return t.length > max ? t.slice(t.length - max) : t;
}

// incorporacion: lo que se añade al final del próximo mensaje al agente para que sepa de las preguntas aparte que
// el usuario ha querido incorporar (sin gastar un turno solo para eso; al final, para no romper una «/skill»).
export const incorporacion = (l: { pregunta: string; respuesta: string }[]) => l.length === 0 ? '' :
  '\n\n[Preguntas que hice aparte mientras trabajabas, para que las tengas en cuenta (no hace falta que las respondas)]\n' +
  l.map(x => `Pregunta: ${x.pregunta}\nRespuesta: ${recorta(x.respuesta, 2000)}`).join('\n\n');

// sumaUso: el consumo acumulado de las preguntas aparte de una sesión (se suma al gasto de la conversación).
export const sumaUso = (a: Uso | undefined, b: Uso): Uso => ({ entrada: (a?.entrada || 0) + b.entrada, salida: (a?.salida || 0) + b.salida, coste: (a?.coste || 0) + (b.coste || 0) });

export const NOMBRES: Record<Destino, string> = { conversacion: 'En la conversación', turno: 'Al turno en curso', estado: 'Estado', suelta: 'Suelta', contexto: 'Con contexto' };

// Etiqueta del destino mientras se escribe (antes de enviar).
export function etiqueta(d: Decision, ocupado = false): string {
  if (d.destino === 'conversacion') return d.duda ? '→ Conversación o aparte: lo decide pgocode al enviar' : ocupado ? '→ Conversación (a la cola)' : '→ Conversación';
  if (d.destino === 'turno') return '→ Al turno en curso (el agente lo lee ya)';
  return '→ ' + (d.destino === 'estado' ? 'Estado (sin tokens)' : 'Aparte · ' + NOMBRES[d.destino].toLowerCase());
}

// siguiente: el destino al pulsar la etiqueta (conversación → al turno en curso, si se puede → aparte con contexto
// → suelta → estado → conversación).
export const siguiente = (d: Destino, conTurno = false): Destino => {
  const orden: Destino[] = conTurno ? ['conversacion', 'turno', 'contexto', 'suelta', 'estado'] : ['conversacion', 'contexto', 'suelta', 'estado'];
  return orden[(orden.indexOf(d) + 1) % orden.length];
};

function ultimoTexto(m: Mensaje): string {
  const ps = m.partes || [];
  for (let i = ps.length - 1; i >= 0; i--) {
    const p = ps[i];
    if (p.tipo === 'texto' && p.texto.trim()) return p.texto.trim();
  }
  return ps.length ? '' : (m.texto || '').trim();
}
const recorta = (t: string, n: number) => { const x = t.replace(/\s+/g, ' ').trim(); return x.length > n ? x.slice(0, n - 1) + '…' : x; };
const corto = (r: string) => r.split('/').slice(-2).join('/');
const tokens = (n: number) => n >= 1e6 ? (n / 1e6).toFixed(1) + ' M' : n >= 1000 ? Math.round(n / 1000) + 'k' : String(n);
function duracion(s: number) {
  if (s < 60) return Math.round(s) + ' s';
  const m = Math.floor(s / 60);
  return m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${m % 60} min`;
}
