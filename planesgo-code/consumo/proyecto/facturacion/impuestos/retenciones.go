package impuestos

import "facturacion-demo/facturacion"

// RetencionIRPF es la retención de IRPF (profesionales): no es un impuesto de la línea, se resta del total.
func RetencionIRPF(base float64, cliente *facturacion.Cliente) float64 {
	if cliente == nil || cliente.RetencionIRPF == 0 {
		return 0
	}
	return facturacion.Redondea(base*cliente.RetencionIRPF/100, 2)
}
