# Google en pgocode (Drive, Docs, Sheets, Slides y Gmail, solo lectura)

pgocode y sus agentes pueden **buscar, listar y leer** en Google Drive, Docs, Sheets, Slides y **Gmail** con la
sesión de Google que el usuario tenga abierta **en el navegador integrado de pgocode** (el panel Web). No hay nada que
configurar: ni Propiedades, ni cliente OAuth, ni `gcloud`. Basta con iniciar sesión en Google una vez en el panel Web,
como en cualquier navegador.

## Cómo se usa

1. Abrir el panel **Web** de un proyecto, ir a `accounts.google.com` (o a `drive.google.com`) e iniciar sesión.
   Si un agente pide algo de Google sin sesión, pgocode abre ahí mismo `accounts.google.com` (como mucho cada 2
   minutos por proyecto) y el agente recibe «inicia sesión en Google en el panel Web de pgocode».
2. Pedirle al agente lo que sea («resume el documento https://docs.google.com/document/d/…», «busca en Drive el
   informe de ventas», «¿qué me ha escrito Ana esta semana?»).

La sesión del navegador es la del panel Web **del proyecto** (cada proyecto tiene la suya: `persist:navegador-<id>`).
Si en la del proyecto no hay sesión de Google, pgocode prueba las de los demás proyectos (la más reciente primero, 4
como mucho) y recuerda la primera que funciona; así basta con iniciar sesión en uno.

## Cómo funciona (lo hace el navegador, como lo haría el usuario)

- El agente pide con `pgo drive …` o `pgo gmail …`. El backend (Go, `internal/gbrowser`) decide qué página cargar y
  se la pide a la app de escritorio por un canal local: Electron (`electron/googleReader.js`) pregunta al backend si
  hay trabajo (`GET /api/escritorio/google/trabajo`, espera hasta 25 s) y entrega lo leído
  (`POST /api/escritorio/google/resultado`), los dos con la cookie secreta de la app de escritorio
  (`pgo_escritorio`); ni el agente ni la interfaz pueden usar ese canal.
- Electron carga la página en una **ventana oculta** con la misma sesión que el panel Web y devuelve **el texto de la
  página** (guiones fijos que solo leen el DOM) o, si la página es una exportación que el navegador descarga, el texto
  de ese fichero (se guarda en una carpeta temporal, se lee y se borra; solo las descargas de esas ventanas: las del
  panel Web siguen igual).
- **Límites**: pgocode no lee, copia, exporta ni reutiliza cookies, tokens ni cabeceras de autenticación, y no hace
  peticiones por su cuenta (nada de `net.request`, `session.fetch` ni `session.cookies`): solo navega. Los guiones no
  hacen clic ni escriben en ningún formulario. La ventana oculta solo carga páginas de Google por https (`*.google.com`,
  `*.googleusercontent.com`); si una página lleva a otro sitio, se para. Sin ventanas nuevas, sin sonido.
- El navegador se anuncia como Chrome (sin «Electron/…»), porque Google no deja iniciar sesión en navegadores que se
  anuncian como integrados.
- En modo navegador (`pgocode -navegador`, sin Electron) no está disponible y se dice: «Google se lee a través del
  navegador integrado de la aplicación de escritorio de pgocode…».

Qué página carga cada cosa:

| Orden | Página |
|---|---|
| `pgo drive leer` de un Doc | `docs.google.com/document/d/<id>/export?format=md` (si no da un fichero, `format=txt` y, si tampoco, `…/mobilebasic`) |
| de una hoja de cálculo | las pestañas, de `…/spreadsheets/d/<id>/htmlview`; cada hoja, `…/export?format=csv&gid=<gid>[&range=A1:D20]` |
| de una presentación | `…/presentation/d/<id>/export/txt` |
| de una carpeta | `drive.google.com/drive/folders/<id>` (como `listar`) |
| un ID suelto | antes, `drive.google.com/open?id=<id>`, solo para ver a qué tipo de página lleva |
| `pgo drive buscar` | `drive.google.com/drive/search?q=<texto> [type:document…]`; con `--carpeta`, el listado de la carpeta filtrado por nombre |
| `pgo drive listar` | `drive.google.com/drive/my-drive` o la carpeta |
| `pgo gmail buscar` / `listar` | `mail.google.com/mail/u/0/#search/<consulta>` / `#inbox` (las filas de la lista) |
| `pgo gmail leer` | la vista de impresión del hilo, `mail.google.com/mail/u/0/?ui=2&view=pt&search=all&th=<id>` |

