# connectors — conexión con equipos remotos

Conectores de pgocode para entrar en servidores remotos, la persona y el agente.

```
connectors/
└── instala.sh        instala sin root lo que necesitan (Linux)
```

PowerShell remoto (WinRM) ya no es un conector: es parte de pgocode, en Go (ver más abajo).

Pendientes: `ssh/` (túnel para RDP, SQL Server, ficheros y WinRM) y otros cuando hagan falta.

Solo protocolos estándar y sus versiones modernas: RDP con FreeRDP (sin VNC), sin compatibilidad con servidores
antiguos. Las contraseñas las escribe siempre la persona (en un diálogo o en un terminal); los conectores no las
reciben ni las guardan. Los dos escritorios remotos (el de la persona y el del agente) son parte de pgocode, en Go;
aquí solo queda lo que instalan.

Por ahora son **solo para Linux** (Xvfb, xdotool, zenity, paquetes de Ubuntu). pgocode tiene que funcionar también
en Windows: allí el cliente de la persona sería el de Windows (`mstsc` con un fichero `.rdp`) y el resto necesita
otro enfoque.

## Instalación

```sh
connectors/instala.sh
```

Todo va en `~/.local/opt/rdp-agente`, sin `sudo` (no funciona dentro de la app):

- **FreeRDP** compilado desde la última versión publicada (`FREERDP_VERSION` y su SHA-256 en el script) y, si los hay,
  los parches de `freerdp/*.patch`. Se compila una vez: solo se recompila si cambian la versión o los parches. Cada
  compilación va en su carpeta (`freerdp-<versión>-<huella de los parches>`) y el enlace `freerdp` se cambia al final,
  de golpe: los escritorios abiertos siguen con la suya y las carpetas viejas se borran cuando nadie las usa:
  cliente SDL3 para la persona (Wayland nativo), cliente X11 para el agente, H.264 (AVC444) por ffmpeg, sonido por
  PulseAudio/PipeWire, impresoras CUPS, portapapeles con ficheros (FUSE) y Kerberos. Sin ALSA ni OSS, servidor,
  proxy, tarjetas inteligentes ni USB.
- **Xvfb, xdotool** (el escritorio del agente) **y el servidor RDP de prueba** (`freerdp-shadow-cli`): paquetes de
  Ubuntu desempaquetados. Las capturas y el vídeo usan `import` (ImageMagick) y `ffmpeg` del sistema.

Los datos (carpetas de intercambio y grabaciones) van en `~/.local/share/pgocode-connectors`, fuera del código y fuera
de la carpeta de datos de pgocode, con permisos 700. Cada conexión tiene su propia carpeta de intercambio:
`intercambio/agente/<proyecto>/<sesión>` para el escritorio del agente e `intercambio/persona/<proyecto>` para el de la
persona. Así, desde Windows no se ven los ficheros de otro proyecto ni de otra sesión. Las grabaciones van aparte, en
`grabaciones/agente/<proyecto>/<sesión>`, fuera de lo que se comparte.

## Escritorio remoto para la persona: `pgocode --rdp`

Es parte de pgocode, no de esta carpeta: `pgocode --rdp` abre la app en modo escritorio remoto (con pgocode ya abierto,
la misma instancia y la misma sesión; si no, el acceso de siempre). Código: `electron/main.js` (instancia única y
ventana; lanza el ayudante de escritorio), `web/src/componentes/EscritorioRemoto.tsx` (la ventana, con el estilo de
pgocode), `internal/desktop` (el ayudante de escritorio del cliente: abre FreeRDP en el equipo de la persona; lo común
con el escritorio del agente, en `internal/freerdp`) e `internal/servidor/remotesessions.go` (el servidor solo lleva
las sesiones y su tiempo). La ventana pide:

