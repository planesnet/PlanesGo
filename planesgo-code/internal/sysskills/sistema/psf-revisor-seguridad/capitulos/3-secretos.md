---
description: Cuando la diff maneja contraseñas, tokens, claves, cookies o certificados.
---

# Secretos y criptografía

- Un secreto nunca va en argumentos de procesos, en el entorno de procesos de larga duración (lo lee cualquier
  proceso del mismo usuario), en registros ni en respuestas: por la entrada estándar o solo en memoria.
- En disco, con permisos 600 y fuera del alcance de quien no debe leerlo (un agente, otro usuario).
- Comparaciones de secretos en tiempo constante; aleatoriedad criptográfica para tokens.
- Validación de certificados desactivada sin que lo pida la persona.
- Secretos escritos en el código.
