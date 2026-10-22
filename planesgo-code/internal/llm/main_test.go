package llm

import (
	"os"
	"testing"
)

// Las pruebas no tocan la caché del usuario (acumulados de consumo, configuración de MCP…).
func TestMain(m *testing.M) {
	d, err := os.MkdirTemp("", "pgocode-llm-pruebas-")
	if err == nil {
		os.Setenv("XDG_CACHE_HOME", d)
	}
	c := m.Run()
	if d != "" {
		os.RemoveAll(d)
	}
	os.Exit(c)
}
