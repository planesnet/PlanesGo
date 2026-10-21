package proceso

import (
	"os"
	"os/exec"
	"sync"
	"time"
)

// Procesos de un agente marcados por entorno: cada ejecución lleva PGO_EJECUCION=<marca>, que heredan todos sus
// descendientes aunque se vayan a otro grupo de procesos (los Bash en segundo plano de Claude) o se queden
// huérfanos. Así se encuentran todos para vigilarlos y, al cancelar, para no dejar ninguno suelto.

const VarMarca = "PGO_EJECUCION"

// Gracia: lo que se espera tras pedir al agente que termine (cierra sus tareas) antes de matar lo que quede.
var Gracia = 5 * time.Second

var (
	muMarcas sync.Mutex
	marcas   = map[string]struct{}{}
)

// Agente prepara el proceso de un agente: grupo propio, la marca en su entorno y la cancelación en dos tiempos
// (SIGTERM al agente; tras la gracia, SIGKILL a todo lo que lleve la marca).
func Agente(cmd *exec.Cmd, marca string) {
	GrupoPropio(cmd)
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, VarMarca+"="+marca)
	muMarcas.Lock()
	marcas[marca] = struct{}{}
	muMarcas.Unlock()
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		termina(cmd.Process.Pid)
		go func() {
			time.Sleep(Gracia)
			MataMarca(marca)
		}()
		return nil
	}
}

// MataMarca mata todos los procesos que llevan la marca.
func MataMarca(marca string) {
	for _, pid := range ConMarca(marca) {
		mata(pid)
	}
}

// marcasVivas cuenta las ejecuciones con algún proceso vivo (y olvida las que ya no tienen ninguno).
func marcasVivas() int {
	muMarcas.Lock()
	defer muMarcas.Unlock()
	n := 0
	for m := range marcas {
		if len(ConMarca(m)) > 0 {
			n++
		} else {
			delete(marcas, m)
		}
	}
	return n
}
