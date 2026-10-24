package comprobacion

import (
	"math"
	"testing"

	"facturacion-demo/facturacion"
)

func TestFixedDiscount(t *testing.T) {
	p := facturacion.Producto{Codigo: "a", Nombre: "a", Precio: 10}
	if got := (facturacion.Linea{Producto: p, Cantidad: 2, Descuento: 10, DescuentoFijo: 3}).BaseImponible(); math.Abs(got-15) > 1e-9 {
		t.Errorf("10 € × 2, 10 %% y 3 € de descuento fijo: %v, se esperaba 15", got)
	}
	if got := (facturacion.Linea{Producto: p, Cantidad: 2}).BaseImponible(); math.Abs(got-20) > 1e-9 {
		t.Errorf("10 € × 2 sin descuentos: %v, se esperaba 20", got)
	}
}
