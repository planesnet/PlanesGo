//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// separaProceso deja la autoactualización en segundo plano en su propia sesión, para que sobreviva al padre.
func separaProceso(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
