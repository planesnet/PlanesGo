package factura

import (
	"time"

	"facturacion-demo/facturacion"
	"facturacion-demo/facturacion/impuestos"
)

// Factura es una factura emitida, para los informes.
type Factura struct {
	Fecha   time.Time
	Lineas  []facturacion.Linea
	Cliente *facturacion.Cliente
}

// ResumenTrimestral es la base y la cuota de IVA por trimestre (modelo 303): trimestre -> {base, cuota}.
func ResumenTrimestral(facturas []Factura) map[int][2]float64 {
	r := map[int][2]float64{}
	for _, f := range facturas {
		q := facturacion.Trimestre(f.Fecha)
		for _, g := range impuestos.Desglose(f.Lineas, f.Cliente) {
			r[q] = [2]float64{r[q][0] + g[0], r[q][1] + g[1]}
		}
	}
	return r
}
