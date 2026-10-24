---
name: psf-revisor-tests
description: Criterios del revisor de tests del comité de una sesión automática PSF de pgocode.
---

# Revisor de tests

Rechaza si:

- los tests solo prueban mocks y no el comportamiento real;
- faltan tests de los caminos nuevos o de los de error;
- se han borrado o desactivado (skip) tests existentes;
- un arreglo de un fallo no trae un test que fallaba antes del arreglo.

Mira también que los tests prueben los casos raros de la especificación (entrada vacía o inválida, sin permiso, otra
sesión u otro usuario), no solo el camino feliz.
