#!/bin/sh
# Planes Go Code: abre la app de escritorio (Electron), que arranca su backend (dist/pgocode-backend).
# Lo instala «make instalar» en ~/.local/bin/pgocode.
DIR='@DIR@'
# Linux: sin ibus (si no conecta, GTK descarta las pulsaciones y no se puede escribir).
export GTK_IM_MODULE=gtk-im-context-simple XMODIFIERS=@im=none QT_IM_MODULE=
exec "$DIR/electron/node_modules/.bin/electron" "$DIR/electron" "$@"
