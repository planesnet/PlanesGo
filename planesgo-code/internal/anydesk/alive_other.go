//go:build !windows && !linux

package anydesk

import (
	"errors"
	"syscall"
)

// Alive: whether the process exists (signal 0 does nothing to it; EPERM: it exists, of another user).
func Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
