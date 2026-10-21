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

// GruposConMarca: los grupos de procesos con algún proceso marcado (grupo → sus procesos marcados).
func GruposConMarca(marca string) map[int][]int {
	m := map[int][]int{}
	for _, pid := range ConMarca(marca) {
		if pg, err := syscall.Getpgid(pid); err == nil {
			m[pg] = append(m[pg], pid)
		}
	}
	return m
}

// Orden: la línea de órdenes de un proceso (los argumentos separados por espacios).
func Orden(pid int) string {
	b, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	return string(bytes.TrimSpace(bytes.ReplaceAll(b, []byte{0}, []byte{' '})))
}

// MarcaDe: la marca (PGO_EJECUCION) del entorno de un proceso, si la tiene.
func MarcaDe(pid int) string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/environ")
	if err != nil {
		return ""
	}
	for _, kv := range bytes.Split(b, []byte{0}) {
		if v, ok := bytes.CutPrefix(kv, []byte(VarMarca+"=")); ok {
			return string(v)
		}
	}
	return ""
}
