package proceso

import (
	"bytes"
	"os"
	"strconv"
	"syscall"
)

// ConMarca: los procesos (del usuario) cuyo entorno lleva la marca, leyendo /proc/<pid>/environ.
func ConMarca(marca string) []int {
	buscada := []byte("\x00" + VarMarca + "=" + marca + "\x00")
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	yo := os.Getpid()
	var l []int
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == yo {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/environ")
		if err != nil || len(b) == 0 {
			continue
		}
		if bytes.Contains(append([]byte{0}, b...), buscada) {
			l = append(l, pid)
		}
	}
	return l
}

func termina(pid int) { syscall.Kill(pid, syscall.SIGTERM) }

// mata: el proceso y, si encabeza un grupo, el grupo entero.
func mata(pid int) {
	if pg, err := syscall.Getpgid(pid); err == nil && pg == pid {
		syscall.Kill(-pid, syscall.SIGKILL)
	}
	syscall.Kill(pid, syscall.SIGKILL)
}
