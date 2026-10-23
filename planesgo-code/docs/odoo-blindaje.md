# Blindaje del Odoo de PlanesNet

> «A través de una sesión no puede manipularse la base de datos de planesnet.autopyme.com; esta únicamente la
> manipula el sistema de medición de tiempos y tokens. Esto no es una skill: se tiene que controlar por programa.»

En el Odoo de PlanesNet (por defecto `https://planesnet.autopyme.com`, base `ap113`) solo escribe la imputación de
pgocode (`internal/imputacion`). Ningún agente lanzado por pgocode puede hacerlo: ni Claude Code, ni OpenCode, ni
Antigravity, ni sus subagentes, ni las sesiones hijas de un reparto, ni las preguntas aparte. Tampoco leer, salvo lo
que pgocode ya les da (el proyecto de la sesión en su prompt).

El host no está fijo en el código: es el de `-odoo` (o `ODOO_URL`). El servidor de PlanesGo (`-planesgo` o
`PLANESGO_URL`, por defecto `https://planesgo.autopyme.com`) también se bloquea, porque su API de imputación escribe
en ese mismo Odoo.

## Auditoría: las vías que había (versión 0.4.97)

| Vía | Qué permitía | Estado en 0.4.97 |
|---|---|---|
| **Sesión de Odoo en disco**: `~/.local/share/pgocode/imputacion-dia.json` (`odoo_sesion` en cada periodo y línea del día) | Con esa cookie `session_id`, `call_kw` a Odoo: crear o cambiar líneas de horas, tareas… con el usuario de Luis | **Abierta**: fichero 0600, pero del mismo usuario que el agente (comprobado: la sesión viva estaba en el fichero) |
| `~/.local/share/pgocode/sesiones-reinicio.json` | La sesión de Odoo y el token de la cookie de pgocode | Solo existe unos segundos al aplicar una actualización (y se borra al leerlo); con nada en marcha |
| **MCP «pgo»** (`pgo-mcp`, de PlanesGo: `pgo_project`, `pgo_time`) | Vincular proyecto y añadir minutos en Odoo vía PlanesGo | `configMCP` lo añadía **siempre** a Claude Code; OpenCode y Antigravity lo cargan de su configuración global. Dormido solo por `PLANESGO_GESTOR=pgocode` en su entorno |
| **CLI `pgo-mcp --time/--link/--beat/--stop`** | Lo mismo desde Bash | **Abierta**: la CLI no miraba `PLANESGO_GESTOR` (solo lo hacían los hooks antes de llamarla) |
| `~/.planesgo_auth.json` (token de PlanesGo) y `PLANESGO_TOKEN`/`ANTIGRAVITY_TOKEN` | `curl` a la API de PlanesGo (`/antigravity/update_tasks`), que escribe en Odoo | **Abierta** (fichero legible; las variables, si existían, se heredaban) |
| Hooks de PlanesGo (`~/.claude/hooks/planesgo_claude_hook.py`) y orden `/pgo` | Imputan llamando a `pgo-mcp`; `/pgo imputar on` cambia `~/.planesgo/gestor` | Hooks dormidos por `PLANESGO_GESTOR`; `/pgo` seguía disponible como skill |
| Entorno heredado | `ODOO_*`, `PLANESGO_TOKEN`, `PGO_TOKEN_ESCRITORIO` (secreto de la app), un `PGO_TOKEN` de otro pgocode | **Abierta**: los agentes recibían `os.Environ()` entero |
| **Servidor de pgocode** con el token de agente (`PGO_TOKEN`, `/api/pgo/*`) | `ejecutar`, `parar`, `procesos`, `salida`, `abrir`, `drive-*`, `gmail-*` | Ninguna toca Odoo. El token era por proyecto y para toda la ejecución (no caducaba) |
| Servidor de pgocode con la **cookie** de la interfaz | `/api/imputacion/unificar` (escribe en Odoo), `/api/proyecto/tareas`, `/api/imputacion/resumen`, `/api/chat`… | Un agente que leyera la cookie (almacén de cookies de Electron; con `-navegador` basta la cookie) podía usarlas |
| Red directa (`curl`, `python xmlrpc`, `WebFetch`) a planesnet.autopyme.com | Sin sesión, Odoo pide usuario y contraseña; con la sesión robada, todo | Sin ninguna regla |
| Skills: `pgo-imputacion`, orden `/pgo`, `odoo-rpc-api` | `pgo-imputacion` ya estaba desactivada (`skills.Sustituidas`); `/pgo` no; `odoo-rpc-api` es genérica (habla con el Odoo de un proyecto, no lleva credenciales de PlanesNet) | `/pgo` abierta |

