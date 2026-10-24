#!/usr/bin/env python3
"""Escritorio remoto (RDP) manejable por un agente: FreeRDP en una pantalla virtual (Xvfb), capturas de pantalla
y ratón y teclado con xdotool. La contraseña la escribe la persona en el terminal donde se arranca la sesión
(FreeRDP la pide); este programa nunca la recibe ni la guarda.

  rdp.py inicia --host 192.168.125.229 --usuario USUARIO [--dominio DOM] [--tamaño auto|1920x1080]
                                                auto: la resolución del servidor (si el puente WinRM está
                                                arrancado); si no, 1600x900
                                                [--teclado unicode|teclas] unicode (por defecto) para Windows
  rdp.py tamaño 1920x1080                       cambia el tamaño en caliente (el servidor ajusta su sesión)
  rdp.py captura [fichero.png] [--escala 0.6]   captura de la pantalla remota (coordenadas: las de esa captura)
  rdp.py clic X Y [--doble | --derecho]         clic en coordenadas de la última captura
  rdp.py arrastra X1 Y1 X2 Y2
  rdp.py rueda arriba|abajo [N]                 (en la posición actual del ratón)
  rdp.py mueve X Y
  rdp.py escribe "texto"
  rdp.py tecla ctrl+s [Return ...]              teclas con nombres de xdotool
  rdp.py graba inicio|fin                       vídeo de la sesión (evidencias) en intercambio/grabaciones
  rdp.py estado | para

Comunicaciones: la carpeta ~/.local/share/pgocode-connectors/intercambio aparece en el servidor como unidad
\\\\tsclient\\intercambio (pasar ficheros en los dos sentidos) y el portapapeles se comparte. Instalación: instala.sh.
"""
import argparse
import json
import os
import shutil
import signal
import stat
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
STATE_DIR = os.path.join(os.environ.get("XDG_RUNTIME_DIR") or sys.exit("falta XDG_RUNTIME_DIR"), "rdp-agente")
STATE = os.path.join(STATE_DIR, "estado.json")
# Text goes through FreeRDP's keyboard pipe as Unicode (accents, ñ, €… whatever the server's keyboard layout);
# xdotool type loses the characters the virtual X keymap does not have.
KBD_PIPE = os.path.join(STATE_DIR, "teclado.fifo")
# Data out of the code, and out of pgocode's data folder (its agents cannot read it, and they need the shared
# folder): shared folder and recordings.
DATA = os.path.expanduser("~/.local/share/pgocode-connectors")
EXCHANGE = os.path.join(DATA, "intercambio")
DISPLAY = ":77"
# La pantalla virtual es un lienzo fijo de este tamaño máximo; lo que cambia de tamaño es la ventana de FreeRDP
# (en la esquina 0,0) y, con su resolución dinámica, la sesión del servidor. Capturas y vídeo se recortan
# a la ventana. (Xvfb no admite modos nuevos con xrandr: no puede cambiar el tamaño de la pantalla.)
MAX_W, MAX_H = 3840, 2160
DEFAULT_SIZE = "1600x900"
# Solo RDP, nada de VNC. FreeRDP: compilado desde la última versión (instala.sh); Xvfb y xdotool: paquetes de
# Ubuntu desempaquetados sin root. Sus librerías van solo a Xvfb y xdotool: con LD_LIBRARY_PATH global, FreeRDP
# cargaría las de Ubuntu (más viejas) en vez de las suyas.
LOCAL = os.path.expanduser("~/.local/opt/rdp-agente/root")
FREERDP = os.path.expanduser("~/.local/opt/rdp-agente/freerdp/bin/xfreerdp")
os.environ["PATH"] = os.path.join(LOCAL, "usr/bin") + os.pathsep + os.environ.get("PATH", "")
LOCAL_LIBS = os.path.join(LOCAL, "usr/lib/x86_64-linux-gnu")


def private_dir(path):
    """Creates (if needed) and checks a runtime folder: ours and 700. No /tmp fallback: there another user could
    create it first and plant a socket or a pipe."""
    os.makedirs(path, mode=0o700, exist_ok=True)
    st = os.lstat(path)
    if not stat.S_ISDIR(st.st_mode) or st.st_uid != os.getuid() or st.st_mode & 0o077:
        sys.exit(f"{path}: no es una carpeta propia con permisos 700; no se usa")
    return path


def load():
    try:
        return json.load(open(STATE))
    except (OSError, ValueError):
        return {}


def save(st):
    private_dir(STATE_DIR)
    json.dump(st, open(STATE, "w"))


def alive(pid):
    try:
        os.kill(pid, 0)
        return True
    except (OSError, TypeError):
        return False


def stop(pid, sig=signal.SIGTERM):
    """Stops a process; SIGKILL if it is still there after 2 s."""
    if not alive(pid):
        return
    os.kill(pid, sig)
    for _ in range(20):
        time.sleep(0.1)
        if not alive(pid):
            return
    os.kill(pid, signal.SIGKILL)


