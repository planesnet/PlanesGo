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
      ...(o.inyecta ? [{ clave: 'Alt+Intro', que: 'Al turno en curso, aunque «Siempre al turno» esté desactivado (con adjuntos, a la cola)' }] : []),
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
      { clave: '/issue [título]', que: 'Crear una issue en GitHub en el repositorio del proyecto (la resuelve la resolución automática o «Resolver» en Flujo)' },
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
      { clave: 'Para que lo haga el agente', que: 'Dilo con más palabras («compila tú y revisa los avisos»), pon «>» delante o pulsa la etiqueta antes de enviar. Se desactiva en las propiedades de la sesión' },
    ] },
    { titulo: 'Quién hace qué (enrutador)', entradas: [
      { clave: 'Orden', que: 'Cada mensaje va a un solo sitio, el primero que encaja: órdenes de pgocode → funciones directas (0 tokens) → pregunta aparte → reparto en subtareas → el agente. En la duda, al agente' },
      { clave: 'Aparte', que: 'Estado: pgocode (0 tokens) · con contexto: una copia de la sesión del agente · suelta: la IA local del equipo (si no está, un modelo barato)' },
      { clave: 'Etiqueta y distintivo', que: 'La etiqueta encima del cuadro dice adónde irá; al pasar el ratón, quién lo hace, quién lo ha decidido y por qué. Cada mensaje enviado lleva su ruta' },
      { clave: 'Reparto', que: 'Una petición con varias partes independientes (una lista de 3 o más) se puede repartir en subtareas en paralelo: cada una en su sesión y su copia de trabajo; al acabar se unen a tu rama' },
    ] },
    { titulo: 'Contexto del agente', entradas: [
      { clave: 'Hilo de limpieza', que: 'El indicador del contexto: cuánto ocupa la conversación del agente y lo que ya no hace falta. Antes de cada turno pgocode retira lo que no está vigente (lecturas superadas, salidas largas…) y lo repone si vuelve a hacer falta' },
      { clave: 'Barrer ahora', que: 'Retira ya lo que no hace falta, sin esperar al próximo turno (puede costar caché: el aviso dice si compensa)' },
      { clave: 'Relevo', que: 'Si el agente no deja tocar su historial (o la caché ha caducado), sigue en una conversación nueva con una memoria de trabajo de lo vigente' },
      { clave: 'Continuar desde aquí', que: 'En una respuesta: el agente sigue desde ese punto, olvidando lo de después (sigue en la vista)' },
    ] },
    { titulo: 'Sesiones, programas y copias', entradas: [
      { clave: 'Duplicar sin historial', que: 'Menú ⋮ de la sesión: otra con los mismos ajustes y la conversación vacía (el agente empieza de cero)' },
      { clave: 'Archivar', que: 'Menú ⋮ de la sesión: sale de la lista (queda en «Archivadas», con su conversación)' },
      { clave: 'Programas', que: 'Los programas del proyecto (un Odoo, un servicio…) se definen en Propiedades de la sesión → Programas y se ejecutan o paran desde Actividad o el lanzador, sin tokens' },
      { clave: 'pgo ejecutar', que: 'El agente arranca un programa con «pgo ejecutar <nombre> <orden>» (lo para con «pgo parar <nombre>»): sale en Actividad con su salida' },
      { clave: 'pgo drive', que: 'El agente lee Google Drive, Docs, Sheets y Slides por su enlace (solo lectura): «pgo drive leer <enlace o id> [--hoja X] [--rango A1:D20]» descarga el fichero como «Archivo → Descargar» (Docs en Markdown, Sheets en CSV; PDF y demás quedan en .pgocode/salidas). No busca ni lista: pégale el enlace. Acceso: con tu sesión de Google en el navegador de pgocode (panel Web): inicia sesión una vez, como en cualquier navegador; sin ajustes' },
      { clave: 'pgo gmail', que: 'El agente lee un correo de Gmail por su enlace, en solo lectura (no busca, envía, borra ni archiva nada): «pgo gmail leer <enlace>» descarga el mensaje original (.eml) y da su texto sin lo citado y los adjuntos (nombre, tipo, tamaño). Pégale el enlace del correo (el de la barra de direcciones). Usa la misma sesión de Google del navegador de pgocode' },
      { clave: 'Copias de trabajo', que: 'Cada rama de una sesión vive en su copia (git worktree). Actividad las lista y deja quitar las que no pierden nada; las sesiones que la usaban se archivan' },
    ] },
    { titulo: 'En la interfaz', entradas: [
      { clave: 'Etiqueta de destino', que: 'Clic para cambiar adónde va el mensaje antes de enviarlo' },
      { clave: 'Siempre al turno', que: 'Lo escrito con el agente trabajando le llega ya, en vez de esperar en la cola' },
      { clave: 'Trabajando… · N tokens', que: 'Tokens de la conversación hasta ahora; cada acción lleva los suyos' },
    ] },
  ];
}
