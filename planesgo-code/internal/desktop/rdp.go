package desktop

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"planesgo-code/internal/freerdp"
	"planesgo-code/internal/proceso"
)

// Remote desktop (RDP): the person's FreeRDP window (SDL3), opened on this computer with the shared secure settings
// (internal/freerdp). The password goes to FreeRDP's standard input, which is closed right after it; it is never an
// argument, never logged and never sent to the server.

// RDPClient: the FreeRDP client (PGO_FREERDP, or the one connectors/instala.sh builds); "" if it is not installed.
func RDPClient() string {
	p := freerdp.SDLClient()
	if st, err := os.Stat(p); p == "" || err != nil || st.IsDir() {
		return ""
	}
	return p
}

// rdpArgs: the person's window on top of the shared secure settings. Everything is allowed (clipboard, the shared
// folder «Intercambio», sound, microphone and this computer's printers) and the person decides what to use; full
// screen whenever wanted (Ctrl+Alt+Intro in the window).
func rdpArgs(name, host, login string) []string {
	a := append(freerdp.SecureArgs(host, login), "/title:"+name,
		"+auto-reconnect", "/auto-reconnect-max-retries:20", "+dynamic-resolution", "/log-level:ERROR",
		// Without grabbing the keyboard: GNOME asks every time for permission to inhibit its shortcuts. Alt+Tab and
		// the Windows key stay on this desktop.
		"-grab-keyboard",
		"/size:85%", "+clipboard", "/sound:sys:pulse", "/microphone:sys:pulse", "/printer")
	return append(a, freerdp.DriveArgs()...)
}

// RDPWindow: a FreeRDP window that is open.
type RDPWindow struct {
	cmd  *exec.Cmd
	log  *bytes.Buffer
	ctx  context.Context
	stop context.CancelFunc
}

// StartRDP opens FreeRDP (bin) on host as user, with the password through its input. Nothing of pgocode's
// environment (tokens) reaches it, and it runs in its own process group (Close ends it all).
func StartRDP(bin, name, host, user, password string) (*RDPWindow, error) {
	if err := freerdp.ValidateTarget(host, user); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, rdpArgs(name, host, user)...)
	proceso.GrupoPropio(cmd)
	cmd.Env = proceso.CleanEnviron()
	var log bytes.Buffer
	cmd.Stdout, cmd.Stderr = &log, &log
	stdin, err := cmd.StdinPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		cancel()
		return nil, fmt.Errorf("no arranca FreeRDP: %w", err)
	}
	freerdp.SendPassword(stdin, password)
	return &RDPWindow{cmd: cmd, log: &log, ctx: ctx, stop: cancel}, nil
}

// Close ends the window (and everything FreeRDP started).
func (w *RDPWindow) Close() { w.stop() }

// Wait waits for the window to close and explains why it ended ("" if normally or by Close).
func (w *RDPWindow) Wait() string {
	err := w.cmd.Wait()
	closed := w.ctx.Err() != nil // ended from pgocode: not an error
	w.stop()
	if closed {
		return ""
	}
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	return freerdp.Explain(w.log.String(), code)
}
