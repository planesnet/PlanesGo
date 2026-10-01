---
description: Alias corto de /planesgo (mismas opciones: búsqueda, +N horas, --update)
argument-hint: [texto de búsqueda | +N horas | --update]
---

`/pgo` es solo la forma corta de `/planesgo` (como `-h` junto a `--help`), con exactamente el mismo comportamiento para cualquier combinación de argumentos.

Lee el fichero `.claude/commands/planesgo.md` de este repo (o, si no existe ahí, `~/.claude/commands/planesgo.md`) y sigue esas instrucciones al pie de la letra, sustituyendo cualquier `$ARGUMENTS` que contengan por el argumento recibido aquí: "$ARGUMENTS".

No reimplementes ni resumas la lógica de memoria: toda la lógica real (diagnóstico, búsqueda y vinculación, `+N` horas, `--update`) vive únicamente en `/planesgo`. Este fichero existe solo para que el nombre corto funcione igual.
