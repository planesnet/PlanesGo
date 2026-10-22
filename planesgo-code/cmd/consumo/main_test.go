package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"planesgo-code/internal/llm"
)

func banco(t *testing.T) Banco {
	t.Helper()
	var b Banco
	bs, err := os.ReadFile("../../consumo/pruebas.json")
	if err != nil || json.Unmarshal(bs, &b) != nil {
		t.Fatalf("pruebas.json: %v", err)
	}
	return b
}

// Las expresiones de las pruebas aceptan respuestas buenas y rechazan las malas.
func TestTextosEsperados(t *testing.T) {
	b := banco(t)
	casos := map[int]struct{ bien, mal []string }{
		1: {[]string{"OK", "OK.", "**OK**"}, []string{"Vale, OK", "No"}},
		2: {[]string{"8473", "El puerto es 8473."}, []string{"84730", "8069"}},
		3: {[]string{"facturacion/impuestos/calculo.py:21", "`facturacion/impuestos/calculo.py:17`", "facturacion/impuestos/calculo.py, línea 21"},
			[]string{"facturacion/impuestos/calculo.py:12", "facturacion/lineas.py:21"}},
	}
	for _, p := range b.Pruebas {
		c, ok := casos[p.ID]
		if !ok {
			continue
		}
		for _, x := range c.bien {
			if f := comprueba(p.Turnos[0], x, t.TempDir()); f != "" {
				t.Errorf("prueba %d, %q: %s", p.ID, x, f)
			}
		}
		for _, x := range c.mal {
			if comprueba(p.Turnos[0], x, t.TempDir()) == "" {
				t.Errorf("prueba %d acepta %q", p.ID, x)
			}
		}
	}
}

// El proyecto de prueba: con el fallo, las pruebas fallan; con el arreglo, pasan, y tocar las pruebas no vale.
func TestProyectoDePrueba(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("sin python3")
	}
	b := banco(t)
	arreglo := b.Pruebas[3].Turnos[0]
	d := filepath.Join(t.TempDir(), "p")
	if err := copia("../../consumo/proyecto", d); err != nil {
		t.Fatal(err)
	}
	if comprueba(arreglo, "", d) == "" {
		t.Fatal("sin arreglar, las pruebas pasan")
	}
	f := filepath.Join(d, "facturacion", "lineas.py")
	bs, _ := os.ReadFile(f)
	os.WriteFile(f, []byte(strings.Replace(string(bs), "self.bruto() - self.descuento", "self.bruto() * (1 - self.descuento / 100)", 1)), 0o644)
	if r := comprueba(arreglo, "", d); r != "" {
		t.Fatalf("arreglado: %s", r)
	}
	os.WriteFile(filepath.Join(d, "tests", "test_lineas.py"), []byte("\n"), 0o644)
	if comprueba(arreglo, "", d) == "" {
		t.Error("tocar las pruebas tendría que fallar")
	}
}

// Antigravity sin pgocode da el consumo acumulado de su conversación: se resta el del turno anterior.
func TestAcumuladoAntigravity(t *testing.T) {
	c := &recoge{agente: "antigravity", previo: &llm.Uso{Entrada: 1000, Salida: 10, CacheLeida: 500}}
	c.evento(llm.Evento{Uso: &llm.Uso{Entrada: 1800, Salida: 25, CacheLeida: 900}})
	r := c.fin()
	if r.Entrada != 800 || r.Salida != 15 || r.CacheLeida != 400 {
		t.Errorf("%+v", r.Medida)
	}
}
