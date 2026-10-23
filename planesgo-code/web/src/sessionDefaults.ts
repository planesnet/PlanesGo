// Valores con los que nace una sesión nueva: el alojamiento de la última sesión del proyecto (es obligatorio y casi
// siempre el mismo), el último modelo elegido con ese agente y el modo «editar» (nunca «auto» heredado: ejecuta
// órdenes sin preguntar; si se quiere, se elige a mano). Funciones puras salvo lo guardado en el navegador.
import type { TipoHosting } from './ajustes';
import type { Sesion } from './sesiones';

const modelKey = (login: string, agent: string) => `pgo:modelo:${login.toLowerCase()}:${agent}`;

// rememberModel: el modelo elegido con un agente (en cualquier sesión), para la próxima sesión con él.
export function rememberModel(login: string, agent: string, model: string | undefined) {
  if (!login || !agent || !model) return;
  try { localStorage.setItem(modelKey(login, agent), model); } catch { /* sin almacenamiento */ }
}

// rememberedModel: el último modelo elegido con ese agente, si sigue en su lista.
export function rememberedModel(login: string, agent: string, models: readonly string[]): string | undefined {
  try {
    const m = localStorage.getItem(modelKey(login, agent)) || '';
    return m && models.includes(m) ? m : undefined;
  } catch { return undefined; }
}

// lastHosting: el alojamiento de la última sesión creada del proyecto que lo tenga; si ninguna, el del proyecto.
export function lastHosting(sessions: readonly Sesion[], projectHosting?: TipoHosting): TipoHosting | undefined {
  const s = [...sessions].filter(x => x.hosting && !x.madre).sort((a, b) => (b.creada || 0) - (a.creada || 0))[0];
  return s?.hosting || projectHosting;
}

// newSessionDefaults: completa una sesión recién creada (aún sin guardar) con lo recordado.
export function newSessionDefaults(s: Sesion, o: { sessions: readonly Sesion[]; projectHosting?: TipoHosting; model?: string }): Sesion {
  return { ...s, permisos: 'editar', hosting: s.hosting || lastHosting(o.sessions, o.projectHosting), modelo: s.modelo || o.model };
}
