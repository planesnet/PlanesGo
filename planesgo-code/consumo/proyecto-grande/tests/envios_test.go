package tests

import (
	"testing"

	"almacen-demo/almacen"
)

func TestZona(t *testing.T) {
	for cp, want := range map[string]int{"28013": 1, "35001": 5} {
		if z, err := almacen.ZonaDe(cp); err != nil || z != want {
			t.Errorf("zona de %s: %d %v, quería %d", cp, z, err, want)
		}
	}
}

func TestCoste(t *testing.T) {
	if got := almacen.CosteEnvio(0.5, 1); got != 3.9 {
		t.Errorf("coste: %v, quería 3.9", got)
	}
}
