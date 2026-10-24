---
description: Cuando la diff toca bucles, arranques, caminos que se ejecutan a menudo o entrada/salida.
---

# Eficiencia

- Cálculo o entrada/salida repetidos (leer un fichero en cada vuelta de un bucle o de un sondeo).
- Operaciones independientes hechas en serie.
- Trabajo bloqueante añadido al arranque o a un camino caliente.
- Objetos de larga vida que capturan mucho contexto en un cierre (closure).

Di la alternativa más barata.
