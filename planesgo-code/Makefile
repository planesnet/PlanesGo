# PSFCode: compilar, probar y empaquetar los componentes.
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS    := -s -w -X main.version=$(VERSION)
DIST       := dist
PLATAFORMAS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all test vet web psfcode studio-tv escritorio dist clean

all: vet test web psfcode studio-tv

# studio-tv es un módulo propio (también lo usa planesgo-server): se prueba aparte
vet:
	go vet ./...
	cd studio-tv && go vet ./...

test:
	go test -race ./...
	cd studio-tv && go test -race ./...

# Interfaz web (React + Vite) que se incluye en el binario psfcode
web:
	cd web && npm ci --no-fund --no-audit && npm run build

# Backend del arnés (con la interfaz dentro), en dist/psfcode
psfcode:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/psfcode ./cmd/psfcode

# Ventana de escritorio (Electron) sobre dist/psfcode
escritorio: web psfcode
	cd electron && npm ci --no-fund --no-audit && npm start

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
