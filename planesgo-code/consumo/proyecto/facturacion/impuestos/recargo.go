package impuestos

import "facturacion-demo/facturacion"

// Recargos de equivalencia (comerciantes minoristas) por tipo de IVA: se suman al IVA.
var Recargos = map[float64]float64{21: 5.2, 10: 1.4, 4: 0.5, 0: 0}

// RecargoEquivalencia es el recargo de una base con su tipo de IVA, si el cliente está en el régimen.
func RecargoEquivalencia(base, porcentajeIVA float64, cliente *facturacion.Cliente) float64 {
	if cliente == nil || !cliente.RecargoEquivalencia {
		return 0
	}
	return facturacion.Redondea(base*Recargos[porcentajeIVA]/100, 2)
}
