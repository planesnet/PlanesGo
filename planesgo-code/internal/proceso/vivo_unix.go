//go:build !windows

package proceso

import "syscall"

// grupoVivo: la señal 0 al grupo dice si queda algún proceso en él (sin hacerle nada).
func grupoVivo(pgid int) bool { return syscall.Kill(-pgid, 0) == nil }
