# PlanesGo - Guía de Estilo y Reglas de Diseño

Documento normativo de identidad visual, componentes y principios de interfaz de usuario para **PlanesGo**, basado en la identidad corporativa de **PLANES Soluciones Informáticas**.

---

## 1. Identidad de Marca y Paleta de Colores

### 1.1 Colores Principales
- **Planes Azul Corporativo (`--planes-blue`):** `#0072CE` / `#0284C7` (Tailwind `sky-600` / `blue-600`)  
  *Uso:* Botones principales, enlaces activos, selecciones, bordes de foco y acentos primarios.
- **Planes Azul Marino Profundo (`--planes-navy`):** `#0B0F19` / `#0F172A` (Tailwind `slate-950` / `slate-900`)  
  *Uso:* Cabecera de marca, texto de títulos principales y contraste de alta visibilidad.
- **Planes Verde Lima (`--planes-lime`):** `#B4D330` / `#84CC16` (Tailwind `lime-500` / `lime-600`)  
  *Uso:* Acentos de éxito, estados activos, badges destacados y detalle sutil en componentes de impacto.

### 1.2 Superficies y Escala de Grises
- **Fondo General de Aplicación:** `#F8FAFC` (Tailwind `slate-50`)
- **Superficie de Tarjetas / Tablas:** `#FFFFFF` (Blanco puro con micro-borde `border-slate-200/80`)
- **Líneas divisorias y bordes:** `#E2E8F0` / `#F1F5F9` (`border-slate-100` y `border-slate-200`)
- **Texto Principal:** `#0F172A` (`text-slate-900`)
- **Texto Secundario:** `#64748B` (`text-slate-500`)
- **Texto Muted / Metadatos:** `#94A3B8` (`text-slate-400`)

---

## 2. Tipografía

- **Familia tipográfica:** `Inter`, `-apple-system`, `BlinkMacSystemFont`, `Segoe UI`, `Roboto`, `sans-serif`.
- **Pesos y Tamaños:**
  - **Títulos / KPIs:** `font-bold` / `font-semibold` (24px - 30px, tabular-nums para valores numéricos).
  - **Encabezados de sección:** `text-sm font-semibold text-slate-900 uppercase tracking-wider`.
  - **Cuerpo y Tablas:** `text-sm text-slate-700 font-normal`.
  - **Etiquetas y Metadatos:** `text-xs font-medium text-slate-500`.

---

## 3. Principios de Diseño Funcional y Minimalista

1. **Jerarquía Visual Inmediata:**  
   La información más importante (KPIs globales y tabla de imputaciones) debe ser visible en el primer impacto sin scroll innecesario.
2. **Cero Ruido Visual:**  
   Evitar gradientes pesados o sombras excesivas. Utilizar micro-bordes limpios de `1px` (`border-slate-200/80`) con sombras muy suaves (`shadow-sm`).
3. **Densidad de Información Optimizada:**  
   El espacio en pantalla debe ser eficiente, permitiendo ver múltiples registros a la vez sin fatiga visual.
4. **Micro-interacciones Inmediatas:**  
   Filtros reactivos en tiempo real (búsqueda por texto, filtrado por proyecto y empleado) con recálculo instantáneo de horas y contadores.
5. **Iconografía Coherente:**  
   Iconos vectoriales en línea fina (24x24 / 16x16) con trazo constante de `1.5px` a `2px`.

---

## 4. Componentes Estándar

### 4.1 Logotipo Corporativo
- Ubicado en `/static/img/logo.png`.
- En la cabecera principal y pantalla de autenticación, mantener proporciones exactas con fondo oscuro o superficie neutra de alto contraste.

### 4.2 Botones
- **Primario:** Fondo Planes Azul (`bg-sky-600 hover:bg-sky-700 text-white font-semibold rounded-xl px-4 py-2 text-sm shadow-sm`).
- **Secundario / Filtro:** Fondo `bg-white hover:bg-slate-50 text-slate-700 border border-slate-200 rounded-xl px-3 py-2 text-sm`.
- **Peligro / Salir:** Fondo `bg-red-50 hover:bg-red-100 text-red-700 border border-red-200 rounded-xl px-3 py-1.5 text-xs`.

### 4.3 Tarjetas KPI
- Fondo blanco puro, esquina redondeada `rounded-2xl`, borde `border-slate-200/80`.
- Icono en contenedor cuadrado redondeado con fondo tintado suave (`bg-sky-50 text-sky-600`, `bg-lime-50 text-lime-700`, etc.).

### 4.4 Tabla de Datos
- Encabezado sticky o con fondo tenue `bg-slate-50/90 text-slate-500 uppercase tracking-wider text-xs`.
- Filas con separación `divide-y divide-slate-100` y efecto hover sutil `hover:bg-slate-50/80`.
- Badge de horas: Píldora destacada `bg-sky-50 text-sky-700 border border-sky-100 font-bold font-mono`.

---

## 5. Regla de Aplicación Obligatoria
Cualquier nueva vista, componente o ajuste en **PlanesGo** debe ceñirse a las definiciones y tokens de este archivo para garantizar una experiencia uniforme, corporativa y minimalista.
