package comprobacion

import (
	"math"
	"testing"

	"almacen-demo/almacen"
)

func TestRefund(t *testing.T) {
	for motivo, want := range map[string]float64{"desistimiento": 100, "defectuoso": 107.5} {
		if got, err := almacen.Reembolso(100, 7.5, motivo); err != nil || math.Abs(got-want) > 0.005 {
			t.Errorf("Reembolso(100, 7.5, %q) = %v %v, se esperaba %v", motivo, got, err, want)
		}
	}
}
