# Lo fijo de cada llamada al modelo: herramientas, skills y subagentes

Cada llamada al modelo (cada paso de un turno) reenvía el prompt de sistema y las definiciones de todas las
herramientas, con la lista de skills (nombre y descripción de cada una) y las herramientas de los servidores MCP.
Eso es el **coste fijo**: lo que cuesta un «Responde solo: OK» en una sesión nueva. En una conversación se paga en
cada paso (en caché, más barato, pero también cuenta para el contexto).

Medido el 23-10-2026 en el equipo de Luis (Claude Code 2.1.309, OpenCode 1.19.3, agy 1.4.6), con sus skills,
plugins y servidores MCP instalados, en una carpeta sin Odoo (un repositorio git con `.planesgo.json`). Modelos
baratos: haiku, gemini-3.1-flash-lite y gemini-3.8-flash-low (el coste fijo apenas depende del modelo). Tokens
de entrada de la única llamada.

## Resumen

| Agente | Sin pgocode | pgocode antes | pgocode ahora | Ahorro |
|---|--:|--:|--:|--:|
| Claude Code (solo lectura) | 24.568 | 14.935 | 10.338 | −4,6k (−31 %) |
| Claude Code (auto, con Edit y Write) | — | 15.519 | 11.225 | −4,3k (−28 %) |
| OpenCode (auto) | — | 15.810 | 10.840 | −5,0k (−31 %) |
| OpenCode (auto) en un proyecto de Odoo | — | 15.810 | 14.383 | −1,4k (−9 %) |
| Antigravity (plan) | 18.406 | 18.406 | 17.606 | −0,8k (−4 %) |

Comprobado también en la app (backend en 127.0.0.1:8063, Odoo simulado en 8163, Claude Code con haiku, solo
lectura): el «OK» de una sesión nueva costó 10.634 tokens de entrada, y la orden lanzada lleva `--settings`.

## Claude Code

Ya estaba (antes de esta ronda): perfil de herramientas propias (`herramientasClaude`, 12 o 14 en vez de 32),
solo los servidores MCP de la sesión (`--strict-mcp-config`: sin él entran también los conectores de claude.ai y
los MCP del usuario, 110 herramientas) y la carga bajo demanda (`ToolSearch`): las definiciones de las herramientas
diferidas (las de los MCP, TaskCreate…, WebFetch, WebSearch) no van hasta que el agente las busca.

| Variante (solo lectura, MCP «pgo») | Tokens |
|---|--:|
| Sin nada (todas las herramientas y MCP) | 24.568 |
| Solo `--tools` del perfil (con todos los MCP) | 18.475 |
| Perfil + solo «pgo» (lo de antes) | 14.935 |
| … sin ToolSearch (todo cargado de entrada) | 19.059 (+4,1k: ToolSearch ahorra) |
| … `ENABLE_TOOL_SEARCH=true` | 14.935 (ya está activo por defecto) |
| … sin skills (`--disable-slash-commands`, quita también Skill) | 9.380 |
| … `CLAUDE_CODE_DISABLE_BUNDLED_SKILLS=1` | 12.794 |
| … `includeGitInstructions: false` | 14.370 (−565: no compensa quitarle cómo hacer commits) |
| … skillOverrides «off» para las inútiles | 13.155 |
| … skillOverrides «off» + «name-only» (**lo nuevo**) | 10.338 |
| Solo Bash | 5.986 |
| Sin herramientas | 3.571 |

Lo que más pesaba era la **lista de skills** (unas 30, 5,5k tokens): las incluidas en Claude Code y las del plugin
de la organización. Ahora pgocode pasa `--settings '{"skillOverrides":{…}}'` (`internal/llm/fixed.go`, fijo: no
cambia entre turnos ni rompe la caché):

- `off` (ni se listan): las que no sirven sin nadie delante o sin los conectores de claude.ai, y las que hace
  pgocode: design, design-sync, update-config, fewer-permission-prompts, doctor, loop, schedule, workflow-authoring,
  run-skill-generator, batch, plugin-authoring, keybindings-help, morning, import-memory, docs, google-workspace y
  pgo-imputacion (antes solo estaba denegada: seguía en la lista).
