package tests

import (
	"testing"

	"almacen-demo/almacen"
)

func compruebaDescuentos(t *testing.T, casos map[float64]int) {
	t.Helper()
	for importe, want := range casos {
		if got := almacen.DescuentoVolumen(importe); got != want {
			t.Errorf("descuento de %v: %d, quería %d", importe, got, want)
		}
	}
}

func TestTramos(t *testing.T) {
	compruebaDescuentos(t, map[float64]int{199.99: 0, 350: 2, 2000: 10})
}

func TestLimites(t *testing.T) {
	compruebaDescuentos(t, map[float64]int{200: 2, 500: 5, 1000: 10})
}

func TestAplica(t *testing.T) {
	if got := almacen.AplicaDescuento(500); got != 475 {
		t.Errorf("con descuento: %v, quería 475", got)
	}
}
