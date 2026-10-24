// Package factura junta las líneas de una factura: totales, exportación e informes.
package factura

import (
	"facturacion-demo/facturacion"
	"facturacion-demo/facturacion/impuestos"
)

// Totales son los importes de una factura.
type Totales struct {
	Base, IVA, Recargo, IRPF, Total float64
}

// CalculaTotales suma las líneas de una factura para un cliente (nil: sin cliente).
func CalculaTotales(lineas []facturacion.Linea, cliente *facturacion.Cliente) Totales {
	var base, iva, recargo float64
	for _, l := range lineas {
		base += l.Subtotal()
		iva += impuestos.IVALinea(l, cliente)
		recargo += impuestos.RecargoEquivalencia(l.Subtotal(), l.PorcentajeIVA(), cliente)
	}
	t := Totales{Base: facturacion.Redondea(base, 2), IVA: facturacion.Redondea(iva, 2), Recargo: facturacion.Redondea(recargo, 2)}
	t.IRPF = impuestos.RetencionIRPF(t.Base, cliente)
	t.Total = facturacion.Redondea(t.Base+t.IVA+t.Recargo-t.IRPF, 2)
	return t
}
