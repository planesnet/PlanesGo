# Plan: subagentes, tareas en segundo plano y listas de tareas en pgocode

## Contexto

Los tres agentes que lanza pgocode tienen procesos internos que hoy no se ven o se ven mal. Están probados con llamadas reales a Claude Code 2.1.305, OpenCode 1.19.3 y Antigravity (agy) 1.4.3.

| | Claude Code | OpenCode | Antigravity |
|---|---|---|---|
| Subagentes | `Agent`/`Task`, en primer o segundo plano | `task` (segundo plano solo experimental) | `invoke_subagent` |
| Comandos en segundo plano | `Bash run_in_background`, `Monitor` | no hay (`cmd &` bloquea) | `run_command` asíncrono, `manage_task` |
| Lista de tareas | `TaskCreate/TaskUpdate` (con `CLAUDE_CODE_ENABLE_TODO_TOOLS=1`) | `todowrite` | `task_list.md` del «brain» |
| Eventos en vivo | `system/task_started · task_progress · task_updated · task_notification · background_tasks_changed`; mensajes del subagente con `parent_tool_use_id` | solo al terminar (`tool_use` completado) | `step_update` ACTIVE/DONE con `step_type: subagent` |
| Parar una tarea concreta | sí, por stdin (`control_request stop_task`), con `--input-format stream-json` | sí, con `opencode serve` (`/session/{id}/abort`) | no: solo el proceso entero |
| Detalle en disco | `~/.claude/projects/…/<sesion>/subagents/agent-<id>.jsonl`, `/tmp/claude-<uid>/…/tasks/<id>.output` | base de datos (`session.parent_id`, `todo`), `opencode export` | `~/.gemini/antigravity-cli/brain/<conv>/.system_generated/{tasks,subagents,messages,logs}` |

### Lo que pgocode pierde hoy

**Claude**
- Descarta los mensajes de los subagentes y los eventos `task_*`.
- Un `Agent` en segundo plano sale como «ok» al instante.
- Suma `total_cost_usd`, que ya es acumulado: el coste sale de más cuando hay varios `result`.

**Antigravity**
- Un `invoke_subagent` se queda «en curso» para siempre: el ACTIVE llega con `step_type: tool` y el DONE con `subagent`.
- Ignora la línea `root agent idle; waiting … background task(s)`.

**OpenCode**
- `task` y `todowrite` salen como herramientas sueltas, sin enlace a la subsesión hija ni a la lista.

**Procesos**
- Al cancelar, el SIGKILL al grupo de Claude deja huérfanos sus Bash en segundo plano, que tienen su propio grupo.
- El bloqueo de actualizaciones tampoco los ve.
- El chat se queda «ocupado» mientras el agente espera a sus tareas, sin explicar por qué.

## Concepto unificado: «Tarea»

Todo lo que un agente hace aparte de la conversación principal es una **Tarea**. Se emite como `Evento{Tarea: …}`, con actualizaciones parciales por `ID`, igual que hoy `Herramienta`:

```go
type Tarea struct {
    ID            string // id propio del agente: task_id de Claude, callID/ses_ de OpenCode, "<conv>/task-N" de Antigravity
    Tipo          string // subagente | segundo_plano | lista
    Padre         string // Tarea padre ("" = conversación principal): subagentes que lanzan comandos
    HerramientaID string // la herramienta que la creó (enlaza con la tarjeta de la herramienta)
    Titulo        string // descripción / prompt / orden
    Subtipo       string // general-purpose, explore, research…
    Estado        string // curso | ok | error | parada
    SegundoPlano  bool
    Progreso      string // última herramienta, tokens, duración
    Salida        string // resumen final (recortado)
    Detalle       string // fichero con el detalle completo (salida, transcript)
    SesionHija    string // subagente: agent_id / ses_… / conversation_id
    Elementos     []Elemento // Tipo=lista: {Texto, Estado pendiente|curso|hecho}
    Acciones      []string   // ver_detalle, parar (según lo que permita cada agente)
    Inicio, Fin   time.Time
}
```

Los mensajes internos de un subagente (sus herramientas y su texto) van **dentro de su Tarea**, no en la conversación principal.

## Interfaz