Las búsquedas y listados de Drive y de Gmail se leen del DOM de sus páginas (filas con `data-id` en Drive; `tr.zA`,
`.bog`, `.y2`… en Gmail). Esas páginas no son una API: si Google cambia su maquetación, puede que haya que retocar
los guiones de `electron/googleReader.js`. Leer por URL o ID (las exportaciones) es lo más estable.

## Uso (agentes)

El uso se les añade solo al mensaje que menciona Drive, Docs, Sheets, «documento de Google» o una URL de
docs.google.com (y el de Gmail, al que habla del correo, el email, Gmail, la bandeja o el buzón):

```sh
pgo drive buscar <texto> [--tipo doc|hoja|presentacion|pdf|carpeta|imagen] [--carpeta <id|url>] [--max N]
pgo drive listar [<carpeta id|url>]          # sin carpeta: «Mi unidad»
pgo drive leer <id|url> [--hoja <nombre|n>] [--rango A1:D20] [--max N]
pgo gmail buscar <consulta de Gmail> [--max N]   # from:ana newer_than:7d, subject:factura has:attachment, is:unread…
pgo gmail listar [--max N]                       # lo más reciente de la bandeja de entrada
pgo gmail leer <id de hilo> [--max N]
```

- Acepta IDs y URLs de Drive, Docs, Sheets y Slides (la hoja del `#gid=` de una URL de Sheets también).
- Búsquedas y listados: una línea por fichero, `[tipo] nombre · fecha · id` (lo que enseña Drive; 30 por defecto,
  100 como mucho).
- Lectura: Docs en Markdown (o texto), Sheets en CSV (todas las hojas, una con `--hoja`, un rango con `--rango`),
  Slides en texto, carpetas como listado; de los demás ficheros (PDF, imágenes…), solo el enlace.
- Recorte: 40 000 caracteres por defecto (`--max` hasta 200 000), con un aviso al final de cómo pedir menos.
- Gmail: dos líneas por correo, `fecha · remitente · asunto · id · no leído` y el fragmento (20 por defecto, 50
  como mucho); `leer` da el hilo, cada mensaje sin lo citado de los anteriores, recortado a 20 000 caracteres
  repartidos entre los mensajes. El ID es el del hilo (el que da `buscar`/`listar`, o el hexadecimal de una URL
  antigua `#inbox/<id>`).
- **Privacidad del correo**: solo se lee cuando se pide; nada se guarda; el registro del backend dice solo la orden,
  cuántos correos y cuánto ha tardado (`gmail: buscar · 12 correos · 840ms`). Nada de enviar, borrar, archivar ni
  etiquetar. Abrir un hilo en la vista web de Gmail puede marcarlo como leído, como cuando lo abre el usuario.

## Lo de antes (OAuth)

Hasta la 0.4.97, pgocode leía Google con su propio cliente OAuth (Propiedades → Google Workspace → «Conectar con
Google») y, si no, con la `gcloud` del equipo. Ya no existe: al arrancar, el backend borra de su carpeta de datos
`google-oauth-client.json` y los `google/<usuario>.json` (con sus tokens de renovación) y la carpeta `google/` si queda
vacía (solo ficheros `.json` normales; lo demás no lo toca). El permiso concedido en Google se puede quitar en
https://myaccount.google.com/permissions («PlanesGo»).

## Código y pruebas

- `internal/gbrowser` (canal con Electron, qué página cargar, formato compacto, Gmail, borrado de lo de antes),
  `internal/servidor/google.go` (rutas del canal y de `pgo drive|gmail`), `electron/googleReader.js` (ventana
  oculta, descargas, guiones de lectura, sesiones), `cmd/pgocode/pgo.go`, `internal/llm/pgohint.go` (aviso).
- `go test ./internal/gbrowser ./internal/servidor -run 'Google|Bridge|Read|Search|Gmail|OAuth'` (con un Electron
  simulado) y `node --test electron/*.test.js` (con una ventana simulada).
- Para probar la app de escritorio sin Google: `PGO_GOOGLE_PRUEBA=http://127.0.0.1:<puerto>` (en el entorno de
  Electron, que se lo pasa al backend) hace que el backend pida las páginas a ese servidor local en lugar de a Google
  y que la ventana oculta lo acepte.
