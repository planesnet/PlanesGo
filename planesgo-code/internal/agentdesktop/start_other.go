//go:build !linux

package agentdesktop

import "os"

// The agent's desktop needs X11 (Xvfb, xdotool): elsewhere it does not start.
func (d *Desktop) start(string) error { return ErrUnsupported }

func terminate(p *os.Process) { p.Kill() }

func writePipe(string, string) error { return ErrUnsupported }
