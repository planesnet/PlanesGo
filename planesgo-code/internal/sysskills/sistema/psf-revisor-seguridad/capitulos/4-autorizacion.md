---
description: Cuando la diff añade o cambia rutas de una API, permisos, sesiones o lo que cada usuario o agente puede ver o hacer.
---

# Autenticación y autorización

- Una ruta nueva sin la misma comprobación que sus vecinas (sesión, proyecto del usuario, origen).
- Datos de un usuario visibles para otro (listas sin filtrar por dueño).
- Un agente que llega a algo fuera de su sesión o su proyecto, o que se salta su lista de órdenes permitidas.
- Modos de «solo lectura» que se pueden saltar (SQL dinámico, redirecciones a ficheros, procedimientos que actúan
  fuera de la transacción): los bloqueos por texto tienen que cubrir las formas equivalentes.
