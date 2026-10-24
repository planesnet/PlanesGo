package comprobacion

import (
	"math"
	"testing"

	"facturacion-demo/facturacion"
	"facturacion-demo/facturacion/impuestos"
)

func TestVATRoundDown(t *testing.T) {
	if got := impuestos.CuotaIVA(0.5, 21); math.Abs(got-0.1) > 1e-9 {
		t.Errorf("CuotaIVA(0.5, 21) = %v, se esperaba 0.1 (redondeo hacia abajo)", got)
	}
	if got := facturacion.Redondea(0.105, 2); math.Abs(got-0.11) > 1e-9 {
		t.Errorf("Redondea(0.105, 2) = %v, se esperaba 0.11 (el redondeo por defecto no cambia)", got)
	}
}
