//go:build !windows

package proceso

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// Lo que un agente deja en segundo plano mantiene vivo su grupo aunque el agente haya terminado.
func TestGrupoConHijoEnSegundoPlano(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "sleep 1 &")
	GrupoPropio(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	Vigila(cmd.Process.Pid)
	cmd.Wait() // el «agente» ya ha terminado
	if GruposVivos() != 1 {
		t.Fatal("el hijo en segundo plano debería contar")
	}
	// Se espera a que acabe el hijo (con un máximo), sin un tiempo fijo.
	for fin := time.Now().Add(10 * time.Second); GruposVivos() != 0; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(fin) {
			t.Fatal("al acabar el hijo ya no debería contar")
		}
	}
}
