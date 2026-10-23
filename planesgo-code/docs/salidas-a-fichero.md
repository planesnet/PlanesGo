# Salidas enormes a fichero

Cada paso de un agente reenvía su historial entero. Una salida grande (la lectura de un módulo largo, una orden con
miles de líneas, una búsqueda) se paga en todos los pasos que siguen, aunque el agente solo necesitara una línea.
Lo que hace Claude Code con las suyas, pgocode lo extiende a todo: en el contexto del agente queda **el principio, el
final y la ruta de la salida completa**, y el agente lee con offset/limit (o grep) solo el trozo que le haga falta.

## Dónde se recorta

| Momento | OpenCode | Claude Code | Coste de caché |
|---|---|---|---|
| Al llegar (antes de que el modelo la vea) | `tool_output` en su configuración: 12 KB / 500 líneas (el suyo, 50 KB / 2000). Órdenes: la cola; lo demás: el principio. La completa, en su carpeta `tool-output`. Las lecturas tienen su propio límite (50 KB). | `bashOutputMaxChars` = 12 000 en `--settings` (el suyo, 30 000): por encima, la salida a un fichero y el modelo recibe los primeros 2 KB y la ruta. | Ninguno |
| En el historial (barrido, `prune_offload.go`) | Salidas de más de 12 000 caracteres pasados los 8 pasos protegidos (también a mitad de turno, en el barrido entre pasos); de los intercambios anteriores, las lecturas de más de 12 000 y las salidas largas (más de 60 líneas o 6000 caracteres). | Lo mismo, entre turnos (el historial JSONL; se guarda el original y se puede reponer). | El del barrido: va con lo demás que sale, con su economía (`decideSweep`, `pays`, `claudeCacheWorth`) |

En el historial, lo que queda lleva el aviso nativo de cada agente (con textos propios el modelo se desdecía de lo
deducido; ver `ClearedText`):

- lecturas: `(Lines 18-483 of /ruta/fichero.py omitted to save context. Use Read with offset=18 and limit=466 to view
  them again.)`: el resto está en el propio fichero;
- lo demás, OpenCode: `...N bytes truncated (L lines, M KB in total)... The tool call succeeded but the output was
  truncated. Full output saved to: <carpeta>/.pgocode/salidas/<llamada>.txt …`; Claude Code: `<persisted-output>
  Output too large (M KB, L lines). Full output saved to: … Preview … End …</persisted-output>`.

`.pgocode/salidas/` está en la carpeta de la sesión y fuera de git (`.git/info/exclude`, como los adjuntos); sus
ficheros se borran a los 30 días.

**Antigravity** no deja tocar su historial (lo guarda cifrado en `~/.gemini/antigravity-cli/conversations/<id>.pb`;
ver `prune_chat.go`) ni tiene un límite configurable de sus salidas desde la línea de órdenes: no se le aplica nada
de esto. Lo que sí le llega recortado es `pgo salida` y `pgo ejecutar` (abajo); para el resto, el relevo.

## `pgo ejecutar` y `pgo salida`

- Cada programa lanzado (por el agente o por el usuario) guarda su salida completa en
  `<carpeta>/.pgocode/salidas/programa-<nombre>.log` (hasta 20 MB); en memoria siguen solo las últimas 1000 líneas.
- `pgo ejecutar`: si el programa acaba en los primeros segundos, además de cómo ha acabado da sus últimas 20 líneas y
  la ruta del registro (antes hacía falta otra llamada, con todo el contexto, para ver la salida).
- `pgo salida <nombre>`: las últimas líneas que caben en 8000 caracteres (de las 100 últimas; cada línea, como mucho
  1000) y, si falta algo, delante, cuántas líneas faltan y la ruta del registro. Con `n` explícito (para filtrar con
  grep), las n que pide, del registro si pasan de las que hay en memoria.

## Umbrales: de dónde salen

Medido en las sesiones reales de Luis (solo lectura, sobre copias):

- OpenCode (10 036 partes): las lecturas son el 90 % de los bytes de salidas (12,2 MB; mediana 4 KB, p90 16 KB, p99
  57 KB); órdenes, p90 3,5 KB y p99 46 KB. Lecturas de más de 12 000 caracteres: 266, que suman 16 M tokens·paso más
  allá de los 8 pasos protegidos.
- Claude Code (16 679 órdenes de Bash): p99 12,6 KB; las de más de 12 000 siguen en el contexto 47 pasos de mediana
  **en el mismo turno** (361 hasta el final de la sesión o la compactación).

Con 12 000 caracteres se recorta el 1-2 % de las salidas, las que más se arrastran.

## Medidas

- `TestOffloadSim` (barrido al empezar cada turno, solo reglas, sobre las 23 sesiones de OpenCode con 10 mensajes o
  más; 228 turnos): la poda retira el 61,0 % del historial frente al 57,7 % sin las salidas a fichero (+650k tokens;
  +3,1 % de tokens·paso retirados).
- `TestOffloadReal`, escenario largo (cada salida grande seguida de 10 órdenes pequeñas):
  - OpenCode (gemini-3.1-flash-lite), turnos comparables (12 llamadas en los dos modos): entrada 573 882 → 195 173 y
    189 090 tokens (−66 %), coste 0,0254 → 0,0121 y 0,0096 $; contexto al acabar el turno 49k → 17k.
  - Claude Code (haiku): 1,51 M → 1,17 M tokens de entrada (−23 %) y 0,0383 → 0,0307 $ (−20 %) en los cinco
    turnos (con 32 llamadas frente a 39: haiku no siempre separa las órdenes); contexto al acabar el tercer turno
    55,8k → 42,1k.
  - En los dos, las preguntas que necesitan lo recortado se responden releyendo el trozo (grep y lectura con
    offset/limit, o `tail` del fichero de la salida).
- Escenario corto (cinco turnos de una o dos llamadas): sin diferencia apreciable (0,0156 frente a 0,0163 $ con
  Claude Code): si no hay pasos detrás, no hay nada que arrastrar; y si se le pide leer «entero», el agente lee el
  fichero de la salida entero.
- En la app (Claude Code, haiku): `pgo ejecutar` de un programa que acaba con 3000 líneas responde con el final y
  la ruta en la misma llamada; `pgo salida largo 3000 | grep …` encuentra una línea que ya no está en memoria (del
  registro): 3 llamadas en lugar de 4.

Para repetirlo (gasta céntimos):

```sh
PGO_PRUEBA_SALIDAS=claude-code PGO_PRUEBA_SALIDAS_ESCENARIO=largo PGO_PRUEBA_SALIDAS_DIR=~/.cache/pgo-salidas \
  go test -run TestOffloadReal -v -timeout 60m ./internal/llm
# OpenCode: además HOME=<carpeta de prueba> y GOOGLE_APPLICATION_CREDENTIALS (sus sesiones van a HOME/.local/share/pgocode)
```

`PGOCODE_OFFLOAD_WRITE=0` quita los límites al llegar y `PGOCODE_OFFLOAD_SWEEP=0` las salidas a fichero del barrido.
