package comprobacion

import (
	"math"
	"testing"

	"facturacion-demo/facturacion"
)

func TestRoundMode(t *testing.T) {
	for _, c := range []struct {
		got, want float64
		call      string
	}{
		{facturacion.Redondea(0.105, 2), 0.11, `Redondea(0.105, 2)`},
		{facturacion.Redondea(0.105, 2, "mitad"), 0.11, `Redondea(0.105, 2, "mitad")`},
		{facturacion.Redondea(0.105, 2, "abajo"), 0.1, `Redondea(0.105, 2, "abajo")`},
		{facturacion.Redondea(2.999, 2, "abajo"), 2.99, `Redondea(2.999, 2, "abajo")`},
	} {
		if math.Abs(c.got-c.want) > 1e-9 {
			t.Errorf("%s = %v, se esperaba %v", c.call, c.got, c.want)
		}
	}
}
