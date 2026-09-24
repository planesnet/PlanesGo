# PlanesGo - Documentación Técnica y Funcional del Sistema

**Versión:** 1.2.36  
**Tecnología Central:** Go 1.21+ (Librería Estándar, sin frameworks externos) + Odoo ERP (JSON-RPC)  
**Entorno y Marca:** Planes Soluciones Informáticas  

---

## 1. Visión General del Sistema

**PlanesGo** (identificado internamente como `pasigo`) es una aplicación web y PWA (Progressive Web App) de alto rendimiento desarrollada en Go puro. Su objetivo primordial es permitir a los miembros del equipo de **PLANES Soluciones Informáticas** gestionar de manera ágil, visual y en tiempo real sus imputaciones de horas trabajadas (*timesheets*) en **Odoo ERP**, consultar el estado de sus proyectos, tareas asignadas y tickets de helpdesk, y cronometrar sus actividades con sincronización instantánea entre múltiples dispositivos.

### Principales Ventajas Arquitecturales
1. **Compilación Nativa y Cero Dependencias Externas:** Se ejecuta como un binario único autocontenido (con plantillas y assets embebidos o servidos estáticamente), sin depender de runtimes como Node.js ni frameworks web pesados.
2. **Conexión Eficiente con Odoo (JSON-RPC Keep-Alive):** Implementa un cliente nativo optimizado con pool de conexiones HTTP persistentes (`sharedTransport`), reutilización de sesiones autenticadas y cachés en memoria para datos maestros poco volátiles (proyectos, empleados, tickets, avatares).
3. **Sincronización en Tiempo Real (SSE):** Dispone de un hub de *Server-Sent Events* (`SSEHub`) que propaga cambios de estado (arranque, pausa, reanudación y guardado de cronómetros o partes de horas) entre todas las sesiones abiertas de un usuario (ordenador, ventana emergente pop-out y teléfono móvil).
4. **Diseño Visual Corporativo y Ergonómico:** Basado en la guía de diseño `DESIGN.md` de Planes (azul corporativo `#0072CE`, verde lima `#84CC16`, contraste oscuro `#0F172A`), con modos adaptativos para escritorio y móvil.

---

## 2. Arquitectura de Módulos y Componentes

El código fuente está estructurado de manera modular y limpia:

```
├── main.go                     # Punto de entrada, enrutamiento HTTP y ciclo de vida del servidor
├── models.go                   # Estructuras de sesión, estado de la app y métodos de cronómetro
├── events.go                   # Motor de Server-Sent Events (SSEHub) para tiempo real
├── middleware.go               # Middleware de logging estructurado y recuperación de pánicos
├── handlers_auth.go            # Controladores de Login local y Google OAuth 2.0
├── handlers_dashboard.go       # Controlador principal del dashboard, KPIs y vistas
├── handlers_api.go             # Endpoints de la API REST (timesheets, tareas, tickets, timers)
├── handlers_settings.go        # Gestión de configuración de usuario y test de conexión con Odoo
├── config/                     # Carga de variables de entorno (.env y variables de sistema)
├── store/                      # Almacenamiento JSON concurrente para preferencias de usuario
├── odoo/                       # Cliente JSON-RPC con pool, caché y modelos del ORM de Odoo
├── templates/                  # Plantillas Go html/template modularizadas por vistas y parciales
│   ├── index.html              # Plantilla base del dashboard
│   ├── express_standalone.html # Vista compacta Express (standalone/popout)
│   ├── login.html              # Pantalla de acceso (Google + Odoo)
│   ├── settings.html           # Pantalla de configuración del usuario
│   └── partials/               # Componentes reutilizables (list, calendar, gantt, express, kpis, sidebar, timer)
└── static/                     # Archivos estáticos: JS modular (app, timer, views, timesheets), PWA (manifest, sw) e imágenes
```

---

## 3. Autenticación y Gestión de Sesiones

PlanesGo admite dos mecanismos complementarios de autenticación:

### 3.1 Google OAuth 2.0 (Método Principal)
- **Flujo:** `/auth/google` redirige a la pantalla de consentimiento de Google solicitando los scopes de perfil y correo electrónico (`openid`, `email`, `profile`).
- **Restricción de Dominio Corporativo:** Valida que el correo pertenezca al dominio corporativo configurado en `GOOGLE_ALLOWED_DOMAIN` (por defecto `@planesnet.com`).
- **Callback (`/auth/google/callback`):** Intercambia el código por token, valida el estado anti-CSRF mediante cookie segura `planesgo_oauth_state`, obtiene la identidad del usuario y establece la cookie de sesión `planesgo_session`.
- Si el usuario tiene una API Key o credenciales de Odoo guardadas en `data/user_settings.json`, se enlazan automáticamente.

### 3.2 Acceso Directo con Credenciales de Odoo
- Compatible con usuario/contraseña o API Key de Odoo.
- Valida la autenticación directamente contra el servicio `/jsonrpc` (`common.authenticate`) de Odoo.

