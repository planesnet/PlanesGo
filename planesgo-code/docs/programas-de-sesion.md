# Programas de una sesión: acceso a sus servicios

Un programa lanzado desde una sesión de trabajo de pgocode (por ejemplo, pgosage) puede usar los servicios de esa
sesión:

- sus conexiones con SQL Server;
- su PowerShell remoto (WinRM);
- el escritorio del agente;
- la imputación de su tiempo.

Se lanza desde «Programas» de la sesión o con `pgo ejecutar` de su agente. Código: `internal/servidor/programtoken.go`.

## Entorno del programa

| Variable | Qué es |
|---|---|
| `PGO_API` | La dirección del servidor (`http://127.0.0.1:7801`). |
| `PGO_PROYECTO` | El id del proyecto de Odoo. |
| `PGO_SESION` | La sesión de trabajo. |
| `PGO_PROGRAMA` | El nombre del programa en «Programas». |
| `PGO_CODIGO_PROGRAMA` | Un código de **un solo uso**, válido 10 minutos. Solo existe si se lanzó desde una sesión. |

## Cómo se entra

Nada más arrancar, el programa cambia el código por su token:

```
POST $PGO_API/api/pgo/programa-token
X-PGO: 1
X-PGO-Programa: <PGO_CODIGO_PROGRAMA>
→ {"token": "…"}
```

Cada petición posterior lleva el token en `X-PGO-Programa` (y `X-PGO: 1`). El token se guarda solo en memoria y deja
de valer cuando el programa termina.

El entorno de un proceso lo puede leer cualquier proceso del mismo usuario, también un agente (con `ps e` o
`/proc`). Por eso en el entorno solo va el código: usado una vez, ya no sirve. Además, las peticiones con token de
programa que vienen de un proceso de agente se rechazan.

## Órdenes (`/api/pgo/<orden>`)

| Orden | Método | Cuerpo | Respuesta |
|---|---|---|---|
| `sql-conexiones` | GET | — | `[{"name","server","database","user","allow_changes"}]` |
| `sql` | POST | `{"conexion","sql","n","escribir"}` | Como `pgo sql`: `{"sets":[{"columns","types","rows","truncated"}],"affected","rolled_back","ms"}`; hasta `n` filas por resultado (5000 como mucho). |
| `sql-flujo` | POST | `{"conexion","sql","escribir"}` | Líneas JSON (`application/x-ndjson`), una por evento (ver abajo). |
| `ps`, `ps-download`, `ps-upload` | POST | Como `pgo ps` | — |
| `escritorio` | POST | `{"carpeta","desktop":{…}}` | Como `pgo escritorio`. |
| `actividad` | POST | `{"titulo"}` | La persona está usando el programa: su tiempo va a la sesión. Se envía mientras haya teclado o ratón, como mucho cada 20 s. Solo vale si el programa lo lanzó una persona desde la interfaz. |

Notas de las órdenes de SQL:

- `conexion` es el nombre de una de las conexiones de la sesión; vacío, la principal.
- Cada conexión tiene su modo: solo lectura (lo que cambie la orden se deshace) o actualizable (`escribir: true`).
  Ese modo lo decide la persona en Propiedades de la sesión → SQL Server.
- Cada orden queda en la auditoría con el programa como origen.

`sql-flujo` es para leer catálogos grandes. Da todas las filas, hasta 2 000 000 por orden y 10 minutos, a medida que
llegan. Cada línea es un evento:

```
{"columns":[…],"set":0,"types":[…]}
{"row":[…]}
…
{"end":{"affected":…,"ms":…,"rolled_back":…,"rows":…}}
```

Si la orden falla a mitad, la última línea es `{"error":"…"}`.

Todo lo demás (lanzar programas, Google, abrir páginas) no está al alcance de un programa.
