#!/usr/bin/env bash
# ==============================================================================
# Script de compilación multiplataforma para pgo-mcp (servidor MCP «pgo» de PlanesGo)
# Genera binarios estáticos para Linux y Windows (amd64) desde el paquete cmd/pgo-mcp.
# planesgo-mcp queda como enlace simbólico a pgo-mcp, para los hooks que aún lo llaman así.
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$ROOT_DIR/bin"
MCP_DIR="$ROOT_DIR/cmd/pgo-mcp"

mkdir -p "$BIN_DIR"

echo "==> Compilando pgo-mcp para Linux (amd64)..."
(cd "$ROOT_DIR" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$BIN_DIR/pgo-mcp" ./cmd/pgo-mcp)
cp "$BIN_DIR/pgo-mcp" "$BIN_DIR/pgo-mcp-linux-amd64"
ln -sfn pgo-mcp "$BIN_DIR/planesgo-mcp"
if [ -d "$HOME/.local/bin" ]; then
    cp --remove-destination "$BIN_DIR/pgo-mcp" "$HOME/.local/bin/pgo-mcp"
    ln -sfn pgo-mcp "$HOME/.local/bin/planesgo-mcp"
fi

echo "==> Preparando metadatos y manifiesto para Windows (amd64)..."
if command -v x86_64-w64-mingw32-windres &>/dev/null && [ -f "$MCP_DIR/versioninfo.rc" ]; then
    (cd "$MCP_DIR" && x86_64-w64-mingw32-windres -i versioninfo.rc -o rsrc_windows_amd64.syso -O coff)
fi

echo "==> Compilando pgo-mcp para Windows (amd64)..."
# Se preserva la información de símbolos de Go y se incrusta el recurso de Windows
# para evitar falsos positivos heurísticos de antivirus (Microsoft Defender Wacatac.B!ml).
(cd "$MCP_DIR" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "$BIN_DIR/pgo-mcp.exe" .)
cp "$BIN_DIR/pgo-mcp.exe" "$BIN_DIR/pgo-mcp-windows-amd64.exe"
rm -f "$MCP_DIR/rsrc_windows_amd64.syso"

echo "==> Compilación completada con éxito. Binarios generados en $BIN_DIR:"
ls -lh "$BIN_DIR"/pgo-mcp* "$BIN_DIR/planesgo-mcp"
