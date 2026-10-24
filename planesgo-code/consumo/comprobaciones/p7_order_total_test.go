package comprobacion

import (
	"math"
	"testing"

	"almacen-demo/almacen"
)

func TestOrderTotal(t *testing.T) {
	p := almacen.NuevoPedido("P", "C-0001")
	if err := p.Anade("AL-0001", 1000, 0.5); err != nil {
		t.Fatal(err)
	}
	// 500 € with 5 % off, plus 32 kg to zone 1 (13.45 €); urgent adds 3.50 €.
	for _, c := range []struct {
		urgent bool
		want   float64
	}{{false, 488.45}, {true, 491.95}} {
		p.Urgente = c.urgent
		got, err := almacen.TotalPedido(p, "28013")
		if err != nil || math.Abs(got-c.want) > 0.005 {
			t.Errorf("TotalPedido (urgente %v) = %v %v, se esperaba %v", c.urgent, got, err, c.want)
		}
	}
}
