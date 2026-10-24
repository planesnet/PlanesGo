#!/bin/sh
# Instala sin root lo que necesitan los conectores (Linux):
#  - FreeRDP compilado desde la última versión publicada (SDL3 para la persona, X11 para el agente), con solo lo
#    moderno: H.264 por ffmpeg, PulseAudio, CUPS, FUSE y Kerberos. Código verificado con su SHA-256 y con los
#    parches de connectors/freerdp, si los hay.
#  - Xvfb y xdotool (el escritorio del agente de pgocode) y el servidor RDP de prueba (freerdp-shadow-cli): paquetes
#    de Ubuntu desempaquetados. Las capturas y el vídeo usan import (ImageMagick) y ffmpeg del sistema.
#  - pywinrm (PowerShell remoto) en un venv.
# Todo va en ~/.local/opt/rdp-agente. Se puede repetir: lo ya hecho no se rehace.
set -eu
FREERDP_VERSION=3.32.2
FREERDP_SHA256=e95a505c0057e4d06a3b120477c421028769deba4db9c639ef3465d050f0955c
D="$HOME/.local/opt/rdp-agente"
HERE=$(cd "$(dirname "$0")" && pwd)
# Parches propios de FreeRDP (connectors/freerdp/*.patch): su huella forma parte de la carpeta de cada compilación.
PATCHES=$(cat "$HERE"/freerdp/*.patch 2>/dev/null | sha256sum | cut -c1-12)
B="$D/build"
S="$B/sysroot"
P="$D/freerdp-$FREERDP_VERSION-$PATCHES"
LIBDIR=/usr/lib/x86_64-linux-gnu
mkdir -p "$D/debs" "$D/root" "$B/debs" "$S" "$HOME/.local/share/pgocode-connectors/intercambio/grabaciones"

# 1. Herramientas de Ubuntu: pantalla virtual, ratón y teclado, y el servidor RDP de prueba
cd "$D/debs"
apt-get download xvfb xdotool libxdo3 freerdp-shadow-x11 libfreerdp-shadow3-3 libfreerdp-shadow-subsystem3-3 libwinpr-tools3-3
for f in *.deb; do dpkg -x "$f" "$D/root"; done

# 2. FreeRDP desde el código fuente
# Se compila una vez y se guarda: solo se recompila con otra versión de FreeRDP o con otros parches. Cada compilación
# va en su carpeta (versión y huella de los parches) y el enlace «freerdp» se cambia al final, de golpe: los
# escritorios remotos abiertos siguen con la suya intacta (su binario y sus canales) y los nuevos usan la nueva.
if [ ! -f "$P/.completo" ]; then
  rm -rf "$P"
  # Cabeceras que no trae el sistema (las de ffmpeg, OpenSSL, X11 y Kerberos ya están)
  cd "$B/debs"
  apt-get download libsdl3-dev libsdl3-ttf-dev libsdl3-image-dev libsdl3-ttf0 libsdl3-image0 libwayland-dev \
    libpulse-dev libcups2-dev libfuse3-dev libjson-c-dev libxkbcommon-dev
  for f in *.deb; do dpkg -x "$f" "$S"; done
  for f in "$S"/usr/lib/x86_64-linux-gnu/pkgconfig/*.pc "$S"/usr/share/pkgconfig/*.pc; do
    [ -f "$f" ] && sed -i "s|=/usr|=$S/usr|g" "$f"
  done
  # Los .so de las cabeceras apuntan a librerías del sistema: se enlazan
  L="$S/usr/lib/x86_64-linux-gnu"
  for l in $(find "$L" -maxdepth 1 -xtype l); do
    t=$(readlink "$l"); [ -e "$LIBDIR/$t" ] && ln -sf "$LIBDIR/$t" "$l"
  done
  for base in $(ls "$L" | grep -E '\.so$' | sed 's/\.so$//'); do
    for f in "$LIBDIR/$base".so.*; do [ -e "$f" ] && [ ! -e "$L/$(basename "$f")" ] && ln -s "$f" "$L/"; done
  done

  mkdir -p "$B/src" && cd "$B/src"
  tgz="freerdp-$FREERDP_VERSION.tar.gz"
  [ -f "$tgz" ] || curl -fsSLO "https://pub.freerdp.com/releases/$tgz"
  echo "$FREERDP_SHA256  $tgz" | sha256sum -c -
  rm -rf "freerdp-$FREERDP_VERSION" build && tar xzf "$tgz"
  for p in "$HERE"/freerdp/*.patch; do [ -f "$p" ] && patch -p1 -d "freerdp-$FREERDP_VERSION" < "$p"; done
  PKG_CONFIG_PATH="$S/usr/lib/x86_64-linux-gnu/pkgconfig:$S/usr/share/pkgconfig" cmake -S "freerdp-$FREERDP_VERSION" -B build \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$P" -DCMAKE_PREFIX_PATH="$S/usr" -DBUILD_TESTING=OFF \
    -DWITH_SERVER=OFF -DWITH_SHADOW=OFF -DWITH_PROXY=OFF -DWITH_SAMPLE=OFF -DWITH_MANPAGES=OFF \
    -DWITH_CLIENT_SDL=ON -DWITH_CLIENT_SDL3=ON -DWITH_CLIENT_SDL2=OFF -DWITH_X11=ON -DWITH_WAYLAND=OFF \
    -DWITH_PULSE=ON -DWITH_ALSA=OFF -DWITH_OSS=OFF -DWITH_CUPS=ON -DWITH_FUSE=ON -DWITH_KRB5=ON \
    -DWITH_FFMPEG=ON -DWITH_SWSCALE=ON -DWITH_DSP_FFMPEG=ON -DWITH_PCSC=OFF -DWITH_SMARTCARD_PCSC=OFF \
    -DWITH_SMARTCARD_EMULATE=OFF -DCHANNEL_URBDRC=OFF -DWITH_CAIRO=OFF -DWITH_JPEG=OFF -DWITH_OPUS=OFF \
    -DWITH_ICU=OFF -DWITH_UNICODE_BUILTIN=ON -DWITH_WEBVIEW=OFF -DWITH_SDL_IMAGE_DIALOGS=OFF -DWITH_CCACHE=OFF \
    -DWITH_VERBOSE_WINPR_ASSERT=OFF
  cmake --build build -j"$(nproc)"
  cmake --install build
  cp -a "$L"/libSDL3_ttf.so.0* "$L"/libSDL3_image.so.0* "$P/lib/"  # el binario solo busca en su lib/
  touch "$P/.completo"  # una compilación a medias nunca cuenta
fi
ln -sfn "$P" "$D/freerdp"
# Las compilaciones anteriores, fuera, salvo las que use algún escritorio remoto abierto
for old in "$D"/freerdp-*/; do
  old=${old%/}
  [ "$old" = "$P" ] && continue
  grep -qsF "$old/" /proc/[0-9]*/maps || rm -rf "$old"
done

# 3. PowerShell remoto
[ -x "$D/venv/bin/python" ] || python3 -m venv "$D/venv"
"$D/venv/bin/python" -m pip install -q pywinrm

# Comprobación
for b in "$D/freerdp/bin/sdl-freerdp" "$D/freerdp/bin/xfreerdp"; do
  missing=$(env -u LD_LIBRARY_PATH ldd "$b" | grep 'not found' || true)
  [ -z "$missing" ] || { echo "$b: faltan librerías:"; echo "$missing"; exit 1; }
done
for b in Xvfb xdotool; do
  missing=$(LD_LIBRARY_PATH="$D/root/usr/lib/x86_64-linux-gnu" ldd "$D/root/usr/bin/$b" | grep 'not found' || true)
  [ -z "$missing" ] || { echo "$b: faltan librerías:"; echo "$missing"; exit 1; }
done
env -u LD_LIBRARY_PATH "$D/freerdp/bin/xfreerdp" /version | head -1
echo "Listo: $D"
