# connectors — conexión con equipos remotos

Conectores de pgocode para entrar en servidores remotos, la persona y el agente.

```
connectors/
├── instala.sh        instala sin root lo que necesitan (Linux)
├── rdp/
│   ├── cliente.py    cliente de escritorio remoto para la persona (ventana normal)
│   └── rdp.py        escritorio remoto para el agente (pantalla virtual: capturas, ratón, teclado, vídeo)
└── winrm/
    └── ps.py         PowerShell remoto: órdenes, ficheros y auditoría
```

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

- **FreeRDP** compilado desde la última versión publicada (`FREERDP_VERSION` y su SHA-256 en el script):
  cliente SDL3 para la persona (Wayland nativo), cliente X11 para el agente, H.264 (AVC444) por ffmpeg, sonido por
  PulseAudio/PipeWire, impresoras CUPS, portapapeles con ficheros (FUSE) y Kerberos. Sin ALSA ni OSS, servidor,
  proxy, tarjetas inteligentes ni USB.
- **Xvfb, xdotool y el servidor RDP de prueba** (`freerdp-shadow-cli`): paquetes de Ubuntu desempaquetados.
- **pywinrm** en un venv.

Los datos (carpeta de intercambio, grabaciones y auditoría) van en `~/.local/share/pgocode-connectors`, fuera del
código y fuera de la carpeta de datos de pgocode (sus agentes no la pueden leer y necesitan la de intercambio).

## Cliente de escritorio remoto: `rdp/cliente.py`

Se abre con **`pgocode --rdp`** (el lanzador de pgocode lo reconoce y abre este cliente en vez de la app; el
paquete de la app incluye `connectors/`). En desarrollo, también con `python3 -I connectors/rdp/cliente.py`:

1. Lista de conexiones guardadas o «Nueva conexión…» (nombre, equipo `nombre o IP[:puerto]`, usuario, dominio).
2. Opciones de esa conexión (se recuerdan): portapapeles (texto y ficheros), impresoras de este equipo (todas las
   de CUPS), sonido, micrófono, carpeta compartida (en Windows, «Intercambio») y pantalla completa.
3. **Proyecto de Odoo al que se imputa el tiempo (obligatorio)**: se ofrece el último de la conexión o se busca
   por nombre. Sin proyecto no se conecta.
4. Contraseña en su propio diálogo: pasa del diálogo a FreeRDP por una tubería; no se guarda en ningún sitio.

**Imputación.** El cliente es independiente: no necesita pgocode abierto, ni git, ni carpeta de proyecto. El tiempo
va a Odoo por PlanesGo con `pgo-mcp` (el token de PlanesGo de este equipo), como las sesiones de Claude Code o
Antigravity fuera de pgocode: antes de conectar se comprueba que PlanesGo admite imputar en ese proyecto
(`--check`); con la sesión conectada y uso del teclado o el ratón (inactividad de GNOME), un latido cada 30 s
(`--beat`, tarea «Escritorio remoto», tipo Desarrollo); tras 10 minutos sin uso se cierra el parte (`--stop`) y,
al volver, se abre otro; al terminar se cierra. Si un latido falla, avisa sin cortar la sesión. Dentro de pgocode
no se lanza así (allí imputa pgocode: `pgo-mcp` está bloqueado).

Solo seguridad moderna: NLA (`/sec:nla`), nivel 2 de OpenSSL (`/tls:seclevel:2`), certificado aceptado la primera
vez y aviso si cambia (`/cert:tofu`). Gráficos H.264 4:4:4 (`/gfx:AVC444`). La ventana se puede redimensionar y
Windows ajusta su resolución; Ctrl+Alt+Intro alterna la pantalla completa; se reconecta sola si se corta la red. Si
algo falla, explica el motivo (usuario o contraseña, cuenta bloqueada, contraseña caducada, equipo inalcanzable…).

Las conexiones (sin contraseña) se guardan en `~/.config/pgocode-connectors/conexiones.json`.

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
- **Tamaño**: con `--tamaño auto` (por defecto) toma la resolución del servidor si el puente WinRM está arrancado;
  si no, 1600x900. La pantalla virtual es un lienzo fijo de 3840x2160 y lo que cambia es la ventana de FreeRDP,
  que con resolución dinámica pide al servidor que ajuste su sesión; capturas y vídeo se recortan a ella.
- **Teclado**: `--teclado unicode` (por defecto) manda el texto como Unicode por la tubería de FreeRDP: acentos, ñ
  y € llegan tal cual sea cual sea la distribución de teclado del servidor (Windows lo admite).
  `--teclado teclas` manda códigos de tecla: solo ASCII fiable.
- **Comunicaciones**: la carpeta de intercambio aparece en el servidor como `\\tsclient\intercambio` y el
  portapapeles de texto se comparte.
- `--acepta-certificado` es solo para servidores de prueba.

## PowerShell remoto: `winrm/ps.py`

```sh
connectors/winrm/ps.py puente --host 192.168.125.229 --usuario 'DOMINIO\usuario'   # pide la contraseña; dejarlo abierto
connectors/winrm/ps.py "Get-ChildItem 'C:\Program Files'"
connectors/winrm/ps.py -f script.ps1
connectors/winrm/ps.py baja 'C:\ruta\remota.log' ./local.log
connectors/winrm/ps.py sube ./local.txt 'C:\ruta\remota.txt'
```

El puente guarda la contraseña solo en su memoria y atiende por un socket local con permisos 600. Cada orden
queda en `~/.local/share/pgocode-connectors/auditoria.log` (la orden, no su salida).

Por defecto va por HTTP (5985), donde NTLM cifra cada mensaje. Con `--https` va por HTTPS (5986) validando el
certificado del servidor (`--ca` para una CA propia); hay que activarlo antes en Windows (un listener HTTPS de
WinRM con su certificado). El servidor de Sage (192.168.125.229) no lo tiene abierto.

`rdp.py` y `ps.py` guardan su estado, la tubería del teclado y el socket del puente en `$XDG_RUNTIME_DIR/rdp-agente`.
Sin `XDG_RUNTIME_DIR` no arrancan (nada de `/tmp`, donde otro usuario podría crear antes la carpeta), y si la carpeta
no es propia con permisos 700 se niegan a usarla.

## Probado

Contra servidores RDP locales de prueba (`freerdp-shadow-cli` en 127.0.0.1:3390 sobre otra pantalla virtual, con y
sin NLA):

- `rdp/cliente.py`: lista de conexiones, nueva conexión, opciones recordadas, diálogo de contraseña, conexión,
  cancelar sin conectar y mensajes de error comprensibles. En el registro de FreeRDP: el portapapeles se carga, el
  sonido negocia formatos, el micrófono abre su flujo en PulseAudio y el servicio de impresoras se carga.
- `rdp/rdp.py` con FreeRDP 3.32.2 y NLA: conexión, contraseña incorrecta rechazada, captura, clic, escritura y
  teclas (modo `teclas`), cambio de tamaño en caliente y grabación.

Sin probar todavía, porque necesitan el Windows real: la ventana SDL3 en el escritorio, que se vean las impresoras
y se oiga el sonido, el modo `unicode`, la resolución automática y `winrm/ps.py`. El paso de `instala.sh` que
compila FreeRDP reproduce lo hecho a mano, pero no se ha ejecutado desde cero.
