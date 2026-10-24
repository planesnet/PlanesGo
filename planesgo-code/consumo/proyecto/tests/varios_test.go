package tests

import (
	"testing"
	"time"

	"facturacion-demo/facturacion"
)

func TestNumeracion(t *testing.T) {
	if got := facturacion.Siguiente("F2026-00041", "F", 2026); got != "F2026-00042" {
		t.Errorf("siguiente: %s", got)
	}
	if got := facturacion.Siguiente("F2025-00041", "F", 2026); got != "F2026-00001" {
		t.Errorf("siguiente de otro año: %s", got)
	}
}

func TestNIF(t *testing.T) {
	if !facturacion.NIFValido("12345678Z") || facturacion.NIFValido("12345678A") {
		t.Error("NIF")
	}
}

func TestFechas(t *testing.T) {
	if got := facturacion.Trimestre(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)); got != 2 {
		t.Errorf("trimestre: %d", got)
	}
	want := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	if got := facturacion.Vencimiento(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), 30); !got.Equal(want) {
		t.Errorf("vencimiento: %s", got)
	}
}
