---
description: Siempre. Cómo leer cada trozo de la diff y la función que lo contiene.
---

# Línea a línea

Lee cada trozo de la diff y, después, la función entera que lo contiene: los fallos en líneas que no cambian de una
función tocada también cuentan (el cambio los vuelve a exponer o no los arregla). En cada línea pregúntate qué
entrada, estado, momento o plataforma la hace fallar:

- condiciones invertidas o mal agrupadas;
- off-by-one en índices, rangos y límites;
- nulos o vacíos sin comprobar;
- `await` que falta, promesas sin atender;
- el cero o la cadena vacía tomados como «falso» cuando son un valor válido;
- la variable equivocada al copiar y pegar;
- errores tragados en un `catch` o ignorados;
- metacaracteres de expresiones regulares sin escapar;
- condiciones de carrera entre goroutines o peticiones concurrentes.
