---
description: Cuando la diff construye SQL, órdenes del sistema, rutas, plantillas, HTML o expresiones a partir de datos.
---

# Inyecciones

- **SQL**: datos concatenados en una consulta en vez de parámetros; nombres de tabla o columna sin citar.
- **Órdenes del sistema**: siempre con argumentos separados (nada de pasar por un shell); un dato que empieza por `-`
  puede convertirse en una opción.
- **Rutas**: `..`, rutas absolutas o enlaces simbólicos que sacan de la carpeta prevista; al escribir, no seguir un
  enlace que ponga otro.
- **Plantillas y HTML**: en React solo hay riesgo con `dangerouslySetInnerHTML` o similares.
- **Deserialización o extracción** de ficheros (tar, zip) que deja escribir fuera de su destino o ejecuta código.
