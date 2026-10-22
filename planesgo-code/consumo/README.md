# Banco de consumo de tokens

Mide cuántos tokens gasta cada agente (Claude Code, Antigravity, OpenCode) en 5 tareas fijas, **sin pgocode**
(el CLI del agente a pelo: es la calibración) y **con pgocode** (con su capa de agentes, `internal/llm`, como los
lanza la app), para comparar y para ver si una mejora de pgocode ahorra de verdad.

No depende del ERP: ni Odoo (tampoco el simulado), ni login, ni proyectos.

```sh
make consumo                                        # todo: 3 agentes × 5 pruebas × 3 repeticiones × 2 modos
make consumo ARGS="-pruebas 1,2,3 -repeticiones 1"  # ronda rápida (menos de 1 $)
make consumo ARGS="-modo pgocode"                   # solo con pgocode, frente a la última calibración
make consumo ARGS="-agentes claude-code"            # un solo agente
```

Gasta tokens de verdad: no entra en `make test`.

## Qué hay aquí

- `pruebas.json`: los agentes (modelo y esfuerzo) y las 5 pruebas, con sus turnos y cómo se comprueba cada uno
  (expresión regular de la respuesta, orden que ha de acabar bien, ficheros que no se pueden tocar…).
- `proyecto/`: el proyecto de prueba (una librería de facturación en Python con un fallo en el descuento de las
  líneas). Cada ejecución trabaja en una copia propia, como repositorio git nuevo.
- `calibracion.json`: las medianas sin pgocode de la última ronda completa (la escribe `cmd/consumo`).

| # | Prueba | Permisos | Qué mide |
|---|---|---|---|
| 1 | Saludo | lectura | Coste fijo de cada llamada (instrucciones, herramientas, MCP, skills) |
| 2 | Lectura | lectura | Una herramienta y una respuesta corta |
| 3 | Búsqueda | lectura | Si busca con precisión o lee de más |
| 4 | Arreglo | auto | Editar, ejecutar las pruebas y corregir |
| 5 | Conversación | auto | Tres turnos: crecimiento del contexto y caché |

## Resultados

En `~/.local/share/pgocode/consumo/`: `<fecha>.jsonl` (cada ejecución, según acaba) y `<fecha>.md` (el informe:
medianas por agente y prueba, con y sin pgocode, y la diferencia). Las copias de trabajo van a
`~/.cache/pgocode-consumo/<fecha>/` y se borran al acabar (`-conservar` las deja).

## Cómo se lanza cada agente

- **Sin pgocode**: el CLI con lo mínimo para el modelo, el esfuerzo, los permisos de la prueba y la salida en JSON
  (OpenCode, además, sus permisos y Vertex por entorno: sin ellos no funciona en modo `run`).
- **Con pgocode**: el proveedor de `internal/llm` con la petición que arma el servidor para la interfaz: sus
  argumentos, los MCP que filtra, el idioma, su entorno y la entrada por stdin de Claude Code.

En los dos, el entorno del usuario sin las variables de una sesión de Claude Code ni de pgocode, y con
`PLANESGO_GESTOR=pgocode` (los hooks de PlanesGo se duermen: no imputan nada).
