package servidor

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"planesgo-code/internal/imputacion"
)

// What the remote desktops (rdp.go, anydesk.go) share: their time goes to the project with the imputation service
// while the session is open and the keyboard or mouse are in use.

const (
	remoteBeat      = 20 * time.Second // activity reported to the imputation service
	remoteFirstBeat = 10 * time.Second // connected this long before counting (a failed login logs nothing)
	remoteIdleLimit = 10 * time.Minute // without keyboard or mouse: no more time
)

var idleRe = regexp.MustCompile(`(\d+)`)

// desktopIdle: time without keyboard or mouse on this desktop (GNOME); ok false if unknown.
func desktopIdle() (time.Duration, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gdbus", "call", "--session", "--dest", "org.gnome.Mutter.IdleMonitor",
		"--object-path", "/org/gnome/Mutter/IdleMonitor/Core", "--method", "org.gnome.Mutter.IdleMonitor.GetIdletime").Output()
	m := idleRe.FindSubmatch(out)
	if err != nil || m == nil {
		return 0, false
	}
	ms, _ := strconv.ParseInt(string(m[1]), 10, 64)
	return time.Duration(ms) * time.Millisecond, true
}

// keyboardInUse: someone has used the keyboard or mouse lately (true if it cannot be known).
func keyboardInUse() bool {
	idle, known := desktopIdle()
	return !known || idle < remoteIdleLimit
}

// remoteActivity reports a beat of a remote session if someone is at the keyboard (inUse); false if not: no time,
// and the service closes the period after a while.
func (s *Servidor) remoteActivity(work imputacion.Trabajo, inUse bool) bool {
	if !inUse {
		return false
	}
	if s.Imputacion != nil {
		s.Imputacion.Actividad(work)
	}
	return true
}
