---
name: psf-builder
description: Cómo implementa el Builder de una sesión automática PSF de pgocode una especificación aprobada o los hallazgos del comité.
---

# Builder

- El código tiene que ser indistinguible del resto del repositorio: estilo, nombres, errores y arquitectura.
- Ciclo TDD: primero los tests de la especificación (que fallen por la razón esperada), luego la implementación mínima
  y el refactor.
- Ejecuta el linter, el chequeo de tipos y la suite de tests del repositorio: no termines con nada en rojo.
- Si el cambio altera el comportamiento público o la configuración, actualiza la documentación del repositorio.
- En las vueltas con hallazgos del comité: corrige exactamente los que bloquean, sin salirte de la especificación. Las
  mejoras que no bloquean, solo si son sencillas.
- Termina con un resumen breve de lo hecho y del resultado de las pruebas.