### 3.3 Almacén de Ajustes de Usuario (`store.UserSettingsStore`)
- Los datos de conexión específicos de cada usuario (URL de Odoo, base de datos, API Key/token personal y preferencias de visualización) se guardan en `data/user_settings.json`.
- Cada operación de lectura/escritura está protegida mediante bloqueos de lectura/escritura (`sync.RWMutex`).

---

## 4. Funcionalidades Principales

### 4.1 Imputación y Gestión de Partes de Horas (Timesheets)
- **Modelo de Odoo:** Mapea directamente con `account.analytic.line`.
- **Campos Sincronizados:** Fecha, descripción/concepto (`name`), horas (`unit_amount`), proyecto (`project_id`), tarea (`task_id`), empleado (`employee_id`), cliente (`partner_id`), referencia de facturación (`timesheet_invoice_id` / `billing_ref`).
- **Protección de Horas Facturadas:** Mediante `entry.IsInvoiced()`, el sistema detecta si una imputación ya ha sido procesada administrativamente o facturada en Odoo, bloqueando su modificación accidental o eliminación desde la interfaz.
- **Acciones CRUD:** Creación modal (`modal_timesheet.html`), edición inline o modal, y eliminación confirmada (`modal_delete.html`).

### 4.2 Métricas y KPIs en Tiempo Real
- **Horas Totales del Periodo:** Suma de horas imputadas en la semana o rango de fechas seleccionado.
- **Objetivo Semanal (Target):** Comparativa frente a la jornada semanal objetivo (ej. 40 horas).
- **Ratio de Facturación:** Porcentaje de horas asociadas a tareas/proyectos facturables.
- **Días con Registro:** Conteo visual de días completados en la semana en curso.

### 4.3 Sistema de Cronómetro en Vivo (Live Stopwatch)
- Permite iniciar el cómputo de tiempo con un solo clic sobre cualquier proyecto, tarea o ticket de soporte.
- **Estados del Cronómetro:**
  - `Activo`: Cómputo de segundos en curso, emitiendo heartbeats (`/api/timer/tick`).
  - `Pausado`: El tiempo acumulado se retiene sin incrementar los segundos activos.
  - `Detenido / Confirmado`: Abre el diálogo de confirmación para ajustar la descripción final y registrar las horas calculadas en Odoo (`/api/timer/confirm`).
- **Resistencia a Recargas:** El cronómetro guarda el timestamp de inicio y los segundos acumulados en memoria de servidor (`AppState.activeTimers`) y en el almacenamiento local del navegador, sobreviviendo a cierres de pestaña o recargas.
- **Barra de Cronómetro Flotante:** Componente omnipresente (`timer_bar.html`) que muestra el tiempo transcurrido, la tarea en curso y accesos rápidos de parada/pausa.

### 4.4 Sincronización Multi-Dispositivo con Server-Sent Events (SSE)
- Endpoint `/api/events` con `text/event-stream`.
- Cada usuario autenticado dispone de un canal de eventos donde recibe notificaciones instantáneas de:
  - `timer_started`, `timer_ticked`, `timer_paused`, `timer_resumed`, `timer_stopped`.
  - `timesheet_created`, `timesheet_updated`, `timesheet_deleted`.
- Si el usuario inicia o pausa el cronómetro en su teléfono móvil (`/m`), la pantalla del ordenador refleja el cambio inmediatamente sin recargar.

---

## 5. Vistas de la Interfaz de Usuario

El usuario puede alternar entre distintas perspectivas según sus necesidades operativas:

| Vista | Parcial / Ruta | Descripción |
| :--- | :--- | :--- |
| **Vista Lista** | `view_list.html` | Tabla detallada con desglose por fecha, cliente, proyecto, tarea, descripción, horas e indicador de facturación. Permite filtrado rápido en cliente y ordenación. |
| **Vista Calendario** | `view_calendar.html` | Vista semanal/diaria tipo rejilla que ubica los bloques de trabajo por horas y días, facilitando detectar huecos o solapamientos. |
| **Vista Gantt** | `view_gantt.html` | Representación temporal de la dedicación a lo largo de proyectos y semanas consecutivas. |
| **Vista Express (Móvil/Standalone)** | `view_express.html`<br>`/express`, `/m` | Interfaz ultrarrápida optimizada en **1 sola columna**, diseñada para teléfonos móviles o pequeñas ventanas emergentes en el escritorio. Muestra primero la **tarjeta activa en ejecución** (con degradado esmeralda luminoso) seguida de las **15 tareas más recientes**. |

### Elementos Auxiliares de Interfaz
- **Barra Lateral Plegable (`sidebar.html`):** Acceso directo a proyectos favoritos, tareas en curso y tickets asignados pendientes de resolución con avatares de los clientes.
- **Selector de Semanas (`week_selector.html`):** Navegación fluida entre semana anterior, semana actual y semanas futuras con recálculo dinámico del total de horas.
- **Barra de Filtros (`filters_bar.html`):** Búsqueda por texto libre, filtrado por empleado y selector de proyecto.

---

## 6. Integración con Odoo ERP (JSON-RPC)

