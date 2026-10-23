# Google Workspace en pgocode (Drive, Docs, Sheets y Gmail, solo lectura)

pgocode y sus agentes pueden **buscar, listar y leer** en Google Drive, Docs, Sheets y **Gmail** con la cuenta de
Google del usuario. Nada de escritura: pgocode solo pide permisos de lectura (`drive.readonly` y `gmail.readonly`) y
solo hace peticiones `GET` a las APIs de Google (en Gmail: nada de enviar, borrar, marcar como leído ni tocar
etiquetas).

Funciona como Claude u otras aplicaciones de escritorio: la empresa registra **una vez** una aplicación OAuth en
Google Cloud y cada usuario pulsa **«Conectar con Google»**, ve la pantalla de Google y acepta. No hace falta `gcloud`.

## 1. Registrar la aplicación (una vez, quien administra Google Cloud)

En la [consola de Google Cloud](https://console.cloud.google.com/), proyecto `planes-dev`:

1. **APIs y servicios → Biblioteca**: habilitar **Google Drive API**, **Google Sheets API** y **Gmail API** (las
   cuotas de las peticiones van a este proyecto). Docs se lee exportando por Drive.
2. **Google Auth Platform → Marca / Pantalla de consentimiento**: tipo **Interna** (solo cuentas de planesnet.com:
   sin verificación de Google ni pantalla de «aplicación no verificada», aunque `gmail.readonly` sea un ámbito
   restringido). Nombre: «PlanesGo».
3. **Acceso a los datos**: ámbitos `openid`, `…/auth/userinfo.email`, `…/auth/drive.readonly` y
   `…/auth/gmail.readonly`.
4. **Clientes → Crear cliente**: tipo **Aplicación de escritorio**, nombre «PlanesGo Code». No lleva URI de redirección:
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
   la cuenta de planesnet.com y aceptar (Google enseña dos permisos: ver los ficheros de Drive y leer el correo). La
   pestaña dice «pgocode ya puede leer tu cuenta de Google» y la sección pasa a «Conectado como luis@planesnet.com ·
   solo lectura», con una línea por servicio: **Drive, Docs y Sheets** y **Gmail**, cada uno «autorizado» o «sin
   autorizar».
   - Quien se conectó antes de que pgocode leyera Gmail ve Gmail «sin autorizar» y el botón **Volver a conectar**:
     pide los dos permisos (con `include_granted_scopes=true`, el token nuevo lleva también lo ya concedido).
   - Si en la página de Google se desmarca uno de los dos, el otro funciona y el desmarcado sale «sin autorizar»
     (con «Volver a conectar» para pedirlo). Si se desmarcan los dos, no se guarda nada.
4. **Desconectar** revoca el permiso en Google y borra la conexión del equipo. También se puede quitar en
   https://myaccount.google.com/permissions (pgocode lo nota en la siguiente renovación y borra la conexión).

## Cómo funciona

- OAuth 2.0 de aplicación nativa ([RFC 8252](https://www.rfc-editor.org/rfc/rfc8252)) con PKCE S256: el backend abre
  un puerto efímero en `127.0.0.1`, la URL de consentimiento lleva `state`, el reto PKCE, `access_type=offline` y
  `prompt=consent`; Google vuelve a `http://127.0.0.1:<puerto>/?code=…&state=…`, el backend cambia el código por los
  tokens (con el verificador) y cierra el puerto. Se espera como mucho 10 minutos; un nuevo «Conectar» sustituye al
  anterior.
- Ámbitos que se piden: `openid email …/drive.readonly …/gmail.readonly`. Los concedidos se guardan con la conexión
  y se renuevan con el token: Drive sin su permiso responde «la conexión no incluye el permiso de Drive», Gmail sin el
  suyo «Gmail no está autorizado: … Volver a conectar», sin llegar a llamar a Google. Si en la pantalla de Google se
  desmarcan los dos, no se guarda nada y se revoca lo concedido.
- El **token de renovación** se guarda solo en el equipo, uno por usuario de pgocode:
  `~/.local/share/pgocode/google/<usuario>.json` (carpeta 0700, fichero 0600), con la cuenta y los ámbitos. El
  **token de acceso** vive en memoria y se renueva solo al caducar o si Google lo rechaza.
- Los endpoints de Google son fijos en el código (no se toman del JSON del cliente).
- Usuario de cada petición: la interfaz usa la conexión de quien ha entrado en pgocode; los agentes (`pgo drive`), la
  del último usuario que ha usado la interfaz o, si en el equipo solo hay una conexión, esa.
- **Respaldo silencioso**: si el usuario no ha conectado su cuenta y en el equipo hay una `gcloud` ya autorizada con
  Drive, se usa su token (en memoria; Gmail solo si ese token lleva un ámbito de Gmail). La interfaz lo dice («Con las credenciales de Google de este equipo») pero no
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

Gmail (orden de terminal; el uso se añade solo al mensaje que habla del correo, el email, Gmail, la bandeja o el
buzón, y no a «el campo email», `partner.email` y parecidos):

```sh
pgo gmail buscar <consulta de Gmail> [--max N]   # from:ana newer_than:7d, subject:factura has:attachment, is:unread…
pgo gmail listar [--max N]                       # lo más reciente de la bandeja de entrada
pgo gmail leer <id de mensaje o hilo> [--mensaje] [--max N]
```

- Búsqueda y listado: dos líneas por correo, `fecha · remitente · asunto · id (hilo X) · no leído` y el fragmento
  (140 caracteres); 20 por defecto, 50 como mucho, con aviso si Gmail tiene más. Se piden las cabeceras de cada
  correo (`format=metadata`, 8 a la vez).
- Lectura: con el ID de un hilo, el hilo entero (de más antiguo a más nuevo) **sin lo citado** de los mensajes
  anteriores («El … escribió:», «On … wrote:», «-----Mensaje original-----», la cabecera «De:/Enviado:» de
  Outlook, las líneas con `>`; lo reenviado se queda); con el de un mensaje (o `--mensaje`), solo ese. Cabeceras
  De, Para, Cc (recortadas si son largas), fecha y asunto; el cuerpo en `text/plain` o, si no hay, el HTML
  convertido a texto (sin estilos, guiones, imágenes ni enlaces de seguimiento); juegos de caracteres UTF-8,
  latin-1, latin-9 y windows-1252. Adjuntos: solo nombre, tipo y tamaño (**nunca se descargan**; solo se pide aparte
  el cuerpo de texto cuando Gmail lo deja fuera por grande).
- Recorte: 20 000 caracteres por defecto (`--max` hasta 200 000), repartidos entre los mensajes del hilo, con aviso.
- IDs: los de la API (los que da `pgo gmail buscar`). Las URL de Gmail antiguas (`#inbox/<id hexadecimal>`) valen;
  las nuevas no llevan ese ID y se dice que se busque el correo.

**Privacidad del correo**: solo se lee cuando el usuario o el agente lo piden; nada se guarda en disco aparte del
token (ni caché ni copia); el registro del backend dice solo la orden, cuántos correos y cuánto ha tardado
(`gmail: buscar · 12 correos · 840ms`), nunca la consulta, los IDs ni el contenido.

API del backend (con sesión): `GET /api/google[?probar=1]` (estado), `POST /api/google/conectar` (devuelve la URL de
Google), `POST /api/google/desconectar`, `POST /api/google/cliente {contenido}` (el JSON del cliente) y
`GET /api/google/{buscar|listar|leer}` (Drive) y `GET /api/google/gmail/{buscar|listar|leer}` (Gmail: `q`, `max`,
`ref`, `mensaje=1`); los agentes, `GET /api/pgo/drive-{buscar|listar|leer}` y `GET /api/pgo/gmail-{buscar|listar|leer}`
con el token de su sesión y turno (`PGO_TOKEN`, ver [odoo-blindaje.md](odoo-blindaje.md)). El estado (`/api/google`) lleva `drive` y `gmail` (autorizado o no).

## Código y pruebas

- `internal/gworkspace/oauth.go` (cliente, conexión, PKCE, renovación, revocación), `auth.go` (token y estado;
  respaldo de gcloud), `drive.go` (Drive y Sheets, formato compacto), `gmail.go` (Gmail: búsqueda, lectura, MIME,
  HTML a texto, citas), `internal/servidor/google.go`, `cmd/pgocode/pgo.go` (`pgo drive`, `pgo gmail`), `internal/llm/pgohint.go` (aviso), `web/src/componentes/GoogleSettings.tsx`.
- Pruebas con un Google simulado (autorización, intercambio con PKCE, renovación, revocación, errores, estado; Gmail:
  `messages.list`, `messages.get` en `metadata` y `full`, `threads.get`, MIME multipart con texto y HTML, base64url
  con y sin relleno, adjuntos, ámbito no concedido, 401 y renovación):
  `go test -race ./internal/gworkspace ./internal/servidor -run 'OAuth|Client|Drive|Google|Status|Read|Gmail|Mail'`.
- Prueba real (opcional; en una carpeta de pruebas con el `google-oauth-client.json`, no la de datos de pgocode):

  ```sh
  PGO_PRUEBA_GOOGLE=1 PGO_PRUEBA_GOOGLE_DATOS=<carpeta> PGO_PRUEBA_GOOGLE_USUARIO=luis@planesnet.com \
    [PGO_PRUEBA_GOOGLE_REF=<id o url de un documento>] [PGO_PRUEBA_GOOGLE_DESCONECTAR=1] \
    [PGO_PRUEBA_GOOGLE_RECONECTAR=1] [PGO_PRUEBA_GOOGLE_CORREO=<consulta de Gmail>] \
    go test ./internal/gworkspace -run Real -v -timeout 10m
  ```

  Si no hay conexión (o con `PGO_PRUEBA_GOOGLE_RECONECTAR=1`, para añadir Gmail a una conexión de antes), escribe la
  URL de Google: abrirla en el navegador y aceptar (espera 5 minutos). Con Gmail autorizado, lista 3 correos (los
  recientes o los de la consulta) y lee el hilo del primero recortado a 1500 caracteres.
