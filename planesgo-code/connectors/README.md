# connectors — conexión con equipos remotos

Conectores de pgocode para entrar en servidores remotos, la persona y el agente.

```
connectors/
├── instala.sh        instala sin root lo que necesitan (Linux)
├── rdp/
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

La ventana de Windows es la nativa de FreeRDP (SDL3), con solo seguridad moderna: NLA, nivel 2 de OpenSSL,
certificado aceptado la primera vez y aviso si cambia, y gráficos H.264 4:4:4. Mientras haya un escritorio abierto,
pgocode no se actualiza. Las conexiones (sin contraseña) se guardan en el navegador de la app.

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

- `pgocode --rdp`: conexión real al servidor de Sage (192.168.125.229) con imputación en su proyecto; pruebas de
  `internal/servidor/rdp_test.go` (proyecto obligatorio, contraseña solo por la entrada de FreeRDP, opciones,
  usuario con dominio y mensajes de error).
- `rdp/rdp.py` con FreeRDP 3.32.2 y NLA: conexión, contraseña incorrecta rechazada, captura, clic, escritura y
  teclas (modo `teclas`), cambio de tamaño en caliente y grabación.

Sin probar todavía, porque necesitan el Windows real: la ventana SDL3 en el escritorio, que se vean las impresoras
y se oiga el sonido, el modo `unicode`, la resolución automática y `winrm/ps.py`. El paso de `instala.sh` que
compila FreeRDP reproduce lo hecho a mano, pero no se ha ejecutado desde cero.