def env(local_libs=True):
    e = dict(os.environ, DISPLAY=DISPLAY)
    e.pop("WAYLAND_DISPLAY", None)
    if local_libs:
        e["LD_LIBRARY_PATH"] = LOCAL_LIBS
    else:
        e.pop("LD_LIBRARY_PATH", None)
    return e


def need(*progs):
    falta = [p for p in progs if not shutil.which(p)]
    if falta:
        sys.exit("falta: " + ", ".join(falta) + " (ver ~/.local/opt/rdp-agente: apt-get download + dpkg -x)")


def xdo(*args):
    need("xdotool")
    subprocess.run(["xdotool", *map(str, args)], env=env(), check=True)


def real(st, x, y):
    """Coordenadas de la última captura (quizá escalada) a las de la pantalla remota."""
    k = st.get("escala", 1.0)
    return round(int(x) / k), round(int(y) / k)


def parse_size(s):
    w, h = (int(x) for x in s.lower().split("x"))
    if not (320 <= w <= MAX_W and 200 <= h <= MAX_H):
        sys.exit(f"tamaño fuera de rango (máximo {MAX_W}x{MAX_H})")
    return w, h


def server_size():
    """La resolución de pantalla del servidor, preguntada por el puente WinRM (None si no está arrancado)."""
    sys.path.insert(0, os.path.join(HERE, "..", "winrm"))
    try:
        import ps
        r = ps.call("$v = Get-CimInstance Win32_VideoController | Where-Object CurrentHorizontalResolution | "
                    "Select-Object -First 1; \"$($v.CurrentHorizontalResolution)x$($v.CurrentVerticalResolution)\"",
                    "resolución de pantalla")
        return parse_size(r["out"].strip()) if r.get("code") == 0 else None
    except (SystemExit, ValueError, OSError):
        return None


def rdp_window(st):
    r = subprocess.run(["xdotool", "search", "--onlyvisible", "--pid", str(st.get("rdp", 0))],
                       env=env(), capture_output=True, text=True)
    ids = r.stdout.split()
    return ids[0] if ids else None


