#!/usr/bin/env python3
"""
Planes Software Factory (PSF) - Hooks de PlanesGo para Claude Code
Equivalente para Claude Code de planesgo_guard.py + planesgo_heartbeat_hook.py (Antigravity).

Modos (argv[1]):
  session-start  SessionStart      -> check en segundo plano o aviso de proyecto no vinculado
  prompt         UserPromptSubmit  -> recuerda al agente que pregunte y verifique el proyecto mientras no esté vinculado
  guard          PreToolUse        -> bloquea Edit/Write/NotebookEdit/Bash en proyectos sin .planesgo.json válido
  track          PostToolUse       -> latidos de imputación (el primero abre el cronómetro)
  session-end    SessionEnd        -> cierra e imputa los cronómetros abiertos en la sesión

Contrato de no interferencia: ante cualquier error inesperado el hook termina en 0 sin salida.
"""
import sys
import json
import os
import re
import shutil
import subprocess
import time

HOME = os.path.expanduser("~")
TASK_NAME = "Claude Code - Sesión IA"
TASK_TYPE = "Desarrollo"
DESC = "Trabajo de codificación asistido con Claude Code"
# El backend distingue Claude de Antigravity por este valor
AI_MODEL_LABEL = "Claude Code"
STATE_ROOT = "/tmp/planesgo-claude"

EXEMPT_PREFIXES = (
    os.path.join(HOME, ".claude"),
    os.path.join(HOME, ".gemini"),
    "/tmp",
    "/var/tmp",
)

SAFE_BINS = {
    "planesgo-mcp", "planesgo-track", "ls", "pwd", "grep", "find",
    "cat", "head", "tail", "echo", "which", "test", "true", "false",
    "uname", "id", "whoami", "ps", "env", "printenv",
}
SAFE_GIT = {"status", "log", "diff", "branch", "show", "rev-parse"}
SHELL_META = re.compile(r"[;&|<>`]|\$\(")


def is_exempt(path):
    p = os.path.abspath(path)
    return p == HOME or p == "/" or any(p == e or p.startswith(e + os.sep) for e in EXEMPT_PREFIXES)


def git_root(path):
    """Raíz git que contiene path, sin ascender al home ni a /."""
    curr = os.path.abspath(path)
    while curr and curr not in ("/", HOME):
        if os.path.exists(os.path.join(curr, ".git")):
            return curr
        parent = os.path.dirname(curr)
        if parent == curr:
            break
        curr = parent
    return None


def load_config(root):
    """.planesgo.json estrictamente en la raíz del proyecto (sin padres ni hijos)."""
    cand = os.path.join(root, ".planesgo.json")
    try:
        with open(cand, "r", encoding="utf-8") as f:
            cfg = json.load(f)
        if int(cfg.get("odoo_project_id", 0) or 0) > 0:
            return cfg
    except Exception:
        pass
    return None


def project_root_for_dir(d):
    """Raíz del proyecto para un directorio de trabajo: el propio dir si tiene .planesgo.json, si no la raíz git, si no el dir."""
    if not d or is_exempt(d):
        return None
    d = os.path.abspath(d)
    if os.path.isfile(os.path.join(d, ".planesgo.json")):
        return d
    return git_root(d) or d


def project_root_for_file(path, session_dir):
    """Raíz del proyecto al que pertenece un fichero, o None si no pertenece a ninguno."""
    if not path or is_exempt(path):
        return None
    p = os.path.abspath(path)
    root = git_root(os.path.dirname(p))
    if root:
        return root
    sd = project_root_for_dir(session_dir)
    if sd and (p == sd or p.startswith(sd + os.sep)):
        return sd
    return None


def has_token():
    # PLANESGO_TOKEN es el nombre vigente; ANTIGRAVITY_TOKEN se mantiene por compatibilidad
    if os.environ.get("PLANESGO_TOKEN") or os.environ.get("ANTIGRAVITY_TOKEN"):
        return True
    try:
        with open(os.path.join(HOME, ".planesgo_auth.json"), "r", encoding="utf-8") as f:
            return bool(json.load(f).get("antigravity_token"))
    except Exception:
        return False


