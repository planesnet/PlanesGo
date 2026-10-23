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

func TestDriveQuery(t *testing.T) {
	for _, c := range []struct {
		args  []string
		orden string
		want  string
	}{
		{[]string{"buscar", "informe", "trimestral", "--tipo", "doc"}, "buscar", "q=informe+trimestral&tipo=doc"},
		{[]string{"buscar", "--tipo=hoja", "ventas"}, "buscar", "q=ventas&tipo=hoja"},
		{[]string{"listar"}, "listar", ""},
		{[]string{"listar", "https://drive.google.com/drive/folders/1AbCdEfGhIjK"}, "listar", "carpeta=https%3A%2F%2Fdrive.google.com%2Fdrive%2Ffolders%2F1AbCdEfGhIjK"},
		{[]string{"leer", "1AbCdEfGhIjK", "--hoja", "Ventas", "--rango", "A1:D20", "--max", "5000"}, "leer", "hoja=Ventas&max=5000&rango=A1%3AD20&ref=1AbCdEfGhIjK"},
	} {
		o, q, err := driveQuery(c.args)
		if err != nil || o != c.orden || q.Encode() != c.want {
			t.Errorf("%v: %s %s %v", c.args, o, q.Encode(), err)
		}
	}
	for _, bad := range [][]string{nil, {"borrar", "x"}, {"buscar"}, {"leer"}, {"leer", "a", "b"}, {"leer", "x", "--hoja"}, {"buscar", "x", "--escribir"}} {
		if _, _, err := driveQuery(bad); err == nil {
			t.Errorf("%v debería fallar", bad)
		}
	}
}