El paquete `pasigo/odoo` implementa el cliente nativo que dialoga con los modelos del ORM de Odoo:

- **Modelos de Odoo Consultados / Modificados:**
  - `account.analytic.line`: Lectura y escritura de partes de horas.
  - `project.project`: Catálogo de proyectos activos, horas presupuestadas y totales.
  - `project.task`: Tareas de proyectos, estados y horas efectivas.
  - `helpdesk.ticket`: Incidencias y peticiones de soporte técnico.
  - `hr.employee`: Lista de empleados para mapeo de usuarios y selección.
  - `res.partner`: Nombres y avatares/logos de clientes vinculados.
- **Optimización de Consultas:**
  - Autenticación cacheada por clave de conexión (`poolKey`).
  - Carga en paralelo de proyectos, partes, empleados y tickets mediante `sync.WaitGroup` en `handleDashboard`.
  - Caché de avatares con cabeceras `ETag` y expiración (`/api/partner/avatar`).

---

## 7. Catálogo de Endpoints de la API REST

| Endpoint | Método | Descripción |
| :--- | :---: | :--- |
| `/api/timesheets` | `GET` | Lista las imputaciones con filtros opcionales (`date_from`, `date_to`, `project_id`). Inyecta el temporizador activo si existe. |
| `/api/timesheets` | `POST` | Crea una nueva imputación directa o consolida un temporizador en Odoo. |
| `/api/timesheets/update` | `POST` / `PUT` | Modifica los datos de una imputación existente. |
| `/api/timesheets/delete` | `POST` / `DELETE` | Elimina una imputación no facturada de Odoo. |
| `/api/projects` | `GET` | Devuelve la lista de proyectos activos de Odoo con caché en memoria. |
| `/api/tasks` | `GET` | Devuelve las tareas asociadas a un proyecto (`?project_id=...`). |
| `/api/tickets` | `GET` | Devuelve los tickets de soporte abiertos asignados. |
| `/api/timer/active` | `GET` | Consulta el estado del cronómetro actualmente activo para el usuario. |
| `/api/timer/start` | `POST` | Inicia un nuevo cronómetro sobre un proyecto, tarea o ticket. |
| `/api/timer/tick` | `POST` | Heartbeat periódico para mantener el tiempo acumulado en el servidor. |
| `/api/timer/pause` | `POST` | Pausa el cronómetro en curso. |
| `/api/timer/resume` | `POST` | Reanuda un cronómetro pausado. |
| `/api/timer/stop` | `POST` | Detiene el cronómetro sin registrar aún las horas. |
| `/api/timer/confirm` | `POST` | Guarda definitivamente las horas computadas en Odoo como un nuevo timesheet. |
| `/api/events` | `GET` | Conexión Server-Sent Events (SSE) para actualizaciones bidireccionales en tiempo real. |
| `/api/partner/avatar` | `GET` | Descarga y sirve la imagen del partner en caché HTTP. |
| `/api/settings/test-connection` | `POST` | Comprueba credenciales y conectividad con Odoo. |
| `/api/version` | `GET` | Devuelve la versión actual (`v1.2.36`). |
| `/health` & `/ping` | `GET` | Probes de disponibilidad para monitorización y balanceadores. |

---

## 8. Configuración y Despliegue

### 8.1 Variables de Entorno (`.env`)
```bash
# Servidor HTTP
PORT=8080

# Credenciales de Google OAuth 2.0
GOOGLE_CLIENT_ID=xxxxxxxxxxxx.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=xxxxxxxxxxxxxxxxxxxxxxxx
GOOGLE_ALLOWED_DOMAIN=planesnet.com

# Parámetros por defecto de Odoo ERP
ODOO_URL=https://planesnet.autopyme.com
ODOO_DB=ap113
ODOO_USER=
ODOO_PASSWORD=
```

### 8.2 Compilación Local
```bash
# Compilar binario nativo
go build -o planesgo .

# Ejecutar
./planesgo
```

### 8.3 Despliegue con Docker
El proyecto incluye un `Dockerfile` multi-stage:
1. **Fase de construcción:** `golang:1.22-alpine` para compilar el binario estático sin CGO.
2. **Fase de ejecución:** `alpine:3.19` ultraligera con certificados CA y usuario sin privilegios.

---

## 9. Resumen de Flujo de Trabajo Típico de Usuario

```
[Acceso: Google OAuth o Credenciales]
                 │
                 ▼
     [Dashboard / Express /m]
                 │
   ┌─────────────┴─────────────┐
   ▼                           ▼
[Modo Cronómetro]      [Imputación Manual]
   │                           │
 Iniciar timer sobre        Abrir modal
 tarea o ticket             Seleccionar proyecto,
   │                        fecha, horas y descripción
 Sincronización SSE            │
 en móvil y PC                 ▼
   │                    Guardar en Odoo
 Pausar / Detener / Confirmar
   │
   ▼
[Registro persistido en Odoo account.analytic.line]
   │
   ▼
[Actualización instantánea de KPIs y Vistas]
```