1. **Proyecto** (obligatorio), con el buscador de la barra de proyectos: el tiempo se imputa en él con el servicio
   de imputación de pgocode, mientras la sesión está conectada y hay uso del teclado o el ratón (la inactividad del
   sistema la mide Electron con `powerMonitor`, en Linux, Windows y macOS); tras 10 minutos sin uso, nada. Se rige por las reglas del servicio (tramos y bloques de 15 minutos).
2. **Conexión**: nombre, equipo y usuario de Windows (`DOMINIO\usuario` o `usuario`).
3. **Contraseña**: va de la ventana al ayudante (IPC de Electron y su entrada estándar) y de él a FreeRDP por su
   entrada; no pasa por el servidor ni se guarda.
4. **Opciones**: sonido y micrófono (sí, salvo que se quiten) e impresoras de este equipo (ninguna salvo que se pidan).
   Siempre: portapapeles y carpeta compartida «Intercambio». Pantalla completa: Ctrl+Alt+Intro.

El tiempo se imputa en el proyecto elegido mientras la sesión está abierta y se usa el teclado o el ratón (como el
resto de pgocode). La entrada estándar de FreeRDP se cierra tras la contraseña: no lee nada más.

La ventana de Windows es la nativa de FreeRDP (SDL3), con solo seguridad moderna: NLA, nivel 2 de OpenSSL,
certificado aceptado la primera vez y aviso si cambia, y gráficos H.264 4:4:4. Mientras haya un escritorio abierto,
pgocode no se actualiza. Las conexiones (sin contraseña) se guardan en el navegador de la app. Al cerrar la app, sus
ventanas de RDP se cierran con ella. En el navegador (sin la app de escritorio) no hay visores: la ventana lo avisa.

### Ayudante de escritorio y contrato con el servidor

La app de escritorio arranca el ayudante (`pgocode-backend cliente`, el mismo binario) como hijo suyo tras
emparejarse, con la dirección del servidor por entorno (`PGO_SERVIDOR`) y el token del cliente como primera línea de su
entrada (nunca en el entorno ni como argumentos, que cualquier proceso del usuario puede leer). Hablan en líneas JSON: órdenes por su entrada (`rdp`, `anydesk`, `disconnect` e `idle` cada
5 s con la inactividad del sistema) y eventos por su salida (`ready` con los visores instalados, `reply`, `state` y
`error`). El ayudante cuenta al servidor cada sesión con la cookie del cliente:

- `POST /api/remote/sessions` {kind, direction, project, name, host, user, peer, peer_name} → la sesión (su id). El
  servidor valida el proyecto con la sesión de pgocode abierta en ese cliente (la cookie de la petición o la última
  sesión de usuario vista desde ese cliente).
- `POST /api/remote/sessions/{id}/beat` {active} cada 20 s: imputa si hay alguien al teclado (tras los primeros 10 s).
- `POST /api/remote/sessions/{id}/end` {error}: cierra el periodo; el error, explicado, lo ve la ventana.
- Sin latidos durante 60 s (app cerrada o caída, equipo dormido), el servidor cierra el periodo; si el ayudante
  sigue, la vuelve a dar de alta. Solo el cliente que abrió una sesión la puede latir o cerrar.
- `GET /api/remote/sessions` (interfaz): las sesiones abiertas y las terminadas hace poco, sin contraseñas.

### AnyDesk (misma ventana, pestaña «AnyDesk»)

Sin la API REST de AnyDesk. La ventana pide el **proyecto** (obligatorio) y el **ID de AnyDesk** (cifras o un alias
`equipo@ad`, validado en la web y en el ayudante) y el ayudante abre el cliente instalado (`anydesk <ID>`, sin shell y
sin nada de pgocode en su entorno); AnyDesk pide la contraseña él mismo. Código: `internal/anydesk` (lector de sus
registros), `internal/desktop` (lanza y sigue los registros en el equipo de la persona),
`internal/servidor/anydesk.go` (proyecto de cada equipo, el de las entrantes y la asignación) y
`web/src/componentes/AnyDesk.tsx`.

