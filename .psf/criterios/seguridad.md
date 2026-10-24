# Criterios de seguridad de PlanesGo (pgocode)

Lo leen los revisores de las sesiones PSF antes de revisar un cambio; manda sobre los criterios generales.

## Modelo de amenazas

pgocode es un servidor Go que escucha en 127.0.0.1, con apps cliente (Electron) emparejadas y agentes de IA que
trabajan en el mismo equipo y **con el mismo usuario del sistema**. Quién controla cada entrada:

- **La persona**: de confianza. Escribe sus contraseñas (Odoo, Windows, SQL Server) en la interfaz.
- **Los agentes** (Claude Code, OpenCode, Antigravity): no son de confianza; pueden traer instrucciones inyectadas
  por lo que leen. Solo pueden usar el servidor con la orden `pgo` y el token de su sesión y turno, y no pueden leer
  los datos de pgocode (`internal/llm/odooguard.go`: ficheros prohibidos y órdenes bloqueadas). Cualquier fichero o
  ruta nueva con secretos o datos de otras sesiones tiene que entrar en ese blindaje.
- **Las webs** (navegador del sistema o el integrado): no son de confianza; contra el servidor local, CSRF y
  DNS rebinding (cabeceras `X-PGO`, Host y Origin, cookies SameSite).
- **Los repositorios clonados** bajo las raíces de proyectos: no son de confianza; pueden traer enlaces simbólicos y
  ficheros con nombres que pgocode escribe o lee (`.pgo.json`, `.planesgo.json`, `.pgocode/`). Nunca se sigue un
  enlace al escribir.
- **Los servidores remotos** (SQL Server, WinRM, RDP, AnyDesk, GitHub): sus respuestas no son de confianza.
- **Las issues y sus comentarios en GitHub** (el repositorio es público): no son de confianza. Solo arrancan solas
  las issues de quien tiene papel en el repositorio, y a los agentes no llegan comentarios de desconocidos.

## Lo que no se puede romper

- Los secretos (contraseñas, tokens, el secreto de emparejar, el token de cliente) nunca van en argumentos de
  procesos, en el entorno de procesos de larga duración, en registros ni en respuestas JSON; en disco, con permisos
  600 y fuera del alcance de los agentes.
- Los agentes nunca escriben en Odoo: la imputación la hace pgocode.
- Las conexiones de solo lectura (SQL Server, WinRM) no dejan cambiar nada: ni SQL dinámico, ni procedimientos que
  actúan fuera de la base de datos, ni redirecciones a ficheros.
- Un agente solo usa las conexiones (SQL, WinRM, escritorio del agente) de su propia sesión y su proyecto.
- Cada usuario solo ve lo suyo (sesiones remotas, proyectos de Odoo a los que tiene acceso).
- Las órdenes del sistema, siempre con `exec.Command` y argumentos separados, validando lo que empiece por `-`.
- Lo que se publica fuera (issues de GitHub) pasa por `depuracion.Tapa` y sin la carpeta personal.
