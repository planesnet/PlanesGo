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
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "sleep 2 &")
	GrupoPropio(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	Vigila(cmd.Process.Pid)
	cmd.Wait() // el «agente» ya ha terminado
	if GruposVivos() != 1 {
		t.Fatal("el hijo en segundo plano debería contar")
	}
	time.Sleep(2500 * time.Millisecond)
	if GruposVivos() != 0 {
		t.Fatal("al acabar el hijo ya no debería contar")
	}
}
