#!/usr/bin/env bash
# ==============================================================================
# Script de compilación multiplataforma para planesgo-mcp
# Genera binarios estáticos para Linux y Windows (amd64)
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$ROOT_DIR/bin"
MCP_DIR="$ROOT_DIR/cmd/planesgo-mcp"

mkdir -p "$BIN_DIR"

echo "==> Compilando planesgo-mcp para Linux (amd64)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$BIN_DIR/planesgo-mcp" "$MCP_DIR/main.go"
cp "$BIN_DIR/planesgo-mcp" "$BIN_DIR/planesgo-mcp-linux-amd64"
if [ -d "$HOME/.local/bin" ]; then
    cp "$BIN_DIR/planesgo-mcp" "$HOME/.local/bin/planesgo-mcp"
fi

echo "==> Preparando metadatos y manifiesto para Windows (amd64)..."
if command -v x86_64-w64-mingw32-windres &>/dev/null && [ -f "$MCP_DIR/versioninfo.rc" ]; then
    (cd "$MCP_DIR" && x86_64-w64-mingw32-windres -i versioninfo.rc -o rsrc_windows_amd64.syso -O coff)
fi

echo "==> Compilando planesgo-mcp para Windows (amd64)..."
# Se preserva la información de símbolos de Go y se incrusta el recurso de Windows
# para evitar falsos positivos heurísticos de antivirus (Microsoft Defender Wacatac.B!ml).
(cd "$MCP_DIR" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "$BIN_DIR/planesgo-mcp.exe" .)
cp "$BIN_DIR/planesgo-mcp.exe" "$BIN_DIR/planesgo-mcp-windows-amd64.exe"
rm -f "$MCP_DIR/rsrc_windows_amd64.syso"

echo "==> Compilación completada con éxito. Binarios generados en $BIN_DIR:"
ls -lh "$BIN_DIR"/planesgo-mcp*
