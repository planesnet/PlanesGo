// Modo plan: el agente redacta un plan sin tocar nada; el usuario lo revisa (lo edita a mano o pide cambios) y al
// final lo ejecuta o lo descarta. Funciones puras para poder probarlas: node --test src/plan.test.ts
import type { Mensaje } from './api';

// peticionPlan: lo que se le envía al agente en modo plan. Con un plan ya en marcha (que el usuario puede haber
// editado), se le pasa entero para que lo revise con los cambios pedidos.
export function peticionPlan(peticion: string, planActual?: string): string {
  const reglas = 'sin modificar ningún fichero ni ejecutar nada que cambie el sistema';
  if (planActual?.trim()) {
    return `[Modo plan de pgocode] Este es el plan actual (el usuario puede haberlo editado a mano):\n\n${planActual.trim()}\n\n` +
      `Revísalo con lo que pide el usuario, ${reglas}, y responde únicamente con el plan completo revisado en Markdown.\n\n` +
      `Lo que pide el usuario:\n${peticion}`;
  }
  return `[Modo plan de pgocode] Investiga lo necesario ${reglas}, y responde únicamente con un plan de implementación en ` +
    'Markdown: un título con «#», el contexto, los pasos numerados (con los ficheros afectados), los riesgos y cómo se ' +
    `verificará. No lo ejecutes: el usuario lo revisará antes.\n\nPetición:\n${peticion}`;
}

// peticionEjecucion: el mensaje que pone en marcha un plan aprobado.
export function peticionEjecucion(ruta: string, plan: string): string {
  return `Ejecuta este plan, aprobado por el usuario (guardado en ${ruta}). Síguelo paso a paso y avisa si algo no se ` +
    `puede hacer como dice:\n\n${plan.trim()}`;
}

// textoDelPlan: el plan es lo que el agente escribe tras su última herramienta (lo de antes suele ser su
// razonamiento en voz alta mientras investiga).
export function textoDelPlan(m: Mensaje | undefined): string {
  if (!m || m.rol !== 'assistant' || m.error) return '';
  const ps = m.partes || [];
  let ultima = -1;
  ps.forEach((p, i) => { if (p.tipo === 'herramienta') ultima = i; });
  const tras = ps.slice(ultima + 1).filter(p => p.tipo === 'texto').map(p => (p as { texto: string }).texto).join('');
  return (tras.trim() ? tras : ps.length ? '' : m.texto).trim();
}

// tituloPlan: el primer título del Markdown o, si no tiene, el principio de la petición.
export function tituloPlan(plan: string, peticion: string): string {
  const h = plan.split('\n').find(l => /^#{1,3}\s+\S/.test(l));
  const t = (h ? h.replace(/^#+\s+/, '') : peticion.split('\n')[0]).replace(/[*_`]/g, '').trim();
  return t.length > 80 ? t.slice(0, 79).trimEnd() + '…' : t || 'Plan';
}

// Órdenes de planes en el chat (los planes son una herramienta, no un modo; van numerados por proyecto):
//   «/plan <petición>» crea uno · «plan N: <cambios>» o «/plan N <cambios>» lo revisa · «ejecuta el plan N» ·
//   «descarta el plan N» · «reactiva el plan N» · «borra el plan N» (estas tres, en local: sin agente ni tokens).
export type OrdenPlan =
  | { tipo: 'nuevo'; peticion: string }
  | { tipo: 'revisar'; numero: number; peticion: string }
  | { tipo: 'ejecutar' | 'descartar' | 'reactivar' | 'borrar'; numero: number };

export function interpreta(texto: string): OrdenPlan | null {
  const t = texto.trim();
  let m = /^\/plan\s+#?(\d+)\s+([\s\S]+)$/i.exec(t) || /^plan\s+#?(\d+)\s*[:,\-–]\s*([\s\S]+)$/i.exec(t);
  if (m) return { tipo: 'revisar', numero: Number(m[1]), peticion: m[2].trim() };
  m = /^\/plan\s+([\s\S]+)$/i.exec(t);
  if (m) return { tipo: 'nuevo', peticion: m[1].trim() };
  const acciones: [RegExp, 'ejecutar' | 'descartar' | 'reactivar' | 'borrar'][] = [
    [/^(ejecuta|ejecutar|lanza|lanzar)\s+(el\s+)?plan\s+#?(\d+)[.!]?$/i, 'ejecutar'],
    [/^(descarta|descartar)\s+(el\s+)?plan\s+#?(\d+)[.!]?$/i, 'descartar'],
    [/^(reactiva|reactivar|recupera|recuperar)\s+(el\s+)?plan\s+#?(\d+)[.!]?$/i, 'reactivar'],
    [/^(borra|borrar|elimina|eliminar)\s+(el\s+)?plan\s+#?(\d+)[.!]?$/i, 'borrar'],
  ];
  for (const [re, tipo] of acciones) {
    const x = re.exec(t);
    if (x) return { tipo, numero: Number(x[3]) };
  }
  return null;
}
