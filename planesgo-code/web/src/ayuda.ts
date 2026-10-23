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
      { clave: '↑ ↓', que: 'Mensajes anteriores (en la primera o la última línea): los de la cola se editan en su sitio; los enviados se cargan para mandarlos otra vez. Esc o ↓ al final: vuelve lo que escribías' },
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
    { titulo: 'Sin el agente (0 tokens)', entradas: [
      { clave: 'compila · pasa los tests', que: 'Si el mensaje es solo eso, pgocode ejecuta la orden del proyecto (programa guardado, Makefile, package.json, go.mod…) en la carpeta de la sesión y deja una nota con el resultado. Si falla: «Pásaselo al agente», con solo los errores' },
      { clave: 'comprueba los tipos · lint', que: 'Igual, si el proyecto lo tiene (si no se sabe la orden con seguridad, va al agente)' },
      { clave: 'haz commit', que: 'git add -A y commit con un mensaje redactado por la IA local (sin ella, por reglas). Nunca push: «sube», «push», «publica» van al agente' },
      { clave: '¿qué cambios hay? · diff', que: 'Estado de git y diff, sin el agente' },
      { clave: '¿qué ficheros has tocado?', que: 'Los que ha editado el agente en la conversación y los que tienen cambios según git' },
      { clave: '¿cuánto llevamos gastado?', que: 'Tokens y coste de la sesión' },
      { clave: '¿en qué rama estamos? · ¿qué has hecho?', que: 'Rama y carpeta; las acciones del agente en su último turno' },
      { clave: 'Para que lo haga el agente', que: 'Dilo con más palabras («compila tú y revisa los avisos»), pon «>» delante o pulsa la etiqueta antes de enviar. Se desactiva en los ajustes de la sesión' },
    ] },
    { titulo: 'Quién hace qué (enrutador)', entradas: [
      { clave: 'Orden', que: 'Cada mensaje va a un solo sitio, el primero que encaja: órdenes de pgocode → funciones directas (0 tokens) → pregunta aparte → reparto en subtareas → el agente. En la duda, al agente' },
      { clave: 'Aparte', que: 'Estado: pgocode (0 tokens) · con contexto: una copia de la sesión del agente · suelta: la IA local del equipo (si no está, un modelo barato)' },
      { clave: 'Etiqueta y distintivo', que: 'La etiqueta encima del cuadro dice adónde irá; al pasar el ratón, quién lo hace, quién lo ha decidido y por qué. Cada mensaje enviado lleva su ruta' },
      { clave: 'Actividad', que: 'Cuántos mensajes de la sesión han ido a cada sitio y cuántos sin tokens de pago. Todo, en docs/enrutador.md' },
    ] },
    { titulo: 'En la interfaz', entradas: [
      { clave: 'Etiqueta de destino', que: 'Clic para cambiar adónde va el mensaje antes de enviarlo' },
      { clave: 'Siempre al turno', que: 'Lo escrito con el agente trabajando le llega ya, en vez de esperar en la cola' },
      { clave: 'Trabajando… · N tokens', que: 'Tokens de la conversación hasta ahora; cada acción lleva los suyos' },
    ] },
  ];
}
