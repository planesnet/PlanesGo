---
description: Cuando la diff borra o sustituye líneas, comprobaciones, tests o caminos de error.
---

# Lo que la diff quita

Para cada línea que la diff borra o sustituye, nombra el invariante o el comportamiento que garantizaba y busca en
el código nuevo dónde se vuelve a garantizar. Si no lo encuentras, es un hallazgo:

- una comprobación o validación quitada o más estrecha;
- un camino de error que desaparece;
- un test borrado que cubría un caso real;
- un permiso o una restricción que ya no se aplica.
