#!/usr/bin/env bash
# ==============================================================================
# Instala el hook unificado de PlanesGo (PSF) para Claude Code a nivel de
# USUARIO: funciona en CUALQUIER proyecto (no solo PlanesGo), bloquea
# Edit/Write/Bash hasta que el proyecto esté vinculado a Odoo (nunca asume
# "PLANESGO" por defecto), y reutiliza el mismo binario/cuenta que Antigravity.
# También instala el comando /pgo (estado, buscar y vincular el proyecto, añadir
# minutos) y el servidor MCP «pgo» (binario pgo-mcp) en cualquier sesión.
#
# Uso genérico en cualquier sesión de Claude Code (nueva o ya abierta), local
# o en la nube — siempre descarga la versión más reciente desde el servidor de PlanesGo
# (no depende de que el repositorio sea público):
#   curl -fsSL https://planesgo.autopyme.com/install/claude-hook.sh | bash
#
# También sirve como "Setup script" del entorno cloud pegando ese mismo
# curl|bash, o el contenido de este fichero directamente.
#
# Requisitos: curl, tar, python3 y go (para compilar pgo-mcp), y $PLANESGO_TOKEN o
# $ANTIGRAVITY_TOKEN, o ~/.planesgo_auth.json. Con token, instala también PSF (Planes Software Factory)
# en ~/.planesgo/PSF; sin token (p. ej. en el «Setup script» de la nube), PSF se descarga al empezar la sesión.
# Idempotente: se puede ejecutar varias veces sin duplicar nada.
# ==============================================================================
set -uo pipefail

PLANESGO_URL="${PLANESGO_URL:-}"
if [ -z "$PLANESGO_URL" ] && [ -f "$HOME/.planesgo_auth.json" ]; then
    PLANESGO_URL="$(python3 -c 'import json,os;print(json.load(open(os.path.expanduser("~/.planesgo_auth.json"))).get("planesgo_url") or "")' 2>/dev/null)"
fi
PLANESGO_URL="${PLANESGO_URL:-https://planesgo.autopyme.com}"
PLANESGO_URL="${PLANESGO_URL%/}"
HOOKS_DIR="$HOME/.claude/hooks"
BIN_DIR="$HOME/.local/bin"
mkdir -p "$HOOKS_DIR" "$BIN_DIR"

# Limpieza: retira el hook bash antiguo (versión anterior) si existe
rm -f "$HOOKS_DIR/planesgo-track.sh"

# 0. Origen de los ficheros: el repositorio local si se ejecuta desde él; si no, el paquete del servidor --
SRC_DIR=""
if [ -d "./cmd/pgo-mcp" ] && [ -f "./.claude/hooks/planesgo_claude_hook.py" ]; then
    SRC_DIR="$(pwd)"
else
    SRC_DIR="$(mktemp -d)"
    echo "==> Descargando PlanesGo desde $PLANESGO_URL ..."
    if ! curl -fsSL "$PLANESGO_URL/install/planesgo.tar.gz" | tar -xz -C "$SRC_DIR"; then
        echo "ERROR: no se pudo descargar $PLANESGO_URL/install/planesgo.tar.gz. Abortando." >&2
        exit 1
    fi
fi

# 1. Hook Python unificado ------------------------------------------------------
cp "$SRC_DIR/.claude/hooks/planesgo_claude_hook.py" "$HOOKS_DIR/planesgo_claude_hook.py"
chmod +x "$HOOKS_DIR/planesgo_claude_hook.py"
echo "OK: hook instalado en $HOOKS_DIR/planesgo_claude_hook.py"

# 1b. Comando /pgo (estado, búsqueda/vinculación, +N minutos, --update, psf); /planesgo ya no existe -------
COMMANDS_DIR="$HOME/.claude/commands"
mkdir -p "$COMMANDS_DIR"
rm -f "$COMMANDS_DIR/planesgo.md"
if cp "$SRC_DIR/.claude/commands/pgo.md" "$COMMANDS_DIR/pgo.md" 2>/dev/null; then
    echo "OK: comando /pgo instalado en $COMMANDS_DIR/pgo.md"
else
    echo "AVISO: no se pudo instalar el comando /pgo (no crítico, el tracking funciona igual)."
fi

