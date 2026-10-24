# Studio TV

Estudio de TV de Planes Go Code para ver en directo lo que hacen los navegadores de las pruebas automáticas: QA de Odoo en backoffice y en móvil, o cualquier otra prueba del pipeline. Funciona sin gastar tokens en la retransmisión y con seguridad (TLS y tokens distintos para emitir y para ver).

Es un solo ejecutable sin dependencias. Hay varios canales a la vez, uno por sesión o agente, y una parrilla con miniaturas de todos.

## Uso rápido (todo en tu equipo)

```bash
make studio-tv            # o descarga el binario de tu sistema de dist/
./dist/studio-tv
```

Al arrancar muestra dos líneas:

- **Ver:** ábrela en el navegador. Sin `canal` muestra la parrilla; pulsa un canal para verlo en grande con la escaleta de pasos.
- **Emitir:** define `PCO_URL` y `PCO_TOKEN` en la consola donde se lanza la prueba. Si emites varias pruebas a la vez, pon también `PCO_CANAL=<nombre>` (letras, números, `.`, `_` o `-`; por defecto `principal`).

El cliente para pruebas escritas en Go es el paquete [`emitter`](emitter/emitter.go) (`emitter.New(emitter.Config{}, 8)`, luego `Frame(jpeg)` y `State(…)`): emite en segundo plano a 8 imágenes por segundo como mucho y descarta imágenes si el estudio va lento, así que nunca frena la prueba. Sin las variables lee `~/.config/pgocode/studio_tv.env`, que pgocode deja apuntando a su estudio local; y a las campañas de QA que lanza pgocode les pasa `PCO_URL`, `PCO_TOKEN` y `PCO_CANAL` en el entorno, sea cual sea su lenguaje.

## Estudio en un servidor (para ver pruebas que corren en la nube)

Las sesiones de Claude en la nube no admiten conexiones entrantes y tienen la salida restringida. Para verlas, el estudio tiene que estar en un servidor con dominio y HTTPS, y ese dominio tiene que estar en el *Network access* del entorno.

```bash
export STUDIO_TV_EMIT_TOKEN=<largo> STUDIO_TV_VIEW_TOKEN=<otro largo>
studio-tv-linux-amd64 -addr :8443 -cert fullchain.pem -key privkey.pem
# o detrás de nginx: studio-tv-linux-amd64 -addr 127.0.0.1:8080
```

A la sesión de la nube pásale **solo** `PCO_URL` y el token de **emitir**. El de ver es solo para quien mira.

## Opciones

| Opción | Por defecto | Qué hace |
|---|---|---|
| `-addr` | `127.0.0.1:8080` | Dirección de escucha |
| `-emit-token` / `STUDIO_TV_EMIT_TOKEN` | se genera | Token para emitir |
| `-view-token` / `STUDIO_TV_VIEW_TOKEN` | se genera | Token para ver |
| `-cert`, `-key` | — | HTTPS con tu certificado (PEM) |
| `-selfsigned` (+ `-hosts`) | — | HTTPS con certificado propio generado al arrancar; en el emisor, `PCO_INSEGURO=1` |
| `-version` | — | Muestra la versión |

Mejor pasar los tokens por variable de entorno: los argumentos se ven en la lista de procesos.

## API

El canal va en `?canal=` o en la cabecera `X-Canal`; sin él se usa `principal`. Los tokens van en `Authorization: Bearer …` o en `?t=`.

| Ruta | Token | Qué hace |
|---|---|---|
| `POST /api/frame` | emitir | Fotograma JPEG (máx. 8 MB) |
| `POST /api/estado` | emitir | JSON `{caso, titulo, paso, estado: curso\|fin, seg, marcadores: [{t, txt}]}` |
| `GET /api/canales` | ver | Lista de canales: `canal, en_directo, ultima, espectadores, con_imagen, estado` |
| `GET /snapshot` | ver | Último fotograma del canal |
| `GET /stream` | ver | Vídeo MJPEG (`multipart/x-mixed-replace`) |
| `GET /events` | ver | Estados del canal por SSE |
| `GET /` | ver | Parrilla, o un canal con `?canal=` |
| `GET /healthz` | — | Comprueba que está vivo |

Comportamiento:
- Si un espectador va lento, se le descartan fotogramas: nunca se bloquea al emisor.
- Un canal se crea con el primer envío y caduca tras una hora sin señal ni espectadores.
- Como mucho hay 64 canales.

## Siguientes pasos

1. Grabación en el propio estudio (segmentos MP4) y marcadores clicables sobre la línea de tiempo.
2. Comentarios del espectador ligados al segundo del vídeo, como en el visor de QA, que vuelvan al agente.
3. Despliegue en un servidor de Planes con certificado (Let's Encrypt) y usuarios en lugar de tokens.
4. CI que compile y publique los binarios.
