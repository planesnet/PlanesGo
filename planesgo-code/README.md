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

## Instalar la app de escritorio (Linux)

```bash
curl -fsSL https://planesgo.autopyme.com/install/pgocode/instalar.sh | sh
```

Se instala para el usuario en `~/.local/share/pgocode` (desde el repositorio, `make instalar`). En Ubuntu 24.04+,
que restringe los «user namespaces» que necesita el sandbox de Electron, el motor va en `/opt/pgocode/motor`
(propiedad de root) con su perfil de AppArmor en `/etc/apparmor.d/pgocode`: el instalador pide sudo solo para eso,
la primera vez y cuando cambia la versión de Electron. El perfil no puede apuntar a una carpeta del usuario: con
ella, cualquier programa copiado ahí se saltaría la restricción. Las instalaciones hechas hasta ahora tenían el
motor en `~/.local/share/pgocode/motor` y un perfil así: basta con volver a ejecutar el instalador, que mueve el
motor y sustituye el perfil.