# 2. settings.json de usuario: fusiona los hooks sin machacar lo que ya haya -
SETTINGS_FILE="$HOME/.claude/settings.json"
mkdir -p "$HOME/.claude"
python3 - "$SETTINGS_FILE" <<'PY_EOF'
import json, sys, os

path = sys.argv[1]
data = {}
if os.path.exists(path):
    try:
        with open(path) as f:
            data = json.load(f) or {}
    except Exception:
        data = {}

hooks = data.setdefault("hooks", {})

def ensure(event, matcher, mode, timeout):
    entries = hooks.setdefault(event, [])
    for group in entries:
        for h in group.get("hooks", []):
            cmd = h.get("command", "")
            if "planesgo_claude_hook.py" in cmd and cmd.strip().endswith(mode):
                if matcher:
                    group["matcher"] = matcher
                return
    entry = {"hooks": [{"type": "command",
                         "command": f'python3 "$HOME/.claude/hooks/planesgo_claude_hook.py" {mode}',
                         "timeout": timeout}]}
    if matcher:
        entry["matcher"] = matcher
    entries.append(entry)

ensure("SessionStart", None, "session-start", 10)
ensure("UserPromptSubmit", None, "prompt", 10)
ensure("PreToolUse", "Edit|Write|MultiEdit|NotebookEdit|Bash", "guard", 10)
ensure("PreToolUse", "mcp__(pgo|planesgo(-mcp)?)__.*", "mcp", 10)
ensure("PostToolUse", "Edit|Write|MultiEdit|NotebookEdit|Bash", "track", 25)
ensure("Stop", None, "turn-end", 10)
ensure("SessionEnd", None, "session-end", 30)

with open(path, "w") as f:
    json.dump(data, f, indent=2, ensure_ascii=False)
    f.write("\n")

print(f"OK: hooks globales (PSF unificado) fusionados en {path}")
PY_EOF

