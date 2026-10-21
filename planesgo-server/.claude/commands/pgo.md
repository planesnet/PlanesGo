---
description: "Imputación en Odoo — estado, activar en los agentes (imputar on|off), buscar y vincular el proyecto (pgo <texto>), añadir minutos (+N [tarea]), actualizar (--update) o PSF (psf on|off|estado)"
argument-hint: "[imputar on|off | texto de búsqueda | +N minutos [tarea] | --update | psf on|off|estado]"
---

Argumento recibido: "$ARGUMENTS". Responde en español y de forma breve. La imputación de tiempo es automática (pgocode o, fuera de pgocode, los hooks): este comando solo hace lo que se pide aquí.

Herramientas: las del servidor MCP `pgo` (`pgo_project` y `pgo_time`); si no están disponibles, la línea de órdenes `pgo-mcp`.
Si `~/.planesgo/gestor` contiene `pgocode`, la imputación la lleva pgocode: dilo y, salvo `psf` o `--update`, no hagas nada más.

## Sin argumento: estado
Indica, sin llamar a ninguna herramienta: si la imputación la lleva pgocode (`~/.planesgo/gestor`) o los hooks; y el proyecto vinculado (`odoo_project_name` de `.planesgo.json` en la raíz del repositorio) o que no hay ninguno.

## `imputar on|off`: imputación de los agentes fuera de pgocode
Es un interruptor global (`~/.planesgo/gestor`) que vale al momento, también para las sesiones ya abiertas. Hazlo con la herramienta de órdenes, sin preguntar:
- `imputar on`: `mkdir -p ~/.planesgo && echo agentes > ~/.planesgo/gestor`. Di que desde ahora los agentes usados fuera de pgocode (como esta sesión) imputan solos, y en qué proyecto (el `odoo_project_name` de `.planesgo.json` en la raíz del repositorio; si no hay, sugiere `/pgo <texto>` para vincularlo).
- `imputar off`: `echo pgocode > ~/.planesgo/gestor` y después `python3 ~/.claude/hooks/planesgo_claude_hook.py cierra-partes` (cierra e imputa los partes abiertos). Di que la imputación queda solo en pgocode.
- `imputar` sin más: lee el fichero y di si está activada en los agentes (`agentes`) o solo en pgocode (`pgocode`).

## `+N [tarea]`: añadir minutos
Añade N **minutos** al proyecto vinculado con `pgo_time` (`minutes` = N y, si se da, `task`; si la tarea no existe, se crea). Alternativa: `pgo-mcp --time N [--task "<tarea>"]`. Si no hay proyecto vinculado, haz antes la búsqueda.

## `--update`: actualizar
Ejecuta `pgo-mcp --self-update` y di la versión resultante (`pgo-mcp --version`).

## `psf on|off|estado`: Planes Software Factory en este proyecto
No tiene que ver con la imputación. Localiza PSF (`$PSF_DIR`, `~/.planesgo/PSF` o la ruta del plugin `psf@psf`) y sigue el apartado correspondiente de `<PSF>/commands/psf.md`. Nunca hagas commit ni push sin que se pida.

## Cualquier otro texto: buscar y vincular el proyecto
1. Busca con `pgo_project` (`query` = el texto; alternativa: `pgo-mcp --project "<texto>"`).
2. Muestra las coincidencias numeradas y **pregunta al usuario cuál es el definitivo**. No elijas tú, aunque solo haya una.
3. Con su respuesta, vincula con `pgo_project` (`project_id` y `project_path` = raíz del repositorio; alternativa: `pgo-mcp --project-id <id> --link --path "<raíz>"`) y muestra el nombre del proyecto vinculado.
