package llm

import (
	"context"
	"os"
	"testing"
	"time"
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

// drainCollectors: espera a que los hilos de los recolectores acaben lo que tienen en cola (lo que dejó en segundo
// plano otra prueba), antes de cambiar o restaurar sus funciones (pruneNow, pruneDirFunc…): si no, carrera.
func drainCollectors() {
	muCollectors.Lock()
	cs := make([]*pruneCollector, 0, len(collectors))
	for _, c := range collectors {
		cs = append(cs, c)
	}
	muCollectors.Unlock()
	ctx, cancela := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancela()
	for _, c := range cs {
		c.do(ctx, func() {})
	}
}
