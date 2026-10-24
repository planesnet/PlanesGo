// Package impuestos calcula los impuestos de las líneas de factura: IVA, recargo de equivalencia e IRPF.
package impuestos

import "facturacion-demo/facturacion"

// Exenta dice si la operación va sin IVA: exportaciones y entregas intracomunitarias a empresas.
func Exenta(linea facturacion.Linea, cliente *facturacion.Cliente) bool {
	if cliente == nil {
		return false
	}
	return !cliente.EsNacional()
}

// CuotaIVA es la cuota de IVA de una base con un porcentaje dado.
func CuotaIVA(base, porcentaje float64) float64 {
	return facturacion.Redondea(base*porcentaje/100, 2)
}

// IVALinea es el importe del IVA de una línea de factura.
func IVALinea(linea facturacion.Linea, cliente *facturacion.Cliente) float64 {
	if Exenta(linea, cliente) {
		return 0
	}
	return CuotaIVA(linea.Subtotal(), linea.PorcentajeIVA())
}

// Desglose son las bases y cuotas agrupadas por porcentaje de IVA: porcentaje -> {base, cuota}.
func Desglose(lineas []facturacion.Linea, cliente *facturacion.Cliente) map[float64][2]float64 {
	grupos := map[float64][2]float64{}
	for _, linea := range lineas {
		p := linea.PorcentajeIVA()
		if Exenta(linea, cliente) {
			p = 0
		}
		g := grupos[p]
		grupos[p] = [2]float64{g[0] + linea.Subtotal(), g[1] + IVALinea(linea, cliente)}
	}
	for p, g := range grupos {
		grupos[p] = [2]float64{facturacion.Redondea(g[0], 2), facturacion.Redondea(g[1], 2)}
	}
	return grupos
}
