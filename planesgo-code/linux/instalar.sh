#!/bin/sh
# Instalador de Planes Go Code (Linux x64), para el usuario actual:
#   curl -fsSL https://planesgo.autopyme.com/install/pgocode/instalar.sh | sh
# Deja el motor (Electron, con su ejecutable renombrado a «pgocode») en ~/.local/share/pgocode/motor, la app en
# ~/.local/share/pgocode/app, el comando «pgocode»
# en ~/.local/bin y la entrada en el menú de aplicaciones. Después la app se mantiene al día sola.
# Ubuntu 24.04+ (apparmor_restrict_unprivileged_userns=1): el motor va en /opt/pgocode/motor, propiedad de root,
# con su perfil de AppArmor en /etc/apparmor.d/pgocode. Pide sudo solo para eso (la primera vez y cuando cambia la
# versión de Electron o el perfil); la app y sus actualizaciones siguen siendo del usuario. El motor de /opt lo
# comparten los usuarios del equipo.
# Uso interno («make instalar»): PGO_LOCAL=<dir con version.json y el paquete>, PGO_ELECTRON_LOCAL=<dist de Electron>,
# PGO_DESARROLLO=1 (sin autoactualización).
set -eu
URL="${PGO_ACTUALIZACIONES:-https://planesgo.autopyme.com/install/pgocode}"
D="$HOME/.local/share/pgocode"
BIN="$HOME/.local/bin"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# Solo https (salvo 127.0.0.1/localhost, para pruebas)
case "$URL" in
  *@*) echo "Planes Go Code: servidor de actualizaciones no válido ($URL)"; exit 1 ;;
  https://*|http://127.0.0.1|http://127.0.0.1[:/]*|http://localhost|http://localhost[:/]*|http://\[::1\]*) ;;
  *) echo "Planes Go Code: el servidor de actualizaciones tiene que ser https ($URL)"; exit 1 ;;
esac

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

# Dónde va el motor: con la restricción de user namespaces, en /opt (root), que es adonde apunta su perfil.
if [ "$(cat /proc/sys/kernel/apparmor_restrict_unprivileged_userns 2>/dev/null)" = "1" ]; then
  MOTOR=/opt/pgocode/motor
  falta sudo
  echo "Ubuntu restringe el sandbox de Electron: el motor va en $MOTOR y su perfil de AppArmor en /etc/apparmor.d (con sudo, si hay que cambiarlos)."
else
  MOTOR="$D/motor"
fi

# Motor (Electron) solo si cambia de versión. Su ejecutable se renombra a «pgocode». Se prepara en $TMP y
# después se pone en su sitio (en /opt, propiedad de root y sin escritura para nadie más).
if [ "$(cat "$MOTOR/.version" 2>/dev/null)" != "$ELECTRON" ]; then
  if [ -n "${PGO_ELECTRON_LOCAL:-}" ]; then
    cp -r "$PGO_ELECTRON_LOCAL" "$TMP/motor"
  else
    falta curl; falta unzip
    echo "Descargando el motor (Electron $ELECTRON)…"
    ZIP="electron-v$ELECTRON-linux-x64.zip"
    REL="https://github.com/electron/electron/releases/download/v$ELECTRON"
    curl -fL# "$REL/$ZIP" -o "$TMP/$ZIP"
    curl -fsSL "$REL/SHASUMS256.txt" -o "$TMP/SHASUMS256.txt"
    SUMA=$(awk -v f="*$ZIP" -v g="$ZIP" '$2 == f || $2 == g { print $1 }' "$TMP/SHASUMS256.txt")
    [ -n "$SUMA" ] && echo "$SUMA  $TMP/$ZIP" | sha256sum -c --quiet - || {
      echo "La huella del motor (Electron $ELECTRON) no coincide con su SHASUMS256.txt: no se instala"; exit 1; }
    mv "$TMP/$ZIP" "$TMP/electron.zip"
    mkdir -p "$TMP/motor" && unzip -q "$TMP/electron.zip" -d "$TMP/motor"
  fi
  mv "$TMP/motor/electron" "$TMP/motor/pgocode"
  echo "$ELECTRON" > "$TMP/motor/.version"
  if [ "$MOTOR" = "$D/motor" ]; then
    rm -rf "$D/motor.nuevo" && mv "$TMP/motor" "$D/motor.nuevo"
    rm -rf "$D/motor" && mv "$D/motor.nuevo" "$D/motor"
  else
    sudo sh -c 'set -e; mkdir -p /opt/pgocode; chown root:root /opt/pgocode; chmod 755 /opt/pgocode
      rm -rf /opt/pgocode/motor.nuevo; cp -r "$1" /opt/pgocode/motor.nuevo
      chown -R root:root /opt/pgocode/motor.nuevo; chmod -R u+rwX,go+rX,go-w /opt/pgocode/motor.nuevo
      rm -rf /opt/pgocode/motor; mv /opt/pgocode/motor.nuevo /opt/pgocode/motor' sh "$TMP/motor"
  fi
fi
# Ubicaciones antiguas (con el motor en /opt, también la de ~/.local/share)
rm -rf "$D/electron"
[ "$MOTOR" = "$D/motor" ] || rm -rf "$D/motor" "$D/motor.nuevo"

# Perfil de AppArmor (sustituye al antiguo, que apuntaba a ~/.local/share y no era seguro)
if [ "$MOTOR" != "$D/motor" ]; then
  trae apparmor-pgocode "$TMP/apparmor-pgocode"
  grep -q "^profile pgocode $MOTOR/pgocode " "$TMP/apparmor-pgocode" || { echo "Perfil de AppArmor no válido"; exit 1; }
  if ! cmp -s "$TMP/apparmor-pgocode" /etc/apparmor.d/pgocode; then
    sudo install -o root -g root -m 644 "$TMP/apparmor-pgocode" /etc/apparmor.d/pgocode
    sudo apparmor_parser -r /etc/apparmor.d/pgocode
  fi
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
exec "$MOTOR/pgocode" "$D/app" "\$@"
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
