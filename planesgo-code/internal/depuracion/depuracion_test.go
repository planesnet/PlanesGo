package depuracion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Los secretos conocidos no llegan al registro.
func TestTapa(t *testing.T) {
	for _, c := range []string{
		"entorno añadido: PGO_TOKEN=abc123def456",
		"ANTHROPIC_API_KEY=sk-ant-api03-AAAAAAAAAAAAAAAAAAAA",
		`{"password": "prueba-secreta", "login": "x"}`,
		"Authorization: Bearer ya29.a0AfH6SMBxxxxxxxxxxxxxxxxxxxxxxx",
		"clave AIzaSyA1234567890abcdefghijklmnopqrstuv en el texto",
		"cookie session_id=0123456789abcdef0123",
		"token ghp_0123456789abcdefghijklmnopqrstuvwxyz",
	} {
		r := Tapa(c)
		for _, s := range []string{"abc123def456", "sk-ant-api03", "prueba-secreta", "ya29.a0", "AIzaSy", "0123456789abcdef0123", "ghp_0123"} {
			if strings.Contains(r, s) {
				t.Errorf("%q → %q: deja ver %q", c, r, s)
			}
		}
	}
	if r := Tapa("go test ./... ok en 3 s"); r != "go test ./... ok en 3 s" {
		t.Errorf("texto normal cambiado: %q", r)
	}
}

// Rota al pasar del tamaño y guarda solo los 5 anteriores.
func TestRota(t *testing.T) {
	f := filepath.Join(t.TempDir(), "d.log")
	if err := Activa(f); err != nil {
		t.Fatal(err)
	}
	linea := strings.Repeat("x", 64<<10)
	for i := 0; i < 8*(maxTam/len(linea)+1); i++ {
		Log("p", "%s", linea)
	}
	for i := 1; i <= anteriores; i++ {
		if _, err := os.Stat(f + "." + string(rune('0'+i))); err != nil {
			t.Errorf("falta %d: %v", i, err)
		}
	}
	if _, err := os.Stat(f + ".6"); err == nil {
		t.Error("guarda más de 5 anteriores")
	}
	if st, _ := os.Stat(f); st.Mode().Perm() != 0o600 {
		t.Errorf("permisos %v", st.Mode().Perm())
	}
}
