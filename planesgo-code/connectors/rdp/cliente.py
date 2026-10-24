#!/usr/bin/env python3
"""Cliente de escritorio remoto (RDP) para Windows, con ventana normal en el escritorio.

Pantalla de inicio de sesión (zenity): conexiones guardadas o una nueva (equipo, usuario, dominio) y opciones
por conexión: portapapeles, impresoras, sonido, micrófono, carpeta compartida y pantalla completa. La contraseña
se pide en su propio diálogo y pasa del diálogo a FreeRDP por una tubería: este programa no la lee ni la guarda.

La ventana se puede redimensionar y la sesión de Windows se adapta a su tamaño. Ctrl+Alt+Intro: pantalla completa.
Las conexiones (sin contraseña) se guardan en ~/.config/pgocode-connectors/conexiones.json.

Imputación: toda conexión exige un proyecto de Odoo, que se pide al conectar (se recuerda el último). Se lanza de
forma independiente (pgocode --rdp), sin sesión de pgocode, git ni carpeta de proyecto: el tiempo va a Odoo por
PlanesGo con pgo-mcp (su token en este equipo). Mientras la sesión está conectada y hay uso del teclado o el ratón
(inactividad de GNOME), un latido cada 30 s; tras 10 minutos sin uso se cierra el parte y, al volver, se abre otro.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import threading
import time
import uuid

# FreeRDP built from source by instala.sh (latest upstream release; SDL3 client: native Wayland, HiDPI)
FREERDP = os.path.expanduser("~/.local/opt/rdp-agente/freerdp")
CLIENT = os.path.join(FREERDP, "bin", "sdl-freerdp")
CONFIG = os.path.expanduser("~/.config/pgocode-connectors/conexiones.json")
SHARED = os.path.expanduser("~/.local/share/pgocode-connectors/intercambio")
TITLE = "Escritorio remoto"
NEW = "➕ Nueva conexión…"
OTHER = "🔎 Otro proyecto…"

# Time logging through PlanesGo (pgo-mcp). PGO_MCP overrides the binary (tests).
PGO_MCP = os.environ.get("PGO_MCP") or shutil.which("pgo-mcp") or os.path.expanduser("~/.local/bin/pgo-mcp")
WORK_DIR = os.path.expanduser("~/.local/share/pgocode-connectors")  # pgo-mcp wants a folder; no project folder needed
TASK = "Escritorio remoto"
TASK_TYPE = "Desarrollo"
BEAT_EVERY = 30         # seconds between heartbeats
FIRST_BEAT_AFTER = 10   # seconds connected before the first one (a failed login logs nothing)
IDLE_LIMIT = 10 * 60    # seconds without keyboard or mouse: the part is closed

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


def pgo_mcp(*args, timeout=30):
    """Runs pgo-mcp; returns (ok, output)."""
    try:
        r = subprocess.run([PGO_MCP, *args], capture_output=True, text=True, timeout=timeout)
        return r.returncode == 0, (r.stdout + r.stderr).strip()
    except (OSError, subprocess.TimeoutExpired) as e:
        return False, str(e)


def search_projects(text):
    """[(id, name)] of the Odoo projects matching text (pgo-mcp prints «N. [id] name»)."""
    ok, out = pgo_mcp("--project", text)
    found = [(int(m[1]), m[2].strip()) for m in re.finditer(r"^\s*\d+\.\s*\[(\d+)\]\s*(.+)$", out, re.M)]
    return found, (None if ok or found else out)


def choose_project(c):
    """The Odoo project to log the time to (mandatory). The connection's last one is offered first."""
    cur = c.get("proyecto")
    if cur:
        pick = zenity("--list", "--width=520", "--height=260", "--text=Proyecto de Odoo al que se imputa el tiempo:",
                      "--column=ID", "--column=Proyecto", str(cur["id"]), cur["nombre"], "", OTHER)
        if pick is None:
            return None
        if pick.split("|")[0] == str(cur["id"]):
            return c
    text = zenity("--entry", "--width=420", "--text=Proyecto de Odoo al que se imputa el tiempo (busca por nombre):",
                  f"--entry-text={cur['nombre'] if cur else ''}")
    if text is None:
        return None
    if not text.strip():
        return choose_project(c)
    found, err = search_projects(text.strip())
    if not found:
        error(f"No se ha podido buscar en PlanesGo:\n{err}" if err else f"Ningún proyecto coincide con «{text}».")
        return choose_project(c)
    rows = [x for pid, name in found for x in (str(pid), name)]
    pick = zenity("--list", "--width=520", "--height=360", f"--text=Proyectos que coinciden con «{text}»:",
                  "--column=ID", "--column=Proyecto", *rows)
    if pick is None:
        return choose_project(c)
    pid = int(pick.split("|")[0])
    c["proyecto"] = {"id": pid, "nombre": dict(found)[pid]}
    return c


