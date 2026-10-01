#!/usr/bin/env bash
# ==============================================================================
# Instala el hook unificado de PlanesGo (PSF) para Claude Code a nivel de
# USUARIO: funciona en CUALQUIER proyecto (no solo PlanesGo), bloquea
# Edit/Write/Bash hasta que el proyecto esté vinculado a Odoo (nunca asume
# "PLANESGO" por defecto), y reutiliza el mismo binario/cuenta que Antigravity.
# También instala el comando /planesgo para revisar el estado del tracking
# (proyecto vinculado, token, hook, binario) en cualquier sesión, a demanda.
#
# Uso genérico en cualquier sesión de Claude Code (nueva o ya abierta), local
# o en la nube — siempre descarga la versión más reciente del propio repo:
#   curl -fsSL https://raw.githubusercontent.com/planesnet/PlanesGo/master/scripts/install-claude-hook.sh | bash
#
# También sirve como "Setup script" del entorno cloud pegando ese mismo
# curl|bash, o el contenido de este fichero directamente.
#
# Requisitos: curl, python3, git y go (para compilar planesgo-mcp si no existe
# ya), y $PLANESGO_TOKEN o $ANTIGRAVITY_TOKEN, o ~/.planesgo_auth.json.
# Idempotente: se puede ejecutar varias veces sin duplicar nada.
# ==============================================================================
set -uo pipefail

REPO_RAW="https://raw.githubusercontent.com/planesnet/PlanesGo/master"
HOOKS_DIR="$HOME/.claude/hooks"
BIN_DIR="$HOME/.local/bin"
mkdir -p "$HOOKS_DIR" "$BIN_DIR"

# Limpieza: retira el hook bash antiguo (versión anterior) si existe
rm -f "$HOOKS_DIR/planesgo-track.sh"

# 1. Hook Python unificado: siempre la versión actual del repo -----------------
echo "==> Descargando el hook unificado desde $REPO_RAW ..."
if ! curl -fsSL "$REPO_RAW/.claude/hooks/planesgo_claude_hook.py" -o "$HOOKS_DIR/planesgo_claude_hook.py"; then
    echo "ERROR: no se pudo descargar planesgo_claude_hook.py. Abortando." >&2
    exit 1
fi
chmod +x "$HOOKS_DIR/planesgo_claude_hook.py"
echo "OK: hook instalado en $HOOKS_DIR/planesgo_claude_hook.py"

# 1b. Comando /planesgo (diagnóstico bajo demanda): siempre la versión actual del repo -----
COMMANDS_DIR="$HOME/.claude/commands"
mkdir -p "$COMMANDS_DIR"
if curl -fsSL "$REPO_RAW/.claude/commands/planesgo.md" -o "$COMMANDS_DIR/planesgo.md"; then
    echo "OK: comando /planesgo instalado en $COMMANDS_DIR/planesgo.md"
else
    echo "AVISO: no se pudo descargar el comando /planesgo (no crítico, el tracking funciona igual)."
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

def has_mode(entries, mode):
    for group in entries:
        for h in group.get("hooks", []):
            cmd = h.get("command", "")
            if "planesgo_claude_hook.py" in cmd and cmd.strip().endswith(mode):
                return True
    return False

def ensure(event, matcher, mode, timeout):
    entries = hooks.setdefault(event, [])
    if has_mode(entries, mode):
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
ensure("PreToolUse", "mcp__planesgo__.*", "mcp", 10)
ensure("PostToolUse", "Edit|Write|MultiEdit|NotebookEdit|Bash", "track", 25)
ensure("SessionEnd", None, "session-end", 30)

with open(path, "w") as f:
    json.dump(data, f, indent=2, ensure_ascii=False)
    f.write("\n")

print(f"OK: hooks globales (PSF unificado) fusionados en {path}")
PY_EOF

# 3. Binario planesgo-mcp: siempre se reconstruye con la versión actual del repo ---
if command -v go >/dev/null 2>&1; then
    SRC_DIR=""
    if [ -f "./cmd/planesgo-mcp/main.go" ]; then
        SRC_DIR="."
    else
        SRC_DIR="$(mktemp -d)"
        echo "==> Clonando planesnet/PlanesGo (solo para compilar planesgo-mcp)..."
        if command -v git >/dev/null 2>&1 && git clone --depth 1 https://github.com/planesnet/PlanesGo "$SRC_DIR" >/dev/null 2>&1; then
            :
        else
            echo "AVISO: no se pudo clonar PlanesGo; omitiendo compilación de planesgo-mcp."
            SRC_DIR=""
        fi
    fi
    if [ -n "$SRC_DIR" ] && [ -f "$SRC_DIR/cmd/planesgo-mcp/main.go" ]; then
        echo "==> Compilando planesgo-mcp..."
        (cd "$SRC_DIR" && CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN_DIR/planesgo-mcp" cmd/planesgo-mcp/main.go) \
            && echo "OK: planesgo-mcp instalado en $BIN_DIR/planesgo-mcp" \
            || echo "AVISO: fallo al compilar planesgo-mcp."
    fi
elif [ -x "$BIN_DIR/planesgo-mcp" ]; then
    echo "AVISO: 'go' no está disponible; se mantiene el planesgo-mcp ya instalado (puede no tener las últimas funciones)."
else
    echo "AVISO: 'go' no está disponible; no se pudo instalar planesgo-mcp. El hook quedará en no-op hasta que lo instales manualmente."
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
echo '  planesgo-mcp --set-project "Nombre exacto del proyecto en Odoo" --path "/ruta/al/proyecto"'
echo "El propio asistente te lo pedirá automáticamente si intenta trabajar sin vincular."
