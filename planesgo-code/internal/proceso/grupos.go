package proceso

import "sync"

// Grupos de procesos que han lanzado los agentes. Aunque el agente termine, lo que dejó en segundo plano
// (un servidor con nohup, un subagente…) sigue en su grupo: mientras quede algo vivo, la app está ocupada.
var (
	muGrupos sync.Mutex
	grupos   = map[int]struct{}{}
)

// Vigila apunta el grupo de un proceso ya arrancado (su pid es el del grupo).
func Vigila(pid int) {
	muGrupos.Lock()
	grupos[pid] = struct{}{}
	muGrupos.Unlock()
}

// GruposVivos cuenta los grupos vigilados que aún tienen algún proceso (y olvida los que ya no), más las
// ejecuciones de agentes con algún proceso marcado vivo (aunque esté en otro grupo).
func GruposVivos() int {
	n := marcasVivas()
	muGrupos.Lock()
	defer muGrupos.Unlock()
	for g := range grupos {
		if grupoVivo(g) {
			n++
		} else {
			delete(grupos, g)
		}
	}
	return n
}
