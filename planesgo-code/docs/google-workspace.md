# Google Workspace en pgocode (Drive, Docs y Sheets, solo lectura)

pgocode y sus agentes pueden **buscar, listar y leer** en Google Drive, Docs y Sheets con la cuenta de Google del
usuario. Nada de escritura: pgocode solo pide permiso de lectura (`drive.readonly`) y solo hace peticiones `GET` a
las APIs de Google.

Funciona como Claude u otras aplicaciones de escritorio: la empresa registra **una vez** una aplicación OAuth en
Google Cloud y cada usuario pulsa **«Conectar con Google»**, ve la pantalla de Google y acepta. No hace falta `gcloud`.

## 1. Registrar la aplicación (una vez, quien administra Google Cloud)

En la [consola de Google Cloud](https://console.cloud.google.com/), proyecto `planes-dev`:

1. **APIs y servicios → Biblioteca**: habilitar **Google Drive API** y **Google Sheets API** (las cuotas de las
   peticiones van a este proyecto). Docs se lee exportando por Drive.
2. **Google Auth Platform → Marca / Pantalla de consentimiento**: tipo **Interna** (solo cuentas de planesnet.com:
   sin verificación de Google ni pantalla de «aplicación no verificada»). Nombre: «PlanesGo».
3. **Acceso a los datos**: ámbitos `openid`, `…/auth/userinfo.email` y `…/auth/drive.readonly`.
4. **Clientes → Crear cliente**: tipo **Aplicación de escritorio**, nombre «PlanesGo». No lleva URI de redirección:
   Google admite para estos clientes la vuelta a `http://127.0.0.1:<cualquier puerto>`.
5. **Descargar JSON** del cliente (`client_secret_….apps.googleusercontent.com.json`, con
   `{"installed":{"client_id":…,"client_secret":…}}`) y pasárselo a cada usuario por un canal interno.

El secreto de un cliente de escritorio no es confidencial para Google (va en cada instalación y lo protege PKCE),
pero pgocode no lo enseña en la interfaz ni lo escribe en los registros.

Más adelante, el servidor de PlanesGo podrá servir el cliente a todo el equipo (endpoint de solo lectura,
autenticado): el gestor ya tiene el gancho (`Manager.ClientSource`); el fichero local manda si existe.

## 2. Conectar la cuenta (cada usuario)

1. Menú del logo → **Propiedades → Google Workspace**.
2. Si dice «Falta registrar la aplicación de Google», **Cargar el fichero del cliente…** (o arrastrar el JSON a la
   sección). Se guarda en `~/.local/share/pgocode/google-oauth-client.json` (0600; solo `client_id`,
   `client_secret` y `project_id`). Se hace una vez por equipo.
3. **Conectar con Google**: se abre la página de Google en el **navegador del sistema** (Google bloquea el inicio de
   sesión en navegadores integrados como el de Electron: «Este navegador o aplicación pueden no ser seguros»). Elegir
   la cuenta de planesnet.com y aceptar. La pestaña dice «pgocode ya puede leer tu Google Drive» y la sección pasa a
   «Conectado como luis@planesnet.com · solo lectura de Drive».
4. **Desconectar** revoca el permiso en Google y borra la conexión del equipo. También se puede quitar en
   https://myaccount.google.com/permissions (pgocode lo nota en la siguiente renovación y borra la conexión).

## Cómo funciona

- OAuth 2.0 de aplicación nativa ([RFC 8252](https://www.rfc-editor.org/rfc/rfc8252)) con PKCE S256: el backend abre
  un puerto efímero en `127.0.0.1`, la URL de consentimiento lleva `state`, el reto PKCE, `access_type=offline` y
  `prompt=consent`; Google vuelve a `http://127.0.0.1:<puerto>/?code=…&state=…`, el backend cambia el código por los
  tokens (con el verificador) y cierra el puerto. Se espera como mucho 10 minutos; un nuevo «Conectar» sustituye al
  anterior.
- Si en la pantalla de Google se desmarca el permiso de Drive, no se guarda nada y se revoca lo concedido.
- El **token de renovación** se guarda solo en el equipo, uno por usuario de pgocode:
  `~/.local/share/pgocode/google/<usuario>.json` (carpeta 0700, fichero 0600), con la cuenta y los ámbitos. El
  **token de acceso** vive en memoria y se renueva solo al caducar o si Google lo rechaza.
- Los endpoints de Google son fijos en el código (no se toman del JSON del cliente).
- Usuario de cada petición: la interfaz usa la conexión de quien ha entrado en pgocode; los agentes (`pgo drive`), la
  del último usuario que ha usado la interfaz o, si en el equipo solo hay una conexión, esa.
- **Respaldo silencioso**: si el usuario no ha conectado su cuenta y en el equipo hay una `gcloud` ya autorizada con
  Drive, se usa su token (en memoria). La interfaz lo dice («Con las credenciales de Google de este equipo») pero no
  enseña órdenes de gcloud; conectar la cuenta propia tiene preferencia.

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

API del backend (con sesión): `GET /api/google[?probar=1]` (estado), `POST /api/google/conectar` (devuelve la URL de
Google), `POST /api/google/desconectar`, `POST /api/google/cliente {contenido}` (el JSON del cliente) y
`GET /api/google/{buscar|listar|leer}`; los agentes, `GET /api/pgo/drive-{buscar|listar|leer}` con el token de su
proyecto (`PGO_TOKEN`).

## Código y pruebas

- `internal/gworkspace/oauth.go` (cliente, conexión, PKCE, renovación, revocación), `auth.go` (token y estado;
  respaldo de gcloud), `drive.go` (Drive y Sheets, formato compacto), `internal/servidor/google.go`,
  `cmd/pgocode/pgo.go` (`pgo drive`), `internal/llm/pgohint.go` (aviso), `web/src/componentes/GoogleSettings.tsx`.
- Pruebas con un Google simulado (autorización, intercambio con PKCE, renovación, revocación, errores, estado):
  `go test -race ./internal/gworkspace ./internal/servidor -run 'OAuth|Client|Drive|Google|Status|Read'`.
- Prueba real (opcional; en una carpeta de pruebas con el `google-oauth-client.json`, no la de datos de pgocode):

  ```sh
  PGO_PRUEBA_GOOGLE=1 PGO_PRUEBA_GOOGLE_DATOS=<carpeta> PGO_PRUEBA_GOOGLE_USUARIO=luis@planesnet.com \
    [PGO_PRUEBA_GOOGLE_REF=<id o url de un documento>] [PGO_PRUEBA_GOOGLE_DESCONECTAR=1] \
    go test ./internal/gworkspace -run Real -v -timeout 10m
  ```

  Si no hay conexión, escribe la URL de Google: abrirla en el navegador y aceptar (espera 5 minutos).