El inicio y el fin de cada sesión salen de los registros de AnyDesk, seguidos como `tail -F` (también cuando AnyDesk
los recorta): en Linux `~/.anydesk/anydesk.trace` (y `/var/log/anydesk.trace` si se puede leer); en Windows
`%AppData%\AnyDesk\ad.trace` y `%ProgramData%\AnyDesk\ad_svc.trace`. Salientes: `app.frontend_session - Session
started.` / `Session stopped.` tras `Sending a connection request for address <ID>.`; entrantes: `app.backend_session
- Incoming session request: <nombre> (<ID>)`, `Session started (ok).` / `Session stopped.`. Se imputa como RDP
(sesión `anydesk-<id>`, cada 20 s con uso del teclado o el ratón; al acabar, se cierra). El proyecto de cada sesión:
el de la conexión abierta desde pgocode, el último de ese equipo o, en las entrantes, el que se haya elegido para
todas; si no hay, no se imputa y pgocode lo pide en un diálogo (en esta ventana y en la principal). Ajustes en
`anydesk.json` de la carpeta de datos (sin secretos).

## Escritorio del agente: propiedades de la sesión y `pgo escritorio`

Es parte de pgocode (en Go), como la conexión con SQL Server de la sesión: un servicio del backend, porque la pantalla
virtual corre donde corren los agentes. Código: `internal/agentdesktop` (pantalla virtual, FreeRDP, capturas, ratón,
teclado y vídeo), `internal/freerdp` (lo común con el escritorio de la persona: ajustes de seguridad, contraseña por
la entrada, errores), `internal/servidor/agentdesktop.go` (la conexión de cada sesión, la orden del agente y la
auditoría), `cmd/pgocode/pgo_desktop.go` (`pgo escritorio`) y `web/src/componentes/AgentDesktopSettings.tsx`.

1. La persona abre las propiedades de la sesión → **Escritorio del agente**: equipo (y `:puerto` si no es el 3389),
   usuario (`DOMINIO\usuario` o `usuario`), contraseña, tamaño (1600x900 por defecto) y **Teclado Unicode**
   (activado por defecto), y pulsa **Iniciar**. Ahí ve el estado, la carpeta de intercambio y el vídeo, y lo para.
2. Los agentes de **esa** sesión lo usan (los de otras sesiones, no):

```sh
pgo escritorio captura [--escala 0.6]        # en .pgocode/salidas de su carpeta; devuelve la ruta para leerla
pgo escritorio clic 300 150 [--doble|--derecho]   # coordenadas de la última captura (con su escala)
pgo escritorio mueve X Y | arrastra X1 Y1 X2 Y2 | rueda arriba|abajo [N]
pgo escritorio escribe "texto"
pgo escritorio tecla ctrl+s Return              # nombres de xdotool
pgo escritorio tamaño 1920x1080                 # cambia el tamaño en caliente
pgo escritorio graba inicio | graba fin         # vídeo en …/pgocode-connectors/grabaciones/agente/<proyecto>/<sesión>
pgo escritorio estado
```

- **Seguridad**: la misma que el cliente de la persona (NLA, `seclevel:2`, certificado aceptado la primera vez y
  aviso si cambia, AVC444). La contraseña va del backend a la entrada estándar de FreeRDP, que se cierra después:
  nunca en argumentos, registros ni ficheros; se pierde al parar. La pantalla virtual (Xvfb) solo escucha por su
  socket local y con una cookie X propia (otro usuario del equipo no puede mirarla). Cada orden de los agentes va a
  `escritorio-auditoria.jsonl` de la carpeta de datos de pgocode (de `escribe`, solo cuántos caracteres).
- **Pantalla**: un lienzo fijo de 3840x2160 en una pantalla virtual libre; lo que cambia es la ventana de FreeRDP,
  que con resolución dinámica pide al servidor que ajuste su sesión; capturas (ImageMagick `import`) y vídeo
  (`ffmpeg`) se recortan a ella.
