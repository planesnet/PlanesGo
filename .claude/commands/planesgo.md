---
description: Revisa el estado del tracking de tiempo de PlanesGo (proyecto vinculado, token, hook) en esta sesión
---

Haz un diagnóstico completo del sistema de tracking de tiempo de PlanesGo (Claude Code → Odoo) para el proyecto de esta sesión, y repórtalo al usuario con un ✅/❌ claro por cada punto:

1. **Proyecto vinculado**: determina la raíz del proyecto actual (raíz del repo git, o el directorio de trabajo si no es un repo). Comprueba si existe `.planesgo.json` en esa raíz con `odoo_project_id > 0`. Si existe, muestra el nombre y el ID del proyecto de Odoo al que está vinculado. Si no, dilo claramente.

2. **Token de autenticación**: comprueba si hay `$PLANESGO_TOKEN` o `$ANTIGRAVITY_TOKEN` en el entorno, o si existe `~/.planesgo_auth.json` con un token válido. No muestres el valor del token, solo si está presente.
   - IMPORTANTE: si el proyecto (punto 1) NO está vinculado, el guard de PlanesGo bloquea cualquier comando Bash compuesto (con `|`, `&&`, `||`, `;`, `>`, `<`) aunque sea inofensivo — es intencional, evita que se cuelen comandos de desarrollo disfrazados. Usa exclusivamente comandos sueltos y simples, uno por uno, sin combinarlos: `printenv PLANESGO_TOKEN`, `printenv ANTIGRAVITY_TOKEN`, `ls ~/.planesgo_auth.json`. Esos SÍ están permitidos siempre, esté o no vinculado el proyecto. Si aun así uno de ellos es denegado, no lo reintentes de otra forma ni concluyas que "todo Bash está bloqueado": simplemente repórtalo como ❓ y continúa con el resto del diagnóstico.

3. **Hook global instalado**: comprueba si existe `~/.claude/hooks/planesgo_claude_hook.py` y si `~/.claude/settings.json` tiene los hooks de PlanesGo conectados (session-start, prompt, guard, mcp, track, session-end apuntando a ese fichero). Si este proyecto además trae su propia copia en `.claude/hooks/` y `.claude/settings.json`, indícalo también (es normal que coexistan, el propio proyecto tiene prioridad).

4. **Binario disponible**: comprueba si `planesgo-mcp` está en el PATH o en `~/.local/bin/planesgo-mcp`.

5. **Conexión real**: si hay binario y el proyecto está vinculado, ejecuta `planesgo-mcp --check` y muestra el resultado tal cual.

Según lo que encuentres:
- Si el proyecto **no está vinculado**: pregunta al usuario a qué proyecto de Odoo corresponde (nunca lo asumas ni uses PLANESGO por defecto). Si no conoce el nombre exacto, búscalo con `planesgo-mcp --search-project "<texto parcial>"` (o la herramienta MCP `planesgo_search_projects`), muéstrale las coincidencias, y una vez confirmado vincula con `planesgo-mcp --set-project "<nombre exacto>"`.
- Si **falta el token**: indica al usuario que lo genere en https://planesgo.autopyme.com/settings y lo guarde en `~/.planesgo_auth.json`, o lo exporte como `$PLANESGO_TOKEN`.
- Si **falta el hook global o el binario**: sugiere ejecutar
  `curl -fsSL https://raw.githubusercontent.com/planesnet/PlanesGo/master/scripts/install-claude-hook.sh | bash`

Termina con un resumen de una frase: si el tracking está operativo para esta sesión, o qué es exactamente lo único que falta.
