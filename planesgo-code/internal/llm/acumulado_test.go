package llm

import "testing"

// Antigravity da el consumo acumulado de la conversación: se emite el de cada turno.
func TestUsoDelTurno(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	acumulados = nil
	c := "conv-prueba"
	if u := usoDelTurno(c, Uso{Entrada: 18078, Salida: 1, Segundos: 5.9}); u.Entrada != 18078 || u.Segundos != 5.9 {
		t.Errorf("primer turno: %+v", u)
	}
	if u := usoDelTurno(c, Uso{Entrada: 20825, Salida: 247, Segundos: 17.3, Contexto: 9000, ContextoCache: 8000}); u.Entrada != 2747 || u.Salida != 246 || u.Segundos < 11.3 || u.Segundos > 11.5 {
		t.Errorf("segundo turno: %+v", u)
	} else if u.Contexto != 9000 || u.ContextoCache != 8000 {
		t.Errorf("la última llamada no es acumulada: %+v", u)
	}
	acumulados = nil // como tras reiniciar pgocode: se lee del fichero
	if u := usoDelTurno(c, Uso{Entrada: 42987, Salida: 303, Segundos: 52.6}); u.Entrada != 22162 || u.Salida != 56 {
		t.Errorf("tras reiniciar: %+v", u)
	}
	if u := usoDelTurno(c, Uso{Entrada: 100, Salida: 1, Segundos: 1}); u.Entrada != 100 {
		t.Errorf("si baja, entero: %+v", u)
	}
}
