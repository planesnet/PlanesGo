// Guía de uso y comportamiento extraída de Material Design 3 (m3.material.io, 1 oct 2026).
// La generan resúmenes de las páginas oficiales de componentes y de layout; no se edita a mano.
window.PizarraGuia = {
 "fuente": "m3.material.io (consultado el 1 de octubre de 2026)",
 "layout": {
  "fundamentos": {
   "clasesTamano": [
    {
     "nombre": "Compact",
     "rango": "< 600 dp",
     "ejemplos": "Teléfono en vertical",
     "paneles": "1",
     "navegacion": "Barra de navegación (navigation bar) o riel de navegación expandido modal (modal expanded navigation rail)"
    },
    {
     "nombre": "Medium",
     "rango": "600–839 dp",
     "ejemplos": "Tablet en vertical; plegable abierto en vertical",
     "paneles": "1 (recomendado) o 2",
     "navegacion": "Barra de navegación (navigation bar) o riel de navegación expandido modal (modal expanded navigation rail)"
    },
    {
     "nombre": "Expanded",
     "rango": "840–1199 dp",
     "ejemplos": "Teléfono en horizontal; tablet en horizontal; plegable abierto en horizontal; escritorio",
     "paneles": "1 o 2 (recomendado)",
     "navegacion": "Riel de navegación expandido modal o estándar (modal or standard expanded navigation rail)"
    },
    {
     "nombre": "Large",
     "rango": "1200–1599 dp",
     "ejemplos": "Escritorio",
     "paneles": "1 o 2 (recomendado)",
     "navegacion": "Riel de navegación expandido modal o estándar (modal or standard expanded navigation rail)"
    },
    {
     "nombre": "Extra-large",
     "rango": "≥ 1600 dp",
     "ejemplos": "Escritorio; monitores ultrapanorámicos",
     "paneles": "1 a 3 (recomendado)",
     "navegacion": "Riel de navegación expandido modal o estándar (modal or standard expanded navigation rail)"
    }
   ],
   "partes": [
    "El andamiaje (layout scaffold) estructura todo el layout en barras (bars), rieles (rails) y paneles (panes).",
    "Las barras enmarcan el contenido y contienen la barra superior (app bar) o la barra de navegación (navigation bar).",
    "Los rieles ocupan el perímetro de los paneles: riel de navegación, barras de herramientas (toolbars), FAB o entrada de chat.",
    "Todo el contenido va en un panel; un layout tiene de 1 a 3 paneles de anchos variables.",
    "El margen es el espacio entre el borde de la pantalla y cualquier elemento interior.",
    "El asa de arrastre (drag handle) redimensiona paneles o colapsa uno fijo para pasar de dos paneles a uno."
   ],
   "reglas": [
    "Diseña por puntos de ruptura (breakpoints), no por dispositivos: el espacio cambia con multiventana, rotación o plegado.",
    "Compact y medium: un panel; expanded y large: dos paneles; extra-large: considera tres paneles.",
    "En medium, usa dos paneles solo con contenido de baja densidad y acciones claras.",
    "Al crecer, pregúntate qué revelar, dividir, redimensionar, reposicionar o intercambiar.",
    "Cambia la barra de navegación (compact) por un riel de navegación (medium/expanded); el riel se expande en large y extra-large.",
    "Ajusta márgenes y estilos de texto para mantener 40–60 caracteres por línea en todos los tamaños."
   ]
  },
  "patrones": {
   "list-detail": {
    "nombreM3": "List-detail",
    "url": "https://m3.material.io/foundations/layout/canonical-examples/list-detail",
    "que": "Divide la ventana en dos paneles: una lista explorable de elementos y el detalle del elemento elegido.",
    "cuandoUsar": [
     "Para acceder rápido al detalle de un elemento dentro de una lista larga.",
     "Con pares padre-hijo: bandeja de correo + correo, mensajes + conversación.",
     "Explorador de archivos + carpeta abierta; ajustes + categoría.",
     "Artista musical + detalle del álbum o pistas."
    ],
    "cuandoNo": [
     "Si el contenido secundario solo complementa al principal (no es padre-hijo), usa panel de apoyo (supporting pane).",
     "No uses dos paneles en medium con contenido de alta densidad o que exige concentración."
    ],
    "comportamiento": [
     "Botón atrás: solo en la vista de detalle de layouts de un panel.",
     "Estado seleccionado: solo en la lista de layouts de dos paneles.",
     "Sin selección: un panel muestra la lista; dos paneles muestran contenido de relleno o estado vacío en el detalle.",
     "Al pasar de uno a dos paneles con selección, se ven ambos y el detalle del elemento.",
     "Al pasar de dos a uno, suele mostrarse el detalle con barra superior (app bar); mantén coherencia con la vista previa.",
     "Guarda estados (leído/no leído) y la posición de scroll del detalle al navegar."
    ],
    "compacto": "Un solo panel: se ve la lista o el detalle, nunca ambos. Teléfono vertical, plegable cerrado o tablet en pantalla dividida.",
    "medio": "Un panel para contenido denso o de concentración; dos paneles para hojear colecciones, con barra de navegación inferior o riel modal.",
    "expandido": "Dos paneles lado a lado (también en large y extra-large): lista y detalle visibles a la vez.",
    "notas": [
     "Usa agrupación explícita e implícita para dirigir el foco en dos paneles.",
     "En multiselección, el panel usado más recientemente debe seguir visible al pasar a un panel.",
     "Un asa de arrastre (drag handle) puede ajustar el ancho de los paneles."
    ]
   },
   "supporting-pane": {
    "nombreM3": "Supporting pane",
    "url": "https://m3.material.io/foundations/layout/canonical-examples/supporting-pane",
    "que": "Organiza el contenido en un área principal (unos dos tercios) y un panel secundario de apoyo contextual.",
    "cuandoUsar": [
     "Cuando el contenido secundario solo tiene sentido en relación con el principal.",
     "Productividad.",
     "Edición de documentos con comentarios.",
     "Exploración de contenido y multimedia."
    ],
    "cuandoNo": [
     "Si el contenido es padre-hijo, usa list-detail en su lugar."
    ],
    "comportamiento": [
     "La ventana se reparte entre un panel de foco y un panel de apoyo.",
     "Según el breakpoint, el panel de apoyo va debajo o al lado del panel de foco.",
     "Debajo: ancho flexible (compact y medium).",
     "Al lado (inicio o final): ancho fijo de 360 dp (expanded)."
    ],
    "compacto": "Panel de apoyo debajo del de foco; una hoja inferior (bottom sheet) da acceso al apoyo sin perder el foco.",
    "medio": "Panel de apoyo debajo del panel de foco, con ancho flexible.",
    "expandido": "Panel de apoyo al lado inicial o final del de foco, con ancho fijo de 360 dp.",
    "notas": [
     "El área principal ocupa la mayor parte de la ventana, típicamente unos dos tercios."
    ]
   },
   "feed": {
    "nombreM3": "Feed",
    "url": "https://m3.material.io/foundations/layout/canonical-examples/feed",
    "que": "Organiza tarjetas o listas en una cuadrícula configurable para hojear y descubrir mucho contenido rápidamente.",
    "cuandoUsar": [
     "Noticias, fotos y redes sociales.",
     "Para mostrar piezas de contenido diversas mediante tarjetas (cards) y listas.",
     "Para ver de forma rápida y cómoda una gran cantidad de contenido."
    ],
    "comportamiento": [
     "La cuadrícula pasa de una a varias columnas según el espacio.",
     "Admite contenido de proporciones y tamaños variados, como tarjetas pequeñas y grandes.",
     "Usa tamaño y posición para establecer relaciones entre elementos.",
     "Los elementos se recolocan (reflow) al rotar, desplegar o entrar en multiventana.",
     "El orden de los elementos lo determina su posición."
    ],
    "compacto": "Apilado vertical, como una lista de tarjetas que ocupan todo el ancho del panel.",
    "medio": "Admite componentes de distintos anchos repartidos en varias columnas.",
    "expandido": "Varias columnas con anchos distintos; el número de columnas suele aumentar (también en large y extra-large).",
    "notas": [
     "El ancho de columna puede crecer en breakpoints mayores.",
     "Los elementos pueden cambiar de tamaño para agrupar contenido."
    ]
   }
  }
 },
 "componentes": {
  "app-bars": {
   "nombreM3": "App bars",
   "url": "https://m3.material.io/components/app-bars/overview",
   "que": "Barra en la parte superior que describe la página actual y ofrece navegación y 1–2 acciones esenciales.",
   "variantes": [
    "Search app bar: en páginas de inicio cuando la búsqueda es clave; lleva campo de búsqueda en vez de título.",
    "Small: en diseños densos o cuando la página está desplazada.",
    "Medium flexible: para un título más grande; puede colapsar a small al desplazar.",
    "Large flexible: para dar énfasis al título de la página.",
    "Medium y large (baseline): ya no recomendadas; sustituir por las flexibles."
   ],
   "cuandoUsar": [
    "Para mostrar título, navegación (Atrás o Menú) y acciones contextuales de la página.",
    "Para controles globales como búsqueda o notificaciones.",
    "Con una acción principal que altere o salga de la página: Enviar, Guardar, Editar.",
    "Con un botón relleno o tonal para destacar una única acción importante."
   ],
   "cuandoNo": [
    "No más de una acción (dos si hace falta); con muchas acciones, usa una toolbar.",
    "Evita el menú de desbordamiento en la app bar cuando sea posible.",
    "No pongas varios botones rellenos o tonales.",
    "No uses esquinas curvas ni alturas menores que la predeterminada.",
    "No ajustes el texto en la small ni trunques el título.",
    "En search app bar, no más de dos iconos finales con avatar."
   ],
   "comportamiento": [
    "Empieza del color del fondo y se rellena con color contrastante al desplazar.",
    "Puede quedarse fija u ocultarse y reaparecer al desplazar.",
    "Medium y large flexible se comprimen a small al desplazar y vuelven arriba del todo.",
    "Opción: contenedor transparente al desplazar con botones de icono rellenos flotando.",
    "Tocar la search app bar abre el componente search view.",
    "Título de hasta dos líneas en las flexibles; alineado al inicio o centrado."
   ],
   "compacto": "Ocupa el 100% del ancho; las acciones finales pueden colapsar en un menú de desbordamiento. Search app bar: hasta dos iconos finales en móvil.",
   "expandido": "Las acciones colapsadas vuelven a mostrarse. La search app bar admite hasta cuatro iconos finales; el campo crece hasta 312dp y luego al 50% del espacio.",
   "accesibilidad": [
    "El texto de búsqueda y su contenedor deben tener contraste mínimo 3:1.",
    "En idiomas RTL la disposición se refleja automáticamente."
   ]
  },
  "badges": {
   "nombreM3": "Badges",
   "url": "https://m3.material.io/components/badges/overview",
   "que": "Indicadores de notificaciones, recuentos o estado sobre iconos y destinos de navegación.",
   "variantes": [
    "Small: círculo simple para indicar una notificación no leída o cambio de estado.",
    "Large: contiene texto con un número o estado cuantificable."
   ],
   "cuandoUsar": [
    "En barras y raíles de navegación, app bars y pestañas.",
    "Large para recuentos cuando no hay riesgo de colisión, como en un navigation rail.",
    "Small cuando el espacio es muy reducido, como en app bars."
   ],
   "cuandoNo": [
    "No cambies su posición arbitrariamente ni lo pongas encima del icono.",
    "Evita colores personalizados; si los usas, contraste mínimo 3:1.",
    "Evita el large si puede solaparse con un elemento posterior.",
    "No dejes que se corte o choque con otro elemento."
   ],
   "comportamiento": [
    "Anclado dentro del área del icono, en su esquina superior final.",
    "El large crece en ancho con el número, manteniendo la posición.",
    "Máximo cuatro caracteres, incluido un «+»; trunca si hace falta.",
    "En navigation bar, oculta el badge cuando se selecciona el destino."
   ],
   "accesibilidad": [
    "Si se usan colores personalizados, contraste mínimo de 3:1.",
    "Cambia la posición del badge en idiomas de derecha a izquierda."
   ]
  },
  "button-groups": {
   "nombreM3": "Button groups",
   "url": "https://m3.material.io/components/button-groups/overview",
   "que": "Contenedores que organizan botones y botones de icono y añaden interacción entre ellos.",
   "variantes": [
    "Standard: los botones adyacentes reaccionan entre sí al pulsar; el grupo se ajusta a su contenido.",
    "Connected: para seleccionar opciones, cambiar vistas u ordenar; sustituye al segmented button."
   ],
   "cuandoUsar": [
    "Para agrupar visualmente botones relacionados y enfatizar el importante.",
    "Connected: con contenido relacionado y botones seleccionables (selección simple o múltiple).",
    "Varios tamaños en un grupo solo para momentos destacados."
   ],
   "cuandoNo": [
    "No uses connected si ningún botón puede alternarse.",
    "No mezcles estilos de color en un grupo connected.",
    "Evita mezclar tamaños con frecuencia; mismo tamaño y forma por defecto.",
    "No dejes que el grupo pase a una segunda línea."
   ],
   "comportamiento": [
    "Al pulsar, el botón cambia de ancho y forma.",
    "En standard, los adyacentes se desplazan y cambian de ancho temporalmente.",
    "En connected, solo cambia la forma del botón pulsado.",
    "Al seleccionar, la forma pasa de redonda a cuadrada o al revés; los toggle también cambian de color.",
    "Admite selección simple, múltiple y selección obligatoria."
   ],
   "compacto": "Usa botones más pequeños y estrechos para que quepan; los finales pueden colapsar en un menú de desbordamiento al final del grupo.",
   "expandido": "Usa botones más grandes y anchos; el connected ocupa todo el ancho, con ancho máximo en ventanas grandes. Mantén la jerarquía del botón principal."
  },
  "buttons": {
   "nombreM3": "Buttons",
   "url": "https://m3.material.io/components/buttons/overview",
   "que": "Botones con texto que lanzan la mayoría de acciones de la interfaz.",
   "variantes": [
    "Elevated: igual que tonal con sombra; solo si necesita separarse de un fondo llamativo.",
    "Filled: máximo impacto tras el FAB; para acciones finales como Guardar o Confirmar, idealmente una por página.",
    "Tonal: prioridad intermedia, p. ej. Siguiente en un onboarding.",
    "Outlined: énfasis medio para acciones secundarias, junto a uno filled.",
    "Text: menor prioridad; habitual en tarjetas, diálogos y snackbars.",
    "Toggle: para selecciones binarias como Guardar o Favorito."
   ],
   "cuandoUsar": [
    "Para acciones concretas en diálogos, modales, formularios, tarjetas y toolbars.",
    "Texto breve de 1–3 palabras, en minúscula inicial (sentence case).",
    "Icono opcional delante del texto.",
    "Cinco tamaños (XS a XL) y dos formas: redonda o cuadrada."
   ],
   "cuandoNo": [
    "No satures con demasiados botones; lleva lo secundario a menús o botones de icono.",
    "No trunques ni partas el texto en dos líneas.",
    "No uses dos iconos en el mismo botón.",
    "Outlined y text no sobre fondos llamativos como imágenes o vídeo.",
    "No subrayes el text button; usa un enlace en el texto.",
    "No fijes un ancho menor que el texto ni estires botones largos y vacíos."
   ],
   "comportamiento": [
    "La forma cambia al pulsar y al seleccionar.",
    "Los toggle pasan de redondo a cuadrado al seleccionarse e icono contorneado a relleno.",
    "En toggle, mantén longitud de etiqueta similar en ambos estados.",
    "El text button solo muestra contenedor al pasar, enfocar o pulsar.",
    "El ancho se ajusta al texto o puede estirarse según el diseño."
   ],
   "compacto": "Por ejemplo, botones rellenos alineados al final bajo la información.",
   "expandido": "Pueden alinearse al inicio junto a la información; limita el ancho o colócalos junto a otros elementos para evitar botones muy largos.",
   "accesibilidad": [
    "Mantén el mismo orden de botones en pantallas grandes y pequeñas para lectores de pantalla y teclado.",
    "En idiomas RTL, el icono va a la derecha del texto."
   ]
  },
  "extended-fab": {
   "nombreM3": "Extended FABs",
   "url": "https://m3.material.io/components/extended-fab/overview",
   "que": "Botón flotante con texto (e icono opcional) para la acción principal o más común de una pantalla.",
   "variantes": [
    "Small (56dp): sustituye al extended FAB baseline.",
    "Medium (80dp): en breakpoints grandes.",
    "Large (96dp): en compacto con una sola acción destacada o en breakpoints grandes."
   ],
   "cuandoUsar": [
    "En vistas largas con scroll que requieren acceso constante a una acción, como un checkout.",
    "Cuando el texto ayuda a entender la acción o se quiere más énfasis.",
    "Cuando un icono solo sería ambiguo."
   ],
   "cuandoNo": [
    "Solo uno por pantalla.",
    "No como opción dentro de un conjunto de acciones; usa botones rellenos.",
    "No sobre toolbars, tarjetas ni dentro de otros contenedores.",
    "No en la mitad superior de una pantalla móvil.",
    "No junto a otros flotantes como la toolbar flotante.",
    "No con icono sin texto; texto de 1–2 palabras sin truncar ni partir."
   ],
   "comportamiento": [
    "Se expande al aparecer en pantalla.",
    "Puede expandirse a cualquier forma o pantalla completa (container transform).",
    "Se reduce a FAB al bajar el scroll y vuelve a extended al subir.",
    "Al pasar de FAB a extended: cambia la forma, el icono va a la izquierda y aparece el texto.",
    "El contenedor se ajusta a la longitud del texto."
   ],
   "compacto": "Abajo, centrado o alineado al borde final de la ventana. Con poco espacio puede transformarse en FAB.",
   "expandido": "Abajo a la derecha (también en RTL) o dentro del navigation rail; al expandir el rail, el FAB se convierte en extended FAB.",
   "accesibilidad": [
    "En idiomas RTL los elementos se reflejan: icono a la derecha del texto."
   ]
  },
  "fab-menu": {
   "nombreM3": "FAB menu",
   "url": "https://m3.material.io/components/fab-menu/overview",
   "que": "Menú que se abre desde un FAB para mostrar de 2 a 6 acciones relacionadas.",
   "variantes": [
    "Primary: con FAB primary o primary container.",
    "Secondary: con FAB secondary o secondary container.",
    "Tertiary: con FAB tertiary o tertiary container."
   ],
   "cuandoUsar": [
    "Cuando hay varias acciones muy relacionadas bajo una sola, como Compartir.",
    "Para sustituir el speed dial o FABs pequeños apilados.",
    "Con cualquier tamaño de FAB."
   ],
   "cuandoNo": [
    "No lo abras desde un extended FAB ni desde otro componente.",
    "No con un solo elemento ni con acciones no relacionadas.",
    "No si el FAB va junto a toolbar flotante o navigation rail.",
    "No quites el texto de los elementos; el icono solo si es necesario.",
    "No cambies formas ni fijes anchos o trunques texto."
   ],
   "comportamiento": [
    "El FAB se transforma en el botón de cerrar del menú.",
    "Los elementos entran animados desde la esquina superior final del FAB.",
    "Aparece siempre en el mismo sitio que el FAB, alineado al borde final.",
    "Con poca altura (móvil horizontal) los elementos se desplazan por detrás del botón cerrar.",
    "Cada elemento puede expandirse a otra superficie (container transform)."
   ],
   "compacto": "Anclado a la misma esquina, con márgenes de 16dp.",
   "expandido": "Usa FABs más grandes y márgenes de 24dp en ventanas grandes; en web usa un componente menu.",
   "accesibilidad": [
    "En web, no tapes por completo el indicador de foco de un elemento accionable.",
    "En RTL se alinea a la izquierda y se refleja la disposición."
   ]
  },
  "floating-action-button": {
   "nombreM3": "Floating action buttons (FABs)",
   "url": "https://m3.material.io/components/floating-action-button/overview",
   "que": "Botón flotante con icono para la acción más importante de la pantalla, delante de todo el contenido.",
   "variantes": [
    "FAB: el más pequeño; en ventanas compactas con otras acciones en pantalla.",
    "Medium FAB: el más recomendado; ventanas compactas y medianas.",
    "Large FAB: acción principal muy destacada; ideal en ventanas expandidas.",
    "Small FAB: ya no recomendado."
   ],
   "cuandoUsar": [
    "Para acciones constructivas y positivas: Crear, Favorito, Compartir, Iniciar proceso.",
    "Con iconos claros y simples como añadir, mensaje o editar.",
    "Puede ir sobre la navigation bar, integrado en ella o en el navigation rail."
   ],
   "cuandoNo": [
    "No varios FABs en una pantalla.",
    "No para acciones menores, poco claras o destructivas (archivar, borrar, alertas).",
    "No para controles propios de toolbar (volumen, color de fuente).",
    "No en tarjetas u otros componentes individuales.",
    "No lo cubras con badges ni repitas acciones ya visibles.",
    "No hace falta en todas las pantallas."
   ],
   "comportamiento": [
    "Aparece expandiéndose desde un punto central; el icono puede animarse.",
    "Permanece fijo al desplazar el contenido.",
    "Puede transformarse en otra superficie, en FAB menu o en extended FAB.",
    "Al cambiar de pestaña o pantalla desaparece y reaparece si es relevante.",
    "En web muestra un tooltip con el texto de la acción al pasar el ratón."
   ],
   "compacto": "Normalmente en la esquina inferior derecha; usar medium FAB en móvil.",
   "expandido": "Considera la esquina superior izquierda, como en el navigation rail; usar large FAB en tablets y pantallas grandes.",
   "accesibilidad": [
    "El contenedor debe contrastar suficientemente con la superficie."
   ]
  },
  "icon-buttons": {
   "nombreM3": "Icon buttons",
   "url": "https://m3.material.io/components/icon-buttons/overview",
   "que": "Botones con un icono de sistema de significado claro para acciones comunes con un toque.",
   "variantes": [
    "Default: acción o abrir elementos como menú o búsqueda; icono relleno.",
    "Toggle: acciones binarias como favorito o marcador.",
    "Filled: máximo énfasis, para acciones clave; con moderación.",
    "Tonal: punto intermedio para acciones secundarias junto a una principal.",
    "Outlined: énfasis medio, cuando no es el foco principal.",
    "Standard: bajo énfasis o sobre superficies coloridas."
   ],
   "cuandoUsar": [
    "Directamente sobre el fondo o en tarjetas, app bars y toolbars.",
    "Agrupados en una toolbar o button group en diseños densos.",
    "Cinco tamaños (32–136dp) y tres anchos para marcar jerarquía.",
    "Mismo tamaño cuando todos tienen la misma importancia."
   ],
   "cuandoNo": [
    "No uses toggle en acciones sin estado seleccionado, como un menú de desbordamiento.",
    "No uses demasiados a la vez.",
    "No abuses del estilo filled."
   ],
   "comportamiento": [
    "Al pasar el ratón muestra un tooltip con la acción, no el nombre del icono.",
    "Toggle: icono contorneado sin seleccionar y relleno al seleccionar.",
    "La forma cambia al pulsar y al seleccionar.",
    "En un button group, los adyacentes reaccionan entre sí al pulsar."
   ],
   "accesibilidad": [
    "La selección debe comunicarse con al menos dos propiedades, no solo color.",
    "Si no hay icono relleno, usa peso semibold (o bold) al seleccionar."
   ]
  },
  "segmented-buttons": {
   "nombreM3": "Segmented buttons",
   "url": "https://m3.material.io/components/segmented-buttons/overview",
   "que": "Botones segmentados para elegir opciones, cambiar vistas u ordenar; ya no recomendados, usar connected button group.",
   "variantes": [
    "Single-select: elegir una opción, cambiar de vista u ordenar (hasta cinco opciones).",
    "Multi-select: elegir de ninguna a todas, p. ej. filtrar por rango de precio."
   ],
   "cuandoUsar": [
    "Para elecciones simples entre 2 y 5 opciones.",
    "Con icono, texto o ambos; etiquetas cortas y de longitud similar.",
    "Sobre bottom sheets o diálogos a pantalla completa."
   ],
   "cuandoNo": [
    "No más de cinco segmentos; usa chips para más opciones.",
    "No mezcles segmentos solo-icono con segmentos de texto.",
    "No dejes que los segmentos pasen a otra línea.",
    "No ocupes todo el ancho en pantallas grandes."
   ],
   "comportamiento": [
    "Con icono y texto, el icono se sustituye por una marca de verificación al seleccionar.",
    "En multi-select la selección no es obligatoria."
   ],
   "compacto": "Deja márgenes adecuados; el contenedor no debe tocar el borde de la pantalla.",
   "expandido": "Fija un padding máximo en los segmentos para que no llenen la pantalla."
  },
  "split-button": {
   "nombreM3": "Split buttons",
   "url": "https://m3.material.io/components/split-button/overview",
   "que": "Botón con acción principal y un botón de menú que abre acciones relacionadas.",
   "variantes": [
    "Tamaños XS, S (por defecto), M, L y XL, como buttons e icon buttons.",
    "Estilos de color: elevated, filled, tonal y outlined."
   ],
   "cuandoUsar": [
    "Para añadir un menú de acciones junto a una acción principal y ocultar opciones extra.",
    "Solo o junto a botones, botones de icono y button groups.",
    "Más grande en breakpoints amplios o para momentos destacados en pantallas pequeñas."
   ],
   "cuandoNo": [
    "Evita etiquetas largas; el botón principal lleva 1–2 palabras e icono.",
    "No cambies el icono de expandir/contraer del botón final.",
    "Evita modificar el menú de formas inusuales."
   ],
   "comportamiento": [
    "El botón de menú gira 180° hacia dentro al abrir y cerrar, con cambio de forma.",
    "Normalmente abre un menú; puede abrir otros componentes como tarjetas.",
    "El menú se alinea con el botón final, a 4dp; si no cabe, con un borde del botón.",
    "En idiomas RTL la disposición se refleja."
   ]
  },
  "cards": {
   "nombreM3": "Cards",
   "url": "https://m3.material.io/components/cards/overview",
   "que": "Contenedores con contenido y acciones sobre un único tema, a menudo entrada a más detalle.",
   "variantes": [
    "Elevated: con sombra; más separación que filled.",
    "Filled: separación sutil del fondo; el menor énfasis.",
    "Outlined: con borde visible; puede dar más énfasis.",
    "La elección depende solo del estilo: misma legibilidad y función."
   ],
   "cuandoUsar": [
    "Para agrupar elementos relacionados: imagen, título, subtítulo, texto y acciones.",
    "Como punto de entrada a más detalle (álbum, viaje).",
    "En colecciones: cuadrícula, lista vertical o carrusel.",
    "Pueden incluir botones, iconos, chips, sliders, enlaces y menú de desbordamiento."
   ],
   "cuandoNo": [
    "No fuerces contenido en tarjetas si basta con espaciado, títulos o divisores.",
    "Evita texto o iconos sobre imágenes; si lo haces, usa scrim para el contraste.",
    "No asignes más de un gesto de deslizar por tarjeta.",
    "No incluyas contenido deslizable dentro (carrusel de imágenes, paginación).",
    "No hagas scroll interno en móvil."
   ],
   "comportamiento": [
    "Pueden ser un único área táctil que se expande a pantalla completa (container transform).",
    "Deslizar descarta o cambia estado (marcar, archivar).",
    "Mantener y arrastrar reordena; la tarjeta eleva su elevación y queda delante.",
    "Contenido demasiado alto se trunca y se muestra al expandir la tarjeta.",
    "En escritorio el contenido puede expandirse y desplazarse dentro de la tarjeta.",
    "Filtros y ordenación se colocan fuera de la colección."
   ],
   "compacto": "Ocupan todo el ancho; tarjetas horizontales. Considera sustituirlas por listas, conservando controles y acciones.",
   "expandido": "Usa varias columnas, filas horizontales o carruseles; las tarjetas pueden volverse verticales y más grandes.",
   "accesibilidad": [
    "El texto sobre imágenes debe cumplir el contraste accesible; usa scrim o forma de fondo."
   ]
  },
  "carousel": {
   "nombreM3": "Carousel",
   "url": "https://m3.material.io/components/carousel/overview",
   "que": "Colección desplazable de elementos visuales (imágenes, vídeo) con texto breve opcional.",
   "variantes": [
    "Multi-browse: explorar muchos elementos visuales a la vez, como fotos.",
    "Uncontained: tipo tradicional, elementos de igual tamaño; admite más texto.",
    "Uncontained multi-aspect ratio: igual pero con elementos de distintos anchos.",
    "Hero: destacar un elemento grande con vista previa del siguiente.",
    "Center-aligned hero: elemento grande centrado con dos pequeños a los lados.",
    "Full-screen: experiencias inmersivas y feeds verticales de vídeo o imagen."
   ],
   "cuandoUsar": [
    "Para explorar muchos tipos de contenido visual.",
    "Snap-scrolling en multi-browse, hero y full-screen.",
    "Scroll libre solo en uncontained.",
    "Full-screen con contenido vertical y solo en orientación vertical."
   ],
   "cuandoNo": [
    "Multi-browse no si los elementos necesitan mucho texto o imágenes complejas.",
    "Si hace falta mucho texto, usa uncontained o tarjetas.",
    "No hagas elementos tan pequeños que no se reconozcan.",
    "Full-screen no en horizontal ni con scroll libre."
   ],
   "comportamiento": [
    "Las imágenes tienen efecto parallax al desplazar.",
    "Los elementos cambian de tamaño (grande, medio, pequeño 40–56dp) al moverse.",
    "Con snap, los elementos encajan en el layout al soltar.",
    "Hero avanza de uno en uno; multi-browse varios a la vez.",
    "El texto se abrevia u oculta en los elementos más pequeños."
   ],
   "compacto": "Hasta tres elementos a la vez si llevan texto; hero muestra uno grande y uno pequeño. Máximo dos líneas de texto.",
   "expandido": "Se muestran más elementos y más grandes; full-screen siempre muestra solo uno.",
   "accesibilidad": [
    "En páginas con scroll vertical, ofrece ver todos los elementos sin scroll horizontal.",
    "Recomendado: botón «Show all» debajo, o flecha en la cabecera, que abre una página vertical."
   ]
  },
  "checkbox": {
   "nombreM3": "Checkbox",
   "url": "https://m3.material.io/components/checkbox/overview",
   "que": "Casilla que permite seleccionar uno o varios elementos de una lista, o activar y desactivar un elemento.",
   "variantes": [
    "Estados: no seleccionada, seleccionada e indeterminada (cuando solo algunos hijos están marcados).",
    "Estados de error para no seleccionada, seleccionada e indeterminada."
   ],
   "cuandoUsar": [
    "Seleccionar una o varias opciones relacionadas de una lista.",
    "Presentar una lista con subselecciones (casilla padre e hijas).",
    "Activar o desactivar un elemento en entorno de escritorio.",
    "Agrupar visualmente opciones similares ocupando menos espacio que los interruptores."
   ],
   "cuandoNo": [
    "Para una sola opción de una lista: usa botones de opción (radio).",
    "Para opciones independientes o más descriptivas, como ajustes: usa interruptores (switch).",
    "No uses interruptores para listas de varias opciones: usa casillas."
   ],
   "comportamiento": [
    "Se pueden marcar varias casillas de una lista a la vez.",
    "Marcar la casilla padre marca todas las hijas; desmarcarla las desmarca todas.",
    "Si solo algunas hijas están marcadas, la padre queda indeterminada; marcarla marca todas.",
    "Al seleccionarse comunica su estado de forma clara e instantánea.",
    "Si activa o desactiva algo, la acción se ejecuta de inmediato.",
    "Las etiquetas deben ser fáciles de escanear; lo seleccionado destaca más que lo no seleccionado."
   ],
   "expandido": "En ventanas expandidas conviene agrupar las casillas en una región contenida, como una hoja lateral, junto a sus acciones."
  },
  "chips": {
   "nombreM3": "Chips",
   "url": "https://m3.material.io/components/chips/overview",
   "que": "Elementos compactos para introducir información, hacer selecciones, filtrar contenido o lanzar acciones contextuales.",
   "variantes": [
    "Assist (asistencia): acción inteligente o automatizada contextual, como «Añadir al calendario»; empieza por verbo.",
    "Filter (filtro): filtra una colección con etiquetas o sustantivos; alternativa a botones segmentados o casillas.",
    "Input (entrada): datos discretos introducidos por la persona, como contactos en el campo «Para».",
    "Suggestion (sugerencia): sugerencias generadas por el producto, como respuestas rápidas en un chat."
   ],
   "cuandoUsar": [
    "Para ofrecer opciones contextuales y complementarias a la tarea actual.",
    "Para filtrar resultados de una lista o búsqueda, bajo un campo de búsqueda o en hoja lateral.",
    "Para convertir texto introducido en elementos verificables y editables (input).",
    "Siempre en grupo: varios chips juntos formando un conjunto."
   ],
   "cuandoNo": [
    "No sustituyas acciones principales: avanzar o retroceder de paso son botones.",
    "No uses chips para el paso final de una tarea.",
    "No muestres un chip solo, ni un filtro con una única opción.",
    "No eleves chips colocados directamente sobre la página; solo sobre imágenes o fondos complejos."
   ],
   "comportamiento": [
    "Filtro: al tocarlo se activa y añade una marca de verificación delante de la etiqueta.",
    "Filtro: selección múltiple o única; no mezcles ambos modos en la misma página.",
    "Filtro: el icono final puede quitar el chip o abrir un menú de opciones.",
    "Input: icono final obligatorio para eliminar; editable, reordenable y movible entre campos; retroceso lo selecciona y borra.",
    "Assist: puede mostrar progreso y confirmación, cambiando el texto (Guardar → Guardado).",
    "Los conjuntos pueden pasar a otra fila o desplazarse en horizontal; etiquetas de 20 caracteres o menos."
   ],
   "compacto": "El icono final es demasiado pequeño; el chip entero debe ejecutar la acción o abrir el menú.",
   "expandido": "En ventanas medianas y expandidas los chips de filtro pueden llevar icono final para quitar o abrir menú.",
   "accesibilidad": [
    "Objetivo táctil mínimo de 48 dp, aunque sobresalga del contenedor visible.",
    "Acciones secundarias con objetivo de 48x48 dp: ancho mínimo del chip 88 dp o etiqueta 42 dp.",
    "Separación mínima de 8 dp entre chips.",
    "El icono final se alinea al final: a la derecha en LTR y a la izquierda en RTL."
   ]
  },
  "date-pickers": {
   "nombreM3": "Date pickers",
   "url": "https://m3.material.io/components/date-pickers/overview",
   "que": "Selector que permite elegir una fecha o un rango de fechas, pasadas, presentes o futuras.",
   "variantes": [
    "Docked (acoplado): campo de fecha con calendario desplegable debajo; ideal en ventanas medianas y expandidas.",
    "Docked: válido para fechas cercanas y lejanas, pasadas o futuras, porque admite teclado y calendario.",
    "Modal: diálogo con calendario; útil para fechas cercanas y rangos, como vuelos u hoteles.",
    "Modal a pantalla completa: recomendado en ventana compacta (móvil) para mejorar lectura y tamaño táctil.",
    "Modal input (entrada modal): diálogo para teclear fecha o rango con números; vale para fechas lejanas.",
    "Alternativa: un campo de texto con texto de ayuda adecuado, por ejemplo en un formulario."
   ],
   "cuandoUsar": [
    "Fecha lejana pasada o futura (p. ej. nacimiento): modal input o docked.",
    "Rango de fechas (reservar vuelo u hotel): modal de rango tocando inicio y fin.",
    "Fechas que no necesitan calendario: modal input puede ser la vista por defecto.",
    "Móvil: incrustado en diálogo; tableta y escritorio: desplegable de un campo de texto."
   ],
   "cuandoNo": [
    "No uses el modal de calendario para fechas muy lejanas, como la fecha de nacimiento.",
    "No escales el selector a un tamaño mayor según el punto de ruptura."
   ],
   "comportamiento": [
    "Docked: muestra el campo y al tocarlo aparece el calendario justo debajo; se puede usar teclado o calendario.",
    "Docked: mes y año se cambian con flechas o menú desplegable; el menú de años sustituye al calendario.",
    "Modal: deslizar en horizontal cambia de mes; tocar el año abre el selector de año con desplazamiento vertical.",
    "Rango: inicio y fin seleccionados con color; los días intermedios se unen con un resaltado suave.",
    "Se cambia entre modal de calendario y modal input con el icono de editar o de calendario.",
    "Cierre: Aceptar (OK) confirma, Cancelar o tocar fuera descarta; pantalla completa añade X y Guardar."
   ],
   "compacto": "En móvil se recomienda el selector modal a pantalla completa, que puede cubrir toda la pantalla.",
   "expandido": "En ventanas medianas y expandidas funciona mejor el docked, con calendario completo desplegable bajo el campo.",
   "accesibilidad": [
    "La versión a pantalla completa en móvil aumenta la legibilidad y el tamaño de los objetivos táctiles.",
    "Hoy y la fecha seleccionada se distinguen por color y relleno."
   ]
  },
  "time-pickers": {
   "nombreM3": "Time pickers",
   "url": "https://m3.material.io/components/time-pickers/overview",
   "que": "Selector modal en diálogo para elegir una hora concreta: horas, minutos o periodo (AM/PM).",
   "variantes": [
    "Dial (esfera): reloj circular; se toca un número o se arrastra la aguja. Ideal para selección táctil en móvil.",
    "Input (entrada): horas y minutos tecleados en campos separados; accesible desde el icono de teclado.",
    "Horizontal (landscape): entrada y esfera lado a lado en apaisado o ventanas con poca altura.",
    "Formato 24 h: pares en anillo interior e impares en exterior; se configura fuera del componente.",
    "Formato 12 h: todos los números en el anillo exterior y selector AM/PM a la derecha de los minutos."
   ],
   "cuandoUsar": [
    "Para poner una alarma.",
    "Para programar una reunión.",
    "Usa input cuando no haya altura suficiente para mostrar la esfera sin desplazamiento."
   ],
   "cuandoNo": [
    "No para selección muy fina, como milisegundos en un cronómetro.",
    "No apliques densidad a la esfera con poco espacio: usa la variante input."
   ],
   "comportamiento": [
    "Es modal sobre un velo (scrim) y retiene el foco hasta confirmar o descartar.",
    "Se teclea hora y minutos, o se elige el campo y se ajusta con la esfera, que actualiza el campo.",
    "El icono de teclado alterna entre esfera e input; el de reloj vuelve a la esfera.",
    "Aceptar (OK) guarda y cierra; Cancelar o tocar fuera lo descarta.",
    "No debe desplazarse ni quedar recortado: cambia orientación o variante para verse entero.",
    "No se desplaza con el contenido de fondo."
   ],
   "compacto": "En móvil vertical se muestra la esfera apilada; con poca altura cambia a apaisado o a la variante input.",
   "expandido": "En puntos de ruptura grandes puede pasar a orientación horizontal para evitar desplazar la esfera."
  },
  "dialogs": {
   "nombreM3": "Dialogs",
   "url": "https://m3.material.io/components/dialogs/overview",
   "que": "Ventana modal delante del contenido que da información crítica o pide una decisión antes de continuar.",
   "variantes": [
    "Basic (básico): alertas, selección rápida y confirmaciones; puede contener listas, selector de fecha u hora.",
    "Full-screen (pantalla completa): tareas de varios pasos, formularios con teclado o cambios no guardados al instante; solo en compacto."
   ],
   "cuandoUsar": [
    "Para avisos que bloquean el funcionamiento normal y exigen tarea, decisión o reconocimiento.",
    "Para confirmar acciones de alto riesgo, como borrar progreso.",
    "Pantalla completa: crear un evento con título, fecha, lugar y hora en móvil.",
    "Pantalla completa: cuando sus componentes abren otros diálogos."
   ],
   "cuandoNo": [
    "No para información de prioridad baja o media: usa un snackbar.",
    "Si no hace falta interrumpir, usa un menú desplegable, menos disruptivo.",
    "No añadas una tercera acción como «Más información»: usa expansión en línea.",
    "No uses pantalla completa en ventanas medianas o expandidas: usa el básico."
   ],
   "comportamiento": [
    "Bloquea la app y permanece hasta confirmar, descartar o completar la acción requerida.",
    "Máximo dos acciones a la derecha; la de confirmar, más cerca del borde. Una sola solo si es reconocimiento.",
    "Desactiva confirmar hasta que se elija algo; descartar nunca se desactiva.",
    "Si el contenido se desplaza, título fijo arriba y botones fijos abajo.",
    "Pantalla completa: solo icono X en la barra; Guardar o verbo claro (Crear, Enviar); al cerrar sin guardar, diálogo de descarte.",
    "Errores de campos en línea; errores generales (red) en un diálogo básico al fallar la confirmación."
   ],
   "compacto": "En móvil se usa el diálogo a pantalla completa para tareas complejas; puede abrirse desde un FAB con transformación de contenedor.",
   "expandido": "En medianas y expandidas el pantalla completa pasa a básico, centrado por defecto o reubicable respetando 56 dp de margen.",
   "accesibilidad": [
    "El titular debe ser claro y breve; evita disculpas, alarmas o preguntas ambiguas como «¿Seguro?».",
    "En idiomas RTL la alineación de botones se invierte y confirmar queda a la izquierda.",
    "Los mensajes de error explican la causa y cómo solucionarla; muéstralos todos a la vez."
   ]
  },
  "divider": {
   "nombreM3": "Divider",
   "url": "https://m3.material.io/components/divider/overview",
   "que": "Línea fina que agrupa contenido en listas u otros contenedores y crea jerarquía.",
   "variantes": [
    "Full-width (ancho completo): separa secciones grandes de contenido no relacionado o zonas interactivas de no interactivas.",
    "Inset (con sangría): separa contenido relacionado dentro de una sección, como correos en una lista con avatares.",
    "Vertical: organiza contenido en pantallas grandes, como texto junto a vídeo o imágenes."
   ],
   "cuandoUsar": [
    "Solo si los elementos no pueden agruparse con espacio en blanco.",
    "Para agrupar elementos, no para separar cada elemento individual.",
    "Combinando ancho completo e inset para reflejar la jerarquía de la información."
   ],
   "cuandoNo": [
    "No abuses de los de ancho completo: saturan la interfaz.",
    "En listas de formato repetitivo puede bastar el margen entre elementos."
   ],
   "comportamiento": [
    "Debe ser visible pero no llamativo.",
    "Puede indicar relaciones padre/hijo anidadas."
   ],
   "expandido": "En pantallas grandes se pueden usar divisores verticales para separar texto de medios."
  },
  "lists": {
   "nombreM3": "Lists",
   "url": "https://m3.material.io/components/lists/overview",
   "que": "Índices verticales continuos de texto e imágenes para encontrar un elemento concreto y actuar sobre él.",
   "variantes": [
    "Estilo estándar o segmentado (con huecos); la lista expresiva se recomienda para diseños nuevos.",
    "Selección única: con botones de opción, sin acciones anidadas.",
    "Selección múltiple: con casillas o interruptores, sin acciones anidadas.",
    "Acción única: todo el elemento hace una acción, como navegar.",
    "Multiacción: acción principal amplia y secundarias (marcador, menú) al final.",
    "No interactiva: solo organiza información escaneable."
   ],
   "cuandoUsar": [
    "Para comunicar o seleccionar elementos discretos, como elegir un color.",
    "Ordena de forma lógica (alfabética, numérica) con elementos cortos y escaneables.",
    "Usa huecos en listas contenidas; divisores solo en listas no contenidas o complejas."
   ],
   "cuandoNo": [
    "No varíes la posición de los elementos entre filas.",
    "No pongas imágenes en el centro de la fila; ancla visuales al borde inicial.",
    "No uses casillas en selección única ni botones de opción en selección múltiple."
   ],
   "comportamiento": [
    "Contenedor y etiqueta obligatorios; texto de apoyo de 1 a 3 líneas, truncable.",
    "La altura la marca el elemento más alto (56, 72 u 88 dp).",
    "El estado seleccionado se aplica a todo el elemento, no solo al control.",
    "Elementos con hijos se expanden y contraen como carpetas.",
    "En Android, deslizar revela botones; un deslizamiento completo ejecuta la acción principal."
   ],
   "compacto": "Ocupa de borde a borde; al tocar un elemento se abre una página de detalle. Muestra menos información.",
   "expandido": "Lista y detalle lado a lado, varias columnas, más texto e imágenes, o cambiar a tarjetas o carrusel.",
   "accesibilidad": [
    "Los elementos deslizables deben ofrecer otra forma de acceder a las acciones ocultas, como un icono de más.",
    "Longitud de línea ideal de 40 a 60 caracteres; hasta 120 en pantallas grandes."
   ]
  },
  "loading-indicator": {
   "nombreM3": "Loading indicator",
   "url": "https://m3.material.io/components/loading-indicator/overview",
   "que": "Indicador animado que muestra que un proceso corto (200 ms a 5 s) está en curso.",
   "variantes": [
    "Sin contenedor: colocado directamente sobre una superficie.",
    "Con contenedor (círculo): sobre otro contenido para más contraste, y en «tirar para actualizar»."
   ],
   "cuandoUsar": [
    "Esperas cortas e indeterminadas, entre 200 ms y 5 s.",
    "Cuando el progreso no es medible o no hace falta indicar la duración.",
    "Dentro de botones cuya acción tarda unos segundos, como validar un formulario.",
    "En «tirar para actualizar» al principio de listas o tarjetas con contenido dinámico."
   ],
   "cuandoNo": [
    "Menos de 200 ms: no muestres indicador, muestra el contenido.",
    "Más de 5 s: usa un indicador de progreso.",
    "No lo transformes en un indicador de progreso determinado.",
    "Nunca como decoración."
   ],
   "comportamiento": [
    "Animación en bucle que transforma siete formas de Material 3.",
    "Centrado en la página o contenedor que carga; al cargar más, en el hueco del nuevo contenido.",
    "Tirar para actualizar: hay que pasar un umbral; revertir el gesto cancela.",
    "Sigue visible hasta que termina la actualización; no debe desplazarse fuera de pantalla."
   ],
   "compacto": "Tamaño por defecto de 48 dp, ideal para móvil y ventanas compactas.",
   "expandido": "Escálalo en proporción al espacio vacío, entre 24 y 240 dp; tamaños grandes solo en ventanas grandes."
  },
  "progress-indicators": {
   "nombreM3": "Progress indicators",
   "url": "https://m3.material.io/components/progress-indicators/overview",
   "que": "Indicador que muestra en tiempo real el estado de un proceso, como cargar, enviar o guardar.",
   "variantes": [
    "Lineal: mejor en el borde de un contenedor; ocupa todo el ancho del elemento.",
    "Circular: mejor centrado en un elemento, como página, tarjeta o botón.",
    "Determinado: progreso y espera conocidos, rellena de 0 % a 100 %.",
    "Indeterminado: espera desconocida, el indicador crece y encoge en bucle.",
    "Forma plana u ondulada (wavy); la ondulada es más expresiva para procesos largos."
   ],
   "cuandoUsar": [
    "Esperas largas, de más de 5 s.",
    "Uno solo para el progreso global de un grupo de elementos.",
    "Lineal arriba de la página si carga todo; en una tarjeta si solo carga ella.",
    "Circular en un botón para indicar que su acción está en curso."
   ],
   "cuandoNo": [
    "No pongas un indicador por cada actividad o por cada botón de una lista.",
    "Lineal no en elementos de menos de 40 dp.",
    "Si la carga es corta, mejor un loading indicator.",
    "Ondulado no en botones muy pequeños: usa la forma plana."
   ],
   "comportamiento": [
    "Pasa de indeterminado a determinado cuando se conoce más del proceso.",
    "Al empezar, el indicador activo aparece como un punto.",
    "Lineal anima del borde inicial al final; circular desde arriba en sentido horario.",
    "Usa la misma variante para el mismo proceso en todo el producto.",
    "Al cargar más elementos, el circular va en el hueco del nuevo contenido."
   ],
   "expandido": "El circular va de 24 a 240 dp; los muy grandes se reservan para ventanas grandes como escritorio.",
   "accesibilidad": [
    "Punto de parada de 4 dp al final del lineal determinado si el contraste de la pista es menor de 3:1.",
    "En botones, el indicador toma el color del texto o icono y se quita la pista para lograr 3:1.",
    "En idiomas RTL el lineal se refleja; el circular no."
   ]
  },
  "menus": {
   "nombreM3": "Menus",
   "url": "https://m3.material.io/components/menus/overview",
   "que": "Lista de opciones en una superficie temporal que aparece al interactuar con un elemento.",
   "variantes": [
    "Menú vertical (recomendado en diseños nuevos) o menú base.",
    "Color estándar (superficie, poco énfasis) o vibrante (terciario, más énfasis; usar con moderación).",
    "Contextual: acciones sobre un elemento; se abre con clic secundario o pulsación larga.",
    "Desplegable de campo de texto, de selección o de desbordamiento (overflow).",
    "Con submenús, en pantallas grandes; con campo de filtro (autocompletar) para listas largas."
   ],
   "cuandoUsar": [
    "Para un conjunto temporal de acciones.",
    "Cuando hace falta ahorrar espacio frente a botones de opción o chips.",
    "Al abrir desde botones de icono, botones divididos, campos de texto, chips de filtro o texto resaltado."
   ],
   "cuandoNo": [
    "Si las acciones deben verse siempre, usa una barra de herramientas.",
    "No uses huecos en menús con desplazamiento; usa divisores.",
    "No metas botones, interruptores u otras acciones directas dentro de un elemento."
   ],
   "comportamiento": [
    "Aparece delante de todo, junto o sobre su origen; se recoloca si fuera a cortarse.",
    "El disparador queda en estado presionado sin cambiar su aspecto.",
    "Selección única deselecciona la anterior; la múltiple sigue abierta hasta cerrarse.",
    "Las opciones no disponibles se muestran desactivadas, no se eliminan.",
    "Si no caben todas, se desplaza con barra de scroll persistente.",
    "Agrupa con 1 o 2 huecos o con divisores; los submenús se abren al lado sin solaparse."
   ],
   "compacto": "En pantallas pequeñas conviene convertir el menú en una hoja inferior (bottom sheet).",
   "expandido": "En medianas y expandidas el menú en contexto es más eficaz; admite más elementos y submenús. En escritorio puede abrirse al instante.",
   "accesibilidad": [
    "Los objetivos de los slots deben medir 48x48 dp o más.",
    "Cada elemento anidado solo debe hacer una acción para no romper teclado ni lector de pantalla.",
    "Mantén el mismo relleno de los elementos al usar slots."
   ]
  },
  "radio-button": {
   "nombreM3": "Radio button",
   "url": "https://m3.material.io/components/radio-button/overview",
   "que": "Control que permite elegir una sola opción de un conjunto mostrando todas las disponibles.",
   "cuandoUsar": [
    "Para seleccionar una única opción de un conjunto.",
    "Cuando conviene mostrar todas las opciones.",
    "Con cinco opciones o menos."
   ],
   "cuandoNo": [
    "Si se pueden elegir varias opciones: usa casillas.",
    "No anides botones de opción.",
    "Si falta espacio, valora un menú desplegable, aunque exige más pasos.",
    "Evita listas horizontales de botones de opción."
   ],
   "comportamiento": [
    "Solo uno seleccionado a la vez; siempre debe haber uno preseleccionado.",
    "Se colocan en vertical, apilados.",
    "Se selecciona tocando el icono o la etiqueta.",
    "Surte efecto inmediato salvo dentro de un diálogo o página que se deba guardar."
   ],
   "accesibilidad": [
    "Cada opción necesita su propia etiqueta adyacente que describa lo que selecciona."
   ]
  },
  "sliders": {
   "nombreM3": "Sliders",
   "url": "https://m3.material.io/components/sliders/overview",
   "que": "Control para elegir valores a lo largo de una pista, como volumen, brillo o intensidad de filtros.",
   "variantes": [
    "Standard (estándar): un valor desde cero o el inicio de una secuencia.",
    "Centered (centrado): valor positivo o negativo cuando el cero o el valor por defecto está en medio.",
    "Range (rango): dos asas para definir mínimo y máximo; solo en horizontal.",
    "Con paradas (stops): el asa salta a valores predefinidos.",
    "Orientación horizontal o vertical; tamaños XS, S, M, L y XL (XL para momentos protagonistas).",
    "Icono interior: solo en estándar de tamaño M, L o XL."
   ],
   "cuandoUsar": [
    "Ajustar ajustes como volumen y brillo.",
    "Cambiar la intensidad de filtros de imagen en tiempo real.",
    "Mostrar el rango completo de valores disponibles."
   ],
   "cuandoNo": [
    "No uses el de rango en vertical: añade carga cognitiva.",
    "Evita demasiadas paradas: se satura y cuesta ajustar.",
    "No pongas icono interior en centrados, de rango ni con pista menor de 40 dp."
   ],
   "comportamiento": [
    "El cambio surte efecto de inmediato mientras se mueve el asa.",
    "Arrastrar el asa o tocar la pista; con paradas, salta a la más cercana.",
    "El asa cambia de forma al pulsarla o arrastrarla.",
    "El valor aparece al interactuar; en rango, solo en un asa a la vez.",
    "Puede sincronizarse con un campo de texto externo que se actualiza en ambos sentidos.",
    "En LTR los valores crecen a la derecha; en RTL al revés."
   ],
   "accesibilidad": [
    "Teclado: Tab enfoca el asa; flechas cambian un valor o parada; espacio y flechas, saltos mayores.",
    "Paradas finales en la pista inactiva para asegurar contraste de 3:1, salvo que ya lo tenga.",
    "Iconos o texto a los lados (más/menos) indican el rango y mejoran la accesibilidad.",
    "Si el valor está en un campo externo, debe poder alcanzarse con Tab justo después del slider."
   ]
  },
  "navigation-bar": {
   "nombreM3": "Navigation bar",
   "url": "https://m3.material.io/components/navigation-bar/overview",
   "que": "Barra fija inferior para cambiar entre 3 y 5 vistas principales de la app en dispositivos pequeños.",
   "variantes": [
    "Barra flexible (Expressive): sustituye a la barra baseline, más baja; la baseline ya no se recomienda.",
    "Ítems verticales (icono sobre texto): en ventanas compactas, como móvil.",
    "Ítems horizontales (icono junto al texto dentro del indicador): en ventanas medianas, como tablet."
   ],
   "cuandoUsar": [
    "Para 3 a 5 páginas principales de igual importancia.",
    "Para destinos de primer nivel en móvil o tablet.",
    "Con destinos fijos y consistentes en todas las pantallas."
   ],
   "cuandoNo": [
    "Con más de 5 destinos: usa pestañas o un rail expandido modal tras un icono de menú.",
    "Con menos de 3 destinos: usa pestañas.",
    "Para tareas sueltas, como ver un único email.",
    "En escritorio: usa un navigation rail o pestañas.",
    "A la vez que un rail o una toolbar inferior."
   ],
   "comportamiento": [
    "Siempre hay un destino activo, con icono relleno e indicador en forma de píldora.",
    "Todos los destinos llevan icono y etiqueta de 1-2 palabras; sin cortar ni encoger texto.",
    "Al cambiar de destino puede conservar el estado (scroll, pestaña, búsqueda) o reiniciarlo.",
    "Volver a tocar el destino activo sube el scroll al principio.",
    "No se navega deslizando entre destinos; sus posiciones son fijas.",
    "Puede ocultarse al hacer scroll hacia abajo y reaparecer al subir."
   ],
   "compacto": "Se usa barra de navegación (o rail modal) con ítems verticales, a ancho completo en la parte inferior; el FAB va encima, alineado a la derecha.",
   "expandido": "En ventanas medianas, barra con ítems horizontales centrados o un navigation rail. En expandidas y mayores, sustituir por navigation rail.",
   "accesibilidad": [
    "Iconos activos e inactivos con contraste mínimo 3:1 frente al contenedor.",
    "No ocultar la barra al hacer scroll si hay un lector de pantalla activo.",
    "Puede quedar tapada temporalmente (teclado, hojas, diálogos), nunca de forma permanente."
   ]
  },
  "navigation-drawer": {
   "nombreM3": "Navigation drawer",
   "url": "https://m3.material.io/components/navigation-drawer/overview",
   "que": "Panel lateral con lista de destinos y funciones de la app para pantallas grandes. Ya no se recomienda en M3 Expressive: usar rail expandido.",
   "variantes": [
    "Estándar: junto al contenido, en ventanas expandidas, grandes y extragrandes; fijo o abrible con icono de menú.",
    "Modal: sobre el contenido con scrim; sobre todo en ventanas compactas y medianas."
   ],
   "cuandoUsar": [
    "Apps con 5 o más destinos de primer nivel.",
    "Apps con 2 o más niveles de jerarquía de navegación.",
    "Navegación rápida entre destinos no relacionados.",
    "Sustituir rail o barra de navegación en pantallas grandes."
   ],
   "cuandoNo": [
    "Junto a otro componente de navegación principal, como una barra de navegación.",
    "En diseños nuevos con M3 Expressive: preferir el navigation rail expandido."
   ],
   "comportamiento": [
    "Siempre hay un destino activo, marcado con un indicador de fondo.",
    "Se abre desde el borde inicial (izquierda en LTR, derecha en RTL).",
    "El modal se abre desde un icono de menú y se cierra al elegir ítem, tocar el scrim o deslizar hacia su borde.",
    "El estándar descartable se abre y cierra con el icono de menú; el permanente no se puede cerrar.",
    "Su lista hace scroll independiente del contenido.",
    "Puede tener cabeceras, etiquetas de sección y divisores entre grupos; iconos en todos o en ninguno."
   ],
   "compacto": "Usar drawer modal o cambiarlo por una barra de navegación. En web, por debajo de 320 px CSS, cambiarlo por barra de navegación.",
   "expandido": "En medianas y expandidas, drawer modal solo o junto a un rail; en expandidas cabe el estándar en layouts de un panel. En grandes y escritorio, drawer estándar o rail que pasa a drawer modal.",
   "accesibilidad": [
    "En web, por debajo de 320 px CSS, sustituirlo por barra de navegación por accesibilidad."
   ]
  },
  "navigation-rail": {
   "nombreM3": "Navigation rail",
   "url": "https://m3.material.io/components/navigation-rail/overview",
   "que": "Columna vertical en el borde inicial con destinos, menú y FAB opcional, para ventanas medianas y mayores.",
   "variantes": [
    "Colapsado: 3 a 7 destinos, siempre visible; sustituye al rail baseline.",
    "Expandido estándar: junto al contenido; ventanas grandes con mucho espacio. Sustituye al drawer.",
    "Expandido modal: sobre el contenido; layouts densos o muchos destinos.",
    "Expandido oculto: en experiencias inmersivas, solo aparece al pulsar el icono de menú."
   ],
   "cuandoUsar": [
    "En ventanas medianas, expandidas, grandes y extragrandes.",
    "Si se prioriza navegación vertical persistente frente a espacio vertical.",
    "Con más de 5 destinos, valorar un rail expandido modal."
   ],
   "cuandoNo": [
    "En ventanas compactas: usar barra de navegación.",
    "En medianas con pocos destinos, considerar la barra de navegación.",
    "A la vez que una barra de navegación.",
    "En horizontal: para eso está la barra de navegación."
   ],
   "comportamiento": [
    "El icono de menú alterna entre colapsado y expandido, y cambia para indicar que se puede colapsar.",
    "Al expandirse puede mostrar destinos secundarios; el FAB pasa a FAB extendido y el contenido se ajusta.",
    "Destino activo con icono relleno e indicador en píldora; etiqueta de una palabra, sin truncar.",
    "Los destinos quedan fijos al hacer scroll vertical.",
    "Menú y FAB siempre arriba; destinos alineados arriba o al centro (centro en tablets).",
    "En Android, el gesto atrás predictivo solo aplica al rail expandido modal."
   ],
   "compacto": "No usar el rail estándar: usar barra de navegación. El rail expandido puede abrirse como modal desde un botón de menú en móvil.",
   "expandido": "En medianas, rail (o barra si hay pocos destinos). En expandidas a extragrandes, siempre rail; elegir estándar o modal según espacio horizontal y número de destinos.",
   "accesibilidad": [
    "Sin relleno de contenedor, todos los ítems deben tener contraste mínimo 3:1.",
    "El área táctil de cada destino ocupa todo el ancho."
   ]
  },
  "search": {
   "nombreM3": "Search",
   "url": "https://m3.material.io/components/search/overview",
   "que": "Permite escribir una palabra o frase para encontrar información relevante, con sugerencias y resultados en lista.",
   "variantes": [
    "Barra de búsqueda: buscar en una vista concreta, como «Busca tus mensajes».",
    "App bar de búsqueda: cuando buscar es la función principal y global.",
    "Botón de icono de búsqueda: cuando buscar es una acción secundaria.",
    "Estilo contenido (recomendado) o dividido (con divisor entre barra y resultados)."
   ],
   "cuandoUsar": [
    "Productos con muchos elementos que gestionar, como archivos o mensajes.",
    "Para navegar un producto mediante consultas.",
    "Como destino propio desde la barra de navegación si buscar es la acción principal."
   ],
   "comportamiento": [
    "Al seleccionarla se abre la búsqueda enfocada y la barra se ensancha.",
    "Puede mostrar historial antes de escribir, sugerencias al escribir o esperar a ejecutar la búsqueda.",
    "Se busca pulsando Enter o eligiendo una sugerencia; los resultados aparecen en lista bajo la barra.",
    "El texto de consulta queda visible, sin foco, tras buscar; icono de borrar opcional.",
    "El icono atrás quita el foco, cierra sugerencias y devuelve la barra a su estado inicial.",
    "En scroll puede ocultarse y reaparecer al subir, o quedar fija arriba."
   ],
   "compacto": "La búsqueda enfocada se muestra a pantalla completa por defecto.",
   "expandido": "En medianas y expandidas, layout acoplado: lista bajo la barra con scrim sobre el contenido. La barra se queda en su panel y escala su ancho.",
   "accesibilidad": [
    "La búsqueda enfocada necesita un indicador claro de que está buscando, como un icono o la etiqueta «Resultados».",
    "No usar surface container high sobre fondo surface container: reduce el contraste."
   ]
  },
  "bottom-sheets": {
   "nombreM3": "Bottom sheets",
   "url": "https://m3.material.io/components/bottom-sheets/overview",
   "que": "Hoja anclada al borde inferior con contenido y acciones secundarias, sobre todo en móvil.",
   "variantes": [
    "Estándar: convive con el contenido principal y permite usar ambos, como un reproductor de música.",
    "Modal: delante del contenido con scrim, bloquea la app; alternativa a menús o diálogos simples en móvil."
   ],
   "cuandoUsar": [
    "En ventanas compactas y medianas.",
    "Para contenido complementario o secundario, no el principal.",
    "Modal: listas largas de acciones o ítems con descripción e iconos."
   ],
   "cuandoNo": [
    "Modal fuera de apps móviles.",
    "Para tareas o flujos complejos en pantallas grandes: considerar una hoja flotante."
   ],
   "comportamiento": [
    "El modal se abre por una acción del usuario y su altura inicial no supera el 50% de la pantalla.",
    "El modal se cierra al tocar un ítem, el scrim, deslizar hacia abajo o el botón de cerrar.",
    "El tirador permite arrastrar o tocar para pasar por alturas predefinidas o cerrar.",
    "Puede alternar entre colapsada y expandida; a pantalla completa lleva icono de colapsar o cerrar.",
    "Hace scroll interno si el contenido supera la altura visible.",
    "En Android, el gesto atrás predictivo la separa de los bordes antes de cerrar."
   ],
   "compacto": "Ocupa todo el ancho de la pantalla y se eleva sobre el contenido principal.",
   "expandido": "En medianas y expandidas tiene un ancho máximo por defecto (640dp), modificable. En escritorio puede cambiarse por una side sheet.",
   "accesibilidad": [
    "El tirador tiene un área táctil accesible de 48dp.",
    "Si hay varias alturas sin tirador, ofrecer una alternativa de un solo puntero para cambiarla."
   ]
  },
  "side-sheets": {
   "nombreM3": "Side sheets",
   "url": "https://m3.material.io/components/side-sheets/overview",
   "que": "Hoja anclada al lateral con contenido y acciones opcionales sin interrumpir el contenido principal.",
   "variantes": [
    "Estándar: visible junto al contenido; sobre todo en ventanas medianas y expandidas.",
    "Modal: sobre el contenido con scrim; preferida en ventanas compactas, hay que cerrarla para seguir."
   ],
   "cuandoUsar": [
    "Acciones que afectan al contenido principal, como filtros.",
    "Contenido y funciones complementarias, como información de una foto.",
    "Controles de formulario, como ajustes de la app."
   ],
   "cuandoNo": [
    "Contenido con scroll horizontal o que lo sugiera.",
    "Muy separada de los bordes, más allá del margen recomendado."
   ],
   "comportamiento": [
    "Ancho fijo y suele ocupar toda la altura; normalmente a la derecha, con margen opcional de 16dp.",
    "Al abrir la estándar, el contenido se estrecha para dejarle sitio.",
    "Botón de cerrar muy recomendado; botón atrás opcional para navegar dentro de la hoja.",
    "Botones de acción opcionales (Guardar, Editar…) separables con divisor.",
    "Scroll vertical interno e independiente; nunca horizontal.",
    "En Android, atrás predictivo: se separa de los bordes y escala en dirección del gesto."
   ],
   "compacto": "Se prefiere la side sheet modal por el poco espacio.",
   "expandido": "En medianas y expandidas se usa la estándar; la modal en pantallas pequeñas puede pasar a estándar en grandes.",
   "accesibilidad": [
    "Un botón de cerrar aumenta la accesibilidad y facilita cerrar la hoja.",
    "En idiomas RTL aparece a la izquierda con los elementos invertidos."
   ]
  },
  "snackbar": {
   "nombreM3": "Snackbar",
   "url": "https://m3.material.io/components/snackbar/overview",
   "que": "Mensaje breve y temporal abajo de la pantalla sobre un proceso que la app ha hecho o hará.",
   "variantes": [
    "Sin acción: desaparece sola tras 4-10 segundos.",
    "Con acción: queda en pantalla hasta que el usuario actúa o la cierra."
   ],
   "cuandoUsar": [
    "Mensajes de baja prioridad que no exigen acción.",
    "Para ofrecer una acción tipo «Deshacer»."
   ],
   "cuandoNo": [
    "Mensajes importantes que exigen acción: usar un diálogo.",
    "Si necesita icono o enlace: usar otro componente o un botón.",
    "Como única vía para un caso de uso esencial."
   ],
   "comportamiento": [
    "Solo una a la vez; nunca apiladas ni lado a lado. Una actualizada puede reemplazar a otra.",
    "Una sola acción como botón de texto de color; cerrar es opcional.",
    "Va abajo, delante del contenido, encima del FAB y sin tapar navegación.",
    "Aparece sin aviso y no bloquea la interacción con la página.",
    "No animar otros componentes, como el FAB, junto a ella."
   ],
   "compacto": "Crece en alto de 48 a 64dp para una o dos líneas, con distancia fija a los bordes laterales e inferior.",
   "expandido": "En medianas y expandidas se ensancha para textos largos, idealmente una línea con botón opcional; alineada a la izquierda o centrada, siempre en el mismo sitio.",
   "accesibilidad": [
    "En web, si se cierra sola, repetir el mensaje en línea cerca de la acción (p. ej. «Guardado»).",
    "Alternativa en web: añadir acción para que no se cierre sola.",
    "No tapar por completo elementos con foco de teclado."
   ]
  },
  "switch": {
   "nombreM3": "Switch",
   "url": "https://m3.material.io/components/switch/overview",
   "que": "Interruptor que activa o desactiva una opción al instante, ideal para ajustes.",
   "cuandoUsar": [
    "Activar o desactivar un único elemento o ajuste.",
    "Opciones independientes o descriptivas en una lista, como ajustes.",
    "Cuando el efecto debe ser inmediato, sin guardar."
   ],
   "cuandoNo": [
    "Para elegir entre opciones opuestas: usar un grupo de botones conectados.",
    "Para varias opciones que requieren guardar: usar casillas.",
    "Para una sola opción de una lista: usar radio buttons.",
    "En lugar de un botón de llamada a la acción."
   ],
   "comportamiento": [
    "Al pulsarlo, el tirador se desliza al otro extremo y la acción se aplica al momento.",
    "El estado activado se indica con un tirador más grande.",
    "Icono opcional en el tirador que comunique claramente on/off (X y check).",
    "Siempre con etiqueta en línea que describa qué hace al activarse.",
    "Suelen apilarse en pantallas de ajustes."
   ],
   "accesibilidad": [
    "No poner texto dentro del switch: sería demasiado pequeño para ser accesible."
   ]
  },
  "tabs": {
   "nombreM3": "Tabs",
   "url": "https://m3.material.io/components/tabs/overview",
   "que": "Pestañas que organizan contenido relacionado del mismo nivel en vistas distintas.",
   "variantes": [
    "Primarias: arriba del panel bajo la app bar, para los destinos de contenido principales.",
    "Secundarias: dentro del contenido, cuando hace falta un segundo nivel; siempre bajo las primarias.",
    "Fijas: todas visibles a la vez, para cambiar rápido entre contenido relacionado.",
    "Desplazables: cuando no caben; admiten más pestañas y etiquetas largas."
   ],
   "cuandoUsar": [
    "Agrupar contenido relacionado en categorías claras.",
    "Organizar contenido dentro de una página (la navegación es para páginas distintas).",
    "Junto a un navigation rail como capa extra de navegación."
   ],
   "cuandoNo": [
    "Para contenido secuencial que se lee en orden.",
    "Con contenido deslizable (mapas, ítems de lista) en el área de pestañas.",
    "Con más de cuatro pestañas fijas: el contenedor queda apretado."
   ],
   "comportamiento": [
    "Pestaña activa con subrayado y cambio de color en texto e icono.",
    "Se cambia tocando la pestaña o deslizando en el área de contenido (fijas).",
    "En desplazables, la primera se desplaza 52dp del borde para indicar que hay más.",
    "Al hacer scroll pueden quedar fijas arriba o salir y volver al subir; se mueven junto a la app bar.",
    "Iconos y etiquetas en todas o en ninguna; insignias de hasta 4 caracteres."
   ],
   "expandido": "En pestañas fijas, el ancho de cada una lo marca la más ancha; el grupo usa margen fluido y se centra o alinea al borde inicial."
  },
  "text-fields": {
   "nombreM3": "Text fields",
   "url": "https://m3.material.io/components/text-fields/overview",
   "que": "Campo para introducir texto en la interfaz, típico de formularios y diálogos.",
   "variantes": [
    "Relleno (filled): mayor énfasis visual.",
    "Delineado (outlined): menos énfasis; simplifica formularios con muchos campos.",
    "Una línea, multilínea (crece al escribir) o área de texto (alto fijo, para respuestas largas)."
   ],
   "cuandoUsar": [
    "Cuando hay que escribir texto, como datos de contacto o de pago.",
    "Elegir variante por estilo de la app y para distinguirse de botones y contenido."
   ],
   "cuandoNo": [
    "Mezclar relleno y delineado en la misma región o formulario.",
    "Campos de una línea para respuestas largas.",
    "Multilínea en web: usar área de texto."
   ],
   "comportamiento": [
    "Al enfocar, la etiqueta pasa del centro a la parte superior del campo.",
    "El estado (vacío, con texto, error…) debe verse de un vistazo.",
    "En una línea, el texto largo se desplaza; en multilínea, el campo crece y empuja el contenido.",
    "Los campos de solo lectura se ven iguales pero se etiquetan como solo lectura."
   ],
   "compacto": "Los campos pueden ocupar todo el ancho de la pantalla.",
   "expandido": "En medianas y expandidas, limitar con márgenes flexibles o contenedores; nunca a ancho completo en pantallas grandes.",
   "accesibilidad": [
    "Icono de error muy recomendado como segundo indicador para personas con baja visión.",
    "Iconos de válido/error para que los errores sean claros a daltónicos.",
    "No aplicar densidad por defecto: reduce el área táctil por debajo de 48x48 px CSS."
   ]
  },
  "toolbars": {
   "nombreM3": "Toolbars",
   "url": "https://m3.material.io/components/toolbars/overview",
   "que": "Barra con acciones frecuentes relacionadas con la página actual.",
   "variantes": [
    "Acoplada (docked): ancho completo abajo, para acciones globales comunes a varias páginas. Sustituye a la bottom app bar.",
    "Flotante: sobre el contenido, para acciones contextuales de la página; horizontal o vertical, puede ir con FAB.",
    "Color estándar: poco énfasis, centra la atención en el contenido.",
    "Color vibrante: destaca controles o indica un modo temporal, como edición."
   ],
   "cuandoUsar": [
    "Para acciones de la página actual, en páginas secundarias.",
    "Flotante como pestañas entre páginas relacionadas (navegación local)."
   ],
   "cuandoNo": [
    "A la vez que una barra de navegación: ambas van abajo.",
    "Acoplada junto a otros elementos anclados abajo.",
    "Vertical o varias toolbars en ventanas compactas."
   ],
   "comportamiento": [
    "Si no caben las acciones, añadir un menú de desbordamiento en el hueco final.",
    "Destacar solo una acción a la vez (botón relleno, color propio o FAB).",
    "La flotante debe verse entera, con margen mínimo de 16dp (24dp si es vertical).",
    "Acoplada: queda fija o sale de pantalla al hacer scroll.",
    "Flotante: fija, sale de pantalla o se colapsa en una acción; nunca colapsar y salir a la vez."
   ],
   "compacto": "Acoplada con elementos repartidos por igual. Una sola toolbar para todas las acciones; evitar verticales.",
   "expandido": "Acoplada con elementos centrados o acción clave al centro; en web puede ser redondeada. Flotante muestra más acciones; puede ser vertical, opuesta al rail, o varias en bordes opuestos.",
   "accesibilidad": [
    "Todos los elementos necesitan un área táctil mínima de 48x48dp."
   ]
  },
  "tooltips": {
   "nombreM3": "Tooltips",
   "url": "https://m3.material.io/components/tooltips/overview",
   "que": "Etiquetas o mensajes breves que dan contexto adicional a un elemento de la interfaz.",
   "variantes": [
    "Simple (plain): describe brevemente elementos sin texto, como botones de solo icono.",
    "Enriquecido (rich): textos más largos como definiciones; título, enlaces y hasta dos botones de texto opcionales.",
    "Enriquecido persistente: aparece al hacer clic o al cargar la página para explicar novedades."
   ],
   "cuandoUsar": [
    "Etiquetar botones de solo icono.",
    "Dar información y acciones extra sobre un elemento o una función nueva."
   ],
   "cuandoNo": [
    "Si el elemento ya tiene etiqueta de texto.",
    "Para información crítica: usar un diálogo.",
    "Tooltips enriquecidos persistentes sobre botones de icono."
   ],
   "comportamiento": [
    "Se muestra al pasar el ratón en escritorio o mantener pulsado en móvil.",
    "Desaparece 1,5 s después de salir de la zona; solo uno a la vez.",
    "El simple va encima del elemento (debajo si está en una app bar).",
    "El enriquecido va abajo a la derecha, se recoloca para no salirse y no tapa al elemento.",
    "El persistente solo se cierra al interactuar con otro elemento."
   ],
   "expandido": "En escritorio puede aparecer centrado bajo el elemento y seguir visible mientras el cursor se mueve dentro de la zona."
  }
 },
 "campos": {
  "etiqueta": [
   "Todo campo debe tener etiqueta que diga qué se pide.",
   "Siempre visible, corta, sin truncar ni ocupar varias líneas.",
   "Alineada con el texto introducido; al enfocar sube a la parte superior.",
   "Puede omitirse si hay una etiqueta adyacente, alineada al borde inicial del campo."
  ],
  "textoAyuda": [
   "Información extra sobre el campo, como su uso.",
   "Idealmente una línea; puede ocupar varias si hace falta.",
   "Puede estar siempre visible o solo al enfocar."
  ],
  "error": [
   "Sustituye el texto de ayuda por el de error; no añadas ambos.",
   "Explica cómo evitar el error (o el más probable si hay varios).",
   "Si es largo, puede ocupar varias líneas: deja espacio entre campos.",
   "Muestra icono de error como segundo indicador."
  ],
  "prefijoSufijo": [
   "Prefijo: por ejemplo, símbolo de moneda.",
   "Sufijo: por ejemplo, unidad de medida o dominio de email."
  ],
  "contadorCaracteres": [
   "Inclúyelo si hay límite de caracteres o palabras.",
   "Muestra caracteres usados frente al límite total."
  ],
  "iconos": [
   "Opcionales; cambian de lado según LTR o RTL.",
   "Indican tipo de entrada: calendario que abre selector de fecha, micrófono para voz.",
   "Icono de borrar, solo visible si hay texto.",
   "Flecha desplegable si el campo tiene un selector anidado.",
   "Icono de válido o error; imágenes de 24dp, como una tarjeta de crédito."
  ],
  "obligatorio": [
   "Marca con asterisco (*) junto a la etiqueta.",
   "Explica el asterisco con texto de ayuda o una nota al inicio del formulario.",
   "Marca todos los campos obligatorios, con el asterisco del mismo color."
  ],
  "tipoEntrada": [
   "Una línea: textos cortos; el texto largo se desplaza.",
   "Multilínea: crece al escribir y empuja el contenido.",
   "Área de texto: alto fijo con scroll; para respuestas largas y en web.",
   "Solo lectura: texto prefijado no editable, etiquetado como tal."
  ]
 }
};
