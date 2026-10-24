//go:build windows

package proceso

import (
	"os/exec"
	"syscall"
)

// Detach starts cmd on its own: it outlives the process that starts it, without a console of its own.
func Detach(cmd *exec.Cmd) {
	const detachedProcess, newProcessGroup = 0x00000008, 0x00000200
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | newProcessGroup, HideWindow: true}
}
