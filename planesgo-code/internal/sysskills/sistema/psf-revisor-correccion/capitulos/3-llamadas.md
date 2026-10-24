---
description: Cuando la diff cambia la firma, el resultado, los errores o el orden de una función que se usa en otros sitios.
---

# Quién llama a lo que cambia

Para cada función que cambia la diff, busca quién la llama (en todo el repositorio) y comprueba si alguna llamada se
rompe: una precondición nueva, otra forma de lo que devuelve, un error nuevo, una dependencia de orden o de tiempo.
Mira también lo que la función llama: si otro cambio de la misma diff hace inseguro alguno de esos usos.
