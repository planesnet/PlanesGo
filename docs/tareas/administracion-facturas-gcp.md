# Tarea: apartado «Administración» con las facturas de Google Cloud

Encargo para implementar en PlanesGo. Pendiente de hacer.

## Requisitos previos en Google Cloud (fuera del código)

1. Activar la exportación de facturación a BigQuery («Datos de coste de uso estándar») en cada cuenta de facturación: Consola de Google Cloud → Facturación → Exportación de facturación → BigQuery. Solo guarda datos desde el día en que se activa.
2. Dar a la cuenta de servicio de PlanesGo `roles/bigquery.jobUser` en el proyecto de la consulta y `roles/bigquery.dataViewer` en el dataset de la exportación.
3. Configurar `GCP_BILLING_EXPORT_TABLE` (y, si hace falta, las demás variables de abajo) en el despliegue.

## Prompt

Implementa en PlanesGo (Go 1.22, net/http + html/template + Tailwind por CDN, módulo `pasigo`) un apartado nuevo «Administración». Sirve para ver las facturas de Google Cloud de los meses ya cerrados, sacadas de la exportación de facturación a BigQuery. Sigue DESIGN.md para el aspecto visual. Todos los textos de la interfaz van en español.

### 1. Acceso: solo luis@planesnet.com e isabel@planesnet.com

- Lista de administradores en la variable de entorno `ADMIN_EMAILS` (separados por comas). Si no está definida, usa por defecto `luis@planesnet.com,isabel@planesnet.com`. La comparación no distingue mayúsculas ni espacios.
- Añade el método `func (s *SessionData) IsAdmin() bool` en models.go. Debe aceptar una sesión nil y mirar `UserEmail`, o `Username` si `UserEmail` está vacío.
- MUY IMPORTANTE (seguridad): el email de la sesión NO es fiable salvo con `AuthMethod == "google"`. En `handleSettings` (POST), un usuario sin sesión puede escribir cualquier `odoo_user` y la sesión se queda con ese `UserEmail` y `AuthMethod "local"`. En `/admin` y en su API, exige las dos cosas: `session.IsAdmin()` y `session.AuthMethod == "google"`.
  - Si el email es de administrador pero la sesión no es de Google y Google OAuth está activo, redirige a `/auth/google?next=/admin`, igual que hace `/m` en handlers_dashboard.go (~línea 816).
  - Sin sesión, redirige a `/login?next=/admin`.
  - Cualquier otro caso devuelve 403. La API responde 401/403 en JSON.

### 2. Botón en el navbar

- En `templates/partials/navbar.html`, añade a la IZQUIERDA del botón «Ajustes» un enlace «Administración» a `/admin`, con el mismo estilo que Ajustes y un icono de línea fina (por ejemplo, un escudo o un documento).
- Envuélvelo en `{{if .Session.IsAdmin}}…{{end}}`.

### 3. Rutas (main.go → setupRoutes)

- `GET /admin` → página `templates/admin.html`. Es una página independiente con la misma cabecera y pie que settings.html, el botón «Volver al Dashboard» y el número de versión.
- `GET /api/admin/gcp-invoices` → JSON. Con `?refresh=1` se salta la caché.
- Pon los handlers en un fichero nuevo, `handlers_admin.go`.

### 4. Lectura de BigQuery (paquete nuevo `gcpbilling/`, SOLO librería estándar, sin dependencias nuevas en go.mod)

Configuración por variables de entorno:

- `GCP_BILLING_EXPORT_TABLE`: tabla de la exportación estándar de costes, `proyecto.dataset.tabla`.
  - Si solo trae `proyecto.dataset`, usa el comodín `proyecto.dataset.gcp_billing_export_v1_*`, que cubre TODAS las cuentas de facturación exportadas a ese dataset.
  - Solo debe coger `gcp_billing_export_v1_*` y no `gcp_billing_export_resource_v1_*`, para no contar los costes dos veces.
- `GCP_BILLING_QUERY_PROJECT` (opcional): proyecto donde se lanza la consulta. Por defecto, el proyecto de la tabla.
- `GCP_BILLING_LOCATION` (opcional): ubicación del dataset (EU, US, europe-west1…).

Credenciales, por este orden:

1. `GCP_BILLING_SA_JSON`: JSON de la clave de una cuenta de servicio, metido directamente en la variable.
2. `GOOGLE_APPLICATION_CREDENTIALS`: ruta a ese mismo JSON.
3. Servidor de metadatos: `http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token`, con la cabecera `Metadata-Flavor: Google`.

Para la cuenta de servicio:

- Firma un JWT RS256 con `crypto/rsa` y pide el token a `https://oauth2.googleapis.com/token` con `grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer`.
- Scope: `https://www.googleapis.com/auth/bigquery`.
- Guarda el token en caché hasta un minuto antes de que caduque.

