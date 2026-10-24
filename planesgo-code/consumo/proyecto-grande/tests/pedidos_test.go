package tests

import (
	"errors"
	"testing"

	"almacen-demo/almacen"
)

func TestImporte(t *testing.T) {
	p := almacen.NuevoPedido("P-1", "C-0001")
	p.Anade("AL-0001", 10, 0.5)
	p.Anade("AL-0002", 2, 3.25)
	if got := p.Importe(); got != 11.5 {
		t.Errorf("importe: %v, quería 11.5", got)
	}
}

func TestTransicionInvalida(t *testing.T) {
	p := almacen.NuevoPedido("P-2", "C-0001")
	var inv *almacen.TransicionInvalida
	if err := p.CambiaEstado("ENVIADO", nil); !errors.As(err, &inv) {
		t.Errorf("quería TransicionInvalida: %v", err)
	}
}
