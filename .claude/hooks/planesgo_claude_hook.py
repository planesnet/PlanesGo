#!/usr/bin/env python3
"""
Planes Software Factory (PSF) - Hooks de PlanesGo para Claude Code
Equivalente para Claude Code de planesgo_guard.py + planesgo_heartbeat_hook.py (Antigravity).

Modos (argv[1]):
  session-start  SessionStart      -> check en segundo plano o aviso de proyecto no vinculado
  prompt         UserPromptSubmit  -> recuerda al agente que pregunte y verifique el proyecto mientras no esté vinculado
  guard          PreToolUse        -> bloquea Edit/Write/NotebookEdit/Bash en proyectos sin .planesgo.json válido
  mcp            PreToolUse        -> añade project_path (proyecto de esta sesión) a las llamadas al MCP planesgo
  track          PostToolUse       -> latidos de imputación (el primero abre el cronómetro)
  turn-end       Stop              -> programa la consolidación: si en 5 minutos no hay actividad, se imputa
  consolidate    (interno)         -> espera, y si el agente sigue parado cierra e imputa el parte en Odoo
  session-end    SessionEnd        -> cierra e imputa los cronómetros abiertos en la sesión

Consolidación: un agente que ha terminado su trabajo no envía más latidos y, sobre todo en la nube, la sesión
puede tardar horas en cerrarse. Por eso, a los 5 minutos de terminar un turno sin actividad nueva, se cierra el
parte con lo hecho hasta ese momento; si el agente vuelve a trabajar, el siguiente latido abre un parte nuevo.

La descripción del parte se construye con lo que se ha hecho en la sesión: commits nuevos,
ficheros editados, rama y, si aún no hay nada de eso, la primera petición del usuario.
Siempre en español: los prefijos de commit se traducen y los textos en inglés se omiten.

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
DESC_MAX = 480
MAX_FILES_LISTED = 5
MAX_COMMITS_LISTED = 6

# Los partes de PlanesGo se redactan siempre en español
COMMIT_TYPES_ES = {
    "feat": "Nueva funcionalidad", "fix": "Corrección", "docs": "Documentación",
    "refactor": "Refactorización", "test": "Pruebas", "tests": "Pruebas", "perf": "Rendimiento",
    "style": "Estilo", "chore": "Mantenimiento", "build": "Compilación",
    "ci": "Integración continua", "revert": "Reversión",
}
COMMIT_PREFIX = re.compile(r"^(?:\[(\w+)\]|(\w+)(?:\([^)]*\))?!?:)\s*(.+)$")
WORDS = re.compile(r"[A-Za-zÁÉÍÓÚÜÑáéíóúüñ]+")
EN_WORDS = {
    "the", "and", "add", "adds", "added", "fix", "fixes", "fixed", "update", "updates", "updated",
    "remove", "removes", "removed", "use", "uses", "for", "with", "to", "of", "in", "on", "when",
    "from", "into", "is", "are", "be", "should", "now", "this", "that", "new", "support", "improve",
    "make", "allow", "change", "rename", "move", "bump", "implement", "create", "delete", "show",
    "handle", "please", "can", "you", "it", "my", "how", "what", "why", "was", "were", "has",
    "have", "had", "not", "an", "by", "at", "as", "if", "but", "or", "all", "missing", "wrong",
    "only", "after", "before", "instead", "via", "per", "its", "too", "also",
}
ES_WORDS = {
    "el", "la", "los", "las", "de", "del", "que", "en", "con", "para", "por", "se", "una", "un",
    "al", "y", "añade", "corrige", "actualiza", "elimina", "usa", "ya", "cuando", "sin", "su", "es",
    "mejora", "nuevo", "nueva", "muestra", "quita", "cambia", "crea", "como", "qué", "cómo",
}
# El backend distingue Claude de Antigravity por este valor
AI_MODEL_LABEL = "Claude Code"
STATE_ROOT = "/tmp/planesgo-claude"
# Espera tras terminar un turno antes de consolidar (PLANESGO_CONSOLIDATE_SECONDS lo cambia, p. ej. en pruebas)
CONSOLIDATE_AFTER = 300

EXEMPT_PREFIXES = (
    os.path.join(HOME, ".claude"),
    os.path.join(HOME, ".gemini"),
    "/tmp",
    "/var/tmp",
)

SAFE_BINS = {
    "planesgo-mcp", "planesgo-track", "cd", "ls", "pwd", "grep", "find",
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
        "1. Pregunta al usuario a qué proyecto de Odoo corresponde este repositorio. No lo asumas ni lo "
        "deduzcas del nombre de la carpeta, y nunca uses PLANESGO por defecto.\n"
        "2. Si no conoces el nombre exacto (o el usuario tampoco), búscalo primero con la herramienta MCP "
        '`mcp__planesgo__planesgo_search_projects` con {"query": "<texto parcial>"} '
        f'(alternativa CLI: `planesgo-mcp --search-project "<texto parcial>"`); no hace falta acento ni '
        "mayúsculas exactas. Muestra las coincidencias al usuario para que confirme cuál es.\n"
        "3. Con el nombre confirmado, vincula con `mcp__planesgo__planesgo_set_project` usando "
        f'{{"project_name": "<nombre exacto>", "project_path": "{root}"}} '
        f'(alternativa CLI: `planesgo-mcp --set-project "<nombre exacto>" --path "{root}"`).\n'
        "4. Si aun así responde que el proyecto no existe, díselo al usuario y repite la búsqueda. "
        "No continúes con la tarea hasta que la vinculación se confirme."
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


def run_bin(args, background=False, payload=None, with_tokens=False, tokens=None):
    binp = find_bin()
    if not binp:
        return False
    cmd = [binp] + args + ["--task", TASK_NAME, "--type", TASK_TYPE, "--model", AI_MODEL_LABEL]
    if payload:
        sid = str(payload.get("session_id") or "").strip()
        if sid:
            cmd += ["--session-id", sid]
    if payload and with_tokens:
        # Solo al cerrar: el transcript puede ser grande y los latidos deben ser baratos
        cmd += tokens_args(*transcript_tokens(payload.get("transcript_path")))
    if tokens:
        cmd += tokens_args(*tokens)
    if background:
        subprocess.Popen(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        return True
    try:
        return subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=20).returncode == 0
    except Exception:
        return False


def tokens_args(tin, tout):
    return ["--tokens-in", str(tin), "--tokens-out", str(tout), "--tokens", str(tin + tout)] if (tin or tout) else []


def transcript_tokens(path):
    """Tokens de la sesión según el transcript de Claude Code (uso real informado por la API).

    Cada respuesta puede ocupar varias líneas con el mismo message.id, así que se cuenta una vez por id.
    La entrada excluye las lecturas de caché: se releen en cada turno y multiplicarían la cifra
    sin reflejar trabajo nuevo.
    """
    if not path or not os.path.isfile(path):
        return 0, 0
    usages = {}
    try:
        with open(path, "r", encoding="utf-8") as f:
            for line in f:
                try:
                    entry = json.loads(line)
                except Exception:
                    continue
                msg = entry.get("message") if isinstance(entry, dict) else None
                if not isinstance(msg, dict) or not isinstance(msg.get("usage"), dict):
                    continue
                key = msg.get("id") or entry.get("uuid") or len(usages)
                usages[key] = msg["usage"]
    except Exception:
        return 0, 0
    tin = sum(int(u.get("input_tokens") or 0) + int(u.get("cache_creation_input_tokens") or 0) for u in usages.values())
    tout = sum(int(u.get("output_tokens") or 0) for u in usages.values())
    return tin, tout


def git(root, *args):
    try:
        res = subprocess.run(["git", "-C", root] + list(args), stdout=subprocess.PIPE,
                             stderr=subprocess.DEVNULL, timeout=5, text=True)
        return res.stdout.strip() if res.returncode == 0 else ""
    except Exception:
        return ""


def read_text(path):
    try:
        with open(path, "r", encoding="utf-8") as f:
            return f.read().strip()
    except Exception:
        return ""


def write_text(path, text):
    try:
        with open(path, "w", encoding="utf-8") as f:
            f.write(text)
    except Exception:
        pass


def record_prompt(payload):
    """Guarda la primera petición de la sesión (el resto suelen ser aclaraciones o "sí")."""
    prompt = " ".join(str(payload.get("prompt") or "").split())
    if not prompt or prompt.startswith("/") or prompt.startswith("<"):
        return
    sdir = session_state_dir(payload)
    os.makedirs(sdir, exist_ok=True)
    path = os.path.join(sdir, "first_prompt")
    if not os.path.exists(path):
        write_text(path, prompt[:300])


def init_project_state(state, root):
    """Punto de partida de la sesión en el proyecto: con él se distinguen los commits nuevos."""
    path = os.path.join(state, "start_head")
    if not os.path.exists(path):
        write_text(path, git(root, "rev-parse", "HEAD") or "-")
        write_text(os.path.join(state, "start_time"), str(int(time.time())))


def record_file(payload, state, root):
    args = payload.get("tool_input") or {}
    path = args.get("file_path") or args.get("notebook_path") or ""
    if not path:
        return
    rel = os.path.relpath(os.path.abspath(path), root)
    if rel.startswith(".."):
        return
    files_path = os.path.join(state, "files")
    files = read_text(files_path).splitlines()
    if rel not in files:
        files.append(rel)
        write_text(files_path, "\n".join(files))


def session_commits(root, state):
    start = read_text(os.path.join(state, "start_head"))
    if start and start != "-":
        out = git(root, "log", "--no-merges", "--format=%s", f"{start}..HEAD")
    else:
        since = read_text(os.path.join(state, "start_time"))
        out = git(root, "log", "--no-merges", "--format=%s", f"--since=@{since}") if since else ""
    return [c.strip() for c in out.splitlines() if c.strip()]


def changed_files(root, state):
    """Ficheros editados con Edit/Write más los que cambian los commits o el árbol de trabajo (p. ej. vía Bash)."""
    files = read_text(os.path.join(state, "files")).splitlines()
    start = read_text(os.path.join(state, "start_head"))
    extra = git(root, "diff", "--name-only", start) if start and start != "-" else git(root, "diff", "--name-only", "HEAD")
    for f in extra.splitlines():
        if f and f not in files:
            files.append(f)
    return files


EN_SHAPE = re.compile(r"(?:tion|tions|ing|ness|ed)$|th|sh|ck|w")
QUOTED = re.compile(r"\"[^\"]*\"|`[^`]*`|'[^']*'")
ES_SHAPE = re.compile(r"(?:ción|ciones|mente|ado|ada|ados|adas|ido|ida|idos|idas)$")


def looks_english(text):
    """Heurística ligera: palabras y formas típicas del inglés frente a las del español (tildes, ñ, -ción…).

    Se ignoran identificadores (CamelCase, SIGLAS) y las formas inglesas cuentan la mitad que las
    palabras clave, para no confundir un commit en español que cita nombres de código.
    """
    # Fuera citas literales ("Invalid field…") y tokens de código (snake_case, versiones, rutas)
    plain = " ".join(t for t in QUOTED.sub(" ", text).split()
                     if not re.search(r"[_\d/.=-]", t.strip(",;:()[]")))
    words = [w.lower() for w in WORDS.findall(plain) if len(w) > 1 and not any(c.isupper() for c in w[1:])]
    en = sum(1.0 if w in EN_WORDS else 0.5 if len(w) > 3 and EN_SHAPE.search(w) else 0.0 for w in words)
    es = sum(w in ES_WORDS or bool(ES_SHAPE.search(w)) for w in words) + sum(ch in "áéíóúñ¿¡" for ch in text.lower())
    return en > es


def commit_es(subject):
    """Traduce el prefijo convencional (feat:, fix(x):, [FEAT]) y devuelve None si el texto está en inglés."""
    m = COMMIT_PREFIX.match(subject)
    if m:
        kind = (m.group(1) or m.group(2) or "").lower()
        if kind in COMMIT_TYPES_ES:
            subject = f"{COMMIT_TYPES_ES[kind]}: {m.group(3)}"
            body = m.group(3)
        else:
            body = subject
    else:
        body = subject
    return None if looks_english(body) else subject


def build_description(payload, root, state):
    """Descripción del parte con lo realmente hecho; DESC si todavía no hay nada que contar."""
    commits = session_commits(root, state)
    files = changed_files(root, state)
    branch = git(root, "rev-parse", "--abbrev-ref", "HEAD")
    parts = []
    spanish = [c for c in (commit_es(c) for c in reversed(commits)) if c]
    if spanish:
        shown = spanish[:MAX_COMMITS_LISTED]
        rest = len(commits) - len(shown)
        more = f" (+{rest} commit{'s' if rest != 1 else ''} más)" if rest > 0 else ""
        parts.append("; ".join(shown) + more)
    elif commits:
        # Commits con mensaje en inglés: se cuentan pero no se copian
        parts.append(f"{len(commits)} commit{'s' if len(commits) != 1 else ''}")
    if not spanish:
        prompt = read_text(os.path.join(session_state_dir(payload), "first_prompt"))
        if prompt and not looks_english(prompt):
            parts.insert(0, f"Petición: {prompt[:200]}")
    if files:
        names = ", ".join(os.path.basename(f) for f in files[:MAX_FILES_LISTED])
        more = f" y {len(files) - MAX_FILES_LISTED} más" if len(files) > MAX_FILES_LISTED else ""
        parts.append(f"{len(files)} fichero{'s' if len(files) != 1 else ''} ({names}{more})")
    if not parts:
        return DESC
    if branch and branch != "HEAD":
        parts.append(f"rama {branch}")
    desc = "[Claude Code] " + " · ".join(parts)
    return desc if len(desc) <= DESC_MAX else desc[:DESC_MAX - 1] + "…"


def on_context(payload, event):
    if event == "UserPromptSubmit":
        try:
            record_prompt(payload)
            # Un mensaje nuevo reanuda el trabajo: la consolidación pendiente ya no toca
            for state, _root in active_projects(session_state_dir(payload)):
                cancel_consolidation(state)
        except Exception:
            pass
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


def record_session_project(payload, root):
    """Recuerda el último proyecto tocado en esta sesión (útil cuando la sesión se abrió en el home)."""
    try:
        sdir = session_state_dir(payload)
        os.makedirs(sdir, exist_ok=True)
        with open(os.path.join(sdir, "current_project"), "w", encoding="utf-8") as f:
            f.write(root)
    except Exception:
        pass


def session_project(payload):
    """Proyecto de esta sesión: el directorio de la sesión o, si es el home, el último proyecto tocado."""
    root = project_root_for_dir(session_dir(payload))
    if root:
        return root
    try:
        with open(os.path.join(session_state_dir(payload), "current_project"), "r", encoding="utf-8") as f:
            return f.read().strip() or None
    except Exception:
        return None


def on_mcp(payload):
    args = dict(payload.get("tool_input") or {})
    if args.get("project_path"):
        return
    root = session_project(payload)
    if not root:
        # Sin proyecto en la sesión el MCP responde que falta project_path y el agente pregunta al usuario
        return
    args["project_path"] = root
    print(json.dumps({"hookSpecificOutput": {
        "hookEventName": "PreToolUse",
        "permissionDecision": "allow",
        "permissionDecisionReason": f"Proyecto de la sesión: {root}",
        "updatedInput": args,
    }}))


def on_guard(payload):
    root = target_root(payload)
    if root:
        record_session_project(payload, root)
    if payload.get("tool_name") == "Bash" and is_safe_command((payload.get("tool_input") or {}).get("command")):
        return
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
    init_project_state(state, root)
    record_file(payload, state, root)
    active = os.path.join(state, "active")
    last_file = os.path.join(state, "last_beat")
    now = time.time()
    cancel_consolidation(state)
    if not os.path.isfile(active):
        # Primer latido en primer plano: solo se marca activo si Odoo confirma el cronómetro,
        # así el stop final nunca queda huérfano
        if run_bin(["--beat", "--path", root, "--desc", build_description(payload, root, state)], payload=payload):
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
        run_bin(["--beat", "--path", root, "--desc", build_description(payload, root, state)],
                background=True, payload=payload)


def cancel_consolidation(state):
    try:
        os.remove(os.path.join(state, "pending"))
    except OSError:
        pass


def active_projects(sdir):
    """(estado, raíz) de cada proyecto con cronómetro abierto en la sesión."""
    try:
        names = os.listdir(sdir)
    except OSError:
        return []
    found = []
    for proj in names:
        state = os.path.join(sdir, proj)
        root = read_text(os.path.join(state, "active"))
        if root:
            found.append((state, root))
    return found


def session_tokens(payload, state):
    """Tokens de la sesión aún no imputados en otro parte (los ya consolidados se descuentan)."""
    tin, tout = transcript_tokens(payload.get("transcript_path"))
    try:
        done_in, done_out = (int(x) for x in read_text(os.path.join(state, "tokens_done")).split())
    except Exception:
        done_in, done_out = 0, 0
    write_text(os.path.join(state, "tokens_done"), f"{tin} {tout}")
    return max(tin - done_in, 0), max(tout - done_out, 0)


def close_part(payload, state, root):
    """Cierra e imputa en Odoo el parte abierto del proyecto y deja la sesión lista para abrir otro."""
    ok = run_bin(["--stop", "--path", root, "--desc", build_description(payload, root, state)],
                 payload=payload, tokens=session_tokens(payload, state))
    if ok:
        # Lo siguiente que haga el agente es trabajo nuevo: otro parte, con su propia descripción
        for name in ("active", "last_beat", "files", "start_head", "start_time"):
            try:
                os.remove(os.path.join(state, name))
            except OSError:
                pass
        init_project_state(state, root)
    return ok


def on_turn_end(payload):
    """Stop: el agente ha terminado su turno. Si en CONSOLIDATE_AFTER segundos no vuelve a trabajar, se consolida."""
    if payload.get("stop_hook_active"):
        return
    sdir = session_state_dir(payload)
    projects = active_projects(sdir)
    if not projects:
        return
    stamp = str(time.time())
    for state, _root in projects:
        write_text(os.path.join(state, "pending"), stamp)
    write_text(os.path.join(sdir, "payload.json"), json.dumps(
        {"session_id": payload.get("session_id"), "transcript_path": payload.get("transcript_path")}))
    subprocess.Popen([sys.executable, os.path.abspath(__file__), "consolidate", sdir, stamp],
                     stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                     start_new_session=True)


def on_consolidate(sdir, stamp):
    try:
        delay = int(os.environ.get("PLANESGO_CONSOLIDATE_SECONDS") or CONSOLIDATE_AFTER)
    except ValueError:
        delay = CONSOLIDATE_AFTER
    time.sleep(max(delay, 0))
    try:
        payload = json.loads(read_text(os.path.join(sdir, "payload.json")) or "{}")
    except Exception:
        payload = {}
    for state, root in active_projects(sdir):
        # Si ha habido un latido o un turno nuevo desde entonces, esta consolidación ya no toca
        if read_text(os.path.join(state, "pending")) != stamp:
            continue
        cancel_consolidation(state)
        close_part(payload, state, root)


def on_session_end(payload):
    sdir = session_state_dir(payload)
    if not os.path.isdir(sdir):
        return
    for state, root in active_projects(sdir):
        # La descripción final resume lo hecho desde el último parte consolidado
        close_part(payload, state, root)
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
    if mode == "consolidate":
        try:
            on_consolidate(sys.argv[2], sys.argv[3])
        except Exception:
            pass
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
        elif mode == "mcp":
            on_mcp(payload)
        elif mode == "track":
            on_track(payload)
        elif mode == "turn-end":
            on_turn_end(payload)
        elif mode == "session-end":
            on_session_end(payload)
    except Exception:
        pass
    sys.exit(0)


if __name__ == "__main__":
    main()
