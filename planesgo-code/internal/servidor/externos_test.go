package servidor

import "testing"

// Las pruebas con procesos reales (python3 y ~/cowork) están en externos_integracion_test.go.
func TestDentro(t *testing.T) {
	c := []string{"/home/luis/cowork/intradema"}
	if !dentro("/home/luis/cowork/intradema/web", c) || !dentro("/home/luis/cowork/intradema", c) || dentro("/home/luis/cowork/intradema2", c) {
		t.Error("dentro")
	}
	if !ordenEn("python /home/luis/cowork/intradema/app.py", c) || ordenEn("python /home/luis/cowork/intradema2/app.py", c) {
		t.Error("ordenEn")
	}
}