1. **En la respuesta**: cada subagente es un bloque plegable con su tipo, título, estado, duración y tokens. Dentro van sus herramientas, como las de la conversación principal. Un comando en segundo plano es una fila con su orden, su estado y «Ver salida».
2. **Barra de tareas sobre el cuadro de texto** mientras haya algo vivo: «⚙ 2 en segundo plano: Sleep 20 · subagente explore». Explica por qué el agente sigue ocupado. Cada elemento tiene «Ver» y, si el agente lo permite, «Parar».
3. **Lista de tareas** del agente (TaskCreate/todowrite/task_list.md) como una lista con casillas, fija arriba de la respuesta en curso y actualizada en vivo.
4. **Visor de detalle**: un panel lateral (el de artefactos) con la salida completa o el transcript del subagente, que se refresca mientras la Tarea siga viva.

## Fases

### Fase 0 — Errores independientes (pequeño, se puede hacer ya)
- Coste de Claude: tomar el último `total_cost_usd`, no sumarlos.
- Antigravity: cerrar el paso `subagent` aunque el ACTIVE llegara como `tool`; ignorar la línea que no es JSON.
- Cancelación limpia de Claude: SIGTERM al proceso, para que cierre sus tareas y emita `stopped`, y SIGKILL al grupo tras 5 s.
- Vigilar también los grupos de procesos hijos (por `/proc`), para el bloqueo de actualizaciones y para no dejar huérfanos.

### Fase 1 — Ver (solo lectura de eventos)
- `llm.Tarea` y `Evento.Tarea`; lectura en cada agente:
  - **Claude**: `system/task_*`, `background_tasks_changed`; los mensajes con `parent_tool_use_id` se anidan en su Tarea.
  - **Antigravity**: pasos `subagent` (con `subagent_info`), `run_command` asíncrono y `manage_task`.
  - **OpenCode**: `task` (con `metadata.sessionId`) y `todowrite` (con `metadata.todos`).
- Interfaz: bloques de subagente, barra de tareas y lista de tareas.
- Pruebas: los lectores, con las muestras reales grabadas como datos de prueba (`internal/llm/testdata/`).

### Fase 2 — Detalle
- `GET /api/tareas/detalle?sesion&tarea` lee, según el agente:
  - el `.output` o el transcript del subagente (Claude);
  - `tasks/task-N.log` o el transcript del brain hijo (Antigravity);
  - `opencode export <ses>` de la subsesión (OpenCode).
- Visor en el panel lateral, con recarga mientras la Tarea siga viva.

### Fase 3 — Gestionar
- **Claude**: lanzar con `--input-format stream-json` y mantener stdin abierto. Eso permite:
  - `stop_task` (parar una tarea concreta);
  - `get_task_output`;
  - `interrupt` (cortar el turno sin matar el proceso, más limpio que hoy).
- **OpenCode**: cambiar a `opencode serve` + `run --attach`, que da `/session/{id}/abort` por subsesión y eventos SSE en vivo de las hijas. Es más trabajo; se puede dejar para el final.
- **Antigravity**: solo «Parar todo», que mata el proceso.
- La barra muestra «Parar» solo en las Tareas cuyo agente lo permite.

### Fase 4 — Listas de tareas en Claude
- Activar `CLAUDE_CODE_ENABLE_TODO_TOOLS=1` en las sesiones de pgocode, para que Claude lleve su lista como en modo interactivo, y leer `TaskCreate/TaskUpdate`.

## Decisiones (Luis, 2026-10-21)
1. **La cola la lleva pgocode, igual para todos los agentes.** No se usa el stdin de Claude para meter mensajes durante el turno.
2. **Prioridad: no crear procesos sin control ni gastar tokens de más.** Por eso:
   - **sin `opencode serve`:** sería otro proceso local vivo por sesión;
   - **sin listas de tareas de Claude:** gastan tokens en cada turno;
   - **fase 3 solo si hace falta:** parar una tarea concreta en Claude exige mantener su stdin abierto.
3. **Fase 0 hecha:**
   - el coste de Claude ya no se cuenta dos veces;
   - los pasos de Antigravity no se quedan colgados;
   - cada ejecución de un agente lleva la marca `PGO_EJECUCION` en el entorno, que heredan todos sus descendientes. Al cancelar se manda SIGTERM al agente y, tras 5 s, SIGKILL a todo lo marcado. El bloqueo de actualizaciones también cuenta esos procesos.

Pendiente de valorar para ahorrar tokens: con tareas vivas, `claude -p` hace **otro turno completo** cuando cada una termina, y eso son tokens. Se podría avisar en la interfaz o limitar el segundo plano en las sesiones de pgocode.

## Verificación
- Pruebas de los lectores con las muestras reales de los tres agentes.
- Prueba manual en pgocode con cada agente:
  - un subagente;
  - un comando en segundo plano de 20 s;
  - parar la tarea desde la barra;
  - comprobar con `ps` que no quedan procesos huérfanos.
