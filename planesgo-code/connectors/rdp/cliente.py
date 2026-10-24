#!/usr/bin/env python3
"""Cliente de escritorio remoto (RDP) para Windows, con ventana normal en el escritorio.

Pantalla de inicio de sesión (zenity): conexiones guardadas o una nueva (equipo, usuario, dominio) y opciones
por conexión: portapapeles, impresoras, sonido, micrófono, carpeta compartida y pantalla completa. La contraseña
se pide en su propio diálogo y pasa del diálogo a FreeRDP por una tubería: este programa no la lee ni la guarda.

La ventana se puede redimensionar y la sesión de Windows se adapta a su tamaño. Ctrl+Alt+Intro: pantalla completa.
Las conexiones (sin contraseña) se guardan en ~/.config/pgocode-connectors/conexiones.json.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

# FreeRDP built from source by instala.sh (latest upstream release; SDL3 client: native Wayland, HiDPI)
FREERDP = os.path.expanduser("~/.local/opt/rdp-agente/freerdp")
CLIENT = os.path.join(FREERDP, "bin", "sdl-freerdp")
CONFIG = os.path.expanduser("~/.config/pgocode-connectors/conexiones.json")
SHARED = os.path.expanduser("~/.local/share/pgocode-connectors/intercambio")
TITLE = "Escritorio remoto"
NEW = "➕ Nueva conexión…"

# (key, label, on by default)
OPTIONS = [
    ("portapapeles", "Portapapeles (copiar y pegar texto y ficheros)", True),
    ("impresoras", "Impresoras de este equipo", True),
    ("sonido", "Sonido", True),
    ("microfono", "Micrófono", False),
    ("carpeta", f"Carpeta compartida ({SHARED.replace(os.path.expanduser('~'), '~')})", True),
    ("completa", "Pantalla completa", False),
]


def zenity(*args, stdin=None):
    """Runs a zenity dialog; returns its output, or None if it was cancelled."""
    r = subprocess.run(["zenity", f"--title={TITLE}", *args], input=stdin, capture_output=True, text=True)
    return r.stdout.rstrip("\n") if r.returncode == 0 else None


def error(msg):
    subprocess.run(["zenity", f"--title={TITLE}", "--error", "--no-markup", "--width=420", f"--text={msg}"])


def load():
    try:
        return json.load(open(CONFIG))
    except (OSError, ValueError):
        return []


def save(conns):
    os.makedirs(os.path.dirname(CONFIG), mode=0o700, exist_ok=True)
    with open(CONFIG, "w") as f:
        json.dump(conns, f, ensure_ascii=False, indent=1)


def choose(conns):
    rows = []
    for c in conns:
        rows += [c["nombre"], c["equipo"], (c["dominio"] + "\\" if c.get("dominio") else "") + c["usuario"]]
    rows += [NEW, "", ""]
    pick = zenity("--list", "--width=560", "--height=360", "--text=Elige una conexión:",
                  "--column=Conexión", "--column=Equipo", "--column=Usuario", *rows)
    if pick is None:
        return None
    pick = pick.split("|")[0]
    if pick in ("", NEW):
        return edit(None)
    return next(c for c in conns if c["nombre"] == pick)


def edit(c):
    c = dict(c or {"nombre": "", "equipo": "", "usuario": "", "dominio": "", "opciones": {}})
    out = zenity("--forms", "--width=420", "--text=Datos de la conexión (la contraseña se pide al conectar)",
                 "--separator=\x1f", "--add-entry=Nombre (para la lista)", "--add-entry=Equipo (nombre o IP[:puerto])",
                 "--add-entry=Usuario", "--add-entry=Dominio (opcional)")
    if out is None:
        return None
    name, host, user, domain = (out.split("\x1f") + [""] * 4)[:4]
    if not host.strip() or not user.strip():
        error("Hacen falta el equipo y el usuario.")
        return edit(c)
    c.update(nombre=name.strip() or host.strip(), equipo=host.strip(), usuario=user.strip(), dominio=domain.strip())
    return c


def options(c):
    cur = c.get("opciones") or {}
    rows = []
    for key, label, default in OPTIONS:
        rows += ["TRUE" if cur.get(key, default) else "FALSE", label]
    out = zenity("--list", "--checklist", "--width=500", "--height=420", f"--text=Opciones para {c['nombre']}:",
                 "--column=", "--column=Opción", "--separator=\x1f", *rows)
    if out is None:
        return None
    chosen = set(out.split("\x1f")) if out else set()
    c["opciones"] = {key: label in chosen for key, label, _ in OPTIONS}
    return c


def freerdp_args(c):
    """Modern settings only: no fallbacks for old servers (this is a development tool, not a product)."""
    o = c["opciones"]
    a = [CLIENT, f"/v:{c['equipo']}", f"/u:{c['usuario']}", "/from-stdin:force", f"/title:{c['nombre']}",
         "/sec:nla",           # network level authentication only: no classic RDP security, no TLS without NLA
         "/tls:seclevel:2",    # OpenSSL security level 2: no old TLS versions or weak keys
         "/cert:tofu",         # trust on first use; warn if the server certificate changes
         "/gfx:AVC444",        # RDP 8+ graphics with H.264 4:4:4
         "/network:auto", "+auto-reconnect", "/auto-reconnect-max-retries:20", "+dynamic-resolution",
         "/log-level:ERROR"]
    if c.get("dominio"):
        a.append(f"/d:{c['dominio']}")
    a.append("+f" if o.get("completa") else "/size:85%")
    a.append("+clipboard" if o.get("portapapeles") else "-clipboard")
    if o.get("impresoras"):
        a.append("/printer")
    if o.get("sonido"):
        a.append("/sound:sys:pulse")  # PulseAudio / PipeWire
    if o.get("microfono"):
        a.append("/microphone:sys:pulse")
    if o.get("carpeta"):
        os.makedirs(SHARED, exist_ok=True)
        a.append(f"/drive:Intercambio,{SHARED}")
    return a


# FreeRDP's ERRCONNECT_* codes (in its log) -> what to tell the person
MESSAGES = [
    (("LOGON_FAILURE", "WRONG_PASSWORD", "AUTHENTICATION_FAILED", "NO_OR_MISSING_CREDENTIALS"),
     "Usuario o contraseña incorrectos."),
    (("PASSWORD_EXPIRED", "PASSWORD_CERTAINLY_EXPIRED", "PASSWORD_MUST_CHANGE"),
     "La contraseña ha caducado o hay que cambiarla: cámbiala desde Windows."),
    (("ACCOUNT_LOCKED_OUT",), "La cuenta está bloqueada (demasiados intentos fallidos)."),
    (("ACCOUNT_DISABLED", "ACCOUNT_EXPIRED", "ACCOUNT_RESTRICTION"), "La cuenta está deshabilitada, caducada o restringida."),
    (("ACCESS_DENIED", "INSUFFICIENT_PRIVILEGES", "LOGON_TYPE_NOT_GRANTED"),
     "Este usuario no tiene permiso para entrar por escritorio remoto en ese equipo."),
    (("DNS_ERROR", "DNS_NAME_NOT_FOUND"), "No se encuentra el equipo: revisa el nombre."),
    (("CONNECT_FAILED", "CONNECT_TRANSPORT_FAILED", "TARGET_BOOTING"),
     "No se puede llegar al equipo: revisa el nombre o la IP y que el escritorio remoto esté activo."),
    (("TLS_CONNECT_FAILED", "SECURITY_NEGO_CONNECT_FAILED", "HYBRID_REQUIRED_BY_SERVER"),
     "No se ha podido negociar la seguridad de la conexión con el servidor."),
    (("KDC_UNREACHABLE",), "No se llega al controlador de dominio (Kerberos)."),
]


def explain(log, code):
    t = log.upper()
    if "CERTIFICATE" in t and ("CHANGED" in t or "MISMATCH" in t):
        return ("El certificado del servidor ha cambiado desde la última conexión. Puede ser normal (se renovó) "
                "o un intento de suplantación: compruébalo con quien administra el servidor.")
    for keys, msg in MESSAGES:
        if any("ERRCONNECT_" + k in t for k in keys):
            return msg
    # Kerberos without a configured realm is tried first and is harmless (NTLM follows): not part of the error
    lines = [l for l in log.strip().splitlines() if "default realm" not in l]
    tail = "\n".join(lines[-6:])
    return f"La conexión terminó con un error (código {code}).\n\n{tail}"


def connect(c):
    # Password: from the dialog straight into FreeRDP's stdin (this process never reads it).
    who = (c["dominio"] + "\\" if c.get("dominio") else "") + c["usuario"]
    pw = subprocess.Popen(["zenity", f"--title={TITLE}", "--entry", "--hide-text", "--width=380",
                           f"--text=Contraseña de {who} en {c['nombre']} ({c['equipo']}):"], stdout=subprocess.PIPE)
    with tempfile.TemporaryFile(mode="w+") as log:
        rdp = subprocess.Popen(freerdp_args(c), stdin=pw.stdout, stdout=log, stderr=subprocess.STDOUT)
        pw.stdout.close()
        if pw.wait() != 0:  # cancelled: FreeRDP gets no password
            rdp.terminate()
            rdp.wait()
            return
        code = rdp.wait()
        log.seek(0)
        text = log.read()
    # 0: closed normally; 1/2/12/13: the user or Windows ended the session (logoff, disconnect); cancelled: the
    # person closed it while connecting
    if code not in (0, 1, 2, 12, 13) and "ERRCONNECT_CONNECT_CANCELLED" not in text:
        error(explain(text, code))


def main():
    if not shutil.which("zenity"):
        sys.exit("falta zenity")
    if not os.access(CLIENT, os.X_OK):
        sys.exit("falta FreeRDP: ejecuta connectors/instala.sh")
    conns = load()
    c = choose(conns)
    if c is None:
        return
    c = options(c)
    if c is None:
        return
    conns = [x for x in conns if x["nombre"] != c["nombre"]] + [c]
    save(sorted(conns, key=lambda x: x["nombre"].lower()))
    connect(c)


if __name__ == "__main__":
    main()
