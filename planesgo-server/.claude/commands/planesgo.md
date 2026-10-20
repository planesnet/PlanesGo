---
description: Revisa el estado del tracking de PlanesGo, vincula un proyecto por búsqueda, imputa horas manuales (+N), actualiza todo (--update), inicializa el proyecto (init), activa/desactiva PSF (psf on|off|estado) o emite las pruebas en directo a Studio TV (stream [canal]|off)
argument-hint: [texto de búsqueda | +N horas | --update | init | psf on|off|estado | stream [canal]|off]
---

Argumento recibido tras el comando (puede venir vacío): "$ARGUMENTS"

## Caso P: argumento empieza por "psf" → activar, desactivar o ver PSF en este proyecto

Si "$ARGUMENTS" es `psf`, `psf on`, `psf off` o `psf estado` (sin distinguir mayúsculas), NO es texto de búsqueda (Caso A). Gestiona Planes Software Factory (PSF, carpeta `PSF/` de PlanesGo, que el servidor de PlanesGo sirve solo con token de empleado) en el proyecto actual. Responde en español y de forma breve.

1. **Localiza PSF** (directorio con `hooks/psf_hook.py` y `commands/psf.md`), en este orden: `$PSF_DIR`; `~/.planesgo/PSF`; la carpeta `PSF/` de un clon de PlanesGo; la ruta «Read from» de `psf@psf` en `claude plugin list`.
   - Si no lo encuentras, descárgalo: `mkdir -p ~/.planesgo && curl -fsSL -H "Authorization: Bearer $PLANESGO_TOKEN" https://planesgo.autopyme.com/install/psf.tar.gz | tar -xz -C ~/.planesgo` (si no hay `$PLANESGO_TOKEN`, usa `antigravity_token` de `~/.planesgo_auth.json`).
   - Si tampoco así, di que PSF no está disponible (falta el token de empleado o el servidor no responde) y para.
2. **`psf on`**:
   - Comprueba primero el proyecto de PlanesGo (`.planesgo.json` con `odoo_project_id > 0`); si falta, haz el Caso B para vincularlo antes de seguir.
   - Lee `<PSF>/commands/psf.md` y sigue su apartado «`on`», usando `<PSF>` donde diga `${CLAUDE_PLUGIN_ROOT}`.
   - Si existe `~/.planesgo/psf-arranque.sh`, ejecútalo y aplica como contexto de la sesión el `additionalContext` que devuelve; si no, ejecuta `CLAUDE_PLUGIN_ROOT=<PSF> python3 <PSF>/hooks/psf_hook.py session-start` y haz lo mismo. Así PSF queda aplicado desde ya, sin esperar a la siguiente sesión.
3. **`psf off`**: lee `<PSF>/commands/psf.md` y sigue su apartado «`off`» (pone `activo: false` en `.psf/proyecto.yml` sin borrar nada). La imputación de PlanesGo sigue igual: no depende de PSF.
4. **`psf` o `psf estado`**: sigue el apartado «`estado`» de `<PSF>/commands/psf.md`; si el proyecto no tiene `.psf/proyecto.yml`, dilo y sugiere `/planesgo psf on`.

Nunca hagas commit ni push de los cambios sin que la persona lo pida, y nunca en la rama de producción del proyecto.

## Caso S: argumento empieza por "stream" → emitir las pruebas en directo a Studio TV

Si "$ARGUMENTS" es `stream`, `stream <canal>` o `stream off` (sin distinguir mayúsculas), NO es texto de búsqueda (Caso A). Activa o desactiva la emisión en directo a Studio TV (servido por PlanesGo en `/studio-tv/`) de lo que haga el navegador de las pruebas automáticas. Responde en español y en 1-3 líneas.

1. **Activar** (`stream` o `stream <canal>`): llama a la herramienta MCP `planesgo_studio_emisor` con `{"canal": "<canal>"}` (sin canal, qa-<repositorio>-<rama> de la carpeta del proyecto, p. ej. qa-madpla-dev-lp). Alternativa CLI: `planesgo-mcp --stream <canal>` (o `--stream on`). Guarda PCO_URL, PCO_TOKEN y PCO_CANAL en `~/.planesgo/studio_tv.env` (0600).
   - El emisor `pco_emisor.py` (el del grabador de QA, o `curl -fsSL https://planesgo.autopyme.com/install/pco_emisor.py`) lee ese fichero solo: las pruebas emiten sin exportar nada.
   - Si la prueba no usa `pco_emisor.py`, carga las variables con `set -a; . ~/.planesgo/studio_tv.env; set +a`.
   - Nunca muestres ni copies PCO_TOKEN en la conversación, en commits ni en ficheros del repositorio.
   - Si la sesión corre en la nube y falla la conexión, el entorno debe permitir salir a `planesgo.autopyme.com` (*Network access*).