## Controles (por programa, en capas)

1. **La sesión de Odoo solo en memoria.** `imputacion.Trabajo.OdooSesion` y `imputacion.Dia.OdooSesion` ya no se
   guardan (`json:"-"`). Al arrancar, lo recuperado del fichero toma la sesión de la sesión de pgocode del usuario
   (`Gestor.SesionOdoo` → `sesiones.Almacen.OdooSesionDe`): tras una actualización, la de `sesiones-reinicio.json`;
   tras un cierre inesperado no hay ninguna y lo pendiente espera, sin marcarse como error, a que el usuario vuelva a
   entrar (`ActualizaSesionOdoo`). Al escribir el estado la primera vez, la versión nueva borra la sesión que dejó en
   el fichero la anterior. `sesiones-reinicio.json` sigue (para no pedir la contraseña tras cada actualización): solo
   existe durante el reinicio, con ningún agente trabajando, y se borra al leerlo.
2. **Token de agente por sesión y turno, con lista blanca** (`internal/servidor/agenttoken.go`). Cada turno (y cada
   pregunta aparte) recibe un `PGO_TOKEN` aleatorio que dice su proyecto y su sesión; se revoca al acabar el turno y
   caduca a las 12 h. `/api/pgo/<orden>` solo atiende `agentEndpoints` (ninguna toca Odoo): lo demás, 404; con otro
   método, 403. Con la cabecera `X-PGO-Agente`, cualquier ruta fuera de `/api/pgo/` da 403.
3. **El servidor no atiende la interfaz a un proceso de agente** (`internal/servidor/peer.go`, Linux). En cada
   conexión se busca el proceso del otro extremo (`/proc/net/tcp` → inodo → `/proc/<pid>/fd`) y, si él o un
   antepasado lleva la marca `PGO_EJECUCION` que este pgocode pone a sus agentes (`<pid>-…`) y a los programas que
   lanza (`prog-<pid>-…`), solo se le permite `/api/pgo/*`: aunque tenga la cookie o el servidor esté con
   `-navegador`. Sobrevive a `env -i` (se mira la línea de antepasados). Un pgocode de desarrollo lanzado por un
   agente pone su propia marca, así que sus pruebas no se ven afectadas.
4. **Sin el MCP de imputación.** `configMCP` nunca añade «pgo» ni un servidor cuya orden sea `pgo-mcp`/`planesgo-mcp`
   (aunque la sesión lo pida); a OpenCode se le desactiva en `OPENCODE_CONFIG_CONTENT` (`"mcp":{"pgo":{"enabled":false}}`,
   probado con OpenCode 1.19.3). Antigravity no admite configuración por ejecución: carga el suyo global, que no
   ofrece herramientas porque `pgo-mcp` ve que lo lanzó pgocode (capa 6).
5. **Entorno sin credenciales** (`proceso.CleanEnviron`): los agentes, los programas que lanzan (`pgo ejecutar`) y
   las órdenes directas no heredan `ODOO_*`, `PLANESGO_TOKEN`, `ANTIGRAVITY_TOKEN`, `PGO_TOKEN_ESCRITORIO` ni un
   `PGO_TOKEN` ajeno. Además, el backend borra `PGO_TOKEN_ESCRITORIO` de su entorno al leerlo.
