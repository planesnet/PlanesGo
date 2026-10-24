# Banco de consumo de tokens

Mide cuántos tokens gasta cada agente (Claude Code, Antigravity, OpenCode) en 7 tareas fijas, **sin pgocode**
(el CLI del agente a pelo: es la calibración) y **con pgocode** (con su capa de agentes, `internal/llm`, como los
lanza la app), para comparar y para ver si una mejora de pgocode ahorra de verdad.

No depende del ERP: ni Odoo (tampoco el simulado), ni login, ni proyectos.

```sh
make consumo                                        # todo: 3 agentes × 7 pruebas × 3 repeticiones × 2 modos
make consumo ARGS="-pruebas 1,2,3 -repeticiones 1"  # ronda rápida (menos de 1 $)
make consumo ARGS="-modo pgocode"                   # solo con pgocode, frente a la última calibración
make consumo ARGS="-agentes claude-code"            # un solo agente
```

Gasta tokens de verdad: no entra en `make test`.

## Qué hay aquí

- `pruebas.json`: los agentes (modelo y esfuerzo) y las 7 pruebas, con sus turnos y cómo se comprueba cada uno
  (expresión regular de la respuesta, orden que ha de acabar bien, comprobación en Go, ficheros que no se pueden
  tocar, un patrón que ha de aparecer al menos N veces…).
- `proyecto/`: el proyecto de prueba, un módulo Go propio (`facturacion-demo`): una librería de facturación con un
  fallo en el descuento de las líneas. Cada ejecución trabaja en una copia propia, como repositorio git nuevo.
- `proyecto-grande/`: el de la prueba 7 (`"proyecto"` en `pruebas.json`), otro módulo Go (`almacen-demo`): la
  gestión de un almacén con seis ficheros grandes (unos 85k tokens entre todos; catálogo, clientes, tarifas, zonas…)
  y un fallo en el descuento por volumen. Los datos son inventados; se guarda tal cual para que no cambie entre rondas.
- `comprobaciones/`: los tests de Go que comprueban un turno (`"comprobacion"` en `pruebas.json`): `cmd/consumo`
  copia el fichero a la copia de trabajo (`comprobacion_consumo/`), lo ejecuta con `go test` y lo borra, así que
  el agente no lo ve. Tiene su propio `go.mod`, como los dos proyectos: ninguno entra en el `go test ./...` de
  planesgo-code. `go test ./cmd/consumo` comprueba con un arreglo de referencia que cada orden y cada comprobación
  fallan antes del cambio y pasan después.
- `calibracion.json`: las medianas sin pgocode de la última ronda completa (la escribe `cmd/consumo`).

**Calibración pendiente.** El 24-10-2026 se rehicieron los proyectos de prueba y se borraron la calibración y los
resultados anteriores (siguen en el historial de git), porque medían otras tareas.
Antes de comparar con pgocode hay que repetir la calibración con una ronda completa sin pgocode
(`make consumo ARGS="-modo directo"`); sin ella, `-modo pgocode` sale sin referencia.

| # | Prueba | Permisos | Qué mide |
|---|---|---|---|
| 1 | Saludo | lectura | Coste fijo de cada llamada (instrucciones, herramientas, MCP, skills) |
| 2 | Lectura | lectura | Una herramienta y una respuesta corta |
| 3 | Búsqueda | lectura | Si busca con precisión o lee de más |
| 4 | Arreglo | auto | Editar, ejecutar las pruebas y corregir |
| 5 | Conversación | auto | Tres turnos: crecimiento del contexto y caché |
| 6 | Ida y vuelta | auto | Diez turnos cortos que dependen de los anteriores (arreglo, prueba, redondeo, renombrado, parámetro, preguntas): cuánto crece el contexto en un ir y venir |
| 7 | Conversación larga | auto | Ocho turnos en `proyecto-grande` que obligan a leer módulos enteros (el contexto pasa de 100k, por encima del umbral real de compactación) y los dos últimos preguntan por cosas del principio: si compactar o relevar ahorra en conversaciones grandes y si se pierde memoria |

## Resultados

