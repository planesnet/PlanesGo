//go:build integracion && linux

package agentdesktop_test

import (
	"crypto/rand"
	"encoding/hex"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"planesgo-code/internal/agentdesktop"
	"planesgo-code/internal/freerdp"
)

// The agent's desktop against a local RDP test server: freerdp-shadow-cli (NLA with a SAM file) on its own virtual
// screen :83, port 3392; the agent's desktop on :84. Nothing appears on the person's desktop (X11 only, no
// Wayland) and every process is stopped by its PID.
func TestAgentDesktopReal(t *testing.T) {
	tools, err := agentdesktop.FindTools()
	if err != nil {
		t.Skip(err)
	}
	root := filepath.Join(freerdp.InstallDir(), "root")
	libs := filepath.Join(root, "usr", "lib", "x86_64-linux-gnu")
	shadow := filepath.Join(root, "usr", "bin", "freerdp-shadow-cli")
	hash := filepath.Join(freerdp.InstallDir(), "freerdp", "bin", "winpr-hash")
	if _, err := os.Stat(shadow); err != nil {
		t.Skip("sin freerdp-shadow-cli (connectors/instala.sh)")
	}
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config")) // the server's certificate and the client's known hosts
	t.Setenv("SDL_VIDEO_DRIVER", "x11")
	os.Unsetenv("WAYLAND_DISPLAY")
	b := make([]byte, 9)
	rand.Read(b)
	password := "Pr-" + hex.EncodeToString(b)

	sam, err := exec.Command(hash, "-u", "prueba", "-p", password, "-f", "sam").Output()
	if err != nil {
		t.Fatalf("winpr-hash: %v", err)
	}
	samFile := filepath.Join(tmp, "sam")
	os.WriteFile(samFile, sam, 0o600)

	env := func(display string) []string {
		return append(os.Environ(), "DISPLAY="+display, "LD_LIBRARY_PATH="+libs)
	}
	startBg := func(name string, cmd *exec.Cmd) {
		cmd.Stdout, cmd.Stderr = nil, nil
		if err := cmd.Start(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		pid := cmd.Process.Pid
		t.Cleanup(func() {
			cmd.Process.Signal(syscall.SIGTERM) // by its PID; Xvfb removes its socket and lock
			done := make(chan struct{})
			go func() { cmd.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				cmd.Process.Kill()
				<-done
			}
			t.Logf("%s (pid %d) parado", name, pid)
		})
	}
	server := exec.Command(tools.Xvfb, ":83", "-screen", "0", "1600x900x24", "-nolisten", "tcp")
	server.Env = env("")
	startBg("Xvfb :83", server)
	time.Sleep(time.Second)
	rdp := exec.Command(shadow, "/port:3392", "/bind-address:127.0.0.1", "/sam-file:"+samFile, "/log-level:ERROR")
	rdp.Env = env(":83")
	startBg("freerdp-shadow-cli", rdp)
	time.Sleep(2 * time.Second)

	opts := agentdesktop.Options{Tools: tools, Display: ":84", Exchange: filepath.Join(tmp, "intercambio")}
	cfg := agentdesktop.Config{Host: "127.0.0.1:3392", User: "prueba", Keyboard: agentdesktop.KeyboardUnicode}

	// A wrong password: it ends with the reason.
	bad := cfg
	bad.Password = "mala"
	d, err := agentdesktop.Start(bad, opts)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 150 && d.Running(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if st := d.Status(); st.State != agentdesktop.StateEnded || !strings.Contains(st.Error, "contraseña") {
		d.Close()
		t.Fatalf("contraseña incorrecta: %+v", st)
	}

	good := cfg
	good.Password = password
	d, err = agentdesktop.Start(good, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for i := 0; i < 200 && d.Status().State == agentdesktop.StateStarting; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if st := d.Status(); st.State != agentdesktop.StateConnected {
		t.Fatalf("no conecta: %+v", st)
	}
	time.Sleep(2 * time.Second) // the first frames
	out := filepath.Join(tmp, "salidas")
	do := func(a agentdesktop.Action) agentdesktop.Result {
		t.Helper()
		r, err := d.Do(a, out)
		if err != nil {
			t.Fatalf("%+v: %v", a, err)
		}
		return r
	}
	size := func(path string) (int, int) {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		c, err := png.DecodeConfig(f)
		if err != nil {
			t.Fatal(err)
		}
		return c.Width, c.Height
	}
	r := do(agentdesktop.Action{Name: "capture", Scale: 0.5})
	if w, h := size(r.Path); w != 800 || h != 450 {
		t.Errorf("captura: %dx%d", w, h)
	}
	// The pointer moves on the server's screen (:83): coordinates of the half-size screenshot, doubled.
	do(agentdesktop.Action{Name: "move", X: 150, Y: 100})
	time.Sleep(500 * time.Millisecond)
	where := exec.Command(tools.Xdotool, "getmouselocation", "--shell")
	where.Env = env(":83")
	loc, _ := where.Output()
	if !strings.Contains(string(loc), "X=300\n") || !strings.Contains(string(loc), "Y=200\n") {
		t.Errorf("el ratón del servidor no está en 300,200: %s", loc)
	}
	do(agentdesktop.Action{Name: "click", X: 10, Y: 10})
	do(agentdesktop.Action{Name: "click", X: 10, Y: 10, Double: true, Right: true})
	do(agentdesktop.Action{Name: "drag", X: 10, Y: 10, X2: 100, Y2: 100})
	do(agentdesktop.Action{Name: "wheel", Up: true, Count: 2})
	do(agentdesktop.Action{Name: "key", Keys: []string{"Escape"}})
	do(agentdesktop.Action{Name: "type", Text: "Año 10 €"})
	do(agentdesktop.Action{Name: "size", Width: 1280, Height: 720})
	time.Sleep(time.Second)
	if w, h := size(do(agentdesktop.Action{Name: "capture"}).Path); w != 1280 || h != 720 {
		t.Errorf("captura tras el cambio de tamaño: %dx%d", w, h)
	}
	v := do(agentdesktop.Action{Name: "record", Record: "start"})
	time.Sleep(2 * time.Second)
	do(agentdesktop.Action{Name: "record", Record: "stop"})
	if fi, err := os.Stat(v.Path); err != nil || fi.Size() == 0 {
		t.Errorf("vídeo: %v", err)
	}
	if r := do(agentdesktop.Action{Name: "status"}); !strings.Contains(r.Text, "conectado") {
		t.Errorf("estado: %s", r.Text)
	}
	d.Close()
	if _, err := os.Stat("/tmp/.X11-unix/X84"); err == nil {
		t.Error("la pantalla virtual :84 sigue abierta")
	}
}
