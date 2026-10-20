#!/bin/sh
set -e

# Si GCP_BILLING_SA_JSON apunta a un archivo accesible por root (ej. secret 0400), leer su contenido
if [ -n "$GCP_BILLING_SA_JSON" ] && [ -f "$GCP_BILLING_SA_JSON" ]; then
    export GCP_BILLING_SA_JSON="$(cat "$GCP_BILLING_SA_JSON")"
elif [ -f "/run/secrets/gcp_billing_sa_json.json" ]; then
    export GCP_BILLING_SA_JSON="$(cat "/run/secrets/gcp_billing_sa_json.json")"
elif [ -f "/run/secrets/gcp_billing_sa_json" ]; then
    export GCP_BILLING_SA_JSON="$(cat "/run/secrets/gcp_billing_sa_json")"
fi

# Si GOOGLE_APPLICATION_CREDENTIALS apunta a un secreto 0400 no legible por appuser,
# copiarlo a /tmp con permisos para que appuser pueda leerlo
if [ -n "$GOOGLE_APPLICATION_CREDENTIALS" ] && [ -f "$GOOGLE_APPLICATION_CREDENTIALS" ]; then
    if [ ! -r "$GOOGLE_APPLICATION_CREDENTIALS" ] || [ "$(stat -c %u "$GOOGLE_APPLICATION_CREDENTIALS" 2>/dev/null)" = "0" ]; then
        mkdir -p /tmp/credentials
        cp "$GOOGLE_APPLICATION_CREDENTIALS" /tmp/credentials/sa.json
        chmod 644 /tmp/credentials/sa.json
        chown -R appuser:appgroup /tmp/credentials
        export GOOGLE_APPLICATION_CREDENTIALS=/tmp/credentials/sa.json
    fi
fi

# Asegurar permisos sobre el directorio persistente de datos
mkdir -p /app/data
chown -R appuser:appgroup /app/data 2>/dev/null || true

# Ejecutar PlanesGo como appuser (no-root)
exec su-exec appuser:appgroup /app/planesgo "$@"
