# Planes Go Code: compilar, probar y empaquetar los componentes.
VERSION    ?= $(shell cat VERSION)
LDFLAGS    := -s -w -X main.version=$(VERSION)
DIST       := dist
PLATAFORMAS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all test vet web pgocode studio-tv escritorio paquete instalar apparmor dist clean

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

# Paquete de la app para la autoactualización (lo publica PlanesGo en /install/pgocode/):
#   dist/pgocode-linux-amd64.tar.gz  (backend + ventana Electron + iconos + VERSION) y dist/version.json.
# Reproducible (mismo código, misma huella): así solo hay actualización cuando cambia algo de verdad.
ELECTRON_VER := $(shell sed -n 's/.*"electron": *"\([0-9.]*\)".*/\1/p' electron/package.json)
paquete: web
	rm -rf $(DIST)/paquete && mkdir -p $(DIST)/paquete/icono
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags '$(LDFLAGS)' -o $(DIST)/paquete/pgocode-backend ./cmd/pgocode
	cp electron/main.js electron/preload.js electron/package.json VERSION $(DIST)/paquete/
	cp electron/icono/pgocode*.png electron/icono/pgocode*.svg $(DIST)/paquete/icono/
	tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -C $(DIST)/paquete -cf - . | gzip -9n > $(DIST)/pgocode-linux-amd64.tar.gz
	@sha=$$(sha256sum $(DIST)/pgocode-linux-amd64.tar.gz | cut -d' ' -f1); bytes=$$(stat -c %s $(DIST)/pgocode-linux-amd64.tar.gz); \
	printf '{"version": "%s", "sha256": "%s", "bytes": %s, "archivo": "pgocode-linux-amd64.tar.gz", "electron": "%s"}\n' \
		"$(VERSION)" "$$sha" "$$bytes" "$(ELECTRON_VER)" > $(DIST)/version.json; cat $(DIST)/version.json

# Instala en ~/.local/share/pgocode lo que hay en este repositorio (versión de desarrollo: sin autoactualización).
# Para instalar la versión publicada (con autoactualización):
#   curl -fsSL https://planesgo.autopyme.com/install/pgocode/instalar.sh | sh
instalar: paquete
	cd electron && npm ci --no-fund --no-audit
	PGO_LOCAL=$(CURDIR)/$(DIST) PGO_ELECTRON_LOCAL=$(CURDIR)/electron/node_modules/electron/dist PGO_DESARROLLO=1 sh linux/instalar.sh

# Ubuntu 24.04+: perfil de AppArmor para que Electron (instalado en ~/.local/share/pgocode) pueda usar su sandbox.
apparmor:
	@echo "Instálalo con:"
	@echo "  sudo cp $(CURDIR)/linux/apparmor-pgocode /etc/apparmor.d/pgocode && sudo apparmor_parser -r /etc/apparmor.d/pgocode"

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
