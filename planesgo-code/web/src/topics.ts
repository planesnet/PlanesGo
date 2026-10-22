// Ramas por tema: cuando, en una sesión que ya ha crecido, el usuario pide otra cosa («Añadir las etiquetas a la
// lista de tareas» en una sesión sobre el diseño), pgocode propone mandarlo a una sesión hija con lo mínimo en vez de
// a la conversación, que así no engorda. La detección es barata: solo con contexto relevante, un modelo barato con el
// mensaje y el tema de la sesión en 2 o 3 líneas (/api/chat/clasifica). En la duda, a la conversación.
// Funciones puras para poder probarlas: node --test src/topics.test.ts
// Trabajan sobre la memoria de trabajo ya recogida (collectWorkingMemory de workingMemory.ts), sin duplicarla.
import type { WorkingMemory } from './workingMemory';

// Por debajo de esto (tokens de la última llamada del agente) no merece la pena: la conversación aún es pequeña.
export const TOPIC_MIN_CONTEXT = 30_000;
// Contexto mínimo de la sesión hija (caracteres; ~1,5k tokens).
export const CHILD_CONTEXT_MAX = 6_000;

export interface TopicCheckInput {
  context?: number;      // uso.contexto de la última respuesta del agente
  text: string;
  attachments?: number;
  planOrder?: boolean;   // orden de planes
  active?: boolean;      // decisión automática de la sesión (decideAparte)
  agent?: boolean;       // el proveedor es un agente (no un modelo sin historial propio)
}

// Lo que sigue claramente la conversación: empieza enlazando con ella (recuerda, ahora, también, y, pero, vale…) o
// se refiere a lo hablado (esto, eso, lo que has…, el método que…). Nunca es otro tema (ni se pregunta).
const CONTINUES = /^(?:recuerda|ahora|también|tambien|además|ademas|y|pero|vale|ok|bien|sí|si|no|sigue|continúa|continua|perfecto|gracias|entonces|luego|después|despues|otra cosa del|lo mismo)\b/i;
const REFERS = /\b(?:esto|eso|esta|ese|aquello|lo que (?:has|hiciste|acabas)|lo (?:anterior|de antes)|el (?:método|metodo|fichero|cambio|error|fallo) que|la (?:función|funcion|prueba) que|antes|arriba)\b/i;
// Una petición de otro tema describe trabajo: unas cuantas palabras como mínimo.
export const TOPIC_MIN_WORDS = 6;

// shouldCheckTopic: ¿merece la pena preguntar al clasificador si es otro tema? Solo con un agente, contexto relevante
// y un mensaje que parezca una petición de trabajo nueva: ni órdenes «/…», ni adjuntos, ni planes, ni mensajes
// cortos, ni los que enlazan con la conversación o se refieren a lo hablado (así no hay falsos positivos como
// «Recuerda esta palabra clave: AZUL»).
export function shouldCheckTopic(i: TopicCheckInput): boolean {
  const t = i.text.trim();
  if (!i.agent || i.active === false || i.attachments || i.planOrder) return false;
  if ((i.context || 0) < TOPIC_MIN_CONTEXT) return false;
  if (t.startsWith('/') || t.startsWith('>') || t.startsWith('?')) return false;
  const plain = t.replace(/^[¿¡«"']+/, '');
  if (CONTINUES.test(plain) || REFERS.test(plain)) return false;
  return t.split(/\s+/).length >= TOPIC_MIN_WORDS;
}

const squash = (t: string) => t.replace(/\s+/g, ' ').trim();
const cut = (t: string, n: number) => { const x = squash(t); return x.length > n ? x.slice(0, n - 1) + '…' : x; };
const base = (f: string) => f.split('/').filter(Boolean).pop() || f;

// topicSummary: el tema de la sesión en 2 o 3 líneas para el clasificador (objetivo, últimas peticiones, ficheros).
export function topicSummary(w: WorkingMemory): string {
  const l: string[] = [];
  if (w.goal) l.push('Objetivo: ' + cut(w.goal, 220));
  const recent = w.requests.slice(-2);
  if (recent.length) l.push('Últimas peticiones: ' + recent.map(r => '«' + cut(r, 140) + '»').join(' · '));
  const files = [...new Set([...w.files, ...w.read].slice(-8).map(base))];
  if (files.length) l.push('Ficheros: ' + files.join(', '));
  return l.join('\n');
}

// childContext: lo que necesita una sesión hija para un tema nuevo: el objetivo de la madre y sus ficheros (no la
// conversación ni las órdenes). Como mucho «max» caracteres. Las rutas, relativas a «root» (la carpeta de la sesión).
export function childContext(w: WorkingMemory, max = CHILD_CONTEXT_MAX, root = ''): string {
  if (!w.goal) return '';
  const r = root.replace(/\/+$/, '') + '/';
  const rel = (f: string) => (root && f.startsWith(r) ? f.slice(r.length) : f);
  const sec: string[] = ['Objetivo de la sesión de la que sale: ' + cut(w.goal, 1200)];
  if (w.files.length) sec.push(`Ficheros creados o editados allí (${w.files.length}): ` + w.files.slice(-25).map(rel).join(', '));
  if (w.read.length) sec.push(`Ficheros consultados allí (${w.read.length}): ` + w.read.slice(-25).map(rel).join(', '));
  const t = sec.join('\n');
  return t.length > max ? t.slice(0, max - 1) + '…' : t;
}

// childMessage: el primer mensaje de la hija: el contexto mínimo y el mensaje del usuario (si el mensaje empieza por
// una orden «/skill …», el contexto va detrás para no romperla).
export function childMessage(context: string, message: string): string {
  if (!context.trim()) return message;
  const head = '[Contexto: esta sesión sale de otra del mismo proyecto para tratar un tema nuevo. De aquella, solo lo que puede servirte; si te falta algo, mira los ficheros]\n' + context + '\n[Fin del contexto]';
  return message.trimStart().startsWith('/') ? `${message}\n\n${head}` : `${head}\n\n${message}`;
}
