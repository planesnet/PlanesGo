# Monorepo PlanesGo: el servidor está en planesgo-server/. Este Dockerfile sigue en la raíz para que el
# despliegue automático no cambie (contexto de compilación = raíz del repositorio).

# --- Planes Go Code (planesgo-code): interfaz web ---
FROM node:20-alpine AS pgocode-web
WORKDIR /src/web
COPY planesgo-code/web/package.json planesgo-code/web/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY planesgo-code/web/ ./
RUN npm run build

# --- Planes Go Code: paquete para la instalación y la autoactualización (/install/pgocode/) ---
# Mismo contenido que «make paquete» (planesgo-code/Makefile): backend + ventana Electron + iconos + VERSION,
# en un tar.gz reproducible, y version.json con su huella sha256.
FROM golang:1.24-alpine AS pgocode
RUN apk add --no-cache tar gzip
WORKDIR /src/planesgo-code
COPY planesgo-code/go.mod planesgo-code/go.sum ./
COPY planesgo-code/studio-tv ./studio-tv
RUN go mod download
COPY planesgo-code/ ./
COPY --from=pgocode-web /src/web/dist ./web/dist
RUN VERSION=$(tr -d '\r\n' < VERSION) && ELECTRON=$(sed -n 's/.*"electron": *"\([0-9.]*\)".*/\1/p' electron/package.json) && \
    mkdir -p /out/paquete/icono /out/publicar && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags "-s -w -X main.version=${VERSION}" -o /out/paquete/pgocode-backend ./cmd/pgocode && \
    cp electron/main.js electron/preload.js electron/package.json VERSION /out/paquete/ && \
    cp electron/icono/pgocode*.png electron/icono/pgocode*.svg /out/paquete/icono/ && \
    tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -C /out/paquete -cf - . | gzip -9n > /out/publicar/pgocode-linux-amd64.tar.gz && \
    SHA=$(sha256sum /out/publicar/pgocode-linux-amd64.tar.gz | cut -d' ' -f1) && BYTES=$(stat -c %s /out/publicar/pgocode-linux-amd64.tar.gz) && \
    printf '{"version": "%s", "sha256": "%s", "bytes": %s, "archivo": "pgocode-linux-amd64.tar.gz", "electron": "%s"}\n' "$VERSION" "$SHA" "$BYTES" "$ELECTRON" > /out/publicar/version.json && \
    cp linux/instalar.sh linux/apparmor-pgocode /out/publicar/ && cat /out/publicar/version.json

# --- Etapa 1: Compilación (Builder) ---
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Instalar certificados CA y herramientas básicas
RUN apk add --no-cache ca-certificates git

# Módulo compartido Studio TV (go.mod: replace planesgo-code/studio-tv => ../planesgo-code/studio-tv)
COPY planesgo-code/studio-tv /planesgo-code/studio-tv

# Descargar dependencias
COPY planesgo-server/go.mod planesgo-server/go.sum ./
RUN go mod download

# Copiar código fuente
COPY planesgo-server/ .

# Compilar binario estático optimizado inyectando la versión desde VERSION
RUN VERSION_STR=$(tr -d '\r\n' < VERSION) && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-s -w -X main.Version=${VERSION_STR}" -o planesgo .

# --- Etapa 2: Imagen Final Ultraligera ---
FROM alpine:3.20

WORKDIR /app

# Instalar certificados para conexiones HTTPS seguras a Odoo, zona horaria y su-exec
RUN apk --no-cache add ca-certificates tzdata su-exec

# Crear usuario no privilegiado para mayor seguridad
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Copiar binario y recursos necesarios desde el builder
COPY --from=builder /app/planesgo /app/planesgo
COPY --from=builder /app/VERSION /app/VERSION
COPY --from=builder /app/templates /app/templates
COPY --from=builder /app/static /app/static
COPY --from=pgocode /out/publicar/ /app/pgocode/
COPY planesgo-server/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

# Crear directorio de datos persistentes y asignar permisos
RUN mkdir -p /app/data && chown -R appuser:appgroup /app
VOLUME ["/app/data"]

# Puerto por defecto para el servicio
EXPOSE 8080

# Variables de entorno por defecto
ENV PORT=8080
ENV TZ=Europe/Madrid
ENV ODOO_URL=https://planesnet.autopyme.com
ENV ODOO_DB=ap113
ENV GCP_BILLING_EXPORT_TABLE=planes-dev.gcp_billing_export

# Comando de inicio seguro (resuelve secretos de Docker con root y ejecuta como appuser)
ENTRYPOINT ["/app/docker-entrypoint.sh"]
