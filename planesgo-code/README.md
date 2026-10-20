# Planes Go Code

Pipeline y agente de desarrollo de Planes Software Factory, escrito en Go.

Este repositorio solo tiene el código del producto. Los datos de cada prueba no van aquí: ni instancias, ni claves, ni vídeos. Se guardan en el proyecto que se revisa, en `<proyecto>/.psf/qa/<campaña>/`, sin versionar.

## Componentes

| Componente | Qué es |
|---|---|
| [`studio-tv/`](studio-tv/README.md) | Estudio de TV para ver en directo las pruebas automáticas, con varios canales y parrilla |

## Compilar y probar

```bash
make            # vet + tests + dist/studio-tv
make dist       # binarios de Linux, macOS y Windows en dist/
```

Requiere Go 1.22 o posterior.