def find_bin():
    # En Claude Code en la web el binario suele compilarse dentro del repo (scripts/build_mcp.sh)
    project = os.environ.get("CLAUDE_PROJECT_DIR", "")
    candidates = [shutil.which("planesgo-mcp"), os.path.join(HOME, ".local/bin/planesgo-mcp")]
    if project:
        candidates += [os.path.join(project, "bin/planesgo-mcp"), os.path.join(project, "planesgo-mcp")]
    for cand in candidates:
        if cand and os.access(cand, os.X_OK):
            return cand
    return None


def unlinked_instructions(root):
    return (
        f"El directorio '{root}' no está vinculado a ningún proyecto de Odoo "
        "(falta .planesgo.json en su raíz o tiene odoo_project_id 0), así que no hay imputación horaria "
        "y las ediciones y comandos de desarrollo están bloqueados. Antes de trabajar:\n"
        "1. Pregunta al usuario el nombre exacto del proyecto de Odoo. No lo asumas ni lo deduzcas del "
        "nombre de la carpeta, y nunca uses PLANESGO por defecto.\n"
        "2. Verifícalo con la herramienta MCP `mcp__planesgo__planesgo_set_project` con "
        f'{{"project_name": "<nombre>", "project_path": "{root}"}} '
        f'(alternativa CLI: `planesgo-mcp --set-project "<nombre>" --path "{root}"`).\n'
        "3. Si responde que el proyecto no existe, díselo al usuario y vuelve a preguntarle. "
        "Repite hasta que la vinculación se confirme; solo entonces continúa con la tarea."
    )


def emit_context(event, text):
    print(json.dumps({"hookSpecificOutput": {"hookEventName": event, "additionalContext": text}}))


def deny(reason):
    print(json.dumps({"hookSpecificOutput": {
        "hookEventName": "PreToolUse",
        "permissionDecision": "deny",
        "permissionDecisionReason": reason,
    }}))


def session_state_dir(payload):
    sid = re.sub(r"[^A-Za-z0-9_-]", "", str(payload.get("session_id") or "default")) or "default"
    return os.path.join(STATE_ROOT, sid)


def session_dir(payload):
    return os.environ.get("CLAUDE_PROJECT_DIR") or payload.get("cwd") or os.getcwd()


def run_bin(args, background=False):
    binp = find_bin()
    if not binp:
        return False
    cmd = [binp] + args + ["--task", TASK_NAME, "--type", TASK_TYPE, "--model", AI_MODEL_LABEL]
    if background:
        subprocess.Popen(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        return True
    try:
        return subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=20).returncode == 0
    except Exception:
        return False


def on_context(payload, event):
    root = project_root_for_dir(session_dir(payload))
    if not root:
        return
    if not has_token():
        emit_context(event, (
            "No hay token de empleado para la imputación horaria (~/.planesgo_auth.json o $PLANESGO_TOKEN). "
            "Avisa al usuario de que debe generarlo en https://planesgo.autopyme.com/settings "
            "y guardarlo en ~/.planesgo_auth.json antes de trabajar en este proyecto."
        ))
        return
    cfg = load_config(root)
    if not cfg:
        emit_context(event, unlinked_instructions(root))
        return
    if event == "SessionStart":
        run_bin(["--check", "--path", root], background=True)
        emit_context(event, (
            f"Imputación horaria activa: proyecto '{cfg.get('odoo_project_name', '')}', "
            f"tarea '{TASK_NAME}', tipo '{TASK_TYPE}'. Los latidos se envían automáticamente."
        ))


def is_safe_command(cmd):
    cmd = (cmd or "").strip()
    if not cmd:
        return True
    parts = cmd.split()
    base = os.path.basename(parts[0])
    if base in ("planesgo-mcp", "planesgo-track"):
        return True
    if SHELL_META.search(cmd):
        return False
    if base in SAFE_BINS:
        return True
    return base == "git" and len(parts) > 1 and parts[1] in SAFE_GIT


