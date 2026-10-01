# Pizarra de bocetos (extensión de Chrome)

Lienzo para bocetar pantallas o dibujar encima de la página que tienes abierta, y compartir la
imagen (por ejemplo, pegándola en un chat).

## Instalar (modo desarrollador)

1. Abre `chrome://extensions` y activa **Modo de desarrollador** (arriba a la derecha).
2. Pulsa **Cargar descomprimida** y elige esta carpeta (`extensions/pizarra-bocetos`).
3. Fija el icono en la barra de extensiones.

## Uso

- Pulsa el icono: se abre una paleta con **Pizarra** y un enlace a **PlanesGo**.
- **Pizarra** abre la pizarra en una pestaña nueva. Si estabas en una página web, arranca con el
  fondo **Captura** (una imagen de esa página) para marcar sobre ella.
- Fondos (solo icono): móvil en vertical, captura o móvil en horizontal, con rejilla.
- Herramientas: seleccionar y mover (V), lápiz (P), rectángulo (R), línea (L), flecha (A),
  texto (T) y borrador (E). **Eliminar** (o Supr) borra el objeto seleccionado.
- **Redimensionar**: con la herramienta de selección, arrastra los tiradores de las esquinas del
  objeto seleccionado. El texto se escala de forma proporcional.
- **Ajustar a rejilla** (G): rectángulos, líneas, flechas, texto, movimientos y redimensionados se
  ajustan a la rejilla; el lápiz va siempre a mano alzada.
- Color (negro y cinco colores) y grosor. Deshacer/rehacer con Ctrl+Z / Ctrl+Y.
- **Copiar** (icono) deja el PNG en el portapapeles para pegarlo con Ctrl+V; **Descargar** (icono)
  lo guarda en un fichero.
- El dibujo se guarda solo en el navegador (borrador local) y no se envía a ningún sitio.

## Permisos

- `activeTab`: capturar la pestaña visible solo cuando abres la paleta y pulsas **Pizarra**.
- `storage`: pasar esa captura a la pizarra (almacenamiento de sesión, se borra al cerrar Chrome).
