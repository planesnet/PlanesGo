# --- Etapa 1: Compilación (Builder) ---
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Instalar certificados CA y herramientas básicas
RUN apk add --no-cache ca-certificates git

# Descargar dependencias
COPY go.mod go.sum ./
RUN go mod download

# Copiar código fuente
COPY . .

# Compilar binario estático optimizado inyectando la versión desde VERSION
RUN VERSION_STR=$(tr -d '\r\n' < VERSION) && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-s -w -X main.Version=${VERSION_STR}" -o planesgo .

# --- Etapa 2: Imagen Final Ultraligera ---
FROM alpine:3.20

WORKDIR /app

# Instalar certificados para conexiones HTTPS seguras a Odoo y zona horaria
RUN apk --no-cache add ca-certificates tzdata

# Crear usuario no privilegiado para mayor seguridad
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Copiar binario y recursos necesarios desde el builder
COPY --from=builder /app/planesgo /app/planesgo
COPY --from=builder /app/VERSION /app/VERSION
COPY --from=builder /app/templates /app/templates
COPY --from=builder /app/static /app/static

# Crear directorio de datos persistentes y asignar permisos
RUN mkdir -p /app/data && chown -R appuser:appgroup /app
VOLUME ["/app/data"]
USER appuser

# Puerto por defecto para el servicio
EXPOSE 8080

# Variables de entorno por defecto
ENV PORT=8080
ENV ODOO_URL=https://planesnet.autopyme.com
ENV ODOO_DB=ap113

# Comando de inicio
ENTRYPOINT ["/app/planesgo"]
