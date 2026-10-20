//go:build windows

package llm

import "os/exec"

// grupoPropio: en Windows se mantiene la cancelación por defecto (mata el proceso directo).
func grupoPropio(cmd *exec.Cmd) {}
