#!/bin/sh
# Instalador de Planes Go Code (Linux x64), para el usuario actual (sin sudo):
#   curl -fsSL https://planesgo.autopyme.com/install/pgocode/instalar.sh | sh
# Deja Electron en ~/.local/share/pgocode/electron, la app en ~/.local/share/pgocode/app, el comando «pgocode»
# en ~/.local/bin y la entrada en el menú de aplicaciones. Después la app se mantiene al día sola.
# Uso interno («make instalar»): PGO_LOCAL=<dir con version.json y el paquete>, PGO_ELECTRON_LOCAL=<dist de Electron>,
# PGO_DESARROLLO=1 (sin autoactualización).
set -eu
URL="${PGO_ACTUALIZACIONES:-https://planesgo.autopyme.com/install/pgocode}"
D="$HOME/.local/share/pgocode"
BIN="$HOME/.local/bin"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

falta() { command -v "$1" >/dev/null 2>&1 || { echo "Planes Go Code: falta «$1»"; exit 1; }; }
falta tar; falta sha256sum
trae() { # trae <fichero> <destino>
  if [ -n "${PGO_LOCAL:-}" ]; then cp "$PGO_LOCAL/$1" "$2"; else falta curl; curl -fsSL "$URL/$1" -o "$2"; fi
}
campo() { sed -n "s/.*\"$1\": *\"\{0,1\}\([^\",}]*\)\"\{0,1\}.*/\1/p" "$TMP/version.json" | head -1; }

trae version.json "$TMP/version.json"
VER=$(campo version); SHA=$(campo sha256); ARCHIVO=$(campo archivo); ELECTRON=$(campo electron)
[ -n "$SHA" ] && [ -n "$ARCHIVO" ] && [ -n "$ELECTRON" ] || { echo "version.json no válido"; exit 1; }
echo "Planes Go Code $VER (Electron $ELECTRON)"
mkdir -p "$D" "$BIN"

# Electron (solo si cambia de versión)
if [ "$(cat "$D/electron/.version" 2>/dev/null)" != "$ELECTRON" ]; then
  rm -rf "$D/electron.nuevo"
  if [ -n "${PGO_ELECTRON_LOCAL:-}" ]; then
    cp -r "$PGO_ELECTRON_LOCAL" "$D/electron.nuevo"
  else
    falta curl; falta unzip
    echo "Descargando Electron $ELECTRON…"
    curl -fL# "https://github.com/electron/electron/releases/download/v$ELECTRON/electron-v$ELECTRON-linux-x64.zip" -o "$TMP/electron.zip"
    mkdir -p "$D/electron.nuevo" && unzip -q "$TMP/electron.zip" -d "$D/electron.nuevo"
  fi
  echo "$ELECTRON" > "$D/electron.nuevo/.version"
  rm -rf "$D/electron" && mv "$D/electron.nuevo" "$D/electron"
fi

# App (paquete con huella comprobada)
trae "$ARCHIVO" "$TMP/app.tar.gz"
echo "$SHA  $TMP/app.tar.gz" | sha256sum -c --quiet - || { echo "La huella del paquete no coincide: no se instala"; exit 1; }
rm -rf "$D/app.nuevo" && mkdir -p "$D/app.nuevo"
tar -xzf "$TMP/app.tar.gz" -C "$D/app.nuevo"
if [ -n "${PGO_DESARROLLO:-}" ]; then echo desarrollo > "$D/app.nuevo/paquete.sha256"; else echo "$SHA" > "$D/app.nuevo/paquete.sha256"; fi
rm -rf "$D/app.anterior" "$D/actualizacion/nuevo"
[ -d "$D/app" ] && mv "$D/app" "$D/app.anterior"
mv "$D/app.nuevo" "$D/app"

# Comando, menú e iconos
cat > "$BIN/pgocode" <<LANZADOR
#!/bin/sh
# Planes Go Code: abre la app de escritorio (instalada en $D).
# Linux: sin ibus (si no conecta, GTK descarta las pulsaciones) y enlace de la ventana con su icono.
export GTK_IM_MODULE=gtk-im-context-simple XMODIFIERS=@im=none QT_IM_MODULE= CHROME_DESKTOP=pgocode.desktop
exec "$D/electron/electron" "$D/app" "\$@"
LANZADOR
chmod 755 "$BIN/pgocode"
for t in 16 32 48 64 128 256 512; do
  mkdir -p "$HOME/.local/share/icons/hicolor/${t}x${t}/apps"
  cp "$D/app/icono/pgocode-$t.png" "$HOME/.local/share/icons/hicolor/${t}x${t}/apps/pgocode.png"
done
mkdir -p "$HOME/.local/share/icons/hicolor/scalable/apps" "$HOME/.local/share/applications"
cp "$D/app/icono/pgocode.svg" "$HOME/.local/share/icons/hicolor/scalable/apps/pgocode.svg"
cat > "$HOME/.local/share/applications/pgocode.desktop" <<ESCRITORIO
[Desktop Entry]
Type=Application
Name=Planes Go Code
Comment=Arnés de desarrollo de Planes Software Factory
Exec=$BIN/pgocode
Icon=pgocode
StartupWMClass=pgocode
Terminal=false
Categories=Development;
ESCRITORIO
gtk-update-icon-cache -q "$HOME/.local/share/icons/hicolor" 2>/dev/null || true

echo "Instalado. Ábrelo desde el menú de aplicaciones o con «pgocode»."
if [ "$(cat /proc/sys/kernel/apparmor_restrict_unprivileged_userns 2>/dev/null)" = "1" ] && ! grep -qs "share/pgocode/electron" /etc/apparmor.d/pgocode; then
  echo
  echo "Ubuntu restringe el sandbox de Electron: una sola vez, instala su perfil de AppArmor con:"
  echo "  curl -fsSL $URL/apparmor-pgocode | sudo tee /etc/apparmor.d/pgocode >/dev/null && sudo apparmor_parser -r /etc/apparmor.d/pgocode"
fi
