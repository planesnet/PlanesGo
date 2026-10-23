package main

import "testing"

func TestFinishedLine(t *testing.T) {
	for _, c := range []struct {
		estado string
		codigo int
		want   string
	}{
		{"terminado", 0, "Ha terminado (código de salida 0). Mira su salida con: pgo salida web"},
		{"error", 2, "Ha fallado (código de salida 2). Mira su salida con: pgo salida web"},
		{"error", -1, "Ha fallado (sin código de salida: no arrancó o lo ha matado una señal). Mira su salida con: pgo salida web"},
		{"parado", -1, "Se ha parado. Mira su salida con: pgo salida web"},
	} {
		if got := finishedLine("web", c.estado, c.codigo); got != c.want {
			t.Errorf("%s %d: %q", c.estado, c.codigo, got)
		}
	}
}
