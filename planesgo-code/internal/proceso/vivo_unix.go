//go:build !windows

package proceso

import "syscall"

// grupoVivo: la señal 0 al grupo dice si queda algún proceso en él (sin hacerle nada).
func grupoVivo(pgid int) bool { return syscall.Kill(-pgid, 0) == nil }

// GrupoVivo dice si queda algún proceso en el grupo.
func GrupoVivo(pgid int) bool { return grupoVivo(pgid) }

// SenalGrupo pide terminar al grupo (SIGTERM) o lo mata (SIGKILL).
func SenalGrupo(pgid int, fuerza bool) {
	s := syscall.SIGTERM
	if fuerza {
		s = syscall.SIGKILL
	}
	syscall.Kill(-pgid, s)
}

// MiGrupo: el grupo de procesos de pgocode.
func MiGrupo() int { return syscall.Getpgrp() }
