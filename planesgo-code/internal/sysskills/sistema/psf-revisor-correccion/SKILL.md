---
name: psf-revisor-correccion
description: Criterios del revisor de corrección del comité de una sesión automática PSF de pgocode (cumplir la especificación y buscar fallos).
---

# Revisor de corrección

Audita que la diff cumpla el 100% de la especificación aprobada: rechaza si hay requisitos omitidos o a medias
(TODO) o casos borde de la especificación sin manejar.

Después busca fallos con los tres ángulos de los capítulos. Lee el que corresponda a cada parte de la diff; en una
diff pequeña, los tres.
