# Planes Go Code: compilar, probar y empaquetar los componentes.
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS    := -s -w -X main.version=$(VERSION)
DIST       := dist
PLATAFORMAS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all test vet web pgocode studio-tv escritorio instalar dist clean

all: vet test web pgocode studio-tv

# studio-tv es un módulo propio (también lo usa planesgo-server): se prueba aparte
vet:
	go vet ./...
	cd studio-tv && go vet ./...

test:
	go test -race ./...
	cd studio-tv && go test -race ./...

# Interfaz web (React + Vite) que se incluye en el binario pgocode
web:
	cd web && npm ci --no-fund --no-audit && npm run build

# Backend del arnés (con la interfaz dentro), en dist/pgocode-backend. Lo arranca la app de escritorio.
pgocode:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/pgocode-backend ./cmd/pgocode

# Ventana de escritorio (Electron) sobre dist/pgocode-backend. En el navegador: dist/pgocode-backend -navegador
escritorio: web pgocode
	cd electron && npm ci --no-fund --no-audit && GTK_IM_MODULE=gtk-im-context-simple XMODIFIERS=@im=none npm start

# Comando «pgocode» (abre la app de escritorio) en ~/.local/bin y entrada en el menú de aplicaciones
BIN_USUARIO ?= $(HOME)/.local/bin
instalar: web pgocode
	cd electron && npm ci --no-fund --no-audit
	mkdir -p $(BIN_USUARIO) $(HOME)/.local/share/applications
	sed 's#@DIR@#$(CURDIR)#' pgocode.sh > $(BIN_USUARIO)/pgocode && chmod 755 $(BIN_USUARIO)/pgocode
	for t in 16 32 48 64 128 256 512; do mkdir -p $(HOME)/.local/share/icons/hicolor/$${t}x$${t}/apps; \
		cp electron/icono/pgocode-$$t.png $(HOME)/.local/share/icons/hicolor/$${t}x$${t}/apps/pgocode.png; done
	mkdir -p $(HOME)/.local/share/icons/hicolor/scalable/apps && cp electron/icono/pgocode.svg $(HOME)/.local/share/icons/hicolor/scalable/apps/pgocode.svg
	sed 's#@BIN@#$(BIN_USUARIO)/pgocode#' pgocode.desktop > $(HOME)/.local/share/applications/pgocode.desktop
	-gtk-update-icon-cache -q $(HOME)/.local/share/icons/hicolor 2>/dev/null; -update-desktop-database -q $(HOME)/.local/share/applications 2>/dev/null
	@echo "Instalado: escribe «pgocode» o búscalo en el menú de aplicaciones."

# Binario para este equipo, en dist/studio-tv
studio-tv:
	go -C studio-tv build -trimpath -ldflags '$(LDFLAGS)' -o $(CURDIR)/$(DIST)/studio-tv ./cmd/studio-tv

# Binarios para todas las plataformas, en dist/studio-tv-<so>-<arq>[.exe]
dist:
	@for p in $(PLATAFORMAS); do \
		so=$${p%/*}; arq=$${p#*/}; ext=; [ $$so = windows ] && ext=.exe; \
		echo "studio-tv $$so/$$arq"; \
		CGO_ENABLED=0 GOOS=$$so GOARCH=$$arq go -C studio-tv build -trimpath -ldflags '$(LDFLAGS)' \
			-o $(CURDIR)/$(DIST)/studio-tv-$$so-$$arq$$ext ./cmd/studio-tv || exit 1; \
	done

clean:
	rm -rf $(DIST)
