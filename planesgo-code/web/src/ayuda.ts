// Ayuda de pgocode («/help» o «/ayuda» en el chat): las teclas y órdenes más habituales. Se muestra en la
// interfaz, sin pasar por el agente (0 tokens) ni quedar en la conversación.
// Funciones puras para poder probarlas: node --test src/ayuda.test.ts

export interface Entrada { clave: string; que: string }
export interface Seccion { titulo: string; entradas: Entrada[] }

export const esAyuda = (t: string) => /^\/(?:help|ayuda)\s*$/i.test(t.trim());

export function secciones(o: { alTurno: boolean; inyecta: boolean }): Seccion[] {
  const intro = o.inyecta && o.alTurno
    ? 'Enviar. Con el agente trabajando, le llega ya: lo lee antes de acabar su turno'
    : 'Enviar. Con el agente trabajando, a la cola: sale sola al acabar el turno';
  return [
    { titulo: 'Teclas', entradas: [
      { clave: 'Intro', que: intro },
      ...(o.inyecta ? [{ clave: 'Alt+Intro', que: 'Al turno en curso, aunque «Siempre al turno» esté desactivado' }] : []),
      { clave: 'Ctrl+Intro · Mayús+Intro', que: 'Nueva línea' },
      { clave: 'Esc', que: 'Cerrar esta ayuda, menús y diálogos' },
      { clave: '← →', que: 'Capturas anterior y siguiente (en el visor)' },
    ] },
    { titulo: 'Prefijos del mensaje', entradas: [
      { clave: '>', que: o.inyecta ? 'Directo a la conversación actual: con el agente trabajando, le llega ya al turno en curso' : 'Directo a la conversación actual (sin que pgocode lo lleve aparte)' },
      { clave: '//', que: 'Pregunta aparte: no entra en la conversación' },
      { clave: '?', que: 'Estado del trabajo, sin gastar tokens' },
    ] },
    { titulo: 'Órdenes', entradas: [
      { clave: '/help · /ayuda', que: 'Esta ayuda' },
      { clave: '/plan <petición>', que: 'Crear un plan antes de tocar nada' },
      { clave: '/plan N <cambios>', que: 'Revisar el plan N' },
      { clave: 'ejecuta el plan N', que: 'Ejecutarlo (también: descarta, reactiva o borra el plan N)' },
      { clave: '/<skill> …', que: 'Usar una skill (Claude Code)' },
    ] },
    { titulo: 'En la interfaz', entradas: [
      { clave: 'Etiqueta de destino', que: 'Clic para cambiar adónde va el mensaje antes de enviarlo' },
      { clave: 'Siempre al turno', que: 'Lo escrito con el agente trabajando le llega ya, en vez de esperar en la cola' },
      { clave: 'Trabajando… · N tokens', que: 'Tokens de la conversación hasta ahora; cada acción lleva los suyos' },
    ] },
  ];
}
