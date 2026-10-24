package agentdesktop

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestActionValidate(t *testing.T) {
	for _, c := range []struct {
		a  Action
		ok bool
	}{
		{Action{Name: "status"}, true},
		{Action{Name: "capture"}, true},
		{Action{Name: "capture", Scale: 0.6}, true},
		{Action{Name: "capture", Scale: 0.05}, false},
		{Action{Name: "capture", Scale: 2}, false},
		{Action{Name: "click", X: 10, Y: 20}, true},
		{Action{Name: "click", X: -1, Y: 20}, false},
		{Action{Name: "drag", X: 1, Y: 2, X2: 3, Y2: -4}, false},
		{Action{Name: "wheel"}, true},
		{Action{Name: "wheel", Count: 51}, false},
		{Action{Name: "type", Text: "año 2026 · 10 €"}, true},
		{Action{Name: "type", Text: ""}, false},
		{Action{Name: "type", Text: "a\x1bb"}, false},
		{Action{Name: "type", Text: strings.Repeat("x", MaxText+1)}, false},
		{Action{Name: "key", Keys: []string{"ctrl+s", "Return", "KP_Enter"}}, true},
		{Action{Name: "key", Keys: []string{"--window"}}, false},
		{Action{Name: "key", Keys: []string{"ctrl+"}}, false},
		{Action{Name: "key"}, false},
		{Action{Name: "size", Width: 1920, Height: 1080}, true},
		{Action{Name: "size", Width: 4000, Height: 1080}, false},
		{Action{Name: "record", Record: "start"}, true},
		{Action{Name: "record", Record: "pause"}, false},
		{Action{Name: "start"}, false},
		{Action{Name: "stop"}, false},
	} {
		if err := c.a.Validate(); (err == nil) != c.ok {
			t.Errorf("%+v: %v", c.a, err)
		}
	}
	a := Action{Name: "wheel"}
	if a.Validate(); a.Count != 3 {
		t.Errorf("rueda por defecto: %d", a.Count)
	}
}

func TestConfigNormalize(t *testing.T) {
	c := Config{Host: " 127.0.0.1:3392 ", User: `DOM\u`, Password: "x"}
	if err := c.Normalize(); err != nil || c.Host != "127.0.0.1:3392" || c.Keyboard != KeyboardUnicode || c.Width != DefaultWidth || c.Height != DefaultHeight {
		t.Fatalf("%+v %v", c, err)
	}
	for _, bad := range []Config{
		{Host: "h", User: "u"},
		{Host: "h", User: "u", Password: "x", Keyboard: "dvorak"},
		{Host: "h", User: "u", Password: "x", Width: 100, Height: 100},
		{Host: "-h", User: "u", Password: "x"},
		{Host: "h", User: "u /p:x", Password: "x"},
	} {
		if err := bad.Normalize(); err == nil {
			t.Errorf("%+v debería fallar", bad)
		}
	}
}

func TestToScreen(t *testing.T) {
	// A 0.6 screenshot of 1600x900 is 960x540: its points are scaled back.
	if x, y, err := toScreen(480, 270, 960, 540, 1600, 900); err != nil || x != 800 || y != 450 {
		t.Errorf("%d %d %v", x, y, err)
	}
	if x, y, err := toScreen(100, 50, 0, 0, 1600, 900); err != nil || x != 100 || y != 50 {
		t.Errorf("sin captura: %d %d %v", x, y, err)
	}
	for _, p := range [][2]int{{960, 10}, {10, 540}, {-1, 0}} {
		if _, _, err := toScreen(p[0], p[1], 960, 540, 1600, 900); err == nil {
			t.Errorf("%v fuera de la pantalla", p)
		}
	}
}

func TestArgs(t *testing.T) {
	cfg := Config{Host: "h", User: `DOM\u`, Keyboard: KeyboardUnicode, Width: 1600, Height: 900}
	a := strings.Join(clientArgs(cfg, "/run/x/teclado.fifo", "/ex"), " ")
	for _, want := range []string{"/v:h", "/u:u", "/d:DOM", "/sec:nla", "/tls:seclevel:2", "/cert:tofu", "/gfx:AVC444",
		"/from-stdin:force", "/kbd:unicode:on,pipe:/run/x/teclado.fifo", "/size:1600x900", "/drive:Intercambio,/ex", "+dynamic-resolution"} {
		if !strings.Contains(a, want) {
			t.Errorf("falta %s en %s", want, a)
		}
	}
	cfg.Keyboard = KeyboardKeys
	if a := strings.Join(clientArgs(cfg, "p", ""), " "); !strings.Contains(a, "/kbd:unicode:off") || strings.Contains(a, "pipe:") || strings.Contains(a, "/drive") {
		t.Errorf("teclas: %s", a)
	}
	if a := strings.Join(xvfbArgs("", "/run/x/Xauthority"), " "); a != "-displayfd 3 -screen 0 3840x2160x24 -nolisten tcp -auth /run/x/Xauthority" {
		t.Errorf("Xvfb: %s", a)
	}
	if a := strings.Join(captureArgs(":5", 1600, 900, 0.6, "/o.png"), " "); a != "-display :5 -window root -crop 1600x900+0+0 +repage -resize 60.0% png:/o.png" {
		t.Errorf("import: %s", a)
	}
	if a := strings.Join(captureArgs(":5", 1600, 900, 1, "/o.png"), " "); strings.Contains(a, "-resize") {
		t.Errorf("import sin escala: %s", a)
	}
	if a := strings.Join(recordArgs(":5", 1601, 901, "/v.mp4"), " "); !strings.Contains(a, "-video_size 1600x900 -i :5") {
		t.Errorf("ffmpeg: %s", a)
	}
	if a := strings.Join(clickArgs(true, true), " "); a != "click --repeat 2 --delay 80 3" {
		t.Errorf("doble derecho: %s", a)
	}
}

func TestXauthority(t *testing.T) {
	b := xauthority(bytes.Repeat([]byte{7}, 16))
	want := append([]byte{0xff, 0xff, 0, 0, 0, 0, 0, 18}, "MIT-MAGIC-COOKIE-1"...)
	want = append(append(want, 0, 16), bytes.Repeat([]byte{7}, 16)...)
	if !bytes.Equal(b, want) {
		t.Errorf("%x", b)
	}
}

// The text goes to FreeRDP's pipe in pieces it reads whole: never a character split between two of them.
func TestPipeChunks(t *testing.T) {
	text := strings.Repeat("ñé€ a", 30) + "😀"
	l := pipeChunks(text, 63)
	if strings.Join(l, "") != text {
		t.Fatal("the pieces do not make the text")
	}
	for _, c := range l {
		if len(c) > 63 || !utf8.ValidString(c) {
			t.Errorf("piece %q (%d bytes)", c, len(c))
		}
	}
	if len(pipeChunks("", 63)) != 0 || len(pipeChunks("hola", 63)) != 1 {
		t.Error("short texts")
	}
}
