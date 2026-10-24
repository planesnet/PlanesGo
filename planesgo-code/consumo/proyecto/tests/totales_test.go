package tests

import (
	"testing"

	"facturacion-demo/facturacion"
	"facturacion-demo/facturacion/factura"
)

func TestFacturaSimple(t *testing.T) {
	got := factura.CalculaTotales([]facturacion.Linea{{Producto: facturacion.Producto{Codigo: "S1", Nombre: "Silla", Precio: 50}, Cantidad: 2}}, nil)
	if want := (factura.Totales{Base: 100, IVA: 21, Recargo: 0, IRPF: 0, Total: 121}); got != want {
		t.Errorf("totales: %+v, quería %+v", got, want)
	}
}

func TestProfesional(t *testing.T) {
	c := &facturacion.Cliente{NIF: "1", Nombre: "Prof", RetencionIRPF: 15}
	got := factura.CalculaTotales([]facturacion.Linea{{Producto: facturacion.Producto{Codigo: "H", Nombre: "Hora", Precio: 40}, Cantidad: 10}}, c)
	if got.Total != 424 {
		t.Errorf("total: %v, quería 424", got.Total)
	}
}
