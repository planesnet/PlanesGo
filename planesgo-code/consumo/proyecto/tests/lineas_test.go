package tests

import (
	"testing"

	"facturacion-demo/facturacion"
)

var tornillo = facturacion.Producto{Codigo: "T1", Nombre: "Tornillo", Precio: 2.50}

func TestSinDescuento(t *testing.T) {
	if got := (facturacion.Linea{Producto: tornillo, Cantidad: 4}).Subtotal(); got != 10 {
		t.Errorf("subtotal: %v, quería 10", got)
	}
}

func TestDescuentoPorcentaje(t *testing.T) {
	if got := (facturacion.Linea{Producto: tornillo, Cantidad: 4, Descuento: 10}).Subtotal(); got != 9 {
		t.Errorf("subtotal: %v, quería 9", got)
	}
}

func TestDescuentoRedondeo(t *testing.T) {
	pieza := facturacion.Producto{Codigo: "P", Nombre: "Pieza", Precio: 3.33}
	if got := (facturacion.Linea{Producto: pieza, Cantidad: 3, Descuento: 15}).Subtotal(); got != 8.49 {
		t.Errorf("subtotal: %v, quería 8.49", got)
	}
}
