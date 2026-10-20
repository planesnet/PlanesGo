//go:build !windows

package llm

import (
	"os/exec"
	"syscall"
)

// grupoPropio lanza el proceso en un grupo nuevo y, al cancelar, mata al grupo entero.
func grupoPropio(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
