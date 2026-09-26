# Tracking de tiempo de codificación con Claude Code → PlanesGo/Odoo

Estos hooks hacen en Claude Code lo mismo que ya hace Antigravity: imputan en
Odoo (vía `cmd/planesgo-mcp`) el tiempo real dedicado a codificar con el
agente, usando la tarea canónica **"Claude Code - Sesión IA"** (tipo
`Desarrollo`) del proyecto vinculado en `.planesgo.json`.

## Qué dispara cada hook (`.claude/settings.json`)

| Hook | Evento | Acción |
| :--- | :--- | :--- |
| `SessionStart` | Al iniciar/reanudar una sesión | `planesgo-track.sh check` — verificación best-effort de conexión (Fase 0), no bloquea nunca |
| `PostToolUse` (`Edit\|Write\|NotebookEdit\|Bash`) | Tras cada uso de una herramienta de codificación | `planesgo-track.sh beat` — inicia el cronómetro en el primer uso y lo mantiene vivo respetando `heartbeat_interval_seconds` de `.planesgo.json` |
| `SessionEnd` | Al terminar la sesión | `planesgo-track.sh stop` — cierra e imputa las horas en Odoo, solo si realmente se inició un cronómetro |

Se excluyen a propósito herramientas de solo lectura (`Read`, `Grep`,
`Glob`, `WebFetch`...) porque el objetivo es medir **trabajo de
codificación**, no exploración.

## Requisitos (por desarrollador)

1. Compilar el binario una vez: `./scripts/build_mcp.sh` (lo instala en
   `bin/planesgo-mcp` y, si existe, en `~/.local/bin/planesgo-mcp`).
2. Tener un token de Antigravity de PlanesGo, generado desde `/settings` en
   la propia PlanesGo, y guardarlo en `~/.planesgo_auth.json`:
   ```json
   { "antigravity_token": "plg_sec_...", "planesgo_url": "https://planesgo.autopyme.com" }
   ```
   (o exportar `ANTIGRAVITY_TOKEN` en el entorno).
3. `.planesgo.json` en la raíz del repo ya vincula el proyecto Odoo
   (`odoo_project_id`); no hace falta tocarlo.

## Garantía de no interferencia

`planesgo-track.sh` **siempre termina en código 0** y nunca escribe en
stdout: si el binario no está compilado o falta el token, el hook es un
no-op silencioso y Claude Code sigue funcionando exactamente igual que sin
él. Solo el primer `beat` de la sesión se ejecuta en primer plano (con el
timeout de red de 15s del propio binario) para confirmar que el
temporizador quedó abierto en Odoo antes de programar el `stop` final;
el resto de llamadas van en segundo plano y no añaden latencia perceptible.

En Windows, estos hooks requieren un shell bash (Git Bash/WSL); no hay
variante en PowerShell.
