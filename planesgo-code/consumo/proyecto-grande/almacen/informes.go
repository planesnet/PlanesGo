package almacen

// Informes del almacén: listados de stock, ventas por familia y por cliente, y textos para imprimir.

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Ancho es el ancho de los informes en caracteres.
const Ancho = 78

var anchosFila = []int{12, 40, 12, 12}

// cabecera es el título en mayúsculas, centrado entre signos «=».
func cabecera(titulo string) string {
	t := strings.ToUpper(titulo)
	falta := Ancho - utf8.RuneCountInString(t)
	if falta <= 0 {
		return t
	}
	return strings.Repeat("=", falta/2) + t + strings.Repeat("=", falta-falta/2)
}

// fila son las columnas recortadas y rellenadas a su ancho.
func fila(columnas ...any) string {
	var b strings.Builder
	for i, c := range columnas {
		if i >= len(anchosFila) {
			break
		}
		s := []rune(fmt.Sprint(c))
		if len(s) > anchosFila[i] {
			s = s[:anchosFila[i]]
		}
		b.WriteString(string(s) + strings.Repeat(" ", anchosFila[i]-len(s)))
	}
	return b.String()
}

// InformeStockTornilleria son las existencias de la familia tornilleria, una fila por producto.
func InformeStockTornilleria(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Existencias · tornilleria")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "tornilleria" {
			continue
		}
		n := inv.Existencias[sku]
		lineas = append(lineas, fila(sku, p.Nombre, n, p.Ubicacion))
		total += float64(n)
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeVentasTornilleria son las ventas de la familia tornilleria, una fila por producto.
func InformeVentasTornilleria(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Ventas · tornilleria")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "tornilleria" {
			continue
		}
		v := ventas[sku]
		lineas = append(lineas, fila(sku, p.Nombre, v, redondea2(float64(v)*p.Precio)))
		total += float64(v) * p.Precio
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeRoturaTornilleria son los productos bajo mínimo de la familia tornilleria, una fila por producto.
func InformeRoturaTornilleria(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Productos bajo mínimo · tornilleria")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "tornilleria" {
			continue
		}
		n := inv.Existencias[sku]
		if n >= p.StockMinimo {
			continue
		}
		lineas = append(lineas, fila(sku, p.Nombre, n, p.StockMinimo))
		total += float64(p.StockMinimo - n)
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeStockElectricidad son las existencias de la familia electricidad, una fila por producto.
func InformeStockElectricidad(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Existencias · electricidad")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "electricidad" {
			continue
		}
		n := inv.Existencias[sku]
		lineas = append(lineas, fila(sku, p.Nombre, n, p.Ubicacion))
		total += float64(n)
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeVentasElectricidad son las ventas de la familia electricidad, una fila por producto.
func InformeVentasElectricidad(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Ventas · electricidad")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "electricidad" {
			continue
		}
		v := ventas[sku]
		lineas = append(lineas, fila(sku, p.Nombre, v, redondea2(float64(v)*p.Precio)))
		total += float64(v) * p.Precio
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeRoturaElectricidad son los productos bajo mínimo de la familia electricidad, una fila por producto.
func InformeRoturaElectricidad(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Productos bajo mínimo · electricidad")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "electricidad" {
			continue
		}
		n := inv.Existencias[sku]
		if n >= p.StockMinimo {
			continue
		}
		lineas = append(lineas, fila(sku, p.Nombre, n, p.StockMinimo))
		total += float64(p.StockMinimo - n)
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeStockFontaneria son las existencias de la familia fontaneria, una fila por producto.
func InformeStockFontaneria(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Existencias · fontaneria")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "fontaneria" {
			continue
		}
		n := inv.Existencias[sku]
		lineas = append(lineas, fila(sku, p.Nombre, n, p.Ubicacion))
		total += float64(n)
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeVentasFontaneria son las ventas de la familia fontaneria, una fila por producto.
func InformeVentasFontaneria(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Ventas · fontaneria")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "fontaneria" {
			continue
		}
		v := ventas[sku]
		lineas = append(lineas, fila(sku, p.Nombre, v, redondea2(float64(v)*p.Precio)))
		total += float64(v) * p.Precio
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeRoturaFontaneria son los productos bajo mínimo de la familia fontaneria, una fila por producto.
func InformeRoturaFontaneria(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Productos bajo mínimo · fontaneria")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "fontaneria" {
			continue
		}
		n := inv.Existencias[sku]
		if n >= p.StockMinimo {
			continue
		}
		lineas = append(lineas, fila(sku, p.Nombre, n, p.StockMinimo))
		total += float64(p.StockMinimo - n)
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeStockPintura son las existencias de la familia pintura, una fila por producto.
func InformeStockPintura(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Existencias · pintura")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "pintura" {
			continue
		}
		n := inv.Existencias[sku]
		lineas = append(lineas, fila(sku, p.Nombre, n, p.Ubicacion))
		total += float64(n)
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeVentasPintura son las ventas de la familia pintura, una fila por producto.
func InformeVentasPintura(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Ventas · pintura")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "pintura" {
			continue
		}
		v := ventas[sku]
		lineas = append(lineas, fila(sku, p.Nombre, v, redondea2(float64(v)*p.Precio)))
		total += float64(v) * p.Precio
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeRoturaPintura son los productos bajo mínimo de la familia pintura, una fila por producto.
func InformeRoturaPintura(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Productos bajo mínimo · pintura")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "pintura" {
			continue
		}
		n := inv.Existencias[sku]
		if n >= p.StockMinimo {
			continue
		}
		lineas = append(lineas, fila(sku, p.Nombre, n, p.StockMinimo))
		total += float64(p.StockMinimo - n)
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeStockHerramienta son las existencias de la familia herramienta, una fila por producto.
func InformeStockHerramienta(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Existencias · herramienta")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "herramienta" {
			continue
		}
		n := inv.Existencias[sku]
		lineas = append(lineas, fila(sku, p.Nombre, n, p.Ubicacion))
		total += float64(n)
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeVentasHerramienta son las ventas de la familia herramienta, una fila por producto.
func InformeVentasHerramienta(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Ventas · herramienta")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "herramienta" {
			continue
		}
		v := ventas[sku]
		lineas = append(lineas, fila(sku, p.Nombre, v, redondea2(float64(v)*p.Precio)))
		total += float64(v) * p.Precio
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeRoturaHerramienta son los productos bajo mínimo de la familia herramienta, una fila por producto.
func InformeRoturaHerramienta(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Productos bajo mínimo · herramienta")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "herramienta" {
			continue
		}
		n := inv.Existencias[sku]
		if n >= p.StockMinimo {
			continue
		}
		lineas = append(lineas, fila(sku, p.Nombre, n, p.StockMinimo))
		total += float64(p.StockMinimo - n)
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeStockJardin son las existencias de la familia jardin, una fila por producto.
func InformeStockJardin(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Existencias · jardin")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "jardin" {
			continue
		}
		n := inv.Existencias[sku]
		lineas = append(lineas, fila(sku, p.Nombre, n, p.Ubicacion))
		total += float64(n)
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeVentasJardin son las ventas de la familia jardin, una fila por producto.
func InformeVentasJardin(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Ventas · jardin")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "jardin" {
			continue
		}
		v := ventas[sku]
		lineas = append(lineas, fila(sku, p.Nombre, v, redondea2(float64(v)*p.Precio)))
		total += float64(v) * p.Precio
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// InformeRoturaJardin son los productos bajo mínimo de la familia jardin, una fila por producto.
func InformeRoturaJardin(inv *Inventario, ventas map[string]int) string {
	lineas := []string{cabecera("Productos bajo mínimo · jardin")}
	total := 0.0
	for _, sku := range skus() {
		p := Catalogo[sku]
		if p.Familia != "jardin" {
			continue
		}
		n := inv.Existencias[sku]
		if n >= p.StockMinimo {
			continue
		}
		lineas = append(lineas, fila(sku, p.Nombre, n, p.StockMinimo))
		total += float64(p.StockMinimo - n)
	}
	lineas = append(lineas, strings.Repeat("-", Ancho), fila("", "Total", "", redondea2(total)))
	return strings.Join(lineas, "\n")
}

