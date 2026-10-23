package actualizacion

import (
	"encoding/json"
	"strings"
	"testing"
)

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

// Sin revisar nunca, «revisado» no va en el JSON (antes salía la fecha cero: «Última comprobación 31/12/01»).
func TestRevisadoNuncaNoVa(t *testing.T) {
	b, err := json.Marshal(Nuevo("", t.TempDir(), t.TempDir(), "0.0.1").Estado())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "revisado") {
		t.Errorf("revisado sin revisar: %s", b)
	}
}