6. **`pgo-mcp` se niega dentro de pgocode** (`planesgo-server/cmd/pgo-mcp/pgocode.go`): si él o un antepasado lleva
   `PGO_EJECUCION` o `PLANESGO_GESTOR=pgocode` (en el entorno con que arrancó, que el agente no puede cambiar), la CLI
   sale con el código 3 sin hablar con PlanesGo y el servidor MCP no ofrece ni ejecuta herramientas. Llega a los
   equipos con la próxima versión publicada de PlanesGo (`pgo-mcp --self-update`); hasta entonces, el `pgo-mcp`
   instalado sigue sin comprobarlo en la CLI, y lo cubren las capas 4, 7 y 8.
7. **Permisos de cada agente** (`internal/llm/odooguard.go`, fijos durante la ejecución: no rompen la caché):
   - Claude Code, en `--settings` (`permissions.deny`, probado con 2.1.309): `WebFetch(domain:<host>)` y
     `Bash(*<host>*)` del Odoo y de PlanesGo; `Read`/`Edit` de `~/.local/share/pgocode/{*.json,*.json.*,*.log,*.log.*,tiempos/**}`
     y de `~/.planesgo_auth.json` (no de `ramas/`, donde trabajan las copias de las sesiones); `Bash(*imputacion-dia*)`,
     `*sesiones-reinicio*`, `*partes-abiertos*`, `*planesgo_auth*`, `*/.claude/hooks/planesgo*`, `*/.local/bin/pgo-mcp*`,
     `*/.local/bin/planesgo-*`; `Bash(pgo-mcp …)`, `planesgo-mcp`, `planesgo-track`; `mcp__pgo__pgo_time` y
     `mcp__pgo__pgo_project`. Y `Skill(pgo)` además de `Skill(pgo-imputacion)` (`skills.Sustituidas`).
   - OpenCode (`OPENCODE_PERMISSION`, probado con 1.19.3): en «auto», `bash` = `{"*":"allow", <los mismos patrones>:
     "deny"}`; `webfetch` = `{"*":"allow","*://<host>*":"deny"}`; fuera de la carpeta, `external_directory` ya era
     `deny`; la skill `pgo`, denegada.
   - Antigravity (`agy` 1.4.6): no tiene opciones de permisos por ejecución (sus reglas `permissions` están en su
     `settings.json` global, que pgocode no toca). Le protegen las capas 1–6.
8. **Odoo exige sesión.** Sin la sesión de Odoo (capas 1 y 5), una petición a Odoo desde un agente no puede escribir
   aunque llegue: el Odoo simulado (`cmd/odoo-prueba`) ahora también la exige, como el real.

## Lo que no se puede garantizar al 100 % (y la siguiente capa)

El agente corre con el mismo usuario del sistema que pgocode. Por eso:

- **Las reglas de permisos (capa 7) son filtros de texto**: un script que construya el host (`"planesnet." + "autopyme.com"`),
  un `python -c` con `xmlrpc`, o un `cd` y un nombre partido pasan. Lo que impide la escritura en ese caso es que no
  hay credenciales a su alcance (capas 1, 5, 8). Las credenciales que quedan legibles para el mismo usuario son:
  `~/.planesgo_auth.json` (token de PlanesGo; las reglas impiden leerlo con las herramientas, no con un script),
  el almacén de cookies de Electron (cookie de pgocode: inútil por la capa 3) y `sesiones-reinicio.json` (segundos,
  durante una actualización).
- **La capa 3 se salta** si el proceso deja de descender del agente (doble `fork` + `setsid`, de modo que lo adopta
  `systemd --user`) **y** además arranca con el entorno vacío (`env -i`). Requiere hacerlo a propósito.
- **Antigravity** solo tiene las capas que no dependen del agente (1–6 y 8).
- **La sesión de Odoo en memoria** del backend: otro proceso del mismo usuario no puede leer su memoria
  (`ptrace_scope=1` de Yama lo impide salvo a sus antepasados), pero sí su `/proc/<pid>/environ` (por eso nada
  secreto va en el entorno del backend salvo `PGO_TOKEN_ESCRITORIO`, que sin la cookie de sesión no sirve).

