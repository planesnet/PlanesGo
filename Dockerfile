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

# Compilar binario estático optimizado
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-s -w" -o planesgo .

# --- Etapa 2: Imagen Final Ultraligera ---
FROM alpine:3.20

WORKDIR /app

# Instalar certificados para conexiones HTTPS seguras a Odoo y zona horaria
RUN apk --no-cache add ca-certificates tzdata

# Crear usuario no privilegiado para mayor seguridad
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Copiar binario y recursos necesarios desde el builder
COPY --from=builder /app/planesgo /app/planesgo
COPY --from=builder /app/templates /app/templates
COPY --from=builder /app/static /app/static
COPY --from=builder /app/config.example.yml /app/config.example.yml
COPY --from=builder /app/config.yml /app/config.yml

# Permisos
RUN chown -R appuser:appgroup /app
USER appuser

# Puerto por defecto para el servicio
EXPOSE 8080

# Variables de entorno
ENV PORT=8080

# Comando de inicio
ENTRYPOINT ["/app/planesgo"]
