package main

import (
	"fmt"
	"strings"
	"testing"
)

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

func TestGoogleQuery(t *testing.T) {
	for _, c := range []struct {
		service string
		args    []string
		want    string
	}{
		{"drive", []string{"leer", "1AbCdEfGhIjK", "--hoja", "Ventas", "--rango", "A1:D20", "--max", "5000"}, "hoja=Ventas&max=5000&rango=A1%3AD20&ref=1AbCdEfGhIjK"},
		{"drive", []string{"leer", "--hoja=2", "https://docs.google.com/spreadsheets/d/1AbCdEfGhIjK/edit"}, "hoja=2&ref=https%3A%2F%2Fdocs.google.com%2Fspreadsheets%2Fd%2F1AbCdEfGhIjK%2Fedit"},
		{"gmail", []string{"leer", "https://mail.google.com/mail/u/0/#inbox/FMfcgz"}, "ref=https%3A%2F%2Fmail.google.com%2Fmail%2Fu%2F0%2F%23inbox%2FFMfcgz"},
		{"gmail", []string{"leer", "https://mail.google.com/mail/u/0/#inbox/x", "--max", "3000"}, "max=3000&ref=https%3A%2F%2Fmail.google.com%2Fmail%2Fu%2F0%2F%23inbox%2Fx"},
	} {
		q, err := googleQuery(c.service, c.args)
		if err != nil || q.Encode() != c.want {
			t.Errorf("%s %v: %s %v", c.service, c.args, q.Encode(), err)
		}
	}
	for _, bad := range [][]string{nil, {"buscar", "x"}, {"listar"}, {"borrar", "x"}, {"leer"}, {"leer", "a", "b"}, {"leer", "x", "--hoja"},
		{"leer", "x", "--escribir"}, {"leer", "x", "--max", "mucho"}} {
		if _, err := googleQuery("drive", bad); err == nil {
			t.Errorf("drive %v debería fallar", bad)
		}
	}
	for _, bad := range [][]string{{"buscar", "from:ana"}, {"listar"}, {"leer", "x", "--hoja", "y"}, {"enviar", "x"}} {
		if _, err := googleQuery("gmail", bad); err == nil {
			t.Errorf("gmail %v debería fallar", bad)
		}
	}
}

// Lo que llega al agente de «pgo salida»: las últimas líneas que caben y, si falta algo, dónde está todo.
func TestTrimmedOutput(t *testing.T) {
	if got := trimmedOutput([]string{"a", "b"}, 2, "/p/.pgocode/salidas/programa-web.log", outputTotalMax); got != "a\nb\n" {
		t.Fatalf("entera: %q", got)
	}
	var l []string
	for i := 0; i < 100; i++ {
		l = append(l, strings.Repeat("x", 150)+fmt.Sprint(i))
	}
	l = append(l, strings.Repeat("y", 5000))
	got := trimmedOutput(l, 2500, "/p/.pgocode/salidas/programa-web.log", outputTotalMax)
	if len(got) > outputTotalMax+300 || !strings.HasSuffix(got, "[4000 caracteres más]\n") {
		t.Fatalf("recortada (%d): %.200q … %q", len(got), got, got[len(got)-80:])
	}
	if !strings.HasPrefix(got, "[… 24") || !strings.Contains(got, "Salida completa: /p/.pgocode/salidas/programa-web.log") {
		t.Fatalf("aviso: %.300q", got)
	}
	if !strings.Contains(got, strings.Repeat("x", 150)+"99\n") {
		t.Fatal("las últimas líneas")
	}
	// Con n explícito (para filtrar con grep), todas las que pide.
	if got := trimmedOutput(l, 2500, "/r.log", 0); strings.Count(got, "\n") != 102 || !strings.HasPrefix(got, "[… 2399 líneas antes. Salida completa: /r.log") {
		t.Fatalf("con n: %d líneas, %.100q", strings.Count(got, "\n"), got)
	}
	if got := trimmedOutput(l[:3], 50, "", outputTotalMax); !strings.HasPrefix(got, "[… 47 líneas antes]\n") {
		t.Fatalf("sin registro: %.80q", got)
	}
}
