---
description: Revisa el estado del tracking de PlanesGo en esta sesión, o vincula directamente un proyecto si se pasa un texto de búsqueda como argumento
argument-hint: [texto de búsqueda del proyecto de Odoo (opcional)]
---

Argumento recibido tras el comando (puede venir vacío): "$ARGUMENTS"

## Caso A0: argumento es "--update" → instalar/actualizar todo

Si "$ARGUMENTS" es exactamente `--update` (o empieza por `--update`), NO lo trates como texto de búsqueda de proyecto (Caso A) ni hagas el diagnóstico (Caso B). Ejecuta directamente:

```bash
curl -fsSL https://raw.githubusercontent.com/planesnet/PlanesGo/master/scripts/install-claude-hook.sh | bash
```

Esto reconstruye en un solo paso el hook global, el comando `/planesgo`, el binario `planesgo-mcp` y el servidor MCP, todos con la versión actual de `master`. Muestra la salida real del instalador (no la resumas) y termina con una frase: todo actualizado, o qué paso concreto falló. Si el servidor MCP se acaba de registrar por primera vez, recuerda que no se carga en caliente — hace falta una sesión nueva para que la herramienta `mcp__planesgo__*` aparezca disponible.

## Caso A: con argumento → vincular directamente

Si "$ARGUMENTS" NO está vacío (y no es `--update`, ya cubierto arriba), trátalo como texto de búsqueda (nombre completo o parcial del proyecto de Odoo, sin acentos ni mayúsculas exactas) y vincula la sesión directamente. Este caso es SOLO vincular, no diagnosticar: no compruebes ni menciones el token, el hook, el binario ni nada del Caso B, y no expliques el proceso (qué comando has usado, qué has comprobado, etc.) — eso es ruido para el usuario. La respuesta debe ser corta, 1-2 líneas.

1. Determina la raíz del proyecto actual (raíz del repo git, o el directorio de trabajo si no es un repo), en silencio.
2. Busca con `planesgo-mcp --search-project "$ARGUMENTS"` (o la herramienta MCP `planesgo_search_projects` con `{"query": "$ARGUMENTS"}`), en silencio.
3. Según el resultado, responde ÚNICAMENTE con una de estas tres salidas (nada más):
   - **Una sola coincidencia**: vincúlala directamente sin pedir confirmación adicional, con `planesgo-mcp --set-project "<nombre exacto>" --path "<raíz>"` (o la herramienta MCP `planesgo_set_project` con `{"project_name": "<nombre exacto>", "project_path": "<raíz>"}`), y responde solo: "✅ Vinculado a **\<nombre\>** (ID \<id\>)."
   - **Varias coincidencias**: no vincules nada, responde solo con la lista numerada (nombre e ID) y "¿cuál de estos es?".
   - **Ninguna coincidencia**: responde solo "❌ Ningún proyecto de Odoo contiene '$ARGUMENTS'. Prueba con otro texto."

## Caso B: sin argumento → diagnóstico completo

Si "$ARGUMENTS" está vacío, haz un diagnóstico completo del sistema de tracking de tiempo de PlanesGo (Claude Code → Odoo) para el proyecto de esta sesión, y repórtalo al usuario con un ✅/❌ claro por cada punto:

1. **Proyecto vinculado**: determina la raíz del proyecto actual (raíz del repo git, o el directorio de trabajo si no es un repo). Comprueba si existe `.planesgo.json` en esa raíz con `odoo_project_id > 0`. Si existe, muestra el nombre y el ID del proyecto de Odoo al que está vinculado. Si no, dilo claramente.

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
  `curl -fsSL https://raw.githubusercontent.com/planesnet/PlanesGo/master/scripts/install-claude-hook.sh | bash`
  y, si el servidor MCP sigue sin aparecer tras eso, que entienda que necesita abrir una sesión nueva para que Claude Code cargue el servidor recién registrado (no se recarga en caliente en la sesión actual).

Termina con un resumen de una frase: si el tracking está operativo para esta sesión, o qué es exactamente lo único que falta.
