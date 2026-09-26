#!/usr/bin/env bash
# Hook de PlanesGo para Claude Code.
#
# Reutiliza el mismo binario y la misma cuenta que ya usa Antigravity
# (cmd/planesgo-mcp) para registrar en Odoo el tiempo dedicado a trabajos
# de codificación (Edit/Write/NotebookEdit/Bash) durante una sesión de
# Claude Code, sin tocar la lógica de negocio existente.
#
# Contrato de no interferencia: este script SIEMPRE termina en 0, nunca
# escribe nada en stdout (Claude Code muestra el stdout de SessionStart
# como contexto al modelo) y se convierte en no-op silencioso si el
# binario no está compilado o no hay token de Antigravity configurado.
set -uo pipefail

ACTION="${1:-beat}" # check | beat | stop
INPUT="$(cat 2>/dev/null || true)"

get_field() {
  local key="$1"
  if command -v jq >/dev/null 2>&1; then
    printf '%s' "$INPUT" | jq -r --arg k "$key" '.[$k] // empty' 2>/dev/null
  elif command -v python3 >/dev/null 2>&1; then
    printf '%s' "$INPUT" | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
    print(d.get('$key') or '')
except Exception:
    pass
" 2>/dev/null
  fi
}

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-}"
[ -z "$PROJECT_DIR" ] && PROJECT_DIR="$(get_field cwd)"
[ -z "$PROJECT_DIR" ] && PROJECT_DIR="$PWD"
cd "$PROJECT_DIR" 2>/dev/null || exit 0

# Localizar el binario planesgo-mcp (compilado con scripts/build_mcp.sh).
# Si no existe, no hacemos nada: el tracking es puramente aditivo.
BIN=""
for candidate in "$(command -v planesgo-mcp 2>/dev/null || true)" \
                 "$PROJECT_DIR/bin/planesgo-mcp" \
                 "$PROJECT_DIR/planesgo-mcp" \
                 "${HOME:-}/.local/bin/planesgo-mcp"; do
  if [ -n "$candidate" ] && [ -x "$candidate" ]; then
    BIN="$candidate"
    break
  fi
done
[ -z "$BIN" ] && exit 0

SCRATCH="$(get_field scratchpad_dir)"
SESSION_ID="$(get_field session_id)"
STATE_DIR="${SCRATCH:-${TMPDIR:-/tmp}/planesgo-track-${SESSION_ID:-default}}/planesgo-track"
mkdir -p "$STATE_DIR" 2>/dev/null || STATE_DIR="${TMPDIR:-/tmp}"
LAST_BEAT_FILE="$STATE_DIR/last_beat"
ACTIVE_FILE="$STATE_DIR/active"

TASK_NAME="Claude Code - Sesión IA"
TASK_TYPE="Desarrollo"
DESC="Trabajo de codificación asistido con Claude Code"

heartbeat_interval() {
  local interval=30
  if [ -f "$PROJECT_DIR/.planesgo.json" ] && command -v python3 >/dev/null 2>&1; then
    local cfg
    cfg=$(python3 -c "
import json
try:
    print(int(json.load(open('.planesgo.json')).get('heartbeat_interval_seconds', 30)))
except Exception:
    print(30)
" 2>/dev/null)
    [ -n "$cfg" ] && interval="$cfg"
  fi
  echo "$interval"
}

case "$ACTION" in
check)
  # Verificación best-effort de conexión/proyecto (Fase 0). Nunca bloquea
  # el arranque de la sesión: se lanza en segundo plano y se descarta.
  nohup "$BIN" --check --task "$TASK_NAME" --type "$TASK_TYPE" >/dev/null 2>&1 &
  disown 2>/dev/null || true
  ;;

beat)
  NOW=$(date +%s)
  if [ ! -f "$ACTIVE_FILE" ]; then
    # Primer latido de la sesión: se ejecuta en primer plano (el binario
    # tiene su propio timeout de red de 15s) para confirmar que el
    # temporizador ha quedado realmente iniciado en Odoo antes de marcar
    # la sesión como activa. Así evitamos un "stop" huérfano al final.
    if "$BIN" --beat --task "$TASK_NAME" --type "$TASK_TYPE" --desc "$DESC" >/dev/null 2>&1; then
      echo "$NOW" >"$ACTIVE_FILE"
      echo "$NOW" >"$LAST_BEAT_FILE"
    fi
  else
    INTERVAL="$(heartbeat_interval)"
    LAST=0
    [ -f "$LAST_BEAT_FILE" ] && LAST=$(cat "$LAST_BEAT_FILE" 2>/dev/null || echo 0)
    if [ $((NOW - LAST)) -ge "$INTERVAL" ]; then
      echo "$NOW" >"$LAST_BEAT_FILE"
      nohup "$BIN" --beat --task "$TASK_NAME" --type "$TASK_TYPE" --desc "$DESC" >/dev/null 2>&1 &
      disown 2>/dev/null || true
    fi
  fi
  ;;

stop)
  # Solo cerramos el parte si realmente llegamos a abrir uno (evita
  # generar imputaciones a 0h cuando la sesión no tocó código).
  if [ -f "$ACTIVE_FILE" ]; then
    "$BIN" --stop --task "$TASK_NAME" --type "$TASK_TYPE" --desc "Sesión Claude Code finalizada" >/dev/null 2>&1 || true
    rm -f "$ACTIVE_FILE" "$LAST_BEAT_FILE" 2>/dev/null || true
  fi
  ;;
esac

exit 0
