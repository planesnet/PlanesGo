//go:build linux

package agentdesktop_test

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"planesgo-code/internal/agentdesktop"
	"planesgo-code/internal/agentdesktop/agentdesktoptest"
)

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, w, h)))
}

func startFake(t *testing.T, keyboard, clientExit string) (*agentdesktop.Desktop, string, string) {
	t.Helper()
	dir := t.TempDir()
	run := filepath.Join(t.TempDir(), "run")
	d, err := agentdesktop.Start(agentdesktop.Config{Host: "127.0.0.1:3392", User: `PRUEBAS\agente`, Password: "S3cr3t! ñ", Keyboard: keyboard},
		agentdesktop.Options{Tools: agentdesktoptest.Tools(t, dir, clientExit), RuntimeDir: run, Exchange: filepath.Join(dir, "intercambio")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d, dir, run
}

// waitFile waits for a fake program to write a file.
func waitFile(t *testing.T, path string) string {
	t.Helper()
	for i := 0; i < 100; i++ {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return string(b)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no aparece %s", path)
	return ""
}

// The password only reaches FreeRDP's input (and nothing else follows it); the arguments are the secure ones; the
// screen has its own X cookie; Stop leaves nothing behind.
func TestStartAndStop(t *testing.T) {
	d, dir, run := startFake(t, "", "")
	if pw := waitFile(t, filepath.Join(dir, "xfreerdp.stdin")); pw != "S3cr3t! ñ" {
		t.Errorf("entrada de FreeRDP: %q", pw)
	}
	args := strings.Join(agentdesktoptest.Args(dir, "xfreerdp"), " ")
	for _, want := range []string{"/v:127.0.0.1:3392", "/u:agente", "/d:PRUEBAS", "/sec:nla", "/tls:seclevel:2", "/cert:tofu", "/gfx:AVC444", "/kbd:unicode:on,pipe:" + run, "/size:1600x900"} {
		if !strings.Contains(args, want) {
			t.Errorf("falta %s en %s", want, args)
		}
	}
	if strings.Contains(args, "S3cr3t") {
		t.Fatal("la contraseña va en los argumentos")
	}
	xvfb := strings.Join(agentdesktoptest.Args(dir, "Xvfb"), " ")
	if !strings.Contains(xvfb, "-displayfd 3") || !strings.Contains(xvfb, "-auth "+run) || !strings.Contains(xvfb, "-nolisten tcp") {
		t.Errorf("Xvfb: %s", xvfb)
	}
	st := d.Status()
	if st.Display != ":99" || st.State != agentdesktop.StateConnected || st.Width != 1600 {
		t.Errorf("estado: %+v", st)
	}
	if fi, err := os.Stat(run); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("carpeta privada: %v %v", fi.Mode(), err)
	}
	d.Close()
	if st := d.Status(); st.State != agentdesktop.StateEnded || st.Error != "" {
		t.Errorf("parado: %+v", st)
	}
	if l, _ := os.ReadDir(run); len(l) != 0 {
		t.Errorf("quedan ficheros: %v", l)
	}
	if _, err := d.Do(agentdesktop.Action{Name: "click", X: 1, Y: 1}, ""); err == nil {
		t.Error("parado no hace clic")
	}
}

// Coordinates are those of the last screenshot (scaled), and every order reaches xdotool as expected.
func TestActions(t *testing.T) {
	d, dir, _ := startFake(t, "", "")
	writePNG(t, agentdesktoptest.Screenshot(dir), 960, 540)
	out := filepath.Join(dir, "salidas")
	r, err := d.Do(agentdesktop.Action{Name: "capture", Scale: 0.6}, out)
	if err != nil || filepath.Dir(r.Path) != out || !strings.Contains(r.Text, "960x540") {
		t.Fatalf("captura: %+v %v", r, err)
	}
	if imp := agentdesktoptest.Args(dir, "import"); !strings.Contains(imp[0], "-display :99 -window root -crop 1600x900+0+0 +repage -resize 60.0%") {
		t.Errorf("import: %v", imp)
	}
	for _, a := range []agentdesktop.Action{
		{Name: "click", X: 480, Y: 270},
		{Name: "click", X: 0, Y: 0, Double: true, Right: true},
		{Name: "move", X: 96, Y: 54},
		{Name: "drag", X: 0, Y: 0, X2: 480, Y2: 270},
		{Name: "wheel", Up: true, Count: 2},
		{Name: "key", Keys: []string{"ctrl+s", "Return"}},
		{Name: "size", Width: 1280, Height: 720},
		{Name: "click", X: 640, Y: 360},
	} {
		if _, err := d.Do(a, out); err != nil {
			t.Fatalf("%+v: %v", a, err)
		}
	}
	if _, err := d.Do(agentdesktop.Action{Name: "click", X: 1280, Y: 10}, out); err == nil {
		t.Error("fuera de la pantalla")
	}
	var got []string
	for _, l := range agentdesktoptest.Args(dir, "xdotool") {
		if !strings.HasPrefix(l, "search") {
			got = append(got, l)
		}
	}
	want := []string{
		"mousemove --sync 800 450 click 1",
		"mousemove --sync 0 0 click --repeat 2 --delay 80 3",
		"mousemove --sync 160 90",
		"mousemove --sync 0 0 mousedown 1 mousemove --sync 800 450 mouseup 1",
		"click --repeat 2 4",
		"key --delay 60 ctrl+s Return",
		"windowsize --sync 4242 1280 720",
		"mousemove --sync 640 360 click 1",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("xdotool:\n%s\nesperado:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if st := d.Status(); st.Width != 1280 || st.Height != 720 {
		t.Errorf("tamaño: %+v", st)
	}
	r, err = d.Do(agentdesktop.Action{Name: "record", Record: "start"}, out)
	if err != nil || !strings.HasPrefix(r.Path, filepath.Join(dir, "intercambio", "grabaciones")) || !d.Status().Recording {
		t.Fatalf("graba: %+v %v", r, err)
	}
	if _, err := d.Do(agentdesktop.Action{Name: "record", Record: "start"}, out); err == nil {
		t.Error("dos grabaciones a la vez")
	}
	if r2, err := d.Do(agentdesktop.Action{Name: "record", Record: "stop"}, out); err != nil || r2.Path != r.Path {
		t.Fatalf("fin de la grabación: %+v %v", r2, err)
	}
	if b, _ := os.ReadFile(r.Path); string(b) != "video\n" || d.Status().Recording {
		t.Errorf("vídeo: %q", b)
	}
	if ff := agentdesktoptest.Args(dir, "ffmpeg"); !strings.Contains(ff[0], "-f x11grab") || !strings.Contains(ff[0], "-video_size 1280x720 -i :99") {
		t.Errorf("ffmpeg: %v", ff)
	}
	if r, err := d.Do(agentdesktop.Action{Name: "status"}, ""); err != nil || !strings.Contains(r.Text, "conectado") || !strings.Contains(r.Text, "1280x720") {
		t.Errorf("estado: %+v %v", r, err)
	}
}

// Unicode keyboard: the text goes through FreeRDP's pipe; keys: through xdotool.
func TestTypeText(t *testing.T) {
	d, dir, _ := startFake(t, agentdesktop.KeyboardUnicode, "")
	waitFile(t, filepath.Join(dir, "xfreerdp.stdin"))
	for i := 0; i < 100; i++ { // the fake FreeRDP creates its pipe
		if _, err := d.Do(agentdesktop.Action{Name: "type", Text: "Año: 10 €"}, ""); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	d.Close()
	if got := waitFile(t, filepath.Join(dir, "typed")); got != "Año: 10 €" {
		t.Errorf("tubería: %q", got)
	}

	d, dir, _ = startFake(t, agentdesktop.KeyboardKeys, "")
	if _, err := d.Do(agentdesktop.Action{Name: "type", Text: "-x hola"}, ""); err != nil {
		t.Fatal(err)
	}
	if x := agentdesktoptest.Args(dir, "xdotool"); x[len(x)-1] != "type --delay 25 -- -x hola" {
		t.Errorf("xdotool: %v", x)
	}
}

// A failed login ends the desktop with FreeRDP's reason in a sentence.
func TestLoginFailure(t *testing.T) {
	d, _, run := startFake(t, "", "[ERROR] ERRCONNECT_LOGON_FAILURE [0x00020014]")
	for i := 0; i < 100 && d.Running(); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if st := d.Status(); st.State != agentdesktop.StateEnded || st.Error != "Usuario o contraseña incorrectos." {
		t.Errorf("estado: %+v", st)
	}
	if l, _ := os.ReadDir(run); len(l) != 0 {
		t.Errorf("quedan ficheros: %v", l)
	}
}

// Missing programs: a clear error before starting anything.
func TestMissingTools(t *testing.T) {
	_, err := agentdesktop.Start(agentdesktop.Config{Host: "h", User: "u", Password: "x"}, agentdesktop.Options{Tools: agentdesktop.Tools{Xvfb: "/x"}})
	if err == nil || !strings.Contains(err.Error(), "xdotool") || !strings.Contains(err.Error(), "instala.sh") {
		t.Errorf("%v", err)
	}
}
