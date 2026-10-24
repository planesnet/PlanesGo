package comprobacion

import (
	"math"
	"testing"

	"almacen-demo/almacen"
)

func TestUrgentShipping(t *testing.T) {
	for _, c := range []struct {
		peso      float64
		zona      int
		surcharge float64
	}{{4, 3, 5.25}, {50, 2, 4.0}} {
		normal, urgent := almacen.CosteEnvio(c.peso, c.zona, false), almacen.CosteEnvio(c.peso, c.zona, true)
		if math.Abs(urgent-(normal+c.surcharge)) > 0.005 {
			t.Errorf("%v kg a la zona %d: urgente %v, normal %v; se esperaba %v más", c.peso, c.zona, urgent, normal, c.surcharge)
		}
	}
	if got := almacen.CosteEnvio(0.5, 1, false); math.Abs(got-3.9) > 0.005 {
		t.Errorf("CosteEnvio(0.5, 1, false) = %v, se esperaba 3.9", got)
	}
}
