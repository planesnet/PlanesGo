# PlanesGo

Repositorio de PlanesGo: el servidor de tiempo y herramientas de Planes Software Factory, y el arnés de desarrollo de los agentes.

| Directorio | Qué es |
|---|---|
| [`planesgo-server/`](planesgo-server/README.md) | Servidor PlanesGo (`planesgo.autopyme.com`): imputación horaria en Odoo, `/m`, instaladores, PSF, MCP `planesgo-mcp`, Studio TV en `/studio-tv/` |
| [`planesgo-code/`](planesgo-code/README.md) | PSFCode: arnés de desarrollo (backend Go + React + Electron), con Studio TV y el login y los proyectos de Odoo |

Cada directorio es un módulo Go independiente con su propio `go.mod`.

## Despliegue

El `Dockerfile` está en la raíz y compila solo `planesgo-server/`. El despliegue automático de `master` no cambia.

```bash
docker build -t planesgo .
```

## Desarrollo

```bash
cd planesgo-server && go test ./...    # servidor
cd planesgo-code && make               # arnés: vet, tests, web y binarios en dist/
cd planesgo-code && make escritorio    # arnés en Electron
```

## Módulos compartidos

`planesgo-code/studio-tv/` es un módulo Go propio, sin dependencias, que usan el arnés (PSFCode) y el servidor (`/studio-tv/`). Los dos lo enlazan con un `replace` en su `go.mod`, y el `Dockerfile` lo copia junto al servidor.
