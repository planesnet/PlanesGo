# Google en pgocode (Drive, Docs, Sheets, Slides y Gmail, solo lectura y solo por enlace)

pgocode y sus agentes pueden **leer por su enlace** ficheros de Google Drive, Docs, Sheets y Slides y **correos de
Gmail**, con la sesión de Google que el usuario tenga abierta **en el navegador integrado de pgocode** (el panel Web).
Leer es **descargar el fichero por su enlace de descarga**, como «Archivo → Descargar»: **nunca se carga ni se lee
una página de Google** (ni su texto ni su DOM). Por eso no hay búsquedas ni listados (necesitarían leer páginas):
el usuario pega el enlace. No hay nada que configurar: ni Propiedades, ni cliente OAuth, ni `gcloud`.

## Cómo se usa

1. Abrir el panel **Web** de un proyecto, ir a `accounts.google.com` e iniciar sesión (una vez, como en cualquier
   navegador). Si un agente pide algo de Google sin sesión, pgocode abre ahí mismo `accounts.google.com` (como mucho
   cada 2 minutos por proyecto) y el agente recibe «inicia sesión en Google en el panel Web de pgocode».
2. Pegarle al agente el enlace («resume https://docs.google.com/document/d/…», «¿qué dice este correo?
   https://mail.google.com/mail/u/0/#inbox/FMfcgz…»).

La sesión del navegador es la del panel Web **del proyecto** (cada proyecto tiene la suya: `persist:navegador-<id>`).
Si en la del proyecto no hay sesión de Google, pgocode prueba las de los demás proyectos (la más reciente primero, 4
como mucho) y recuerda la primera que funciona; así basta con iniciar sesión en uno.

## Cómo funciona (descargas, como las haría el usuario)

- El agente pide con `pgo drive leer` o `pgo gmail leer`. El backend (Go, `internal/gbrowser`) decide qué **enlace
  de descarga** pedir y se lo pasa a la app de escritorio por un canal local: Electron (`electron/googleReader.js`)
  pregunta al backend si hay trabajo (`GET /api/escritorio/google/trabajo`, espera hasta 25 s) y entrega el fichero
  descargado (`POST /api/escritorio/google/resultado`, en base64), los dos con la cookie secreta de la app de
  escritorio (`pgo_escritorio`); ni el agente ni la interfaz pueden usar ese canal.
- Electron crea una **ventana oculta** con la misma sesión que el panel Web, **sin cargar ninguna página** (sin
  JavaScript, sin navegación: `will-navigate` se bloquea) y descarga el enlace con `webContents.downloadURL`. El
  fichero va a una carpeta temporal, se lee entero y se borra (solo las descargas de esas ventanas: las del panel Web
  siguen igual). Como mucho 20 MB.
- **Sin sesión**: se sabe por las URL de la descarga (la cadena de redirecciones lleva a `accounts.google.com`), sin
  leer nada de la página.
- **Lo que no es una descarga no se lee**: si Google responde con un error (4xx: sin acceso, no existe, no es de ese
  tipo; Electron la deja «interrupted») o con una página en vez del fichero (HTML sin `Content-Disposition:
  attachment`: «solicitar acceso», aviso de virus de los ficheros grandes…), el agente recibe «no se ha podido
  descargar: ¿tienes acceso a ese fichero? ¿has iniciado sesión en Google en el panel Web de pgocode?».
- **Límites**: pgocode no lee, copia, exporta ni reutiliza cookies, tokens ni cabeceras de autenticación, y no hace
  peticiones por su cuenta (nada de `net.request`, `session.fetch` ni `session.cookies`): solo descarga el enlace. Sin
  clics, sin ventanas nuevas, sin sonido. Solo enlaces de Google por https (`*.google.com`, `*.googleusercontent.com`);
  si la descarga acaba fuera de Google, se descarta.
- El navegador se anuncia como Chrome (sin «Electron/…»), porque Google no deja iniciar sesión en navegadores que se
  anuncian como integrados.
- En modo navegador (`pgocode -navegador`, sin Electron) no está disponible y se dice: «Google se lee a través del
  navegador integrado de la aplicación de escritorio de pgocode…».

Qué enlace se descarga en cada caso:

| Qué | Enlace de descarga | Se lee como |
|---|---|---|
| Un Doc | `docs.google.com/document/d/<id>/export?format=md` (si no se descarga, `format=txt`) | Markdown (o texto) |
| Una hoja de cálculo | `…/spreadsheets/d/<id>/export?format=xlsx` (el libro entero) | CSV por pestañas (lector XLSX propio en Go) |
| La hoja de un enlace con `#gid=` (sin `--hoja`) | `…/export?format=csv&gid=<gid>[&range=A1:D20]` | CSV |
| Una presentación | `…/presentation/d/<id>/export/txt` | texto |
| Un fichero de Drive (PDF, Word, Excel, texto…) | `drive.google.com/uc?export=download&id=<id>` (Google lo lleva a `drive.usercontent.google.com`) | texto, Word (DOCX) y Excel (XLSX) en texto/CSV, `.eml` como correo; lo demás (PDF, imágenes, ZIP…) se guarda en `.pgocode/salidas/` de la carpeta del agente y se da su ruta, tipo y tamaño |
| Un ID suelto | se prueban, en orden, Doc (md, txt), hoja (xlsx), presentación (txt) y fichero (uc): el primero que se descarga | según lo descargado |
| Un correo de Gmail | `mail.google.com/mail/u/<n>/?ui=2&view=att&th=<id>&attid=0&disp=comp&safe=1&zw` (el mensaje original, como «Descargar mensaje») | el MIME en Go: texto (o el HTML pasado a texto) sin lo citado; adjuntos solo como nombre, tipo y tamaño |

- **Hojas de cálculo**: el XLSX se lee con `archive/zip` y `encoding/xml` (sin dependencias): nombres de las hojas,
  textos compartidos y en línea, números, booleanos y fechas/horas (por el formato de número de la celda). Así se
  saben los nombres de las pestañas sin leer `htmlview`. `--hoja` elige por nombre o número y `--rango` se aplica
  aquí (A1:D20, B2, A:C, 2:10).
- **Carpetas**: no se pueden descargar; se dice que se pida el enlace de un fichero.
- **Gmail**: el enlace es el de la barra de direcciones (`…/mail/u/0/#inbox/FMfcgz…`, o el ID hexadecimal antiguo,
  o `?th=…`, o `permmsgid=msg-f:…`). Los ID nuevos (`FMfcgz…`) son el texto `f:<número>` en base64, escrito en base
  40 con consonantes; se decodifican aquí y el número, en hexadecimal, es el ID del mensaje. Se descarga **un
  mensaje**: el del enlace (en un hilo, el primero). Los de enviados y borradores (`thread-a:…`) no valen. Gmail se
  descarga con la sesión del navegador; si su URL de descarga necesitara el `ik` de la cuenta (que solo está en la
  página), no funcionaría: no se ha podido comprobar sin una cuenta.

## Uso (agentes)

El uso se les añade solo al mensaje que menciona Drive, Docs, Sheets, «documento de Google» o un enlace de
docs.google.com (y el de Gmail, al que habla del correo, el email, Gmail, la bandeja o el buzón):

```sh
pgo drive leer <enlace o id> [--hoja <nombre|n>] [--rango A1:D20] [--max N]
pgo gmail leer <enlace del correo> [--max N]
```

- Recorte: 40 000 caracteres por defecto en Drive y 20 000 en Gmail (`--max` hasta 200 000), con un aviso al final.
- Salida compacta: una cabecera `# nombre [tipo · formato · id]` y el texto; en Gmail, `# asunto [correo <id>]`,
  `De:`, `Para:`, `Cc:`, `Fecha:`, el texto y `Adjuntos: nombre (tipo, tamaño); …`.
- **Privacidad del correo**: solo se lee cuando se pide y con su enlace; nada se guarda; el registro del backend dice
  solo la orden y cuánto ha tardado (`gmail: leer · 840ms`). Nada de enviar, borrar, archivar ni etiquetar.

## Lo de antes (OAuth)

Hasta la 0.4.97, pgocode leía Google con su propio cliente OAuth (Propiedades → Google Workspace → «Conectar con
Google») y, si no, con la `gcloud` del equipo. Ya no existe: al arrancar, el backend borra de su carpeta de datos
`google-oauth-client.json` y los `google/<usuario>.json` (con sus tokens de renovación) y la carpeta `google/` si queda
vacía (solo ficheros `.json` normales; lo demás no lo toca). El permiso concedido en Google se puede quitar en
https://myaccount.google.com/permissions («PlanesGo»).

## Código y pruebas

- `internal/gbrowser` (canal con Electron, qué enlace descargar, lectura de XLSX/DOCX/EML, formato compacto, borrado
  de lo de antes), `internal/servidor/google.go` (rutas del canal y de `pgo drive|gmail leer`),
  `electron/googleReader.js` (ventana oculta, descargas, sesiones), `cmd/pgocode/pgo.go`, `internal/llm/pgohint.go`
  (aviso).
- `go test ./internal/gbrowser ./internal/servidor -run 'Google|Bridge|Read|Mail|Parse|HTML|OAuth'` (con un Electron
  simulado y descargas simuladas: md, txt, csv, xlsx con varias hojas, docx, eml, PDF, páginas de error, sesión) y
  `node --test electron/*.test.js` (con una ventana simulada que solo descarga).
- Para probar la app de escritorio sin Google: `PGO_GOOGLE_PRUEBA=http://127.0.0.1:<puerto>` (en el entorno de
  Electron, que se lo pasa al backend) hace que el backend pida las descargas a ese servidor local en lugar de a
  Google y que la ventana oculta lo acepte.