def cmd_inicia(a):
    need("Xvfb")
    if not os.access(FREERDP, os.X_OK):
        sys.exit("falta FreeRDP: ejecuta connectors/instala.sh")
    if a.tamaño == "auto":
        size = server_size()
        print(f"Resolución del servidor: {size[0]}x{size[1]}" if size else
              f"Sin puente WinRM: no sé la resolución del servidor; uso {DEFAULT_SIZE}")
        w, h = size or parse_size(DEFAULT_SIZE)
    else:
        w, h = parse_size(a.tamaño)
    st = load()
    if not alive(st.get("xvfb")):
        p = subprocess.Popen(["Xvfb", DISPLAY, "-screen", "0", f"{MAX_W}x{MAX_H}x24", "-nolisten", "tcp"],
                             stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        time.sleep(0.8)
        st["xvfb"] = p.pid
    st.update(ancho=w, alto=h)
    save(st)
    os.makedirs(os.path.join(EXCHANGE, "grabaciones"), exist_ok=True)
    if os.path.exists(KBD_PIPE):  # FreeRDP creates it; a stale one makes it fail
        os.remove(KBD_PIPE)
    kbd = f"/kbd:unicode:on,pipe:{KBD_PIPE}" if a.teclado == "unicode" else "/kbd:unicode:off"
    st["teclado"] = a.teclado
    save(st)
    cmdline = [FREERDP, kbd, f"/v:{a.host}", f"/u:{a.usuario}", "/from-stdin:force", f"/size:{w}x{h}",
               "/sec:nla", "/tls:seclevel:2", "/gfx:AVC444", "/network:auto",  # modern only, no fallbacks
               "+clipboard", f"/drive:intercambio,{EXCHANGE}", "+dynamic-resolution", "/log-level:WARN"]
    if a.dominio:
        cmdline.append(f"/d:{a.dominio}")
    if a.acepta_certificado:  # only for test servers: no certificate check
        cmdline.append("/cert:ignore")
    print(f"Sesión de {w}x{h} en la pantalla virtual {DISPLAY}. FreeRDP te pedirá la contraseña aquí.")
    st["rdp"] = os.getpid()  # FreeRDP sustituye a este proceso: mismo pid
    save(st)
    os.execve(FREERDP, cmdline, env(local_libs=False))


def cmd_tamaño(a):
    w, h = parse_size(a.tamaño)
    st = load()
    win = rdp_window(st)
    if not win:
        sys.exit("no encuentro la ventana de FreeRDP (¿está iniciada la sesión?)")
    xdo("windowsize", "--sync", win, w, h)
    st.update(ancho=w, alto=h)
    save(st)
    print(f"{w}x{h}")


def cmd_captura(a):
    need("import")
    st = load()
    out = a.fichero or os.path.join(STATE_DIR, "captura.png")
    raw = out + ".raw.png"
    w, h = st.get("ancho", MAX_W), st.get("alto", MAX_H)
    subprocess.run(["import", "-display", DISPLAY, "-window", "root", "-crop", f"{w}x{h}+0+0", "+repage", raw],
                   check=True)
    if a.escala != 1.0:
        subprocess.run(["convert", raw, "-resize", f"{a.escala * 100:.0f}%", out], check=True)
        os.remove(raw)
    else:
        os.replace(raw, out)
    st["escala"] = a.escala
    save(st)
    print(out)


def cmd_clic(a):
    st = load()
    x, y = real(st, a.x, a.y)
    boton = "3" if a.derecho else "1"
    xdo("mousemove", "--sync", x, y)
    xdo("click", *(["--repeat", "2", "--delay", "80"] if a.doble else []), boton)


def cmd_mueve(a):
    x, y = real(load(), a.x, a.y)
    xdo("mousemove", "--sync", x, y)


def cmd_arrastra(a):
    st = load()
    x1, y1 = real(st, a.x1, a.y1)
    x2, y2 = real(st, a.x2, a.y2)
    xdo("mousemove", "--sync", x1, y1)
    xdo("mousedown", "1")
    xdo("mousemove", "--sync", x2, y2)
    xdo("mouseup", "1")


def cmd_rueda(a):
    xdo("click", "--repeat", a.n, "4" if a.sentido == "arriba" else "5")


def cmd_escribe(a):
    st = load()
    if st.get("teclado") == "unicode" and alive(st.get("rdp")) and os.path.exists(KBD_PIPE):
        fd = os.open(KBD_PIPE, os.O_WRONLY | os.O_NONBLOCK)
        try:
            os.write(fd, a.texto.encode("utf-8"))
        finally:
            os.close(fd)
    else:
        xdo("type", "--delay", "25", "--", a.texto)


def cmd_tecla(a):
    xdo("key", "--delay", "60", *a.teclas)


def cmd_graba(a):
    st = load()
    if a.accion == "inicio":
        if alive(st.get("ffmpeg")):
            sys.exit("ya se está grabando")
        out = os.path.join(EXCHANGE, "grabaciones", time.strftime("rdp-%Y%m%d-%H%M%S.mp4"))
        p = subprocess.Popen(["ffmpeg", "-loglevel", "error", "-f", "x11grab", "-framerate", "10",
                              "-video_size", f"{st.get('ancho', 1600)}x{st.get('alto', 900)}", "-i", DISPLAY,
                              "-pix_fmt", "yuv420p", out], env=env(), stdin=subprocess.PIPE, start_new_session=True)
        st.update(ffmpeg=p.pid, video=out)
        save(st)
        print(out)
    else:
        if alive(st.get("ffmpeg")):
            os.kill(st["ffmpeg"], signal.SIGINT)
        print(st.pop("video", ""))
        st.pop("ffmpeg", None)
        save(st)


def cmd_estado(a):
    st = load()
    print(json.dumps({"pantalla": DISPLAY, "xvfb": alive(st.get("xvfb")), "rdp": alive(st.get("rdp")),
                      "grabando": alive(st.get("ffmpeg")),
                      "tamaño": [st.get("ancho"), st.get("alto")], "intercambio": EXCHANGE}, ensure_ascii=False))


def cmd_para(a):
    st = load()
    for k in ("ffmpeg", "rdp", "xvfb"):
        stop(st.get(k), signal.SIGINT if k == "ffmpeg" else signal.SIGTERM)
    save({})


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sp = ap.add_subparsers(dest="orden", required=True)
    p = sp.add_parser("inicia"); p.add_argument("--host", required=True); p.add_argument("--usuario", required=True)
    p.add_argument("--dominio"); p.add_argument("--tamaño", default="auto")
    p.add_argument("--acepta-certificado", action="store_true", help="no comprobar el certificado (solo servidores de prueba)")
    p.add_argument("--teclado", choices=["unicode", "teclas"], default="unicode",
                   help="unicode: el texto llega tal cual (acentos, ñ, €; servidores Windows); teclas: códigos de tecla (solo ASCII)")
    p = sp.add_parser("tamaño"); p.add_argument("tamaño")
    p = sp.add_parser("captura"); p.add_argument("fichero", nargs="?"); p.add_argument("--escala", type=float, default=1.0)
    p = sp.add_parser("clic"); p.add_argument("x"); p.add_argument("y")
    p.add_argument("--doble", action="store_true"); p.add_argument("--derecho", action="store_true")
    p = sp.add_parser("mueve"); p.add_argument("x"); p.add_argument("y")
    p = sp.add_parser("arrastra"); [p.add_argument(n) for n in ("x1", "y1", "x2", "y2")]
    p = sp.add_parser("rueda"); p.add_argument("sentido", choices=["arriba", "abajo"]); p.add_argument("n", nargs="?", default="3")
    p = sp.add_parser("escribe"); p.add_argument("texto")
    p = sp.add_parser("tecla"); p.add_argument("teclas", nargs="+")
    p = sp.add_parser("graba"); p.add_argument("accion", choices=["inicio", "fin"])
    sp.add_parser("estado"); sp.add_parser("para")
    a = ap.parse_args()
    private_dir(STATE_DIR)  # before anything reads or writes there
    globals()["cmd_" + a.orden](a)


if __name__ == "__main__":
    main()
