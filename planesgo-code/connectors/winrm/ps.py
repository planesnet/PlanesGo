#!/usr/bin/env python3
"""PowerShell remoto (WinRM) para un agente, sin que el agente vea la contraseña.

La persona arranca el puente en un terminal y escribe allí la contraseña; queda solo en la memoria de ese proceso,
que atiende las órdenes por un socket local privado (permisos 600). Cada orden queda en auditoria.log (la orden,
no su salida).

  ps.py puente --host 192.168.125.229 --usuario DOMINIO\\usuario [--https [--ca ca.pem]]   (pide la contraseña)
  ps.py "Get-ChildItem 'C:\\Program Files'"                        una orden PowerShell
  ps.py -f script.ps1                                              un script
  ps.py baja 'C:\\ruta\\remota.log' ./local.log                     traer un fichero
  ps.py sube ./local.txt 'C:\\ruta\\remota.txt'                     llevar un fichero
"""
import base64
import getpass
import json
import os
import socket
import socketserver
import stat
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
RUN = os.path.join(os.environ.get("XDG_RUNTIME_DIR") or sys.exit("falta XDG_RUNTIME_DIR"), "rdp-agente")
SOCK = os.path.join(RUN, "winrm.sock")
AUDIT = os.path.expanduser("~/.local/share/pgocode-connectors/auditoria.log")  # data out of the code
VENV_PY = os.path.expanduser("~/.local/opt/rdp-agente/venv/bin/python")  # pywinrm (instala.sh)
CHUNK = 192 * 1024
UTF8 = "[Console]::OutputEncoding = [Text.Encoding]::UTF8; $ProgressPreference = 'SilentlyContinue'\n"


def private_dir(path):
    """Creates (if needed) and checks a runtime folder: ours and 700. No /tmp fallback: there another user could
    create it first and plant a socket or a pipe."""
    os.makedirs(path, mode=0o700, exist_ok=True)
    st = os.lstat(path)
    if not stat.S_ISDIR(st.st_mode) or st.st_uid != os.getuid() or st.st_mode & 0o077:
        sys.exit(f"{path}: no es una carpeta propia con permisos 700; no se usa")
    return path


def serve(host, user, transport, https=False, ca=None):
    try:
        import winrm  # only the bridge needs it
    except ImportError:
        if os.path.exists(VENV_PY) and sys.executable != VENV_PY:
            os.execv(VENV_PY, [VENV_PY, "-I", os.path.abspath(__file__), *sys.argv[1:]])
        sys.exit("falta pywinrm (instala.sh)")
    os.makedirs(os.path.dirname(AUDIT), exist_ok=True)
    pw = getpass.getpass(f"Contraseña de {user} en {host}: ")
    # HTTPS (5986) validating the server certificate when the server has it; on HTTP (5985) NTLM still encrypts
    # every message.
    url = f"https://{host}:5986/wsman" if https else f"http://{host}:5985/wsman"
    extra = {"server_cert_validation": "validate", **({"ca_trust_path": ca} if ca else {})} if https else {}
    s = winrm.Session(url, auth=(user, pw), transport=transport, **extra,
                      read_timeout_sec=630, operation_timeout_sec=600)
    del pw
    r = s.run_ps(UTF8 + "\"$env:COMPUTERNAME ($env:USERDOMAIN\\$env:USERNAME)\"")
    if r.status_code:
        sys.exit("no conecta: " + r.std_err.decode("utf-8", "replace")[:500])
    print("Conectado a", r.std_out.decode("utf-8", "replace").strip(), "· socket", SOCK)

    class Handler(socketserver.StreamRequestHandler):
        def handle(self):
            req = json.loads(self.rfile.readline())
            with open(AUDIT, "a") as f:
                f.write(time.strftime("%Y-%m-%d %H:%M:%S ") + json.dumps(req.get("audit", req["ps"][:2000]), ensure_ascii=False) + "\n")
            try:
                r = s.run_ps(UTF8 + req["ps"])
                out = {"code": r.status_code, "out": r.std_out.decode("utf-8", "replace"),
                       "err": r.std_err.decode("utf-8", "replace")}
            except Exception as e:  # noqa: BLE001 - the error goes back to the client
                out = {"code": -1, "out": "", "err": repr(e)}
            self.wfile.write(json.dumps(out).encode() + b"\n")

    private_dir(RUN)
    if os.path.exists(SOCK):
        os.remove(SOCK)
    old = os.umask(0o177)
    srv = socketserver.UnixStreamServer(SOCK, Handler)
    os.umask(old)
    try:
        srv.serve_forever()
    finally:
        os.remove(SOCK)


def call(ps, audit=None):
    private_dir(RUN)
    c = socket.socket(socket.AF_UNIX)
    try:
        c.connect(SOCK)
    except OSError:
        sys.exit("el puente no está arrancado (ps.py puente --host … --usuario …)")
    c.sendall(json.dumps({"ps": ps, **({"audit": audit} if audit else {})}).encode() + b"\n")
    buf = b""
    while not buf.endswith(b"\n"):
        d = c.recv(1 << 20)
        if not d:
            break
        buf += d
    return json.loads(buf)


def q(path):
    return "'" + path.replace("'", "''") + "'"


def download(remote, local):
    size = int(call(f"(Get-Item -LiteralPath {q(remote)}).Length", f"baja {remote}")["out"].strip())
    with open(local, "wb") as f:
        for off in range(0, size, CHUNK):
            r = call(f"$fs=[IO.File]::OpenRead({q(remote)}); $fs.Seek({off},0)|Out-Null; $b=New-Object byte[] {min(CHUNK, size - off)};"
                     f" $n=$fs.Read($b,0,$b.Length); $fs.Close(); [Convert]::ToBase64String($b,0,$n)", f"baja {remote} @{off}")
            f.write(base64.b64decode(r["out"].strip()))
    print(f"{local} ({size} bytes)")


def upload(local, remote):
    data = open(local, "rb").read()
    call(f"[IO.File]::WriteAllBytes({q(remote)}, [byte[]]@())", f"sube {remote}")
    for off in range(0, len(data), CHUNK):
        b64 = base64.b64encode(data[off:off + CHUNK]).decode()
        r = call(f"$b=[Convert]::FromBase64String('{b64}'); $fs=[IO.File]::Open({q(remote)},'Append'); $fs.Write($b,0,$b.Length); $fs.Close()",
                 f"sube {remote} @{off}")
        if r["code"]:
            sys.exit(r["err"])
    print(f"{remote} ({len(data)} bytes)")


def main():
    a = sys.argv[1:]
    if not a or a[0] in ("-h", "--help"):
        print(__doc__)
        return
    if a[0] == "puente":
        import argparse
        p = argparse.ArgumentParser()
        p.add_argument("--host", required=True)
        p.add_argument("--usuario", required=True)
        p.add_argument("--transporte", default="ntlm")
        p.add_argument("--https", action="store_true", help="WinRM por HTTPS (5986), validando el certificado")
        p.add_argument("--ca", help="certificado de la CA del servidor (con --https)")
        o = p.parse_args(a[1:])
        serve(o.host, o.usuario, o.transporte, o.https, o.ca)
    elif a[0] == "baja":
        download(a[1], a[2])
    elif a[0] == "sube":
        upload(a[1], a[2])
    else:
        ps = open(a[1]).read() if a[0] == "-f" else " ".join(a)
        r = call(ps)
        sys.stdout.write(r["out"])
        sys.stderr.write(r["err"])
        sys.exit(r["code"])


if __name__ == "__main__":
    main()
