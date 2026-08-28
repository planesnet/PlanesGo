# PlanesGo (v1.0.1)

Aplicación en Go para consultar y visualizar de forma interactiva y en tiempo real los partes de horas trabajadas (timesheets / `account.analytic.line`) de Odoo.

## 🚀 Características

- **Pantalla de Inicio de Sesión (Login):** Solicita de forma limpia y segura la URL del servidor Odoo, base de datos, usuario y contraseña o API Key.
- **Conexión nativa JSON-RPC:** Compatible con todas las versiones de Odoo (12, 13, 14, 15, 16, 17, 18).
- **Configuración persistente:** Opción para guardar o precargar los datos en [`config.yml`](file:///home/luis/cowork/planesnet_tr/pasigo/config.yml).
- **Dashboard interactivo:**
  - Métricas KPI: Total de horas trabajadas, registros recuperados, proyectos activos y usuarios/empleados.
  - Filtros instantáneos por texto/descripción, proyecto y empleado.
  - Tabla de registros detallada con badges de proyectos, tareas y formato de horas.
  - Opción de cerrar sesión y cambiar de cuenta.
- **API JSON:** Endpoint disponible en `/api/timesheets`.

## ⚙️ Configuración (`config.yml`)

```yaml
server:
  port: 8080

odoo:
  url: "https://www.planesnet.com"       # URL de tu instancia de Odoo
  db: ""                                # Base de datos (opcional si se ingresa en login)
  username: ""                          # Usuario / Email
  password: ""                          # Contraseña o API Key
  limit: 200                            # Límite de partes de horas
```

## 🏃 Ejecución

```bash
# Compilar y ejecutar
go build -o planesgo .
./planesgo
```

Acceso desde el navegador:
👉 **[http://localhost:8080](http://localhost:8080)**
