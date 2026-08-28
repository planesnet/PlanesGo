# PlanesGo (v1.0.1)

Aplicación moderna, minimalista y de alto rendimiento en Go para consultar y visualizar de forma interactiva los partes de horas trabajadas (timesheets / `account.analytic.line`) y proyectos (`project.project`) de Odoo.

![Planes Soluciones Informáticas](static/img/logo.png)

---

## 🚀 Características

- **Autenticación Prioritaria con Google OAuth 2.0:** Acceso con un solo clic con cuentas de Google / Google Workspace.
- **Acceso alternativo con credenciales Odoo:** Compatible con usuario y contraseña / API Key.
- **Visualización de Proyectos de Odoo:** Navegación por pestañas entre Partes de Horas y Proyectos (`project.project`), con métricas agregadas y filtros rápidos.
- **Identidad Corporativa Planes:** Diseño visual moderno, funcional y minimalista basado en la marca oficial de **PLANES Soluciones Informáticas**.
- **100% Sin dependencias externas:** Desarrollado con la librería estándar de Go, compilación ultrarrápida y binario autocontenido.
- **Configuración por Entorno (12-Factor App):** Configurable mediante `.env` o variables de entorno del sistema operativo/contenedor.
- **API JSON:** Endpoints disponibles en `/api/timesheets` y `/api/projects`.
- **Despliegue con Docker:** Incluye Dockerfile multi-stage ultraligero listo para Autopyme o Kubernetes.

---

## ⚙️ Configuración (`.env` o Variables de Entorno)

Copia el archivo `.env.example` a `.env` o define las variables en tu contenedor/entorno:

```bash
# Google OAuth 2.0 Credentials
GOOGLE_CLIENT_ID=tu_cliente_id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=tu_client_secret
GOOGLE_ALLOWED_DOMAIN=planesnet.com

# Odoo Conexión (Opcional - por defecto se usa planesnet.com / pasi)
ODOO_URL=https://www.planesnet.com
ODOO_DB=pasi
ODOO_USER=
ODOO_PASSWORD=
PORT=8080
```

---

## 🏃 Ejecución Local

```bash
# Compilar y ejecutar
go build -o planesgo .
./planesgo
```

---

## 🐳 Ejecución con Docker (Autopyme)

```bash
# Construir imagen Docker
docker build -t planesgo:latest .

# Ejecutar contenedor pasando variables de entorno o archivo .env
docker run -d -p 8080:8080 --env-file .env --name planesgo planesgo:latest
```

Acceso desde el navegador:
👉 **[http://localhost:8080](http://localhost:8080)** (o tu dominio configurado en producción).
