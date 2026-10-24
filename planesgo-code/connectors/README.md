# connectors — conexión con equipos remotos

Conectores de pgocode para entrar en servidores remotos, la persona y el agente.

```
connectors/
├── instala.sh        instala sin root lo que necesitan (Linux)
└── rdp/
    └── rdp.py        escritorio remoto para el agente (pantalla virtual: capturas, ratón, teclado, vídeo)
```

PowerShell remoto (WinRM) ya no es un conector: es parte de pgocode, en Go (ver más abajo).

Pendientes: `ssh/` (túnel para RDP, SQL Server, ficheros y WinRM) y otros cuando hagan falta.

Solo protocolos estándar y sus versiones modernas: RDP con FreeRDP (sin VNC), sin compatibilidad con servidores
antiguos. Las contraseñas las escribe siempre la persona (en un diálogo o en un terminal); los conectores no las
reciben ni las guardan.

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
- **Xvfb, xdotool y el servidor RDP de prueba** (`freerdp-shadow-cli`): paquetes de Ubuntu desempaquetados.

Los datos (carpeta de intercambio, grabaciones y auditoría) van en `~/.local/share/pgocode-connectors`, fuera del
código y fuera de la carpeta de datos de pgocode (sus agentes no la pueden leer y necesitan la de intercambio).

## Escritorio remoto para la persona: `pgocode --rdp`

Es parte de pgocode, no de esta carpeta: `pgocode --rdp` abre la app en modo escritorio remoto (con pgocode ya abierto,
la misma instancia y la misma sesión; si no, el acceso de siempre). Código: `electron/main.js` (instancia única y
ventana), `web/src/componentes/EscritorioRemoto.tsx` (la ventana, con el estilo de pgocode) e
`internal/servidor/rdp.go` (abre FreeRDP e imputa). La ventana pide:

1. **Proyecto** (obligatorio), con el buscador de la barra de proyectos: el tiempo se imputa en él con el servicio
   de imputación de pgocode, mientras la sesión está conectada y hay uso del teclado o el ratón (inactividad de
   GNOME); tras 10 minutos sin uso, nada. Se rige por las reglas del servicio (tramos y bloques de 15 minutos).
2. **Conexión**: nombre, equipo y usuario de Windows (`DOMINIO\usuario` o `usuario`).
3. **Contraseña**: va del backend a FreeRDP por su entrada y no se guarda.
4. **Opciones**: sonido y micrófono (sí, salvo que se quiten) e impresoras de este equipo (ninguna salvo que se pidan).
   Siempre: portapapeles y carpeta compartida «Intercambio». Pantalla completa: Ctrl+Alt+Intro.

El tiempo se imputa en el proyecto elegido mientras la sesión está abierta y se usa el teclado o el ratón (como el
resto de pgocode). La entrada estándar de FreeRDP se cierra tras la contraseña: no lee nada más.

La ventana de Windows es la nativa de FreeRDP (SDL3), con solo seguridad moderna: NLA, nivel 2 de OpenSSL,
certificado aceptado la primera vez y aviso si cambia, y gráficos H.264 4:4:4. Mientras haya un escritorio abierto,
pgocode no se actualiza. Las conexiones (sin contraseña) se guardan en el navegador de la app.

### AnyDesk (misma ventana, pestaña «AnyDesk»)

Sin la API REST de AnyDesk. La ventana pide el **proyecto** (obligatorio) y el **ID de AnyDesk** (cifras o un alias
`equipo@ad`, validado en la web y en el backend) y abre el cliente instalado (`anydesk <ID>`, sin shell y sin nada de
pgocode en su entorno); AnyDesk pide la contraseña él mismo. Código: `internal/anydesk` (lector de sus registros),
`internal/servidor/anydesk.go` (lanza, sigue los registros e imputa) y `web/src/componentes/AnyDesk.tsx`.

