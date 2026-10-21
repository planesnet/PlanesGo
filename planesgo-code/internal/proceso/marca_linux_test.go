package proceso

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// Un agente que deja un hijo en otro grupo (como los Bash en segundo plano de Claude): se vigila y, al cancelar,
// no queda nada vivo.
func TestMarcaCancelaHijosEnOtroGrupo(t *testing.T) {
	Gracia = 300 * time.Millisecond
	ctx, cancela := context.WithCancel(context.Background())
	// trap '' TERM: el «agente» ignora el aviso, como uno que no cierra sus tareas.
	cmd := exec.CommandContext(ctx, "sh", "-c", "trap '' TERM; setsid sleep 30 & sleep 30")
	marca := "prueba-" + time.Now().Format("150405.000000")
	Agente(cmd, marca)
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	espera := func(cond func() bool) bool {
		for i := 0; i < 50; i++ {
			if cond() {
				return true
			}
			time.Sleep(50 * time.Millisecond)
		}
		return false
	}
	if !espera(func() bool { return len(ConMarca(marca)) >= 3 }) {
		t.Fatalf("procesos marcados: %v", ConMarca(marca))
	}
	if GruposVivos() == 0 {
		t.Error("debería contar como ocupado")
	}
	cancela()
	cmd.Wait()
	if !espera(func() bool { return len(ConMarca(marca)) == 0 }) {
		t.Fatalf("quedan vivos: %v", ConMarca(marca))
	}
}
