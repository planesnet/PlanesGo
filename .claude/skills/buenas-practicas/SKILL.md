---
name: buenas-practicas
description: Buenas prácticas de programación para pgocode (Planes Go Code). Úsala SIEMPRE antes de escribir, cambiar o revisar código de este repositorio (Go del backend, React/TypeScript de la web, Electron, scripts del Makefile): nombres en inglés y textos en español, código limpio y reutilizable, seguridad, tests, multiplataforma y todo en Go. También al refactorizar o al pedir «arregla el código de paso».
---

# Buenas prácticas en pgocode

pgocode es un único producto: backend en Go (`planesgo-code/`), interfaz React + TypeScript (`planesgo-code/web`) y
la ventana Electron (`planesgo-code/electron`). Estas normas valen para todo lo que se escriba o se toque. Antes de
dar un cambio por bueno, repasa la lista de comprobación del final.

## 1. Idioma

- **Código en inglés**: identificadores (variables, funciones, tipos, paquetes nuevos), comentarios, claves JSON y
  nombres de ficheros nuevos.
- **Textos para el usuario en español**: interfaz, mensajes de error que llegan a la persona, ayuda de `pgo`,
  registros que lee la persona y mensajes de commit.
- **Código existente en español**: lo que toques, pásalo al inglés si es razonable y no desborda el cambio
  (renombrar dentro de la función o del fichero que estás cambiando; no un renombrado masivo de paso).

## 2. Diseño

- **Una sola herramienta**: todo se hace desde pgocode (la app y la orden `pgo …`). Nada de scripts sueltos ni de
  dependencias de PlanesGo (desinstalado): ni hooks, ni MCP `pgo`, ni `PLANESGO_GESTOR`.
- **Todo en Go**: nada de Python en pgocode. Herramientas externas (FreeRDP, ffmpeg…) se lanzan desde Go, se
  compilan una vez y se guardan.
- **Reutiliza antes de escribir**: busca la función, el componente o el patrón que ya existe (p. ej. `Modal`,
  `askConfirm`, `MenuSesion`, `Switch`, `ProjectField`, `api.ts` `get`/`post`; en Go `proceso.GrupoPropio`,
  `repo.ExcludeLocally`, `jsonOK`/`jsonError`, `lee`, `conSesion`). Si dos sitios hacen lo mismo, extrae lo común.
- **Conexiones de la sesión** (SQL Server, escritorio remoto, WinRM…): la persona las define en la sesión; la
  contraseña solo vive en la memoria del backend; los agentes las usan con `pgo …` y solo las de su sesión; cada
  orden va a una auditoría sin secretos ni datos; se cierran al apagar pgocode.
- **Funciones pequeñas y con un propósito**; la lógica pura (sin E/S) en funciones aparte, con tests. Comentarios
  que explican el porqué, no el qué; nada de código muerto ni comentado.
- **Estructura**: cada paquete con una responsabilidad clara (`internal/<tema>`); la web con componentes en
  `componentes/` y la lógica pura en módulos `.ts` con su `.test.ts`.

## 3. Seguridad

- **Valida toda entrada** en el servidor (proyecto con `s.proyecto`, carpetas dentro del proyecto, ramas, ids,
  servidores, rutas): nunca te fíes de la interfaz.
- **Nada de shell** con datos del usuario: `exec.Command` con argumentos separados; valida antes lo que va como
  argumento (que no empiece por `-`, caracteres permitidos).
- **Secretos**: nunca en argumentos de procesos, registros, ficheros del repositorio ni respuestas JSON;
  contraseñas por la entrada estándar o en memoria; ficheros sensibles con permisos 600; `depuracion.Tapa` antes de
  registrar texto que pueda llevarlos.
- **Mínimo privilegio**: solo lectura por defecto (SQL, WinRM); escribir exige permiso explícito de la persona;
  los agentes, solo lo de su sesión y su proyecto.
- **Nada destructivo sin confirmación**: borrar o forzar pide confirmación explicando qué se pierde.

## 4. Multiplataforma

- pgocode tiene que **compilar y funcionar en Linux, Windows y macOS**: nada de `syscall.Kill`, `Setpgid` ni
  similares en código común; usa `internal/proceso` (versiones `_unix`/`_windows`) o build tags. Lo que solo
  existe en Linux (Xvfb, X11) da un error claro en los demás.
- `proceso.GrupoPropio` asigna `cmd.Cancel`: la orden se crea con `exec.CommandContext`.

## 5. Errores y registros

- Los errores se devuelven y se explican en español a la persona (qué ha pasado y qué hacer); no se tragan en
  silencio salvo que se diga por qué en un comentario.
- Un pánico en una petición responde 500 y se informa (internal/bugreport); no dejes `panic` como control de flujo.

## 6. Tests

- Cada cambio con lógica lleva su test: Go (`go test`, con `-race` en lo concurrente) y web (`node --test`).
- Lo que depende de un servidor real va con `-tags integracion` y variables de entorno; nunca contra equipos de
  clientes.
- Prueba los casos raros (entrada vacía, inválida, sin permiso, otra sesión), no solo el camino feliz.

## 7. Git

- Commits pequeños con mensaje en español que explica el porqué; nada de commits ni push sin que lo pida la
  persona; nada de publicar versiones sin su permiso.
- Lo que pgocode deja en un proyecto (`.pgocode/`, `.pgo.json`) se excluye con `repo.ExcludeLocally`, sin tocar el
  `.gitignore` del proyecto.

## Lista de comprobación antes de terminar

Desde `planesgo-code/`:

```sh
gofmt -l cmd internal                       # vacío
for os in linux windows darwin; do GOOS=$os CGO_ENABLED=0 go vet ./... && GOOS=$os CGO_ENABLED=0 go build -o /dev/null ./cmd/...; done
go test ./...                               # y -race en los paquetes tocados
(cd web && npm run build && npm test)
make e2e
```

Y además: ¿nombres en inglés y textos en español? ¿Se reutiliza lo que ya había? ¿Sin secretos en argumentos ni
registros? ¿Entradas validadas en el servidor? ¿Funciona en Windows?