2. **Desactivar** (`stream off`): llama a `planesgo_studio_emisor` con `{"apagar": true}` (CLI: `planesgo-mcp --stream off`).
3. Responde con lo que devuelva la herramienta (canal y URL del estudio, sin token). Quien mira lo ve en PSFCode (artefacto Studio TV) o en `/admin/studio-tv` si es administrador.

## Caso I: argumento es "init" → inicializar el proyecto (PlanesGo + PSF)

Si "$ARGUMENTS" es exactamente `init`:
1. Haz el Caso B (diagnóstico) y, si el proyecto no está vinculado, vincúlalo preguntando a la persona a qué proyecto de Odoo corresponde (nunca lo deduzcas ni uses PLANESGO por defecto).
2. Pregunta si este proyecto va a seguir PSF (opción recomendada: sí). Si dice que sí, haz el Caso P con `psf on`; si no, termina.
3. Termina con un resumen de una frase: proyecto de PlanesGo vinculado y si PSF queda activo.

## Caso A0: argumento es "--update" → instalar/actualizar todo

Si "$ARGUMENTS" es exactamente `--update` (o empieza por `--update`), NO lo trates como texto de búsqueda de proyecto (Caso A) ni hagas el diagnóstico (Caso B). Ejecuta directamente:

```bash
curl -fsSL https://planesgo.autopyme.com/install/claude-hook.sh | bash
```

Esto reconstruye en un solo paso el hook global, el comando `/planesgo`, el binario `planesgo-mcp` y el servidor MCP, todos con la versión actual de `master`. Muestra la salida real del instalador (no la resumas) y termina con una frase: todo actualizado, o qué paso concreto falló. Si el servidor MCP se acaba de registrar por primera vez, recuerda que no se carga en caliente — hace falta una sesión nueva para que la herramienta `mcp__planesgo__*` aparezca disponible.

## Caso A1: argumento es "+N" → latido manual de N horas hoy

Si "$ARGUMENTS" coincide con el patrón `+<número>` (ej. `+3.5`, `+1`, `+0.5`; con punto decimal, no coma), interpreta el número como horas a imputar HOY en el proyecto que YA esté vinculado en esta sesión (ya sea por configuración propia de la sesión en `/tmp/planesgo-sessions/` o por `.planesgo.json` en la raíz del repo). Este caso no busca ni vincula ningún proyecto nuevo, es solo para imputar horas sobre lo que ya hay:

1. Ejecuta directamente la herramienta MCP `planesgo_add_hours` con `{"hours": <N>}` (o por CLI `planesgo-mcp --add-hours <N>`). El hook inyecta automáticamente el `session_id` y `project_path` de esta sesión. Si la sesión no tiene ningún proyecto vinculado todavía, responde solo "❌ Esta sesión no tiene ningún proyecto vinculado todavía. Usa `/planesgo <texto de búsqueda>` primero." y para aquí, sin ejecutar nada más.
2. Esto crea el registro de hoy con esas horas si no existía, o se las añade al que ya hubiera (no lo sobrescribe).
3. Responde en 1-2 líneas con el resultado tal cual lo devuelva el comando (nombre del proyecto, tarea, horas totales). No narres el proceso ni menciones otros puntos del diagnóstico.

## Caso A: con argumento → vincular directamente (por sesión / repositorio)

Si "$ARGUMENTS" NO está vacío (y no es `psf…`, `stream…`, `init`, `--update` ni `+N`, ya cubiertos arriba), trátalo como texto de búsqueda (nombre completo o parcial del proyecto de Odoo, sin acentos ni mayúsculas exactas) y vincula esta sesión directamente. Soporta **multi-sesión**: varias sesiones abiertas sobre el mismo repositorio pueden vincularse a proyectos de cargo distintos (ej. Sesión 1 `/planesgo PRO1`, Sesión 2 `/planesgo PRO2`) sin pisarse entre sí, ya que el hook inyecta el `session_id` de la conversación actual y guarda el vínculo de la sesión en `/tmp/planesgo-sessions/<session_id>.json` (y solo crea `.planesgo.json` en la raíz si el repositorio aún no tenía proyecto por defecto). Este caso es SOLO vincular, no diagnosticar: no compruebes ni menciones el token, el hook, el binario ni nada del Caso B, y no expliques el proceso — eso es ruido para el usuario. La respuesta debe ser corta, 1-2 líneas.

1. Determina la raíz del proyecto actual (raíz del repo git, o el directorio de trabajo si no es un repo), en silencio.
2. Busca con `planesgo-mcp --search-project "$ARGUMENTS"` (o la herramienta MCP `planesgo_search_projects` con `{"query": "$ARGUMENTS"}`), en silencio.
3. Según el resultado, responde ÚNICAMENTE con una de estas tres salidas (nada más):
   - **Una sola coincidencia**: vincúlala directamente sin pedir confirmación adicional, con `planesgo-mcp --set-project "<nombre exacto>" --path "<raíz>"` (o la herramienta MCP `planesgo_set_project` con `{"project_name": "<nombre exacto>", "project_path": "<raíz>"}`), y responde solo: "✅ Vinculado a **\<nombre\>** (ID \<id\>)."
   - **Varias coincidencias**: no vincules nada, responde solo con la lista numerada (nombre e ID) y "¿cuál de estos es?".
   - **Ninguna coincidencia**: responde solo "❌ Ningún proyecto de Odoo contiene '$ARGUMENTS'. Prueba con otro texto."