El inicio y el fin de cada sesión salen de los registros de AnyDesk, seguidos como `tail -F` (también cuando AnyDesk
los recorta): en Linux `~/.anydesk/anydesk.trace` (y `/var/log/anydesk.trace` si se puede leer); en Windows
`%AppData%\AnyDesk\ad.trace` y `%ProgramData%\AnyDesk\ad_svc.trace`. Salientes: `app.frontend_session - Session
started.` / `Session stopped.` tras `Sending a connection request for address <ID>.`; entrantes: `app.backend_session
- Incoming session request: <nombre> (<ID>)`, `Session started (ok).` / `Session stopped.`. Se imputa como RDP
(sesión `anydesk-<id>`, cada 20 s con uso del teclado o el ratón; al acabar, se cierra). El proyecto de cada sesión:
el de la conexión abierta desde pgocode, el último de ese equipo o, en las entrantes, el que se haya elegido para
todas; si no hay, no se imputa y pgocode lo pide en un diálogo (en esta ventana y en la principal). Ajustes en
`anydesk.json` de la carpeta de datos (sin secretos).

## Escritorio remoto para el agente: `rdp/rdp.py`

```sh
connectors/rdp/rdp.py inicia --host 192.168.125.229 --usuario 'DOMINIO\usuario'   # pide la contraseña ahí
connectors/rdp/rdp.py captura f.png --escala 0.6
connectors/rdp/rdp.py clic 300 150 [--doble|--derecho]     # coordenadas de la última captura
connectors/rdp/rdp.py escribe "texto"
connectors/rdp/rdp.py tecla ctrl+s Return
connectors/rdp/rdp.py tamaño 1920x1080                     # cambia el tamaño en caliente
connectors/rdp/rdp.py graba inicio | graba fin             # vídeo en …/pgocode-connectors/intercambio/grabaciones
connectors/rdp/rdp.py estado | para
```

- **Seguridad**: la misma que el cliente (NLA, `seclevel:2`, AVC444). La contraseña se lee de la entrada estándar
  antes de conectar (en un terminal, sin eco).
- **Tamaño**: con `--tamaño auto` (por defecto) toma la resolución del servidor si la sesión tiene conexión WinRM
  (`pgo ps`); si no, 1600x900. La pantalla virtual es un lienzo fijo de 3840x2160 y lo que cambia es la ventana de FreeRDP,
  que con resolución dinámica pide al servidor que ajuste su sesión; capturas y vídeo se recortan a ella.
- **Teclado**: `--teclado unicode` (por defecto) manda el texto como Unicode por la tubería de FreeRDP: acentos, ñ
  y € llegan tal cual sea cual sea la distribución de teclado del servidor (Windows lo admite).
  `--teclado teclas` manda códigos de tecla: solo ASCII fiable.
- **Comunicaciones**: la carpeta de intercambio aparece en el servidor como `\\tsclient\intercambio` y el
  portapapeles de texto se comparte.
- `--acepta-certificado` es solo para servidores de prueba.

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

`rdp.py` guarda su estado y la tubería del teclado en `$XDG_RUNTIME_DIR/rdp-agente`.
Sin `XDG_RUNTIME_DIR` no arranca (nada de `/tmp`, donde otro usuario podría crear antes la carpeta), y si la carpeta
no es propia con permisos 700 se niega a usarla.

## Probado

Contra servidores RDP locales de prueba (`freerdp-shadow-cli` en 127.0.0.1:3390 sobre otra pantalla virtual, con y
sin NLA):

- `pgocode --rdp`: conexión real al servidor de Sage (192.168.125.229) con imputación en su proyecto; pruebas de
  `internal/servidor/rdp_test.go` (proyecto obligatorio, contraseña solo por la entrada de FreeRDP, opciones,
  usuario con dominio y mensajes de error).
- `rdp/rdp.py` con FreeRDP 3.32.2 y NLA: conexión, contraseña incorrecta rechazada, captura, clic, escritura y
  teclas (modo `teclas`), cambio de tamaño en caliente y grabación.

Sin probar todavía, porque necesitan el Windows real: la ventana SDL3 en el escritorio, que se vean las impresoras
y se oiga el sonido, el modo `unicode`, la resolución automática y `pgo ps` (WinRM). El paso de `instala.sh` que
compila FreeRDP reproduce lo hecho a mano, pero no se ha ejecutado desde cero.
