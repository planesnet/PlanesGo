package tests

import (
	"testing"

	"facturacion-demo/facturacion"
	"facturacion-demo/facturacion/impuestos"
)

var (
	libro = facturacion.Producto{Codigo: "L1", Nombre: "Libro", Precio: 20, TipoIVA: "superreducido"}
	silla = facturacion.Producto{Codigo: "S1", Nombre: "Silla", Precio: 50}
)

func TestIVAGeneral(t *testing.T) {
	if got := impuestos.IVALinea(facturacion.Linea{Producto: silla, Cantidad: 2}, nil); got != 21 {
		t.Errorf("IVA: %v, quería 21", got)
	}
}

func TestIVASuperreducido(t *testing.T) {
	if got := impuestos.IVALinea(facturacion.Linea{Producto: libro, Cantidad: 1}, nil); got != 0.8 {
		t.Errorf("IVA: %v, quería 0.8", got)
	}
}

func TestExportacionExenta(t *testing.T) {
	c := &facturacion.Cliente{NIF: "X", Nombre: "GmbH", Pais: "DE"}
	if got := impuestos.IVALinea(facturacion.Linea{Producto: silla, Cantidad: 1}, c); got != 0 {
		t.Errorf("IVA: %v, quería 0", got)
	}
}

func TestIRPF(t *testing.T) {
	if got := impuestos.RetencionIRPF(1000, &facturacion.Cliente{NIF: "1", Nombre: "Prof", RetencionIRPF: 15}); got != 150 {
		t.Errorf("IRPF: %v, quería 150", got)
	}
}

func TestRecargo(t *testing.T) {
	c := &facturacion.Cliente{NIF: "1", Nombre: "Tienda", RecargoEquivalencia: true}
	if got := impuestos.RecargoEquivalencia(100, 21, c); got != 5.2 {
		t.Errorf("recargo: %v, quería 5.2", got)
	}
}
