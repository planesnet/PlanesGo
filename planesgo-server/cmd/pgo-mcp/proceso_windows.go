package main

import (
	"os/exec"
	"syscall"
)

// separaProceso deja la autoactualización en segundo plano en su propio grupo de procesos.
func separaProceso(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
