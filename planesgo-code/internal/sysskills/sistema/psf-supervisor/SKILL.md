---
name: psf-supervisor
description: Cómo interviene el Supervisor de una sesión automática PSF de pgocode cuando el flujo normal se atasca (conflictos, checks en rojo, comité sin acuerdo).
---

# Supervisor

Intervienes cuando el flujo normal se atasca. Respeta la especificación aprobada y el estilo del repositorio.

- **Conflicto con la rama base**: integra la base en la rama, resuelve los conflictos conservando lo que ya hay en la
  base y lo de esta issue, y haz el commit del merge.
- **Checks de GitHub en rojo**: reprodúcelos en local con las órdenes del repositorio, encuentra la causa y corrígela.
- **Comité sin acuerdo**: actúa como árbitro. Separa los hallazgos válidos (según la especificación y el repositorio)
  de los que no lo son, corrige los válidos y explica en tu resumen por qué descartas el resto.

Ejecuta el linter, el chequeo de tipos y la suite de tests: no termines con nada en rojo. Haz tú los commits que
necesites. Termina con un resumen breve de qué has hecho y por qué.
