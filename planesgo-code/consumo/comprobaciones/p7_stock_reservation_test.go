package comprobacion

import (
	"errors"
	"testing"

	"almacen-demo/almacen"
)

func TestStockReservation(t *testing.T) {
	inv := almacen.NuevoInventario()
	inv.Entrada("AL-0001", 30)
	inv.Entrada("AL-0002", 10)
	p := almacen.NuevoPedido("P", "C-0001")
	p.Anade("AL-0001", 20, 0)
	p.Anade("AL-0002", 5, 0)
	if err := p.CambiaEstado("CONFIRMADO", nil); err != nil {
		t.Fatal(err)
	}
	// AL-0002 has only 3 units free (10 minus the safety margin): nothing is reserved and the state stays.
	var sin *almacen.SinStock
	if err := p.CambiaEstado("EN_PREPARACION", inv); !errors.As(err, &sin) {
		t.Fatalf("sin stock de AL-0002 se esperaba SinStock: %v", err)
	}
	if p.Estado != "CONFIRMADO" || inv.Reservado["AL-0001"] != 0 {
		t.Fatalf("tras el error: estado %s, reservado %v; se esperaba CONFIRMADO y nada reservado", p.Estado, inv.Reservado)
	}
	inv.Entrada("AL-0002", 10)
	if err := p.CambiaEstado("EN_PREPARACION", inv); err != nil {
		t.Fatal(err)
	}
	if p.Estado != "EN_PREPARACION" || inv.Reservado["AL-0001"] != 20 || inv.Reservado["AL-0002"] != 5 {
		t.Errorf("estado %s, reservado %v; se esperaba EN_PREPARACION con 20 y 5 reservados", p.Estado, inv.Reservado)
	}
}