Siguientes capas posibles, de menos a más coste:

1. **Token de PlanesGo fuera del alcance** (barato, en PlanesGo): que `pgo-mcp` y los hooks guarden el token en el
   llavero del sistema o que el servidor de PlanesGo rechace imputaciones con `PLANESGO_GESTOR=pgocode`… pero el
   llavero también lo abre el mismo usuario. La solución de verdad es la 3.
2. **Proxy de salida** (medio): pgocode lanza los agentes con `HTTPS_PROXY`/`HTTP_PROXY` a un proxy suyo que rechace
   el host de Odoo y de PlanesGo. Coste: un punto de fallo en todas las llamadas de los agentes a sus modelos, y no
   lo respetan `xmlrpc.client` de Python, `fetch` de Node sin `NODE_USE_ENV_PROXY` ni `curl --noproxy`: no garantiza
   nada más que la capa 7. **No se ha hecho.**
3. **Espacio de nombres de red o usuario del sistema distinto** (caro): lanzar los agentes con `bwrap`/`unshare`
   (Claude Code ya trae «sandbox» con `bubblewrap` y proxy de dominios: `sandbox.enabled` en `--settings`) o con un
   usuario `pgocode-agente` sin acceso a `~/.local/share/pgocode` ni a `~/.planesgo_auth.json`. Es la única que lo
   garantiza por el sistema operativo. Coste: permisos de ficheros de los proyectos y las copias, `gh`/`git`/`gcloud`
   con otras credenciales, OpenCode y Antigravity sin sandbox propio comparable, y un instalador que cree el usuario.
   Recomendado como siguiente paso si hace falta garantía total; el «sandbox» de Claude Code sería la prueba barata
   para ese agente.

## Pruebas

- Go (`go test ./...`):
  - `internal/servidor/agenttoken_test.go`: con el token de agente ninguna ruta fuera de `/api/pgo` (aun con cookie
    válida) y Odoo no recibe nada; lista blanca (órdenes de Odoo inventadas → 404, otro método → 403, las permitidas
    sin tocar Odoo); token de otro proyecto, revocado o caducado → 403; proceso de agente con cookie → 403, y de
    verdad en Linux con `curl` marcado, con `env -i` y con la marca de un programa lanzado → 403 (sin marca o con la de
    otro pgocode → 401).
  - `internal/llm/odooguard_test.go`: reglas de Claude Code y OpenCode generadas desde la configuración (también con
    el Odoo simulado en `127.0.0.1:8070`), MCP de imputación fuera de `--mcp-config` y desactivado en OpenCode, y un
    agente que no hereda credenciales.
  - `internal/imputacion/imputacion_test.go` (`TestSesionOdooNoSeGuarda`): la sesión no llega al fichero; sin sesión
    tras reiniciar no se escribe ni se marca error; al entrar se escribe.
  - `planesgo-server/cmd/pgo-mcp/pgocode_test.go`: la CLI se niega dentro de pgocode y se reconoce lo lanzado por un
    agente aunque use `env -i`.
- En la app (Odoo simulado en 8170, pgocode en 8070 con `ODOO_PASSWORD` y `PLANESGO_TOKEN` falsos en su entorno):
  un agente simulado (un `claude` falso en el `PATH`) intenta `/api/pgo/{tiempo,imputar,imputacion,vincular,proyecto,tareas}`
  → 404; `/api/imputacion` y `/api/imputacion/unificar` con la cookie robada (también con `env -i`) → 403; buscar
  `odoo_sesion` en los datos → 0 ficheros; escribir en Odoo con cada cadena de 32 hexadecimales de los datos o sin
  sesión → 0 aceptadas; su token tras el turno → 403. Claude Code (haiku) y OpenCode (Gemini 3.8 Flash) reales:
  `cat imputacion-dia.json` y `curl` al Odoo → denegados por sus permisos; `echo` → sí; entorno sin `ODOO_*` ni
  `PLANESGO_TOKEN`. La imputación de pgocode siguió escribiendo sus líneas en el Odoo simulado.
