#!/usr/bin/env bash
# ==============================================================================
# Script de compilación multiplataforma para planesgo-mcp
# Genera binarios estáticos para Linux y Windows (amd64 / arm64)
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$ROOT_DIR/bin"

mkdir -p "$BIN_DIR"

echo "==> Compilando planesgo-mcp para Linux (amd64)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$BIN_DIR/planesgo-mcp" "$ROOT_DIR/cmd/planesgo-mcp/main.go"
cp "$BIN_DIR/planesgo-mcp" "$BIN_DIR/planesgo-mcp-linux-amd64"

echo "==> Compilando planesgo-mcp para Windows (amd64)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o "$BIN_DIR/planesgo-mcp.exe" "$ROOT_DIR/cmd/planesgo-mcp/main.go"
cp "$BIN_DIR/planesgo-mcp.exe" "$BIN_DIR/planesgo-mcp-windows-amd64.exe"

echo "==> Compilación completada con éxito. Binarios generados en $BIN_DIR:"
ls -lh "$BIN_DIR"/planesgo-mcp*
