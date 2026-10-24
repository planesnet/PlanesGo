package agentdesktop

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// The person's view of the agent's desktop: the screen as it is (Frame) and their own mouse and keyboard on it
// (Person), in the screen's own coordinates. Whoever acts, the desktop is one: while the person uses it (Hold, renewed
// by each of their actions and given back after HoldFor without one) the agent's orders are refused with ErrHeld, so
// that neither types into what the other is doing (a password, say).

// HoldFor: how long the person keeps the screen after their last action.
const HoldFor = 2 * time.Minute

// ErrHeld: the person is using the screen.
var ErrHeld = errors.New("la persona está usando la pantalla del escritorio del agente: espera a que la suelte y vuelve a probar (pgo escritorio estado)")

// Hold takes (or gives back) the screen for the person.
func (d *Desktop) Hold(on bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if on {
		d.heldUntil = time.Now().Add(HoldFor)
	} else {
		d.heldUntil = time.Time{}
	}
}

// Held: whether the person has the screen now.
func (d *Desktop) Held() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return time.Now().Before(d.heldUntil)
}

// Person runs an action of the person: click, move, drag, wheel, type or key, in the screen's coordinates; it takes
// the screen for HoldFor.
func (d *Desktop) Person(a Action) (Result, error) {
	switch a.Name {
	case "click", "move", "drag", "wheel", "type", "key":
	default:
		return Result{}, fmt.Errorf("la persona no puede usar «%s» desde aquí", a.Name)
	}
	if err := a.Validate(); err != nil {
		return Result{}, err
	}
	d.Hold(true)
	return d.do(a, "", d.screenPoint)
}

// screenPoint: a point of the screen as it is (the person's view is never scaled).
func (d *Desktop) screenPoint(x, y int) (string, string, error) {
	d.mu.Lock()
	w, h := d.width, d.height
	d.mu.Unlock()
	if x < 0 || y < 0 || x >= w || y >= h {
		return "", "", fmt.Errorf("fuera de la pantalla (%dx%d)", w, h)
	}
	return strconv.Itoa(x), strconv.Itoa(y), nil
}

// Frame: the screen now, as a JPEG, and its size (for the person's view, which asks for one again and again). It does
// not change the agent's coordinates.
func (d *Desktop) Frame(ctx context.Context) ([]byte, int, int, error) {
	if !d.Running() {
		return nil, 0, 0, errNotRunning
	}
	d.mu.Lock()
	w, h := d.width, d.height
	d.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.tools.Import, "-display", d.display, "-window", "root",
		"-crop", fmt.Sprintf("%dx%d+0+0", w, h), "+repage", "-quality", "70", "jpg:-")
	cmd.Env = d.env(d.tools.Import)
	out, err := cmd.Output() // only the image: its messages go elsewhere
	if err != nil || len(out) == 0 {
		return nil, 0, 0, fmt.Errorf("no se ha podido ver la pantalla: %v", err)
	}
	return out, w, h, nil
}
