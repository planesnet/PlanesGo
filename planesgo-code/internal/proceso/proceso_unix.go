//go:build !windows

package proceso

import (
	"os/exec"
	"syscall"
)

// GrupoPropio lanza el proceso en un grupo nuevo y, al cancelar, mata al grupo entero.
func GrupoPropio(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
