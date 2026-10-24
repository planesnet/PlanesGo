---
description: Siempre. Quién controla cada entrada y en quién se puede confiar.
---

# Quién controla cada entrada

- **La persona** que usa la aplicación: de confianza para lo suyo, no para lo de otros usuarios.
- **Un agente de IA**: no es de confianza, puede traer instrucciones inyectadas por lo que ha leído. Solo debe llegar
  a lo que su sesión y su proyecto le permiten.
- **Una web** (navegador del sistema o integrado): no es de confianza; contra servicios locales, CSRF y DNS
  rebinding.
- **Un repositorio clonado** o un fichero de un proyecto: no es de confianza; enlaces simbólicos, nombres de fichero
  preparados, contenido que acaba en una orden o en un fichero de la persona.
- **Un servidor remoto** (bases de datos, escritorios remotos, APIs): sus respuestas no son de confianza.
- **El texto de una issue o de un comentario** de GitHub: no es de confianza, sobre todo en un repositorio público.
- **Variables de entorno y parámetros de arranque**: de confianza (los pone quien arranca el programa).
