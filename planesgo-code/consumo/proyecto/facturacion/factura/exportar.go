package factura

import (
	"encoding/csv"
	"strconv"
	"strings"

	"facturacion-demo/facturacion"
)

func num(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// ACSV exporta las líneas y los totales (base, IVA, recargo, IRPF) a CSV.
func ACSV(lineas []facturacion.Linea, cliente *facturacion.Cliente) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	w.Comma = ';'
	w.Write([]string{"codigo", "producto", "cantidad", "descuento", "base", "iva_%"})
	for _, l := range lineas {
		w.Write([]string{l.Producto.Codigo, l.Producto.Nombre, num(l.Cantidad), num(l.Descuento), num(l.Subtotal()), num(l.PorcentajeIVA())})
	}
	t := CalculaTotales(lineas, cliente)
	for _, f := range []struct {
		nombre string
		valor  float64
	}{{"base", t.Base}, {"iva", t.IVA}, {"recargo", t.Recargo}, {"irpf", t.IRPF}, {"total", t.Total}} {
		w.Write([]string{f.nombre, "", "", "", num(f.valor), ""})
	}
	w.Flush()
	return b.String()
}