- `name-only` (solo el nombre; el cuerpo se carga al usarla, como siempre): deep-research, dataviz, verify, debug,
  code-review, simplify, claude-api, run, docx, pdf, pptx, xlsx y skill-creator. Probado con haiku: «Crea un
  documento Word informe.docx…» sigue usando `anthropic-skills:docx` (2 de 2).
- Las skills personales y las del proyecto no se tocan (las pone el usuario a propósito), ni cfo-odoo-esp.

No se cambia: `--exclude-dynamic-system-prompt-sections` (pasa la carpeta, git, etc. del sistema al primer
mensaje, para compartir caché entre sesiones) no reduce tokens y, medido en otra carpeta, la lectura de caché de la
primera llamada no mejoró de forma clara (7,4k frente a 5,8k, con otras diferencias de entorno): es cosa de la
parte de caché. Quitar el tipo de subagente statusline-setup (`--disallowedTools`) no lo quita de la lista.

## OpenCode

| Variante (auto) | Tokens |
|---|--:|
| Lo de antes | 15.810 |
| Sin la herramienta skill | 7.917 (la lista de 57 skills cuesta 7,9k) |
| Sin task | 15.047 (−763) |
| Sin todowrite | 15.265 (−545) |
| Sin webfetch / glob | −237 / −196 |
| Sin el MCP «pgo» | 15.570 (−240: se queda, la imputación lo usa) |
| Denegadas 8 skills inútiles | 14.383 |
| … y las de Odoo fuera de un proyecto de Odoo (**lo nuevo**) | 10.840 |

OpenCode lee skills de `~/.claude/skills` (también las sincronizadas de Claude) y de `~/.agents/skills` (45, casi
todas de Odoo). Una skill denegada en los permisos (`OPENCODE_PERMISSION`, `skill`) no le llega al modelo (las
herramientas con permisos parciales sí siguen en la lista; solo desaparece la que se deniega entera). pgocode
deniega ahora (`permisosOpenCode` con la carpeta, `openCodeHiddenSkills`):

- las de Claude que necesitan sus conectores o su app (morning, import-memory, docs, google-workspace) y las de
  Antigravity (antigravity-agent-manager, antigravity-skill-orchestrator, antigravity-workflows);
- `odoo-*` si la carpeta no es de Odoo (`isOdooProject`: un `__manifest__.py` u `odoo-bin` en ella, hasta 3
  niveles por debajo o 2 por encima), salvo odoo-rpc-api (sirve para hablar con Odoo desde fuera). Comprobado: el
  agente sigue viendo odoo-rpc-api y no las demás.

Las herramientas que se quitan con un permiso «deny» entero (edit, bash en solo lectura) ya se quitaban.

## Antigravity

`agy` no deja elegir herramientas, skills ni MCP por sesión (sus `plugin disable` y `mcp disable` son globales y
cambiarían el Antigravity de Luis fuera de pgocode). Lo único: `--disable-slash-commands` quita la lista de
órdenes de barra (−805 tokens; las skills siguen en la lista: comprobado pidiéndole que las enumere). pgocode lo
pasa salvo si el mensaje empieza por «/» (la compactación de pgocode manda «/compact …», y el usuario puede
escribir una orden).

## Subagentes

pgocode sugiere subagentes a Claude Code cuando la petición pide explorar mucho o lista partes independientes
(`internal/llm/parallel.go`). Medido si compensa extenderlo a OpenCode (herramienta «task», subagente explore),
sobre `consumo/proyecto-grande`, con la misma frase que se le añade a Claude:

| Petición | Modelo | Sin sugerencia | Con sugerencia |
|---|---|--:|--:|
| «Busca en todo el proyecto dónde se usa el margen de seguridad…» | flash-lite | 43.755 tokens, 3 llamadas (2 grep) | 87.439, 9 llamadas (1 task) |
| «Revisa todo el código y haz un inventario de las funciones que calculan importes…» | 3.8-flash | 755.640, 0,17 $ | 1.343.368, 0,49 $ (2 subagentes) |

Con Gemini, OpenCode ya busca con precisión (grep, glob) y cada subagente arranca con su propio coste fijo
(~14k) y relee lo mismo: la sugerencia **gasta el doble**. No se extiende a OpenCode. En Antigravity no se ha
medido (`agy` tiene `--agent` para elegir el agente de la sesión, pero no una herramienta de subagentes a la vista).
