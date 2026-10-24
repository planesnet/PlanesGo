//go:build !windows

package proceso

import (
	"os"
	"strconv"
	"syscall"
)

// DetachedEnv: set by the desktop app when it starts the server on its own, or its desktop helper; the process then
// starts afresh without what it inherited (StartClean).
const DetachedEnv = "PGO_DESACOPLADO"

// StartClean: a process started by another program can inherit its open files and sockets (Chromium leaves some
// without close-on-exec: the server then kept the desktop app's devtools port). With DetachedEnv set, every
// inherited descriptor but the standard ones is marked close-on-exec and the program runs itself again, clean.
// Call it first thing in main. Without DetachedEnv, it does nothing.
func StartClean() {
	if os.Getenv(DetachedEnv) == "" {
		return
	}
	os.Unsetenv(DetachedEnv)
	exe, err := os.Executable()
	if err != nil {
		return
	}
	for _, fd := range openFDs() {
		if fd > 2 {
			syscall.CloseOnExec(fd)
		}
	}
	syscall.Exec(exe, os.Args, os.Environ()) // only returns on error: then it just goes on
}

// openFDs: the process's open descriptors (/proc on Linux; the first 1024 elsewhere).
func openFDs() []int {
	if es, err := os.ReadDir("/proc/self/fd"); err == nil {
		l := make([]int, 0, len(es))
		for _, e := range es {
			if n, err := strconv.Atoi(e.Name()); err == nil {
				l = append(l, n)
			}
		}
		return l
	}
	l := make([]int, 0, 1024)
	for fd := 3; fd < 1024; fd++ {
		l = append(l, fd)
	}
	return l
}