Consulta: `POST https://bigquery.googleapis.com/bigquery/v2/projects/{proyecto}/queries` con `useLegacySql:false` y `timeoutMs:30000`.

- Si la respuesta trae `jobComplete:false` o `pageToken`, sigue con `GET .../queries/{jobId}?location=…&pageToken=…` hasta tener todas las filas.
- Los valores llegan como strings dentro de `rows[].f[].v`.

SQL:

```sql
SELECT
  billing_account_id,
  invoice.month AS invoice_month,              -- 'YYYYMM'
  currency,
  SUM(cost) AS cost,
  SUM(IFNULL((SELECT SUM(c.amount) FROM UNNEST(credits) c), 0)) AS credits,
  SUM(IF(cost_type = 'tax', cost, 0)) AS tax,
  COUNT(DISTINCT project.id) AS projects
FROM `<tabla o comodín>`
WHERE invoice.month IS NOT NULL
  AND invoice.month < FORMAT_DATE('%Y%m', CURRENT_DATE('America/Los_Angeles'))  -- solo meses ya cerrados/facturados
GROUP BY billing_account_id, invoice_month, currency
ORDER BY invoice_month DESC, billing_account_id
```

Cálculo:

- Total de la factura = `cost + credits`. Los créditos son negativos. El coste ya incluye el impuesto y los ajustes de redondeo.
- Fecha de emisión: la exportación no la trae. Google emite la factura o extracto al principio del mes siguiente, así que toma como emisión estimada el día 1 del mes siguiente a `invoice_month`. En la interfaz debe aparecer como «Emisión (estimada)».

Guarda el resultado en memoria 15 minutos (protegido con mutex), para no lanzar una consulta a BigQuery en cada visita.

### 5. JSON de la API

Agrupado por año y luego por mes, todo en orden DESCENDENTE por fecha de emisión:

```json
{
  "success": true,
  "generated_at": "...",
  "source": "proyecto.dataset.gcp_billing_export_v1_*",
  "years": [
    {
      "year": 2026,
      "total_by_currency": {"EUR": 1234.56},
      "months": [
        {
          "invoice_month": "2026-09",
          "label": "Septiembre 2026",
          "issue_date": "2026-10-01",
          "invoices": [
            {"billing_account_id": "XXXXXX-XXXXXX-XXXXXX", "currency": "EUR", "cost": 0, "credits": 0, "tax": 0, "total": 0, "projects": 3}
          ]
        }
      ]
    }
  ]
}
```

Si falta la configuración o falla BigQuery, devuelve `{"success": false, "error": "<mensaje claro en español>"}`. Si el problema es de configuración, el mensaje debe decir qué variable falta.

### 6. Página admin.html

- Se carga y llama a la API con `fetch`. Mientras tanto muestra un estado de carga, y tiene un botón «Actualizar» que llama con `?refresh=1`.
- Muestra una sección por año (con el total del año) y, dentro, una tarjeta o fila por mes, de la más reciente a la más antigua. Por cada factura:
  - Cuenta de facturación, con enlace a `https://console.cloud.google.com/billing/<ID>`.
  - Emisión (estimada).
  - Coste, créditos, impuestos y total, con formato `es-ES` y la moneda que corresponda.
  - Número de proyectos.
- Usa una tabla con el estilo de DESIGN.md §4.4 y la píldora del total en `bg-sky-50 text-sky-700`.
- Si la API devuelve un error, muéstralo en un aviso con instrucciones para activar la exportación: Consola de Google Cloud → Facturación → Exportación de facturación → BigQuery, «Datos de coste de uso estándar». Recuerda en ese aviso que la exportación solo tiene datos desde que se activa.

### 7. Tests (`go test ./...` debe pasar) y documentación

- Tests en el paquete `gcpbilling`:
  - Parseo de una respuesta de jobs.query de ejemplo.
  - Agrupación año/mes y orden descendente.
  - Cálculo de la fecha de emisión, incluido el paso de diciembre a enero.
  - JWT firmado contra un servidor `httptest` que haga de servidor de tokens, usando una clave RSA generada en el propio test.
- Tests de handlers:
  - `/admin` sin sesión → redirección.
  - Sesión de Google con email que no es de administrador → 403.
  - Sesión `local` con el email de luis → no da acceso.
  - Sesión de Google de luis → 200.
- Test de plantilla: el navbar muestra «Administración» solo a los administradores.
- Ejecuta también `gofmt` y `go vet ./...`.
- Documenta las variables nuevas (`ADMIN_EMAILS`, `GCP_BILLING_EXPORT_TABLE`, `GCP_BILLING_QUERY_PROJECT`, `GCP_BILLING_LOCATION`, `GCP_BILLING_SA_JSON`) en README.md, junto a las demás variables de entorno.
- No toques el comportamiento existente de login ni de ajustes.