En el repositorio, para seguir la evolución: `consumo/resultados/<fecha>.jsonl` (cada ejecución, según acaba,
con la medida de cada turno), `<fecha>.md` (el informe: medianas por agente y prueba, con y sin pgocode, y la
diferencia) y `evolucion.md` (todas las rondas). En las pruebas de varios turnos, el informe añade la evolución
turno a turno: el contexto (tokens de la última llamada, lo que se reenvía), la entrada y el coste de cada uno.
Las copias de trabajo van a `~/.cache/pgocode-consumo/<fecha>/` y se borran al acabar (`-conservar` las deja).

## Estrategias

`-estrategia` dice cómo pasa la conversación de un turno al siguiente; se guarda en cada ejecución y la evolución
separa las rondas por estrategia. En el informe, la tabla turno a turno marca entre corchetes lo que ha hecho la
estrategia en cada turno (relevo, compactación).

- `ninguna` (por defecto): reanudar la sesión del agente, que reenvía todo el historial, sin compactar: con pgocode,
  el umbral de compactación va al máximo (1M; la app compacta por defecto a partir de 150k).
- `compactar[:N]`: la compactación gobernada por pgocode (`internal/llm/compaction.go`): cada turno lleva
  `CompactAt` = N miles de tokens (por defecto 20k, el mínimo, para que salte en una prueba corta) y `LastContext`
  (el contexto de la última llamada del turno anterior). Solo con `-modo pgocode`.
- `relevo[:N]`: el relevo con memoria de trabajo de la interfaz: si el contexto de la última llamada del turno
  anterior pasa de N miles (por defecto 20k), el turno va a una sesión nueva del agente con la memoria de trabajo
  delante del mensaje (sin la compactación de pgocode). La memoria y la decisión son las de `web/src/workingMemory.ts` (`buildWorkingMemory`,
  `shouldRelay`, `relayMessage`): `cmd/consumo/relay.mts` las llama con node sobre la conversación del banco en el
  formato `Mensaje` de la interfaz (texto, herramientas y tareas de cada respuesta). Necesita node; solo con
  `-modo pgocode`.
- `poda`: la poda automática del historial de pgocode (`internal/llm/prune.go`, de momento solo OpenCode): al empezar
  cada turno retira del historial del agente lo superado (lecturas releídas o editadas, imágenes vistas, órdenes
  repetidas, salidas largas, el texto de las ediciones y el razonamiento antiguo) y lo que no tiene relación con el
  mensaje nuevo (embeddings locales con Ollama), sin la compactación de pgocode. `poda-reglas`: lo mismo, solo con
  las reglas. La nota del turno dice cuánto ha retirado («poda 24.4k»). Solo con `-modo pgocode`.

```sh
make consumo ARGS="-agentes claude-code -modo pgocode -pruebas 6 -repeticiones 1 -estrategia relevo"
make consumo ARGS="-agentes claude-code -modo pgocode -pruebas 6 -repeticiones 1 -estrategia compactar:30"
make consumo ARGS="-agentes claude-code -modo pgocode -pruebas 7 -repeticiones 1 -estrategia compactar:80"
make consumo ARGS="-agentes opencode -modo pgocode -pruebas 7 -repeticiones 1 -estrategia poda"
```

Para añadir otra, en `cmd/consumo/main.go` se implementa `strategy` (su `before` puede cambiar la sesión y el
mensaje del turno, pedir la compactación y lanzar turnos propios, cuyo consumo cuenta) y se registra en
`strategies`.

## Cómo se lanza cada agente

- **Sin pgocode**: el CLI con lo mínimo para el modelo, el esfuerzo, los permisos de la prueba y la salida en JSON
  (OpenCode, además, sus permisos y Vertex por entorno: sin ellos no funciona en modo `run`).
- **Con pgocode**: el proveedor de `internal/llm` con la petición que arma el servidor para la interfaz: sus
  argumentos, los MCP que filtra, el idioma, su entorno y la entrada por stdin de Claude Code.

En los dos, el entorno del usuario sin las variables de una sesión de Claude Code ni de pgocode, y con
`PLANESGO_GESTOR=pgocode` (los hooks de PlanesGo se duermen: no imputan nada).