## Caso B: sin argumento → diagnóstico completo

Si "$ARGUMENTS" está vacío, haz un diagnóstico completo del sistema de tracking de tiempo de PlanesGo (Claude Code → Odoo) para el proyecto de esta sesión, y repórtalo al usuario con un ✅/❌ claro por cada punto:

1. **Proyecto vinculado (sesión actual / repositorio)**: determina la raíz del proyecto actual (raíz del repo git, o el directorio de trabajo si no es un repo). Comprueba el proyecto activo de esta sesión llamando a `planesgo_status` / `planesgo-mcp --check` (que resuelve primero el proyecto específico de la sesión en `/tmp/planesgo-sessions/<session_id>.json` y, en su defecto, `.planesgo.json` en esa raíz con `odoo_project_id > 0`). Si existe, muestra el nombre y el ID del proyecto de Odoo al que está vinculada esta sesión. Si no, dilo claramente.

2. **Token de autenticación**: comprueba si hay `$PLANESGO_TOKEN` o `$ANTIGRAVITY_TOKEN` en el entorno, o si existe `~/.planesgo_auth.json` con un token válido. No muestres el valor del token, solo si está presente.
   - IMPORTANTE: si el proyecto (punto 1) NO está vinculado, el guard de PlanesGo bloquea cualquier comando Bash compuesto (con `|`, `&&`, `||`, `;`, `>`, `<`) aunque sea inofensivo — es intencional, evita que se cuelen comandos de desarrollo disfrazados. Usa exclusivamente comandos sueltos y simples, uno por uno, sin combinarlos: `printenv PLANESGO_TOKEN`, `printenv ANTIGRAVITY_TOKEN`, `ls ~/.planesgo_auth.json`. Esos SÍ están permitidos siempre, esté o no vinculado el proyecto. Si aun así uno de ellos es denegado, no lo reintentes de otra forma ni concluyas que "todo Bash está bloqueado": simplemente repórtalo como ❓ y continúa con el resto del diagnóstico.

3. **Hook global instalado**: comprueba si existe `~/.claude/hooks/planesgo_claude_hook.py` y si `~/.claude/settings.json` tiene los hooks de PlanesGo conectados (session-start, prompt, guard, mcp, track, session-end apuntando a ese fichero). Si este proyecto además trae su propia copia en `.claude/hooks/` y `.claude/settings.json`, indícalo también (es normal que coexistan, el propio proyecto tiene prioridad).

4. **Binario disponible**: comprueba si `planesgo-mcp` está en el PATH o en `~/.local/bin/planesgo-mcp`.

5. **Conexión real**: si hay binario y el proyecto está vinculado, ejecuta `planesgo-mcp --check` y muestra el resultado tal cual.

6. **Servidor MCP**: comprueba si existe la herramienta `mcp__planesgo__planesgo_check` (o cualquier otra `mcp__planesgo__*`) ya cargada en esta sesión. Si no aparece ninguna, usa el comando `ls ~/.claude.json` (permitido siempre, sin pipes) y, si existe, revisa si contiene `"planesgo-mcp"` bajo `mcpServers` — si está ahí pero la herramienta no apareció, es que se registró después de arrancar esta sesión y hará falta una sesión nueva para que cargue; si no está en absoluto, no se ha registrado todavía.

Según lo que encuentres:
- Si el proyecto **no está vinculado**: pregunta al usuario a qué proyecto de Odoo corresponde (nunca lo asumas ni uses PLANESGO por defecto), o sugiérele usar directamente `/planesgo <texto de búsqueda>` para vincularlo sin más preguntas. Si no conoce el nombre exacto, búscalo con `planesgo-mcp --search-project "<texto parcial>"` (o la herramienta MCP `planesgo_search_projects`), muéstrale las coincidencias, y una vez confirmado vincula con `planesgo-mcp --set-project "<nombre exacto>"`.
- Si **falta el token**: indica al usuario que lo genere en https://planesgo.autopyme.com/settings y lo guarde en `~/.planesgo_auth.json`, o lo exporte como `$PLANESGO_TOKEN`.
- Si **falta el hook global, el binario o el servidor MCP**: sugiere ejecutar
  `curl -fsSL https://planesgo.autopyme.com/install/claude-hook.sh | bash`
  y, si el servidor MCP sigue sin aparecer tras eso, que entienda que necesita abrir una sesión nueva para que Claude Code cargue el servidor recién registrado (no se recarga en caliente en la sesión actual).

Termina con un resumen de una frase: si el tracking está operativo para esta sesión, o qué es exactamente lo único que falta.