- **Teclado**: Unicode manda el texto por la tubería de FreeRDP (acentos, ñ y € llegan tal cual sea cual sea la
  distribución de teclado de Windows); sin él, `xdotool type` (solo ASCII fiable).
- **Comunicaciones**: la carpeta de intercambio aparece en Windows como `\\tsclient\Intercambio` y el portapapeles
  de texto se comparte.
- La cookie X y la tubería del teclado van en `$XDG_RUNTIME_DIR/rdp-agente` (sin `XDG_RUNTIME_DIR` no arranca: nada
  de `/tmp`; si la carpeta no es propia con permisos 700, se niega a usarla). Al cerrar pgocode se para todo y,
  mientras haya uno abierto, pgocode no se actualiza.
- Solo Linux (Xvfb, X11): en Windows y macOS dice que no está disponible.

## PowerShell remoto: conexión WinRM de la sesión (`pgo ps`)

Es parte de pgocode, en Go (`internal/winrm`, `internal/servidor/winrmconn.go`, sección «PowerShell remoto» de las
propiedades de la sesión). La persona define en la sesión el equipo, el usuario (local o `DOMINIO\usuario`) y la
contraseña, que se queda solo en la memoria de pgocode; los agentes de esa sesión la usan:

```sh
pgo ps "Get-ChildItem 'C:\Program Files'"
pgo ps -f script.ps1 [--escribir]
pgo ps baja 'C:\ruta\remota.log' ./local.log
pgo ps sube ./local.txt 'C:\ruta\remota.txt' --escribir
```

Por HTTP (5985) con cada mensaje cifrado por NTLM (claves de 128 bits), o por HTTPS (5986) validando el certificado
(o aceptándolo sin comprobar, si se elige). Solo lectura salvo que la conexión sea actualizable: en solo lectura solo
pasan órdenes de consulta y el script corre en el modo restringido de PowerShell; evita accidentes, no es una
garantía (para eso, un usuario sin permisos de escritura). Cada orden queda en `winrm-audit.jsonl` de la carpeta de
datos de pgocode (la orden, no su salida).

## Probado

Contra servidores RDP locales de prueba (`freerdp-shadow-cli` sobre otra pantalla virtual, con NLA):

- `pgocode --rdp`: conexión real al servidor de Sage (192.168.125.229) con imputación en su proyecto; pruebas de
  `internal/servidor/rdp_test.go` (proyecto obligatorio, contraseña solo por la entrada de FreeRDP, opciones,
  usuario con dominio y mensajes de error).
- Escritorio del agente, con FreeRDP 3.32.2: `go test -tags integracion ./internal/agentdesktop/` arranca
  `freerdp-shadow-cli` en 127.0.0.1:3392 (pantalla :83, usuario de un fichero SAM con una contraseña aleatoria) y el
  escritorio del agente en :84: contraseña incorrecta rechazada con su motivo, conexión, captura escalada, el ratón
  llega al servidor en las coordenadas de la captura, clic, doble clic derecho, arrastre, rueda, teclas, texto por
  la tubería Unicode, cambio de tamaño en caliente, grabación y parada sin dejar nada. Además, pruebas con programas
  simulados (`internal/agentdesktop`, `internal/servidor/agentdesktop_test.go`): argumentos, contraseña solo por la
  entrada, escalado, validación, auditoría sin secretos y que el agente de otra sesión no puede usarlo.

Sin probar todavía, porque necesitan el Windows real: la ventana SDL3 en el escritorio, que se vean las impresoras
y se oiga el sonido, que el texto Unicode llegue a Windows tal cual y `pgo ps` (WinRM). El paso de `instala.sh` que
compila FreeRDP reproduce lo hecho a mano, pero no se ha ejecutado desde cero.
