package actualizacion

import "testing"

// La pausa se recuerda entre arranques (fichero «pausada»).
func TestPausaRecordada(t *testing.T) {
	dir := t.TempDir()
	a := Nuevo("", t.TempDir(), dir, "0.0.1")
	a.Pausa(true)
	if !Nuevo("", t.TempDir(), dir, "0.0.1").Estado().Pausada {
		t.Fatal("la pausa no se recuerda")
	}
	a.Pausa(false)
	if Nuevo("", t.TempDir(), dir, "0.0.1").Estado().Pausada {
		t.Fatal("la pausa no se quita")
	}
}