# 3. Binario pgo-mcp: siempre se reconstruye con la versión actual del repo --------
# planesgo-mcp queda como enlace simbólico a pgo-mcp, para los hooks que aún lo llaman así.
if command -v go >/dev/null 2>&1; then
    if [ -n "$SRC_DIR" ] && [ -d "$SRC_DIR/cmd/pgo-mcp" ]; then
        echo "==> Compilando pgo-mcp..."
        (cd "$SRC_DIR" && CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN_DIR/pgo-mcp.nuevo" ./cmd/pgo-mcp \
            && mv -f "$BIN_DIR/pgo-mcp.nuevo" "$BIN_DIR/pgo-mcp") \
            && echo "OK: pgo-mcp instalado en $BIN_DIR/pgo-mcp" \
            || echo "AVISO: fallo al compilar pgo-mcp."
    fi
elif [ -x "$BIN_DIR/pgo-mcp" ]; then
    echo "AVISO: 'go' no está disponible; se mantiene el pgo-mcp ya instalado (puede no tener las últimas funciones)."
else
    echo "AVISO: 'go' no está disponible; no se pudo instalar pgo-mcp. El hook quedará en no-op hasta que lo instales manualmente."
fi
if [ -x "$BIN_DIR/pgo-mcp" ]; then
    ln -sfn pgo-mcp "$BIN_DIR/planesgo-mcp" && echo "OK: $BIN_DIR/planesgo-mcp enlaza a pgo-mcp (compatibilidad)."
fi

# 3b. Servidor MCP «pgo» como herramienta nativa (user scope) ------------------
# Idempotente: se borra y se vuelve a añadir, porque "claude mcp add" falla si ya existe.
# Se retiran los registros antiguos (planesgo y planesgo-mcp).
if [ -x "$BIN_DIR/pgo-mcp" ] && command -v claude >/dev/null 2>&1; then
    for viejo in planesgo planesgo-mcp pgo; do
        claude mcp remove "$viejo" -s user >/dev/null 2>&1 || true
    done
    if claude mcp add --scope user --transport stdio pgo -- "$BIN_DIR/pgo-mcp" >/dev/null 2>&1; then
        echo "OK: pgo-mcp registrado como servidor MCP «pgo» (user scope)."
    else
        echo "AVISO: no se pudo registrar el servidor MCP «pgo»; se seguirá usando pgo-mcp por CLI."
    fi
else
    echo "AVISO: 'claude' no disponible o falta el binario; se seguirá usando pgo-mcp por CLI."
fi

# 3c. Arranque de PSF en cada sesión (hook SessionStart): descarga PSF con el token de la sesión y carga su
# contexto en los proyectos con perfil .psf/proyecto.yml. En la nube es la única vía, porque el «Setup script» no
# tiene el token. Sustituye al antiguo /opt/psf-arranque.sh (bloque de PSF del Setup script).
mkdir -p "$HOME/.planesgo"
if cp "$SRC_DIR/scripts/psf-arranque.sh" "$HOME/.planesgo/psf-arranque.sh" 2>/dev/null; then
    chmod +x "$HOME/.planesgo/psf-arranque.sh"
    python3 - "$SETTINGS_FILE" <<'PY'
import json, os, sys
path = sys.argv[1]
data = json.load(open(path)) if os.path.exists(path) else {}
ss = data.setdefault("hooks", {}).setdefault("SessionStart", [])
cmd = 'bash "$HOME/.planesgo/psf-arranque.sh"'
for group in ss:
    group["hooks"] = [h for h in group.get("hooks", []) if h.get("command") != "bash /opt/psf-arranque.sh"]
ss[:] = [g for g in ss if g.get("hooks")]
if not any(h.get("command") == cmd for g in ss for h in g.get("hooks", [])):
    ss.append({"hooks": [{"type": "command", "command": cmd, "timeout": 40}]})
json.dump(data, open(path, "w"), indent=2, ensure_ascii=False)
PY
    echo "OK: arranque de PSF registrado (SessionStart)"
fi

# 3d. PSF (Planes Software Factory) en esta máquina: solo con token de empleado ------------------
PSF_TOKEN="${PLANESGO_TOKEN:-${ANTIGRAVITY_TOKEN:-}}"
if [ -z "$PSF_TOKEN" ] && [ -f "$HOME/.planesgo_auth.json" ]; then
    PSF_TOKEN="$(python3 -c 'import json,os;print(json.load(open(os.path.expanduser("~/.planesgo_auth.json"))).get("antigravity_token") or "")' 2>/dev/null)"
fi
PSF_HOME="${PSF_HOME:-$HOME/.planesgo/PSF}"
if [ -n "$PSF_TOKEN" ]; then
    PSF_TMP="$(mktemp -d)"
    if curl -fsSL -H "Authorization: Bearer $PSF_TOKEN" "$PLANESGO_URL/install/psf.tar.gz" | tar -xz -C "$PSF_TMP" 2>/dev/null \
        && [ -f "$PSF_TMP/PSF/hooks/psf_hook.py" ]; then
        rm -rf "$PSF_HOME" && mkdir -p "$(dirname "$PSF_HOME")" && mv "$PSF_TMP/PSF" "$PSF_HOME"
        bash "$PSF_HOME/install.sh" --copy >/dev/null 2>&1 \
            && echo "OK: PSF instalado en $PSF_HOME" \
            || echo "AVISO: PSF descargado en $PSF_HOME, pero su instalador falló (bash $PSF_HOME/install.sh)."
    else
        echo "AVISO: no se pudo descargar PSF desde $PLANESGO_URL (no crítico)."
    fi
    rm -rf "$PSF_TMP"
fi

# 4. Comprobación de autenticación -------------------------------------------
if [ -n "${PLANESGO_TOKEN:-}" ] || [ -n "${ANTIGRAVITY_TOKEN:-}" ]; then
    echo "OK: token de empleado presente en el entorno."
elif [ -f "$HOME/.planesgo_auth.json" ]; then
    echo "OK: ~/.planesgo_auth.json presente."
else
    echo "AVISO: no se encontró PLANESGO_TOKEN/ANTIGRAVITY_TOKEN ni ~/.planesgo_auth.json."
fi

echo ""
echo "Instalación completada. Cada proyecto NUEVO (distinto de PlanesGo) se bloqueará"
echo "para editar hasta que lo vincules una vez a su proyecto de Odoo:"
echo '  /pgo <texto del proyecto>   (o: pgo-mcp --project "<texto>", y luego pgo-mcp --project-id N --link --path "/ruta/al/proyecto")'
echo "El propio asistente te lo pedirá automáticamente si intenta trabajar sin vincular."
