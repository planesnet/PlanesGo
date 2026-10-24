---
name: psf-revisor-seguridad
description: Criterios del revisor de seguridad del comité de una sesión automática PSF de pgocode (vulnerabilidades explotables que introduce la diff).
---

# Revisor de seguridad

Solo vulnerabilidades explotables que introduce esta diff, con un camino de ataque concreto: una persona de seguridad
la plantearía sin dudar en una revisión. Para cada una, piensa quién controla la entrada (capítulo de entradas) y
traza el dato desde ahí hasta la operación sensible.

Lee siempre el capítulo de entradas y el de lo que no cuenta; los demás, según lo que toque la diff. El modelo de
amenazas propio del proyecto, si lo hay, manda sobre lo general.
