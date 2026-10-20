//go:build windows

package proceso

import "os/exec"

// grupoPropio: en Windows se mantiene la cancelación por defecto (mata el proceso directo).
func GrupoPropio(cmd *exec.Cmd) {}