// Pies son los textos de los informes por cliente.
var Pies = map[string]string{
	"C-0001": "Condiciones: 60 días. Entregar en Santa Cruz de Tenerife (CP 38780). Grupo de tarifa: general.",
	"C-0002": "Condiciones: 30 días. Entregar en Baleares (CP 07252). Grupo de tarifa: general.",
	"C-0003": "Condiciones: contado. Entregar en Alicante (CP 03972). Grupo de tarifa: mayorista.",
	"C-0004": "Condiciones: contado. Entregar en Baleares (CP 07135). Grupo de tarifa: mayorista.",
	"C-0005": "Condiciones: transferencia 15 días. Entregar en Huelva (CP 21344). Grupo de tarifa: profesional.",
	"C-0006": "Condiciones: 60 días. Entregar en Girona (CP 17952). Grupo de tarifa: general.",
	"C-0007": "Condiciones: transferencia 15 días. Entregar en Zaragoza (CP 50987). Grupo de tarifa: profesional.",
	"C-0008": "Condiciones: contado. Entregar en Albacete (CP 02446). Grupo de tarifa: profesional.",
	"C-0009": "Condiciones: 30 días. Entregar en Las Palmas (CP 35579). Grupo de tarifa: mayorista.",
	"C-0010": "Condiciones: contado. Entregar en Cádiz (CP 11446). Grupo de tarifa: mayorista.",
	"C-0011": "Condiciones: contado. Entregar en Zamora (CP 49768). Grupo de tarifa: general.",
	"C-0012": "Condiciones: 30 días. Entregar en Baleares (CP 07503). Grupo de tarifa: profesional.",
	"C-0013": "Condiciones: 30 días. Entregar en Girona (CP 17591). Grupo de tarifa: profesional.",
	"C-0014": "Condiciones: contado. Entregar en Ourense (CP 32169). Grupo de tarifa: mayorista.",
	"C-0015": "Condiciones: contado. Entregar en Ceuta (CP 51713). Grupo de tarifa: mayorista.",
	"C-0016": "Condiciones: transferencia 15 días. Entregar en Baleares (CP 07410). Grupo de tarifa: mayorista.",
	"C-0017": "Condiciones: 60 días. Entregar en Soria (CP 42025). Grupo de tarifa: general.",
	"C-0018": "Condiciones: 30 días. Entregar en Madrid (CP 28922). Grupo de tarifa: profesional.",
	"C-0019": "Condiciones: contado. Entregar en Burgos (CP 09544). Grupo de tarifa: profesional.",
	"C-0020": "Condiciones: transferencia 15 días. Entregar en Cáceres (CP 10888). Grupo de tarifa: mayorista.",
	"C-0021": "Condiciones: 60 días. Entregar en Murcia (CP 30449). Grupo de tarifa: mayorista.",
	"C-0022": "Condiciones: 30 días. Entregar en Huesca (CP 22473). Grupo de tarifa: mayorista.",
	"C-0023": "Condiciones: 30 días. Entregar en Gipuzkoa (CP 20772). Grupo de tarifa: mayorista.",
	"C-0024": "Condiciones: 60 días. Entregar en Valladolid (CP 47334). Grupo de tarifa: general.",
	"C-0025": "Condiciones: contado. Entregar en Ciudad Real (CP 13264). Grupo de tarifa: general.",
	"C-0026": "Condiciones: 30 días. Entregar en Lleida (CP 25154). Grupo de tarifa: profesional.",
	"C-0027": "Condiciones: contado. Entregar en Granada (CP 18200). Grupo de tarifa: mayorista.",
	"C-0028": "Condiciones: transferencia 15 días. Entregar en Córdoba (CP 14906). Grupo de tarifa: profesional.",
	"C-0029": "Condiciones: transferencia 15 días. Entregar en La Rioja (CP 26874). Grupo de tarifa: mayorista.",
	"C-0030": "Condiciones: transferencia 15 días. Entregar en Sevilla (CP 41303). Grupo de tarifa: general.",
	"C-0031": "Condiciones: transferencia 15 días. Entregar en Cantabria (CP 39755). Grupo de tarifa: general.",
	"C-0032": "Condiciones: transferencia 15 días. Entregar en Toledo (CP 45587). Grupo de tarifa: general.",
	"C-0033": "Condiciones: transferencia 15 días. Entregar en Soria (CP 42127). Grupo de tarifa: profesional.",
	"C-0034": "Condiciones: contado. Entregar en Sevilla (CP 41717). Grupo de tarifa: profesional.",
	"C-0035": "Condiciones: 30 días. Entregar en Valencia (CP 46729). Grupo de tarifa: profesional.",
	"C-0036": "Condiciones: transferencia 15 días. Entregar en Murcia (CP 30020). Grupo de tarifa: mayorista.",
	"C-0037": "Condiciones: transferencia 15 días. Entregar en Zaragoza (CP 50010). Grupo de tarifa: profesional.",
	"C-0038": "Condiciones: 30 días. Entregar en Girona (CP 17556). Grupo de tarifa: general.",
	"C-0039": "Condiciones: transferencia 15 días. Entregar en Jaén (CP 23103). Grupo de tarifa: mayorista.",
	"C-0040": "Condiciones: 60 días. Entregar en Asturias (CP 33016). Grupo de tarifa: mayorista.",
	"C-0041": "Condiciones: transferencia 15 días. Entregar en Bizkaia (CP 48970). Grupo de tarifa: general.",
	"C-0042": "Condiciones: contado. Entregar en Asturias (CP 33781). Grupo de tarifa: mayorista.",
	"C-0043": "Condiciones: transferencia 15 días. Entregar en Girona (CP 17280). Grupo de tarifa: profesional.",
	"C-0044": "Condiciones: transferencia 15 días. Entregar en Ávila (CP 05428). Grupo de tarifa: mayorista.",
	"C-0045": "Condiciones: 60 días. Entregar en Baleares (CP 07229). Grupo de tarifa: mayorista.",
	"C-0046": "Condiciones: transferencia 15 días. Entregar en A Coruña (CP 15820). Grupo de tarifa: profesional.",
	"C-0047": "Condiciones: contado. Entregar en Burgos (CP 09951). Grupo de tarifa: mayorista.",
	"C-0048": "Condiciones: 30 días. Entregar en Soria (CP 42575). Grupo de tarifa: general.",
	"C-0049": "Condiciones: 30 días. Entregar en Murcia (CP 30301). Grupo de tarifa: profesional.",
	"C-0050": "Condiciones: transferencia 15 días. Entregar en Granada (CP 18721). Grupo de tarifa: mayorista.",
	"C-0051": "Condiciones: transferencia 15 días. Entregar en Teruel (CP 44190). Grupo de tarifa: general.",
	"C-0052": "Condiciones: 60 días. Entregar en Cuenca (CP 16670). Grupo de tarifa: profesional.",
	"C-0053": "Condiciones: contado. Entregar en Madrid (CP 28638). Grupo de tarifa: mayorista.",
	"C-0054": "Condiciones: transferencia 15 días. Entregar en Gipuzkoa (CP 20874). Grupo de tarifa: general.",
	"C-0055": "Condiciones: 30 días. Entregar en Ceuta (CP 51965). Grupo de tarifa: mayorista.",
	"C-0056": "Condiciones: 30 días. Entregar en Tarragona (CP 43011). Grupo de tarifa: general.",
	"C-0057": "Condiciones: 30 días. Entregar en Cantabria (CP 39103). Grupo de tarifa: general.",
	"C-0058": "Condiciones: 30 días. Entregar en Jaén (CP 23803). Grupo de tarifa: general.",
	"C-0059": "Condiciones: contado. Entregar en Segovia (CP 40912). Grupo de tarifa: mayorista.",
	"C-0060": "Condiciones: 30 días. Entregar en Ourense (CP 32709). Grupo de tarifa: mayorista.",
	"C-0061": "Condiciones: contado. Entregar en Tarragona (CP 43903). Grupo de tarifa: mayorista.",
	"C-0062": "Condiciones: 30 días. Entregar en Lugo (CP 27239). Grupo de tarifa: profesional.",
	"C-0063": "Condiciones: 30 días. Entregar en Navarra (CP 31478). Grupo de tarifa: mayorista.",
	"C-0064": "Condiciones: contado. Entregar en Ourense (CP 32168). Grupo de tarifa: general.",
	"C-0065": "Condiciones: transferencia 15 días. Entregar en Toledo (CP 45576). Grupo de tarifa: mayorista.",
	"C-0066": "Condiciones: transferencia 15 días. Entregar en León (CP 24436). Grupo de tarifa: mayorista.",
	"C-0067": "Condiciones: contado. Entregar en Sevilla (CP 41369). Grupo de tarifa: general.",
	"C-0068": "Condiciones: transferencia 15 días. Entregar en Melilla (CP 52096). Grupo de tarifa: profesional.",
	"C-0069": "Condiciones: transferencia 15 días. Entregar en Córdoba (CP 14735). Grupo de tarifa: mayorista.",
	"C-0070": "Condiciones: 60 días. Entregar en Baleares (CP 07882). Grupo de tarifa: profesional.",
	"C-0071": "Condiciones: 30 días. Entregar en Pontevedra (CP 36789). Grupo de tarifa: profesional.",
	"C-0072": "Condiciones: contado. Entregar en Madrid (CP 28257). Grupo de tarifa: profesional.",
	"C-0073": "Condiciones: 60 días. Entregar en Ourense (CP 32413). Grupo de tarifa: mayorista.",
	"C-0074": "Condiciones: 30 días. Entregar en Jaén (CP 23998). Grupo de tarifa: mayorista.",
	"C-0075": "Condiciones: 60 días. Entregar en Huesca (CP 22196). Grupo de tarifa: mayorista.",
	"C-0076": "Condiciones: contado. Entregar en Santa Cruz de Tenerife (CP 38996). Grupo de tarifa: general.",
	"C-0077": "Condiciones: contado. Entregar en Las Palmas (CP 35587). Grupo de tarifa: profesional.",
	"C-0078": "Condiciones: 30 días. Entregar en Álava (CP 01047). Grupo de tarifa: profesional.",
	"C-0079": "Condiciones: transferencia 15 días. Entregar en Las Palmas (CP 35626). Grupo de tarifa: mayorista.",
	"C-0080": "Condiciones: transferencia 15 días. Entregar en Córdoba (CP 14040). Grupo de tarifa: mayorista.",
	"C-0081": "Condiciones: contado. Entregar en Tarragona (CP 43185). Grupo de tarifa: profesional.",
	"C-0082": "Condiciones: 30 días. Entregar en León (CP 24892). Grupo de tarifa: profesional.",
	"C-0083": "Condiciones: contado. Entregar en Castellón (CP 12431). Grupo de tarifa: profesional.",
	"C-0084": "Condiciones: contado. Entregar en Salamanca (CP 37657). Grupo de tarifa: profesional.",
	"C-0085": "Condiciones: transferencia 15 días. Entregar en Zaragoza (CP 50829). Grupo de tarifa: mayorista.",
	"C-0086": "Condiciones: transferencia 15 días. Entregar en Ávila (CP 05014). Grupo de tarifa: mayorista.",
	"C-0087": "Condiciones: contado. Entregar en Zaragoza (CP 50422). Grupo de tarifa: general.",
	"C-0088": "Condiciones: contado. Entregar en Cáceres (CP 10641). Grupo de tarifa: profesional.",
	"C-0089": "Condiciones: contado. Entregar en Teruel (CP 44685). Grupo de tarifa: general.",
	"C-0090": "Condiciones: contado. Entregar en Burgos (CP 09483). Grupo de tarifa: profesional.",
	"C-0091": "Condiciones: 30 días. Entregar en Valladolid (CP 47762). Grupo de tarifa: general.",
	"C-0092": "Condiciones: contado. Entregar en Valladolid (CP 47777). Grupo de tarifa: profesional.",
	"C-0093": "Condiciones: 60 días. Entregar en Tarragona (CP 43954). Grupo de tarifa: general.",
	"C-0094": "Condiciones: contado. Entregar en Almería (CP 04015). Grupo de tarifa: profesional.",
	"C-0095": "Condiciones: 30 días. Entregar en Valladolid (CP 47614). Grupo de tarifa: profesional.",
	"C-0096": "Condiciones: transferencia 15 días. Entregar en León (CP 24971). Grupo de tarifa: profesional.",
	"C-0097": "Condiciones: 60 días. Entregar en Melilla (CP 52119). Grupo de tarifa: mayorista.",
	"C-0098": "Condiciones: transferencia 15 días. Entregar en Navarra (CP 31394). Grupo de tarifa: profesional.",
	"C-0099": "Condiciones: 60 días. Entregar en Granada (CP 18062). Grupo de tarifa: mayorista.",
	"C-0100": "Condiciones: 60 días. Entregar en Cantabria (CP 39852). Grupo de tarifa: mayorista.",
	"C-0101": "Condiciones: transferencia 15 días. Entregar en Lleida (CP 25396). Grupo de tarifa: mayorista.",
	"C-0102": "Condiciones: contado. Entregar en Guadalajara (CP 19705). Grupo de tarifa: profesional.",
	"C-0103": "Condiciones: contado. Entregar en Madrid (CP 28161). Grupo de tarifa: profesional.",
	"C-0104": "Condiciones: transferencia 15 días. Entregar en Granada (CP 18998). Grupo de tarifa: profesional.",
	"C-0105": "Condiciones: 30 días. Entregar en Melilla (CP 52390). Grupo de tarifa: mayorista.",
	"C-0106": "Condiciones: transferencia 15 días. Entregar en Cantabria (CP 39058). Grupo de tarifa: profesional.",
	"C-0107": "Condiciones: contado. Entregar en Santa Cruz de Tenerife (CP 38769). Grupo de tarifa: profesional.",
	"C-0108": "Condiciones: 60 días. Entregar en Las Palmas (CP 35825). Grupo de tarifa: general.",
	"C-0109": "Condiciones: 60 días. Entregar en Santa Cruz de Tenerife (CP 38533). Grupo de tarifa: mayorista.",
	"C-0110": "Condiciones: 30 días. Entregar en Asturias (CP 33603). Grupo de tarifa: general.",
	"C-0111": "Condiciones: 60 días. Entregar en Badajoz (CP 06185). Grupo de tarifa: profesional.",
	"C-0112": "Condiciones: 30 días. Entregar en Zaragoza (CP 50529). Grupo de tarifa: general.",
	"C-0113": "Condiciones: contado. Entregar en León (CP 24887). Grupo de tarifa: profesional.",
	"C-0114": "Condiciones: contado. Entregar en Cáceres (CP 10323). Grupo de tarifa: profesional.",
	"C-0115": "Condiciones: contado. Entregar en Cantabria (CP 39021). Grupo de tarifa: general.",
	"C-0116": "Condiciones: 30 días. Entregar en Santa Cruz de Tenerife (CP 38580). Grupo de tarifa: profesional.",
	"C-0117": "Condiciones: transferencia 15 días. Entregar en Baleares (CP 07969). Grupo de tarifa: mayorista.",
	"C-0118": "Condiciones: 30 días. Entregar en Alicante (CP 03346). Grupo de tarifa: general.",
	"C-0119": "Condiciones: contado. Entregar en Albacete (CP 02052). Grupo de tarifa: mayorista.",
	"C-0120": "Condiciones: contado. Entregar en Ourense (CP 32969). Grupo de tarifa: mayorista.",
}

// FichaCliente es la ficha de un cliente para imprimir.
func FichaCliente(codigo string) string {
	c := Clientes[codigo]
	return strings.Join([]string{cabecera(c.Nombre), "Código: " + c.Codigo, "Provincia: " + c.Provincia, Pies[codigo]}, "\n")
}
