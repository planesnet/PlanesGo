package emitter

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	studiotv "planesgo-code/studio-tv"
)

func testJPEG(t *testing.T) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for x := range 32 {
		for y := range 24 {
			im.Set(x, y, color.RGBA{200, 100, 50, 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, im, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Against a real studio: the state and the picture reach the channel.
func TestEmitterReachesStudio(t *testing.T) {
	studio := studiotv.Nuevo("emitir", "ver")
	srv := httptest.NewServer(studio.Handler())
	defer srv.Close()

	e := New(Config{URL: srv.URL, Token: "emitir", Channel: "qa-prueba-main"}, 50)
	e.State(map[string]any{"caso": "C1", "titulo": "Alta de cliente", "paso": "uno"})
	e.Frame(testJPEG(t))
	e.Close()

	var got *studiotv.InfoCanal
	for _, c := range studio.Canales() {
		if c.Canal == "qa-prueba-main" {
			got = &c
		}
	}
	if got == nil || !got.ConImagen {
		t.Fatalf("channel: %+v", studio.Canales())
	}
	var st map[string]any
	json.Unmarshal(got.Estado, &st)
	if st["caso"] != "C1" || st["paso"] != "uno" {
		t.Errorf("state: %s", got.Estado)
	}
}

// A wrong token or no studio at all: the test is never stopped.
func TestEmitterNeverBlocks(t *testing.T) {
	studio := studiotv.Nuevo("emitir", "ver")
	srv := httptest.NewServer(studio.Handler())
	defer srv.Close()
	bad := New(Config{URL: srv.URL, Token: "otro", Channel: "x"}, 0)
	bad.State(map[string]string{"paso": "uno"})
	bad.Close()
	if len(studio.Canales()) != 0 {
		t.Error("a wrong token must not open a channel")
	}
	none := New(Config{URL: "", Token: "", Channel: "x"}, 0)
	none.Frame([]byte{1})
	none.State(1)
	none.Close()
	if none.Active() {
		t.Error("without a studio, inactive")
	}
	var zero *Emitter
	zero.Frame(nil)
	zero.Close()
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("PCO_URL", "")
	t.Setenv("PCO_TOKEN", "")
	t.Setenv("PCO_CANAL", "")
	os.MkdirAll(filepath.Dir(ConfigFile()), 0o700)
	os.WriteFile(ConfigFile(), []byte("PCO_URL=http://127.0.0.1:7802\nPCO_TOKEN=abc\nPCO_CANAL=auto\n"), 0o600)
	if c := LoadConfig(); c.URL != "http://127.0.0.1:7802" || c.Token != "abc" || c.Channel != "auto" {
		t.Errorf("file: %+v", c)
	}
	t.Setenv("PCO_URL", "https://estudio")
	t.Setenv("PCO_TOKEN", "env")
	if c := LoadConfig(); c.URL != "https://estudio" || c.Token != "env" {
		t.Errorf("the environment wins: %+v", c)
	}
}

func TestDefaultChannel(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip()
	}
	dir := t.TempDir()
	if DefaultChannel(dir) != "principal" {
		t.Error("outside a repository: principal")
	}
	for _, a := range [][]string{{"init", "-q", "-b", "Dev/LP"}, {"remote", "add", "origin", "git@github.com:planesnet/MadPla.git"}} {
		exec.Command("git", append([]string{"-C", dir}, a...)...).Run()
	}
	if got := DefaultChannel(dir); got != "qa-madpla-dev-lp" {
		t.Errorf("channel: %q", got)
	}
}

// After Close, State and Frame do nothing (they used to panic sending on the closed wake channel).
func TestEmitterAfterClose(t *testing.T) {
	studio := studiotv.Nuevo("emitir", "ver")
	srv := httptest.NewServer(studio.Handler())
	defer srv.Close()
	e := New(Config{URL: srv.URL, Token: "emitir", Channel: "x"}, 50)
	e.Close()
	e.State(map[string]string{"paso": "después"})
	e.Frame(testJPEG(t))
	e.Close()
}
