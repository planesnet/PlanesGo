package tests

import (
	"errors"
	"testing"

	"almacen-demo/almacen"
)

func TestReserva(t *testing.T) {
	inv := almacen.NuevoInventario()
	inv.Entrada("AL-0001", 20)
	if err := inv.ReservaStock("AL-0001", 20-almacen.MargenSeguridad); err != nil {
		t.Fatal(err)
	}
	if got := inv.Disponible("AL-0001"); got != 0 {
		t.Errorf("disponible: %d, quería 0", got)
	}
}

func TestSinStock(t *testing.T) {
	inv := almacen.NuevoInventario()
	inv.Entrada("AL-0002", 5)
	var sin *almacen.SinStock
	if err := inv.ReservaStock("AL-0002", 1); !errors.As(err, &sin) {
		t.Errorf("quería SinStock: %v", err)
	}
}
