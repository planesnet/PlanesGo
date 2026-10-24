//go:build !windows

package proceso

import (
	"os/exec"
	"syscall"
)

// GrupoPropio lanza el proceso en un grupo nuevo y, al cancelar, mata al grupo entero. La orden ha de crearse con
// exec.CommandContext: con exec.Command, Start falla porque tiene Cancel.
func GrupoPropio(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
