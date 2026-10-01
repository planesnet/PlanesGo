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
- El lienzo es un tapete con el dispositivo dibujado encima.
- **Patrones** (desplegable de la barra): layouts de Material Design 3 agrupados por categorías.
  Se incrustan en el tapete como un **grupo de objetos dibujados a mano** (rectángulos, líneas,
  círculos y textos) que puedes mover, redimensionar, borrar o editar. Se adaptan a la
  orientación: en horizontal (expandido) el patrón cabe en el dispositivo; en vertical (compacto)
  puede descomponerse en varias pantallas en el tapete, enlazadas con flechas. Si no has tocado
  el grupo, al cambiar de orientación se regenera; si lo has tocado, se queda como está.
  Están en `patterns.js` (y `sketch.js` los convierte en objetos):
  - Layouts canónicos: lista + detalle con búsqueda, panel de apoyo, feed.
  - Navegación: barra de navegación ↔ riel, cajón de navegación, pestañas.
  - Barras de app: barra superior grande, barra inferior con FAB.
  - Contenido: cuadrícula de tarjetas, carrusel, detalle con imagen, ajustes, tema (claro, oscuro,
    automático).
  - Entrada de datos: búsqueda, formulario, inicio de sesión.
  - Superposiciones: diálogo, hoja inferior ↔ lateral.
- Herramientas: seleccionar y mover (V), lápiz (P), rectángulo (R), línea (L), flecha (A),
  texto (T), caja de texto (B) y borrador (E). **Eliminar** (o Supr) borra el objeto seleccionado.
- **Caja de texto**: arrastra para crearla y escribe dentro; el texto se ajusta al ancho de la caja.
  Doble clic sobre una caja o un texto (o clic con la herramienta de texto) para cambiar su texto.
- **Selección y grupos**: clic sobre un objeto de un grupo selecciona el grupo entero; Mayús+clic
  añade o quita; arrastrar en vacío selecciona por área; Ctrl+A selecciona todo. Clic derecho abre
  el **menú contextual**: Agrupar (Ctrl+G), Desagrupar (Ctrl+Mayús+G), Traer al frente, Enviar al
  fondo y Eliminar.
- **Redimensionar**: con la herramienta de selección, arrastra los tiradores de las esquinas del
  objeto seleccionado. El texto se escala de forma proporcional.
- **Ajustar a rejilla** (G): rectángulos, líneas, flechas, texto, movimientos y redimensionados se
  ajustan a la rejilla; el lápiz va siempre a mano alzada.
- Color (negro y cinco colores) y grosor. Deshacer/rehacer con Ctrl+Z / Ctrl+Y.
- **Copiar** (icono) deja el PNG en el portapapeles (con el tapete y el patrón) para pegarlo con Ctrl+V; **Descargar** (icono)
  lo guarda en un fichero.
- El dibujo se guarda solo en el navegador (borrador local) y no se envía a ningún sitio.

## Permisos

- `activeTab`: capturar la pestaña visible solo cuando abres la paleta y pulsas **Pizarra**.
- `storage`: pasar esa captura a la pizarra (almacenamiento de sesión, se borra al cerrar Chrome).
