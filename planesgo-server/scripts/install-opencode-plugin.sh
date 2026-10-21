#!/usr/bin/env bash
# ==============================================================================
# Instala el sistema de telemetría e imputación horaria de PlanesGo (PSF) en
# OpenCode Desktop / OpenCode CLI (v2.1+) a nivel de USUARIO:
#   1. Hook unificado ~/.claude/hooks/planesgo_claude_hook.py (con soporte OpenCode)
#   2. Binario ~/.local/bin/pgo-mcp (y planesgo-mcp como enlace a él)
#   3. Plugin nativo ~/.config/opencode/plugins/planesgo.ts
#   4. Comando /pgo en ~/.config/opencode/commands/
#   5. Servidor MCP «pgo» e instrucciones PSF en ~/.config/opencode/opencode.json
#
# Uso directo desde el repositorio o vía servidor:
#   bash scripts/install-opencode-plugin.sh
#   curl -fsSL https://planesgo.autopyme.com/install/opencode-plugin.sh | bash
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
OPENCODE_DIR="$HOME/.config/opencode"
OPENCODE_PLUGINS_DIR="$OPENCODE_DIR/plugins"
OPENCODE_COMMANDS_DIR="$OPENCODE_DIR/commands"

mkdir -p "$HOOKS_DIR" "$BIN_DIR" "$OPENCODE_PLUGINS_DIR" "$OPENCODE_COMMANDS_DIR"

# 0. Origen de los ficheros: repositorio local o paquete del servidor ----------
SRC_DIR=""
if [ -d "./cmd/pgo-mcp" ] && [ -f "./.opencode/plugins/planesgo.ts" ]; then
    SRC_DIR="$(pwd)"
else
    SRC_DIR="$(mktemp -d)"
    echo "==> Descargando PlanesGo desde $PLANESGO_URL ..."
    if ! curl -fsSL "$PLANESGO_URL/install/planesgo.tar.gz" | tar -xz -C "$SRC_DIR"; then
        echo "ERROR: no se pudo descargar $PLANESGO_URL/install/planesgo.tar.gz. Abortando." >&2
        exit 1
    fi
fi

# 1. Hook Python unificado -----------------------------------------------------
cp "$SRC_DIR/.claude/hooks/planesgo_claude_hook.py" "$HOOKS_DIR/planesgo_claude_hook.py"
chmod +x "$HOOKS_DIR/planesgo_claude_hook.py"
echo "OK: motor de reglas y latidos instalado en $HOOKS_DIR/planesgo_claude_hook.py"

# 2. Binario pgo-mcp (planesgo-mcp queda como enlace, para los hooks que aún lo llaman así) ---
if command -v go >/dev/null 2>&1 && [ -d "$SRC_DIR/cmd/pgo-mcp" ]; then
    echo "==> Compilando pgo-mcp..."
    (cd "$SRC_DIR" && CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN_DIR/pgo-mcp.nuevo" ./cmd/pgo-mcp \
        && mv -f "$BIN_DIR/pgo-mcp.nuevo" "$BIN_DIR/pgo-mcp") \
        && echo "OK: pgo-mcp instalado en $BIN_DIR/pgo-mcp" \
        || echo "AVISO: fallo al compilar pgo-mcp."
elif [ -x "$BIN_DIR/pgo-mcp" ]; then
    echo "OK: pgo-mcp ya presente en $BIN_DIR/pgo-mcp"
else
    echo "AVISO: 'go' no disponible y no existe $BIN_DIR/pgo-mcp."
fi
if [ -x "$BIN_DIR/pgo-mcp" ]; then
    ln -sfn pgo-mcp "$BIN_DIR/planesgo-mcp" && echo "OK: $BIN_DIR/planesgo-mcp enlaza a pgo-mcp (compatibilidad)."
fi

# 3. Plugin nativo de OpenCode (v2.1+) -----------------------------------------
cp "$SRC_DIR/.opencode/plugins/planesgo.ts" "$OPENCODE_PLUGINS_DIR/planesgo.ts"
echo "OK: plugin de OpenCode instalado en $OPENCODE_PLUGINS_DIR/planesgo.ts"

# 4. Comando /pgo en OpenCode (/planesgo ya no existe) -------------------------
rm -f "$OPENCODE_COMMANDS_DIR/planesgo.md"
if cp "$SRC_DIR/.claude/commands/pgo.md" "$OPENCODE_COMMANDS_DIR/pgo.md" 2>/dev/null; then
    echo "OK: comando /pgo instalado en $OPENCODE_COMMANDS_DIR/pgo.md"
fi

# 5. Configuración ~/.config/opencode/opencode.json (fusión idempotente) -------
OPENCODE_JSON="$OPENCODE_DIR/opencode.json"
python3 - "$OPENCODE_JSON" "$BIN_DIR/pgo-mcp" <<'PY_EOF'
import json, os, sys

cfg_path = sys.argv[1]
mcp_bin = sys.argv[2]
home = os.path.expanduser("~")

data = {}
if os.path.exists(cfg_path):
    try:
        with open(cfg_path, "r", encoding="utf-8") as f:
            data = json.load(f) or {}
    except Exception:
        data = {}

data.setdefault("$schema", "https://opencode.ai/config.json")
mcp = data.setdefault("mcp", {})
# Se retiran las entradas antiguas «planesgo» (la plana y la de mcp.servers, de instalaciones anteriores)
mcp.pop("planesgo", None)
servers = mcp.get("servers")
if isinstance(servers, dict):
    servers.pop("planesgo", None)
    if not servers:
        mcp.pop("servers", None)
mcp["pgo"] = {
    "type": "local",
    "command": [mcp_bin],
    "enabled": True,
}

rules_candidates = [
    os.path.join(home, ".gemini/config/plugins/planesgo-plugin/rules/AGENTS.md"),
    os.path.join(home, ".planesgo/PSF/CLAUDE.md"),
]
instructions = data.setdefault("instructions", [])
for cand in rules_candidates:
    if os.path.isfile(cand) and cand not in instructions:
        instructions.append(cand)
        break

with open(cfg_path, "w", encoding="utf-8") as f:
    json.dump(data, f, indent=2, ensure_ascii=False)
    f.write("\n")

print(f"OK: configuración MCP e instrucciones fusionadas en {cfg_path}")
PY_EOF

# 6. Recarga en caliente del demonio de OpenCode Desktop si está activo --------
OPENCODE_CLI=""
if command -v opencode >/dev/null 2>&1; then
    OPENCODE_CLI="opencode"
elif [ -x "/opt/OpenCode/resources/opencode-cli" ]; then
    OPENCODE_CLI="/opt/OpenCode/resources/opencode-cli"
fi

if [ -n "$OPENCODE_CLI" ]; then
    "$OPENCODE_CLI" reload >/dev/null 2>&1 && echo "OK: configuración recargada en OpenCode Desktop." || true
fi

# 7. Comprobación de autenticación ---------------------------------------------
if [ -n "${PLANESGO_TOKEN:-}" ] || [ -n "${ANTIGRAVITY_TOKEN:-}" ] || [ -f "$HOME/.planesgo_auth.json" ]; then
    echo "OK: credenciales de empleado detectadas (~/.planesgo_auth.json o entorno)."
else
    echo "AVISO: falta ~/.planesgo_auth.json (genera tu token en $PLANESGO_URL/settings)."
fi
