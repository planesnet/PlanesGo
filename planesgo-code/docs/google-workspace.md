# Google Workspace en pgocode (Drive, Docs y Sheets, solo lectura)

pgocode y sus agentes pueden **buscar, listar y leer** en Google Drive, Docs y Sheets con la cuenta de Google del
usuario. Nada de escritura: pgocode solo hace peticiones `GET` a las APIs de Google.

No hay cliente OAuth propio ni proyecto nuevo en Google Cloud: pgocode usa la autorización que el usuario da a la
herramienta oficial `gcloud` con su cuenta.

## Lo que tiene que hacer Luis (una vez)

1. En un terminal, dar a las credenciales por defecto de gcloud (ADC) el permiso de leer Drive (abre la página de
   Google para aceptar):

   ```sh
   gcloud auth application-default login --scopes=openid,https://www.googleapis.com/auth/userinfo.email,https://www.googleapis.com/auth/drive.readonly,https://www.googleapis.com/auth/cloud-platform
   ```

   Lleva también `cloud-platform`: estas credenciales son las mismas que usa pgocode para Vertex AI (OpenCode y los
   modelos de Google), que siguen funcionando.

2. En pgocode: menú del logo → **Propiedades → Google Workspace → Comprobar**. Tiene que decir «Conectado como
   luis@planesnet.com · solo lectura de Drive». La misma sección enseña la orden, con un botón para copiarla.

3. Solo si al leer Google dice que la API no está habilitada (lo dice el error y la sección de Propiedades), habilitar
   las APIs en el proyecto de cuota (`planes-dev`, el de `gcloud`):

   ```sh
   gcloud services enable drive.googleapis.com sheets.googleapis.com docs.googleapis.com --project=planes-dev
   ```

   pgocode manda el proyecto de cuota (cabecera `x-goog-user-project`) solo si Google lo pide; es `planes-dev`
   (`GOOGLE_CLOUD_PROJECT` si está puesta, o `PGOCODE_GOOGLE_QUOTA_PROJECT` para otro).

### Si Google responde «Esta aplicación está bloqueada»

La documentación de Google avisa de que pedir ámbitos de fuera de Google Cloud (como Drive) al iniciar sesión de
ADC con la configuración por defecto puede dar ese error, y recomienda un cliente OAuth propio
([Troubleshoot your ADC setup](https://docs.cloud.google.com/docs/authentication/troubleshoot-adc)). Sin cliente
propio, la alternativa es la sesión de la propia gcloud con acceso a Drive (opción oficial `--enable-gdrive-access`
de [`gcloud auth login`](https://docs.cloud.google.com/sdk/gcloud/reference/auth/login)):

```sh
gcloud auth login --enable-gdrive-access
```

Esa da el ámbito de Drive completo, no el de solo lectura; pgocode sigue haciendo solo lecturas, y la sección de
Propiedades lo dice («Drive completo (pgocode solo lee)»). pgocode prueba primero ADC y, si su token no puede leer
Drive, el de la sesión de gcloud.

No se ha podido comprobar contra Google cuál de las dos funciona con la cuenta de planesnet.com (en el desarrollo no
había credenciales reales); la prueba real está más abajo.

## Cómo obtiene pgocode el token

- Cuando lo necesita, ejecuta `gcloud auth application-default print-access-token` (y, si ese token no lleva Drive,
  `gcloud auth print-access-token`). pgocode no lee los ficheros de credenciales ni guarda el token en disco: vive
  en memoria hasta que caduca (como mucho 30 minutos) o Google lo rechaza (entonces lo vuelve a pedir una vez).
- Con `tokeninfo` de Google (el token va en el cuerpo, no en la URL) sabe la cuenta y los ámbitos: lo que enseña
  Propiedades. El token nunca sale del backend.
- Para desconectar: `gcloud auth application-default revoke` (quita también Vertex) o quitar el acceso en
  https://myaccount.google.com/permissions. pgocode no tiene botón de desconectar: las credenciales son de gcloud.

## Uso

Agentes (orden de terminal; el uso se les añade solo al mensaje que menciona Drive, Docs, Sheets, «documento de
Google» o una URL de docs.google.com):

```sh
pgo drive buscar <texto> [--tipo doc|hoja|presentacion|pdf|carpeta|imagen] [--carpeta <id|url>] [--max N]
pgo drive listar [<carpeta id|url>]          # sin carpeta: «Mi unidad»
pgo drive leer <id|url> [--hoja <nombre|n>] [--rango A1:D20] [--max N]
```

- Acepta IDs y URLs de Drive, Docs, Sheets y Slides (la hoja del `#gid=` de una URL de Sheets también).
- Búsquedas y listados: una línea por fichero, `[tipo] nombre · fecha · id` (30 por defecto, 100 como mucho).
- Lectura: Docs en Markdown (o texto), Sheets en CSV (todas las hojas visibles, una con `--hoja`, un rango con
  `--rango`; sin la API de Sheets, la primera hoja exportada por Drive), Slides en texto, ficheros de texto tal cual,
  carpetas como listado; PDF, imágenes y demás, solo metadatos y enlace (Drive no da su texto).
- Recorte: 40 000 caracteres por defecto (`--max` hasta 200 000), con un aviso al final de cómo pedir menos.

API del backend: `GET /api/google[?probar=1]` (estado, con sesión) y `GET /api/google/{buscar|listar|leer}` (con
sesión); los agentes, `GET /api/pgo/drive-{buscar|listar|leer}` con el token de su proyecto (`PGO_TOKEN`).

## Código y pruebas

- `internal/gworkspace` (token por gcloud, tokeninfo, Drive y Sheets, formato compacto), `internal/servidor/google.go`,
  `cmd/pgocode/pgo.go` (`pgo drive`), `internal/llm/pgohint.go` (aviso), `web/src/componentes/GoogleSettings.tsx`.
- Pruebas con un Google simulado: `go test ./internal/gworkspace ./internal/servidor -run 'Drive|Google|Status|Read'`.
- Prueba real (opcional, con la gcloud del equipo ya autorizada):

  ```sh
  PGO_PRUEBA_GOOGLE=1 PGO_PRUEBA_GOOGLE_REF=<id o url de un documento> go test ./internal/gworkspace -run Real -v
  ```

## Opción avanzada (no necesaria)

Si ninguna de las dos órdenes funcionara con la cuenta, la vía que documenta Google es un cliente OAuth propio de
tipo «Aplicación de escritorio» y `gcloud auth application-default login --client-id-file=<fichero> --scopes=…`.
pgocode no cambia: sigue pidiendo el token a gcloud.