def idle_seconds():
    """Seconds without keyboard or mouse on this desktop (GNOME); None if unknown."""
    try:
        r = subprocess.run(["gdbus", "call", "--session", "--dest", "org.gnome.Mutter.IdleMonitor",
                            "--object-path", "/org/gnome/Mutter/IdleMonitor/Core",
                            "--method", "org.gnome.Mutter.IdleMonitor.GetIdletime"],
                           capture_output=True, text=True, timeout=5)
        m = re.search(r"(\d+)", r.stdout)
        return int(m[1]) / 1000 if r.returncode == 0 and m else None
    except (OSError, subprocess.TimeoutExpired):
        return None


class TimeLog(threading.Thread):
    """Logs the remote desktop time to the connection's project while FreeRDP is connected and in use."""

    def __init__(self, c, rdp):
        super().__init__(daemon=True)
        self.rdp = rdp
        self.open = False
        self.warned = False
        self.done = threading.Event()
        self.base = ["--project-id", str(c["proyecto"]["id"]), "--path", WORK_DIR, "--task", TASK,
                     "--type", TASK_TYPE, "--model", TITLE, "--session-id", "rdp-" + uuid.uuid4().hex[:12],
                     "--desc", f"Escritorio remoto en {c['nombre']} ({c['equipo']}) como {c['usuario']}"]

    def warn(self, out):
        if not self.warned:  # once; does not block the session
            self.warned = True
            subprocess.Popen(["zenity", f"--title={TITLE}", "--warning", "--no-markup", "--width=420",
                              f"--text=No se está imputando el tiempo en Odoo:\n{out}"])

    def run(self):
        start = time.monotonic()
        while not self.done.wait(BEAT_EVERY if self.open else min(5, BEAT_EVERY)):
            if self.rdp.poll() is not None or time.monotonic() - start < FIRST_BEAT_AFTER:
                continue
            idle = idle_seconds()
            if idle is None or idle < IDLE_LIMIT:
                ok, out = pgo_mcp("--beat", *self.base)
                self.open = self.open or ok
                if not ok:
                    self.warn(out)
            elif self.open:
                self.close()

    def close(self):
        if self.open:
            ok, out = pgo_mcp("--stop", *self.base)
            self.open = not ok
            if not ok:
                self.warn(out)

    def finish(self):
        self.done.set()
        self.join(timeout=5)
        self.close()


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
        timelog = TimeLog(c, rdp)
        timelog.start()
        code = rdp.wait()
        timelog.finish()
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
    if os.environ.get("PGO_EJECUCION") or os.environ.get("PLANESGO_GESTOR") == "pgocode":
        error("Dentro de pgocode la imputación la hace pgocode: abre el escritorio remoto con «pgocode --rdp».")
        return
    if not os.access(PGO_MCP, os.X_OK):
        error("Falta pgo-mcp (PlanesGo): sin él no se puede imputar el tiempo y no se conecta.")
        return
    conns = load()
    c = choose(conns)
    if c is None:
        return
    c = options(c)
    if c is None:
        return
    c = choose_project(c)  # mandatory: the time always goes to an Odoo project
    if c is None:
        return
    os.makedirs(WORK_DIR, exist_ok=True)
    ok, out = pgo_mcp("--check", "--project-id", str(c["proyecto"]["id"]), "--path", WORK_DIR, "--task", TASK)
    if not ok:
        error(f"PlanesGo no acepta imputar en ese proyecto, así que no se conecta:\n{out}")
        return
    conns = [x for x in conns if x["nombre"] != c["nombre"]] + [c]
    save(sorted(conns, key=lambda x: x["nombre"].lower()))
    connect(c)


if __name__ == "__main__":
    main()
