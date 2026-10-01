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
- **Patrones** (botón de la barra): biblioteca de patrones de Material Design 3 con buscador y
  tres secciones. Cada patrón tiene su **ficha de uso y comportamiento** (qué es, variantes,
  cuándo usarlo y cuándo no, comportamiento, cómo se ve en compacto y en expandido, accesibilidad
  y enlace a su página de m3.material.io), resumida de la web oficial (`guia.js`, 1 oct 2026).
  - **Pantallas**: patrones completos (los layouts canónicos list-detail, supporting pane y feed,
    y pantallas de navegación, barras, contenido, entrada de datos y superposiciones). Sustituyen
    el patrón del dispositivo; incluye la tabla de clases de tamaño de ventana.
  - **Componentes**: los 36 componentes de Material 3 (acciones, comunicación, contención,
    navegación, selección y entrada de texto).
  - **Campos y datos**: campos por tipo de dato (texto, multilínea, entero, decimal, importe,
    porcentaje, fecha, hora, fecha y hora, rango de fechas, duración, correo, teléfono,
    contraseña, URL, desplegable, autocompletar, sí/no, deslizador, etiquetas, archivo, error) y
    visualización de datos (ficha de solo lectura, tabla y cifras destacadas), con formato es-ES y
    atributos HTML de entrada.
  Las pantallas y piezas se incrustan como **grupos de objetos dibujados a mano** que puedes mover,
  redimensionar, borrar o editar; las piezas se colocan en su sitio (barras, rieles, FAB) o en el
  centro. Si no has tocado el grupo de la pantalla, al cambiar de orientación se regenera. Están en
  `patterns.js` (pantallas), `pieces.js` (componentes y campos) y `sketch.js` (paso a objetos).
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
- **Copiar para Claude** (icono `{ }`) copia una explicación en Markdown con un bloque JSON
  (`pizarra-bocetos/v1`): el patrón con su nombre de Material Design 3 y sus componentes, los
  componentes y campos insertados con su guía de uso, comportamiento y formato, su
  comportamiento en **los dos modos** (compacto y expandido) con las pantallas y el flujo entre
  ellas, y tus anotaciones (textos, cajas, flechas, rectángulos y trazos) con color, pantalla,
  zona y posición en dp. El patrón es solo estructura: el estilo (colores, tipografía, formas) se
  deja al tema del proyecto. Pégalo en el chat junto con la imagen (botón Copiar).
- El dibujo se guarda solo en el navegador (borrador local) y no se envía a ningún sitio.

## Permisos

- `activeTab`: capturar la pestaña visible solo cuando abres la paleta y pulsas **Pizarra**.
- `storage`: pasar esa captura a la pizarra (almacenamiento de sesión, se borra al cerrar Chrome).
