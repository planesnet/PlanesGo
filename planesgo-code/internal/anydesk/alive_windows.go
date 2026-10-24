//go:build windows

package anydesk

import (
	"strings"
	"syscall"
	"unsafe"
)

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

var queryFullProcessImageName = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")

// Alive: whether the process exists, is still running and is AnyDesk's (a PID from an old trace line may belong to
// another program by now). Without it, a session whose AnyDesk ended without saying so would be reopened and its time
// counted for ever.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if syscall.GetExitCodeProcess(h, &code) != nil || code != stillActive {
		return false
	}
	buf := make([]uint16, syscall.MAX_PATH)
	n := uint32(len(buf))
	if r, _, _ := queryFullProcessImageName.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return false
	}
	return strings.Contains(strings.ToLower(syscall.UTF16ToString(buf[:n])), "anydesk")
}
