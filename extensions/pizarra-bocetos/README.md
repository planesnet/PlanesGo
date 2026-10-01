# Pizarra de bocetos (extensión de Chrome)

Lienzo para bocetar pantallas o dibujar encima de la página que tienes abierta, y compartir la
imagen (por ejemplo, pegándola en un chat).

## Instalar (modo desarrollador)

1. Abre `chrome://extensions` y activa **Modo de desarrollador** (arriba a la derecha).
2. Pulsa **Cargar descomprimida** y elige esta carpeta (`extensions/pizarra-bocetos`).
3. Fija el icono en la barra de extensiones.

## Uso

- Pulsa el icono: se abre la pizarra en una pestaña nueva. Si estabas en una página web, la
  pizarra arranca con el fondo **Captura** (una imagen de esa página) para marcar sobre ella.
- Fondos: **Vertical** y **Horizontal** (pantalla de móvil con rejilla) o **Captura**.
- Herramientas: seleccionar y mover (V), lápiz (P), rectángulo (R), línea (L), flecha (A),
  texto (T) y borrador (E). **Eliminar** (o Supr) borra el objeto seleccionado.
- **Ajustar a rejilla** (G): rectángulos, líneas, flechas, texto y movimientos se ajustan a la
  rejilla; el lápiz va siempre a mano alzada.
- Color (negro y cinco colores) y grosor. Deshacer/rehacer con Ctrl+Z / Ctrl+Y.
- **Copiar imagen** deja el PNG en el portapapeles (con la pantalla y la nota como pie) para
  pegarlo con Ctrl+V; **Descargar PNG** lo guarda en un fichero.
- El dibujo se guarda solo en el navegador (borrador local) y no se envía a ningún sitio.

## Permisos

- `activeTab`: capturar la pestaña visible solo cuando pulsas el icono.
- `storage`: pasar esa captura a la pizarra (almacenamiento de sesión, se borra al cerrar Chrome).
