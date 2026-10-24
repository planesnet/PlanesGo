# Planes Go Code: compilar, probar y empaquetar los componentes.
VERSION    ?= $(shell cat VERSION)
LDFLAGS    := -s -w -X main.version=$(VERSION)
DIST       := dist
PLATAFORMAS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all test vet consumo web pgocode studio-tv escritorio paquete publicar clave-firma instalar apparmor dist clean \
	verifica formato prueba-rapida integracion web-verifica e2e arbol-limpio

all: vet test web pgocode studio-tv

# studio-tv es un módulo propio (también lo usa planesgo-server): se prueba aparte
vet:
	go vet ./...
	cd studio-tv && go vet ./...

test:
	go test -race ./...
	cd studio-tv && go test -race ./...
	cd web && npm test
	node --test electron/*.test.js

# ---- Control de calidad: nada se publica sin pasar «make verifica» ----

# Todo lo que se exige antes de publicar: formato, vet, pruebas con -race (sin caché), tipos y pruebas de la
# interfaz, y la prueba de humo de punta a punta (Odoo simulado, sin agentes ni tokens).
verifica: formato vet
	go test -race -count=1 ./...
	cd studio-tv && go test -race -count=1 ./...
	$(MAKE) web-verifica e2e

# gofmt sin nada pendiente (go fmt ./... o gofmt -w lo arregla)
GOFMT_DIRS := cmd internal web/embed.go studio-tv
formato:
	@f=$$(gofmt -l $(GOFMT_DIRS)); [ -z "$$f" ] || { echo "Ficheros sin gofmt (arréglalo con: gofmt -w $(GOFMT_DIRS)):"; echo "$$f"; exit 1; }

# Tipos (tsc) y pruebas de la interfaz (sin compilarla) y de la parte de Electron que se puede probar sin él
web-verifica:
	cd web && npx tsc --noEmit -p . && npm test
	node --test electron/*.test.js

# Prueba de humo de punta a punta (cmd/e2e): compila y arranca odoo-prueba y pgocode en puertos libres y con
# datos temporales, recorre la API y lo para todo al acabar. Con más detalle: go run ./cmd/e2e -v
e2e:
	go run ./cmd/e2e

# Banco de consumo de tokens (consumo/): las pruebas con cada agente, sin pgocode (calibración) y con él.
# Gasta tokens de verdad. Opciones: make consumo ARGS="-pruebas 1,2,3 -repeticiones 1"
consumo:
	go run ./cmd/consumo $(ARGS)

# Lo rápido mientras se programa: vet y pruebas sin -race
prueba-rapida: vet
	go test ./...
	cd studio-tv && go test ./...

# Pruebas que dependen del entorno (~/cowork, python3, procesos y puertos reales): go test -tags integracion
integracion:
	go vet -tags integracion ./...
	go test -race -count=1 -tags integracion ./...

# Publicar exige el árbol sin cambios (lo publicado es exactamente lo que hay en git)
arbol-limpio:
	@c=$$(git status --porcelain -- . ':!web/node_modules'); [ -z "$$c" ] || { echo "Hay cambios sin commit: no se publica."; echo "$$c"; exit 1; }

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
	cp electron/main.js electron/preload.js electron/googleReader.js electron/package.json VERSION $(DIST)/paquete/
	cp electron/icono/pgocode*.png electron/icono/pgocode*.svg $(DIST)/paquete/icono/
	cp -r connectors $(DIST)/paquete/connectors
	find $(DIST)/paquete/connectors -name __pycache__ -prune -exec rm -rf {} +  # reproducible: sin .pyc locales
	tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -C $(DIST)/paquete -cf - . | gzip -9n > $(DIST)/pgocode-linux-amd64.tar.gz
	@sha=$$(sha256sum $(DIST)/pgocode-linux-amd64.tar.gz | cut -d' ' -f1); bytes=$$(stat -c %s $(DIST)/pgocode-linux-amd64.tar.gz); \
	printf '{"version": "%s", "sha256": "%s", "bytes": %s, "archivo": "pgocode-linux-amd64.tar.gz", "electron": "%s"}\n' \
		"$(VERSION)" "$$sha" "$$bytes" "$(ELECTRON_VER)" > $(DIST)/version.json; cat $(DIST)/version.json

# Publica la versión (VERSION) en PlanesGo sin reconstruirlo: las apps la ven en menos de un minuto y se
# actualizan solas. Solo administradores: usa el token de PlanesGo ($PLANESGO_TOKEN o ~/.planesgo_auth.json).
# Antes: árbol sin cambios y «make verifica» entero.
# version.json va firmado (ed25519) con la clave privada PGO_CLAVE_FIRMA, que nunca entra en el repositorio; las
# apps solo instalan lo firmado con la clave pública que llevan (internal/actualizacion/actualizacion.go).
PUBLICAR_EN ?= https://planesgo.autopyme.com/admin/pgocode/publicar
PGO_CLAVE_FIRMA ?= $(HOME)/.config/pgocode/pgocode-update-signing-key.pem
# Token del servidor de actualizaciones (fichero 600, fuera del repositorio).
PGO_TOKEN_FICHERO ?= $(HOME)/.config/pgocode/publicar-token
publicar: arbol-limpio verifica paquete
	@tok="$${PGO_TOKEN_PUBLICAR:-$$(cat "$(PGO_TOKEN_FICHERO)" 2>/dev/null)}"; \
	[ -n "$$tok" ] || { echo "Falta el token de publicación (PGO_TOKEN_PUBLICAR o $(PGO_TOKEN_FICHERO))"; exit 1; }; \
	firma=$$(go run ./cmd/firmar-version -clave "$(PGO_CLAVE_FIRMA)" $(DIST)/version.json) || { \
		echo "No se pudo firmar la versión (clave: $(PGO_CLAVE_FIRMA); ver «make clave-firma»)"; exit 1; }; \
	curl -fsS -H "X-Antigravity-Token: $$tok" -F version=$(VERSION) -F electron=$(ELECTRON_VER) -F "firma=$$firma" \
		-F paquete=@$(DIST)/pgocode-linux-amd64.tar.gz $(PUBLICAR_EN) && echo && echo "Publicada la versión $(VERSION)."

# Clave de firma de las versiones (una sola vez; guarda una copia del fichero en el gestor de contraseñas: si se
# pierde, las apps instaladas no aceptarán ninguna versión nueva hasta reinstalarlas a mano).
#   make clave-firma            crea $(PGO_CLAVE_FIRMA) (0600) si no existe y muestra la clave pública
# La pública va en la constante clavePublica de internal/actualizacion/actualizacion.go. Equivale a:
#   openssl genpkey -algorithm ed25519 -out <fichero>
clave-firma:
	@if [ -f "$(PGO_CLAVE_FIRMA)" ]; then go run ./cmd/firmar-version -publica -clave "$(PGO_CLAVE_FIRMA)"; \
	else go run ./cmd/firmar-version -generar -clave "$(PGO_CLAVE_FIRMA)"; fi

# Instala en ~/.local/share/pgocode lo que hay en este repositorio (versión de desarrollo: sin autoactualización).
# Para instalar la versión publicada (con autoactualización):
#   curl -fsSL https://planesgo.autopyme.com/install/pgocode/instalar.sh | sh
instalar: paquete
	cd electron && npm ci --no-fund --no-audit
	PGO_LOCAL=$(CURDIR)/$(DIST) PGO_ELECTRON_LOCAL=$(CURDIR)/electron/node_modules/electron/dist PGO_DESARROLLO=1 sh linux/instalar.sh

# Ubuntu 24.04+: perfil de AppArmor para que el motor (~/.local/share/pgocode/motor/pgocode) pueda usar su sandbox.
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
