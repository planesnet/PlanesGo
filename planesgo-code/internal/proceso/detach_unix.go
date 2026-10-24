//go:build !windows

package proceso

import (
	"os/exec"
	"syscall"
)

// Detach starts cmd on its own (a new session): it outlives the process that starts it, and a signal to that
// process's group does not reach it.
func Detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