def target_root(payload):
    """Proyecto afectado por la herramienta, o None si no afecta a ningún proyecto."""
    name = payload.get("tool_name", "")
    args = payload.get("tool_input") or {}
    sdir = session_dir(payload)
    if name in ("Edit", "Write", "NotebookEdit", "MultiEdit"):
        path = args.get("file_path") or args.get("notebook_path") or ""
        if os.path.basename(path) in (".planesgo.json", ".geminiignore"):
            return None
        return project_root_for_file(path, sdir)
    if name == "Bash":
        return project_root_for_dir(payload.get("cwd") or sdir)
    return None


def on_guard(payload):
    if payload.get("tool_name") == "Bash" and is_safe_command((payload.get("tool_input") or {}).get("command")):
        return
    root = target_root(payload)
    if not root or load_config(root):
        return
    deny("Bloqueo PSF: proyecto no vinculado a Odoo.\n" + unlinked_instructions(root))


def heartbeat_interval(cfg):
    # set-project escribe 0 cuando no se define: se interpreta como valor por defecto
    try:
        interval = int(cfg.get("heartbeat_interval_seconds") or 30)
    except Exception:
        interval = 30
    return interval if interval > 0 else 30


def on_track(payload):
    root = target_root(payload)
    if not root:
        return
    cfg = load_config(root)
    if not cfg:
        return
    state = os.path.join(session_state_dir(payload), str(int(cfg["odoo_project_id"])))
    os.makedirs(state, exist_ok=True)
    active = os.path.join(state, "active")
    last_file = os.path.join(state, "last_beat")
    now = time.time()
    if not os.path.isfile(active):
        # Primer latido en primer plano: solo se marca activo si Odoo confirma el cronómetro,
        # así el stop final nunca queda huérfano
        if run_bin(["--beat", "--path", root, "--desc", DESC]):
            with open(active, "w", encoding="utf-8") as f:
                f.write(root)
            with open(last_file, "w", encoding="utf-8") as f:
                f.write(str(now))
        return
    try:
        with open(last_file, "r", encoding="utf-8") as f:
            last = float(f.read().strip())
    except Exception:
        last = 0.0
    if now - last >= heartbeat_interval(cfg):
        with open(last_file, "w", encoding="utf-8") as f:
            f.write(str(now))
        run_bin(["--beat", "--path", root, "--desc", DESC], background=True)


def on_session_end(payload):
    sdir = session_state_dir(payload)
    if not os.path.isdir(sdir):
        return
    for proj in os.listdir(sdir):
        active = os.path.join(sdir, proj, "active")
        try:
            with open(active, "r", encoding="utf-8") as f:
                root = f.read().strip()
        except Exception:
            continue
        run_bin(["--stop", "--path", root, "--desc", "Sesión Claude Code finalizada"])
    shutil.rmtree(sdir, ignore_errors=True)


GLOBAL_HOOK = os.path.join(HOME, ".claude/hooks/planesgo_claude_hook.py")


def shadowed_by_global_hook():
    """La copia del repo (necesaria en Claude Code en la web) no actúa si ya está instalado el hook global."""
    try:
        return os.path.exists(GLOBAL_HOOK) and os.path.realpath(__file__) != os.path.realpath(GLOBAL_HOOK)
    except Exception:
        return False


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else ""
    if shadowed_by_global_hook():
        sys.exit(0)
    try:
        raw = sys.stdin.read()
        payload = json.loads(raw) if raw.strip() else {}
    except Exception:
        payload = {}
    try:
        if mode == "session-start":
            on_context(payload, "SessionStart")
        elif mode == "prompt":
            on_context(payload, "UserPromptSubmit")
        elif mode == "guard":
            on_guard(payload)
        elif mode == "track":
            on_track(payload)
        elif mode == "session-end":
            on_session_end(payload)
    except Exception:
        pass
    sys.exit(0)


if __name__ == "__main__":
    main()
