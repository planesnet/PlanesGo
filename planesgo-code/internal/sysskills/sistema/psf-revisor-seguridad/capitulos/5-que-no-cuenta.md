---
description: Siempre, antes de dar un hallazgo. Lo que no se plantea como vulnerabilidad.
---

# Lo que no cuenta

No se dan como hallazgos de seguridad:

- denegación de servicio, agotamiento de recursos o falta de límites de peticiones;
- falta de endurecimiento sin un fallo concreto;
- carreras o ataques de tiempo teóricos;
- librerías desactualizadas (se gestionan aparte);
- ficheros que solo son tests, y la documentación;
- ataques que necesitan controlar variables de entorno o parámetros de arranque;
- registrar datos que no son secretos ni personales;
- poner contenido del usuario en las instrucciones de una IA;
- redirecciones abiertas, tabnabbing y fugas sutiles, salvo con mucha confianza.
