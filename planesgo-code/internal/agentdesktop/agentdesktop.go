// Package agentdesktop is the remote desktop an agent drives: a Windows desktop over RDP on a virtual screen
// (Xvfb), with FreeRDP's X11 client, screenshots (ImageMagick's import), mouse and keyboard (xdotool, and text as
// Unicode through FreeRDP's keyboard pipe) and video (ffmpeg). The person starts it from the session properties
// with the password, which only goes to FreeRDP's standard input; the session's agents then use it with
// «pgo escritorio». Only Linux: elsewhere Start returns ErrUnsupported.
package agentdesktop

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"planesgo-code/internal/freerdp"
	"planesgo-code/internal/proceso"
)

const (
	// The virtual screen is a fixed canvas this big; what changes size is FreeRDP's window (at 0,0) and, with its
	// dynamic resolution, the server's session. Screenshots and video are cropped to the window. (Xvfb cannot add
	// modes with xrandr: it cannot resize its screen.)
	CanvasWidth   = 3840
	CanvasHeight  = 2160
	DefaultWidth  = 1600
	DefaultHeight = 900

	KeyboardUnicode = "unicode" // text as Unicode through FreeRDP's pipe: accents, ñ, € whatever the server's layout
	KeyboardKeys    = "keys"    // key codes through xdotool: only ASCII is reliable

	StateStarting  = "starting"
	StateConnected = "connected"
	StateEnded     = "ended"

	MaxText  = 10000 // characters of one «escribe»
	MaxKeys  = 20
	MaxWheel = 50

	startTimeout = 10 * time.Second
	toolTimeout  = 30 * time.Second
	stopTimeout  = 3 * time.Second
	logTail      = 64 << 10
)

// ErrUnsupported: the agent's desktop needs X11 (Xvfb, xdotool): only Linux.
var ErrUnsupported = errors.New("el escritorio del agente solo funciona en Linux (pantalla virtual Xvfb y X11)")

var errNotRunning = errors.New("el escritorio del agente no está conectado: la persona lo inicia en las propiedades de la sesión (Escritorio del agente)")

// Config: where and as whom. The password never leaves the process (it is not in any JSON).
type Config struct {
	Host     string `json:"host"` // name or IP, optionally :port
	User     string `json:"user"` // DOMINIO\usuario or usuario
	Password string `json:"-"`
	Keyboard string `json:"keyboard"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

// Normalize trims the fields, fills the defaults and validates everything before it becomes an argument.
func (c *Config) Normalize() error {
	c.Host, c.User = strings.TrimSpace(c.Host), strings.TrimSpace(c.User)
	if err := freerdp.ValidateTarget(c.Host, c.User); err != nil {
		return err
	}
	switch c.Keyboard {
	case "":
		c.Keyboard = KeyboardUnicode
	case KeyboardUnicode, KeyboardKeys:
	default:
		return errors.New("teclado no válido (unicode o teclas)")
	}
	if c.Width == 0 && c.Height == 0 {
		c.Width, c.Height = DefaultWidth, DefaultHeight
	}
	if err := ValidateSize(c.Width, c.Height); err != nil {
		return err
	}
	if c.Password == "" {
		return errors.New("falta la contraseña")
	}
	return nil
}

// ValidateSize: a window size that fits the virtual screen.
func ValidateSize(w, h int) error {
	if w < 320 || w > CanvasWidth || h < 200 || h > CanvasHeight {
		return fmt.Errorf("tamaño fuera de rango (de 320x200 a %dx%d)", CanvasWidth, CanvasHeight)
	}
	return nil
}

// Action: one order of the agent («pgo escritorio <orden>»).
type Action struct {
	Name   string   `json:"action"` // capture, click, move, drag, wheel, type, key, size, record, status
	X      int      `json:"x,omitempty"`
	Y      int      `json:"y,omitempty"`
	X2     int      `json:"x2,omitempty"`
	Y2     int      `json:"y2,omitempty"`
	Double bool     `json:"double,omitempty"`
	Right  bool     `json:"right,omitempty"`
	Up     bool     `json:"up,omitempty"`
	Count  int      `json:"count,omitempty"`
	Text   string   `json:"text,omitempty"`
	Keys   []string `json:"keys,omitempty"`
	Width  int      `json:"width,omitempty"`
	Height int      `json:"height,omitempty"`
	Scale  float64  `json:"scale,omitempty"`
	Record string   `json:"record,omitempty"` // start or stop
}

// keyName: xdotool key names and combinations (ctrl+s, Return, KP_Enter, super): never an option.
var keyName = regexp.MustCompile(`^[A-Za-z0-9_]{1,32}(\+[A-Za-z0-9_]{1,32}){0,4}$`)

// Validate checks the action and fills its defaults.
func (a *Action) Validate() error {
	coord := func(v ...int) error {
		for _, x := range v {
			if x < 0 || x > 100000 {
				return errors.New("coordenadas no válidas")
			}
		}
		return nil
	}
	switch a.Name {
	case "status":
	case "capture":
		if a.Scale == 0 {
			a.Scale = 1
		}
		if a.Scale < 0.1 || a.Scale > 1 || math.IsNaN(a.Scale) {
			return errors.New("la escala va de 0.1 a 1")
		}
	case "click", "move":
		return coord(a.X, a.Y)
	case "drag":
		return coord(a.X, a.Y, a.X2, a.Y2)
	case "wheel":
		if a.Count == 0 {
			a.Count = 3
		}
		if a.Count < 1 || a.Count > MaxWheel {
			return fmt.Errorf("la rueda va de 1 a %d pasos", MaxWheel)
		}
	case "type":
		if a.Text == "" || !utf8.ValidString(a.Text) {
			return errors.New("falta el texto")
		}
		if utf8.RuneCountInString(a.Text) > MaxText {
			return fmt.Errorf("el texto es demasiado largo (máximo %d caracteres)", MaxText)
		}
		for _, r := range a.Text {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return errors.New("el texto lleva caracteres de control: para teclas especiales, usa «tecla»")
			}
		}
	case "key":
		if len(a.Keys) == 0 || len(a.Keys) > MaxKeys {
			return fmt.Errorf("di de 1 a %d teclas", MaxKeys)
		}
		for _, k := range a.Keys {
			if !keyName.MatchString(k) {
				return fmt.Errorf("tecla no válida: %q (nombres de xdotool: ctrl+s, Return, Tab, F5…)", k)
			}
		}
	case "size":
		return ValidateSize(a.Width, a.Height)
	case "record":
		if a.Record != "start" && a.Record != "stop" {
			return errors.New("graba inicio o graba fin")
		}
	default:
		return errors.New("orden del escritorio desconocida")
	}
	return nil
}

// Tools: the external programs. Xvfb and xdotool may be the Ubuntu packages unpacked by connectors/instala.sh
// (LocalRoot): only they get its libraries (LocalLibs); with them FreeRDP would load Ubuntu's older ones.
type Tools struct {
	Xvfb, Xdotool, XFreeRDP, Import, FFmpeg string
	LocalRoot, LocalLibs                    string
}

// FindTools looks for the programs: Xvfb and xdotool in connectors/instala.sh's folder or in the PATH, FreeRDP's
// X11 client built by connectors/instala.sh, import (ImageMagick) and ffmpeg in the PATH. ffmpeg is only needed to
// record: its absence is said when recording.
func FindTools() (Tools, error) {
	root := filepath.Join(freerdp.InstallDir(), "root")
	t := Tools{LocalRoot: root, LocalLibs: filepath.Join(root, "usr", "lib", "x86_64-linux-gnu")}
	local := func(name string) string {
		p := filepath.Join(root, "usr", "bin", name)
		if executable(p) {
			return p
		}
		p, _ = exec.LookPath(name)
		return p
	}
	t.Xvfb, t.Xdotool = local("Xvfb"), local("xdotool")
	if p := freerdp.X11Client(); executable(p) {
		t.XFreeRDP = p
	}
	t.Import, _ = exec.LookPath("import")
	t.FFmpeg, _ = exec.LookPath("ffmpeg")
	return t, t.check()
}

// check: the programs needed to start are there.
func (t Tools) check() error {
	var missing []string
	for _, x := range []struct{ name, path string }{{"Xvfb", t.Xvfb}, {"xdotool", t.Xdotool}, {"xfreerdp", t.XFreeRDP}, {"import (ImageMagick)", t.Import}} {
		if x.path == "" {
			missing = append(missing, x.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("faltan programas para el escritorio del agente: %s (ejecuta connectors/instala.sh)", strings.Join(missing, ", "))
	}
	return nil
}

func executable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}

// Options: where things go (the defaults, except in tests).
type Options struct {
	Tools      Tools
	Display    string // a fixed display (":84"); empty: Xvfb takes a free one
	RuntimeDir string // private folder (700) for the X cookie and the keyboard pipe; empty: $XDG_RUNTIME_DIR/rdp-agente
	Exchange   string // shared folder (\\tsclient\Intercambio); empty: freerdp.ExchangeDir()
}

// Status: what the interface and «pgo escritorio estado» show.
type Status struct {
	State     string    `json:"state"`
	Error     string    `json:"error,omitempty"`
	Host      string    `json:"host"`
	User      string    `json:"user"`
	Keyboard  string    `json:"keyboard"`
	Display   string    `json:"display"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Recording bool      `json:"recording"`
	Video     string    `json:"video,omitempty"`
	Exchange  string    `json:"exchange"`
	Started   time.Time `json:"started"`
}

// Result: what an action answers. Path: the file it made (a screenshot, a video).
type Result struct {
	Text string `json:"text"`
	Path string `json:"path,omitempty"`
}

// proc: a started program and the end of its Wait.
type proc struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (p *proc) alive() bool {
	if p == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// Desktop: a running agent desktop.
type Desktop struct {
	cfg     Config // without the password
	tools   Tools
	opts    Options
	dir     string // this desktop's private folder: X cookie and keyboard pipe
	xauth   string
	pipe    string
	display string
	started time.Time
	ctx     context.Context
	cancel  context.CancelFunc // last resort: kills the process groups

	act sync.Mutex // one action at a time on the screen

	mu         sync.Mutex
	width      int
	height     int
	capW, capH int // size of the last screenshot: the agent's coordinates
	state      string
	errText    string
	closing    bool
	xvfb       *proc
	client     *proc
	recorder   *proc
	recStdin   io.WriteCloser
	video      string
	log        *tailBuffer
	down       sync.Once
}

// Start launches the virtual screen and FreeRDP; the connection goes on in the background (Status says when it is
// connected or why it ended).
func Start(cfg Config, opts Options) (*Desktop, error) {
	if runtime.GOOS != "linux" {
		return nil, ErrUnsupported
	}
	if err := cfg.Normalize(); err != nil {
		return nil, err
	}
	if err := opts.Tools.check(); err != nil {
		return nil, err
	}
	if opts.Exchange == "" {
		opts.Exchange = freerdp.ExchangeDir()
	}
	password := cfg.Password
	cfg.Password = ""
	ctx, cancel := context.WithCancel(context.Background())
	d := &Desktop{cfg: cfg, tools: opts.Tools, opts: opts, started: time.Now(), ctx: ctx, cancel: cancel,
		width: cfg.Width, height: cfg.Height, capW: cfg.Width, capH: cfg.Height, state: StateStarting, log: &tailBuffer{max: logTail}}
	if err := d.start(password); err != nil {
		d.teardown()
		return nil, err
	}
	return d, nil
}

// clientArgs: FreeRDP's X11 client on the virtual screen with the shared secure settings.
func clientArgs(cfg Config, pipe, exchange string) []string {
	kbd := "/kbd:unicode:off"
	if cfg.Keyboard == KeyboardUnicode {
		kbd = "/kbd:unicode:on,pipe:" + pipe
	}
	a := append(freerdp.SecureArgs(cfg.Host, cfg.User), kbd, fmt.Sprintf("/size:%dx%d", cfg.Width, cfg.Height),
		"+clipboard", "+dynamic-resolution", "/log-level:WARN")
	if exchange != "" {
		a = append(a, "/drive:Intercambio,"+exchange)
	}
	return a
}

// xvfbArgs: the virtual screen, only through its local socket and with an X cookie.
func xvfbArgs(display, xauth string) []string {
	var a []string
	if display != "" {
		a = append(a, display)
	}
	return append(a, "-displayfd", "3", "-screen", "0", fmt.Sprintf("%dx%dx24", CanvasWidth, CanvasHeight),
		"-nolisten", "tcp", "-auth", xauth)
}

// xauthority: an X authority file with one MIT-MAGIC-COOKIE-1 for any display (FamilyWild): Xvfb loads it with
// -auth and the clients (XAUTHORITY) present it. Without it, any local user could look at the screen.
func xauthority(cookie []byte) []byte {
	var b bytes.Buffer
	field := func(x []byte) {
		binary.Write(&b, binary.BigEndian, uint16(len(x)))
		b.Write(x)
	}
	binary.Write(&b, binary.BigEndian, uint16(0xffff)) // FamilyWild
	field(nil)                                         // address
	field(nil)                                         // display number
	field([]byte("MIT-MAGIC-COOKIE-1"))
	field(cookie)
	return b.Bytes()
}

// env: a program's environment: pgocode's without secrets, on this desktop's display and cookie, and the local
// libraries only for the unpacked Ubuntu tools.
func (d *Desktop) env(bin string) []string {
	var e []string
	for _, kv := range proceso.CleanEnviron() {
		switch k, _, _ := strings.Cut(kv, "="); k {
		case "DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "LD_LIBRARY_PATH":
			continue
		}
		e = append(e, kv)
	}
	if d.display != "" {
		e = append(e, "DISPLAY="+d.display)
	}
	e = append(e, "XAUTHORITY="+d.xauth)
	if d.tools.LocalRoot != "" && strings.HasPrefix(bin, d.tools.LocalRoot+string(filepath.Separator)) {
		e = append(e, "LD_LIBRARY_PATH="+d.tools.LocalLibs)
	}
	return e
}

// launch starts a program of this desktop in its own process group.
func (d *Desktop) launch(bin string, args []string, setup func(*exec.Cmd)) (*proc, error) {
	cmd := exec.CommandContext(d.ctx, bin, args...)
	proceso.GrupoPropio(cmd)
	cmd.Env = d.env(bin)
	if setup != nil {
		setup(cmd)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &proc{cmd: cmd, done: make(chan struct{})}
	go func() {
		cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

// run: a short program on this desktop (xdotool, import), with its output.
func (d *Desktop) run(bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(d.ctx, toolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = d.env(bin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %v %s", filepath.Base(bin), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (d *Desktop) xdotool(args ...string) error {
	_, err := d.run(d.tools.Xdotool, args...)
	return err
}

// clientEnded: FreeRDP has exited: the desktop is over (and why, unless it was stopped from pgocode).
func (d *Desktop) clientEnded() {
	<-d.client.done
	code := d.client.cmd.ProcessState.ExitCode()
	d.mu.Lock()
	if !d.closing {
		d.errText = freerdp.Explain(d.log.String(), code)
	}
	d.mu.Unlock()
	d.teardown() // «ended» once the screen is gone too: a new desktop may take its display
}

// teardown stops the recording and the virtual screen and removes the private folder (once).
func (d *Desktop) teardown() {
	d.down.Do(func() {
		d.mu.Lock()
		d.closing = true
		rec, client, xvfb := d.recorder, d.client, d.xvfb
		d.mu.Unlock()
		d.stopRecorder(rec)
		stop(client, stopTimeout)
		stop(xvfb, stopTimeout)
		d.cancel()
		if d.dir != "" {
			os.RemoveAll(d.dir)
		}
		d.mu.Lock()
		d.state = StateEnded
		d.recorder = nil
		d.mu.Unlock()
	})
}

// Close stops everything (pgocode closes, or the person stops it).
func (d *Desktop) Close() error {
	d.teardown()
	return nil
}

// stop asks a program to end and, if it does not within the time, kills it.
func stop(p *proc, wait time.Duration) {
	if !p.alive() {
		return
	}
	terminate(p.cmd.Process)
	select {
	case <-p.done:
	case <-time.After(wait):
		p.cmd.Process.Kill()
		<-p.done
	}
}

// Running: FreeRDP is still there.
func (d *Desktop) Running() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state != StateEnded
}

// Status: the desktop's state; «starting» becomes «connected» when FreeRDP's window appears (after the login).
func (d *Desktop) Status() Status {
	d.mu.Lock()
	starting := d.state == StateStarting
	d.mu.Unlock()
	if starting {
		if w, _ := d.window(); w != "" {
			d.mu.Lock()
			if d.state == StateStarting {
				d.state = StateConnected
			}
			d.mu.Unlock()
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return Status{State: d.state, Error: d.errText, Host: d.cfg.Host, User: d.cfg.User, Keyboard: d.cfg.Keyboard,
		Display: d.display, Width: d.width, Height: d.height, Recording: d.recorder.alive(), Video: d.video,
		Exchange: d.opts.Exchange, Started: d.started}
}

// window: FreeRDP's window ("" while it is not there: it appears after the login).
func (d *Desktop) window() (string, error) {
	d.mu.Lock()
	client := d.client
	d.mu.Unlock()
	if !client.alive() {
		return "", errNotRunning
	}
	out, err := d.run(d.tools.Xdotool, "search", "--onlyvisible", "--pid", strconv.Itoa(client.cmd.Process.Pid))
	if f := strings.Fields(out); err == nil && len(f) > 0 {
		return f[0], nil
	}
	return "", nil
}

// toScreen: a point of the last screenshot (maybe scaled) on the remote screen.
func toScreen(x, y, capW, capH, w, h int) (int, int, error) {
	if capW <= 0 || capH <= 0 {
		capW, capH = w, h
	}
	rx := int(math.Round(float64(x) * float64(w) / float64(capW)))
	ry := int(math.Round(float64(y) * float64(h) / float64(capH)))
	if x < 0 || y < 0 || rx >= w || ry >= h {
		return 0, 0, fmt.Errorf("coordenadas fuera de la pantalla (la última captura mide %dx%d)", capW, capH)
	}
	return rx, ry, nil
}

// point: the action's point (or its second one) on the remote screen.
func (d *Desktop) point(x, y int) (string, string, error) {
	d.mu.Lock()
	rx, ry, err := toScreen(x, y, d.capW, d.capH, d.width, d.height)
	d.mu.Unlock()
	return strconv.Itoa(rx), strconv.Itoa(ry), err
}

// Do runs an action of the agent. captureDir: where screenshots go (the session's .pgocode/salidas).
func (d *Desktop) Do(a Action, captureDir string) (Result, error) {
	if err := a.Validate(); err != nil {
		return Result{}, err
	}
	if a.Name == "status" {
		return Result{Text: statusText(d.Status())}, nil
	}
	d.act.Lock()
	defer d.act.Unlock()
	if !d.Running() {
		return Result{}, errNotRunning
	}
	switch a.Name {
	case "capture":
		return d.capture(a.Scale, captureDir)
	case "click", "move":
		x, y, err := d.point(a.X, a.Y)
		if err != nil {
			return Result{}, err
		}
		args := []string{"mousemove", "--sync", x, y}
		if a.Name == "click" {
			args = append(args, clickArgs(a.Double, a.Right)...)
		}
		return Result{Text: "hecho"}, d.xdotool(args...)
	case "drag":
		x1, y1, err := d.point(a.X, a.Y)
		if err != nil {
			return Result{}, err
		}
		x2, y2, err := d.point(a.X2, a.Y2)
		if err != nil {
			return Result{}, err
		}
		return Result{Text: "hecho"}, d.xdotool("mousemove", "--sync", x1, y1, "mousedown", "1", "mousemove", "--sync", x2, y2, "mouseup", "1")
	case "wheel":
		button := "5"
		if a.Up {
			button = "4"
		}
		return Result{Text: "hecho"}, d.xdotool("click", "--repeat", strconv.Itoa(a.Count), button)
	case "type":
		return Result{Text: "hecho"}, d.typeText(a.Text)
	case "key":
		return Result{Text: "hecho"}, d.xdotool(append([]string{"key", "--delay", "60"}, a.Keys...)...)
	case "size":
		return d.resize(a.Width, a.Height)
	case "record":
		if a.Record == "start" {
			return d.recordStart()
		}
		return d.recordStop()
	}
	return Result{}, errors.New("orden del escritorio desconocida")
}

func clickArgs(double, right bool) []string {
	button := "1"
	if right {
		button = "3"
	}
	if double {
		return []string{"click", "--repeat", "2", "--delay", "80", button}
	}
	return []string{"click", button}
}

// captureArgs: the screen cropped to FreeRDP's window and, if asked, scaled.
func captureArgs(display string, w, h int, scale float64, out string) []string {
	a := []string{"-display", display, "-window", "root", "-crop", fmt.Sprintf("%dx%d+0+0", w, h), "+repage"}
	if scale != 1 {
		a = append(a, "-resize", strconv.FormatFloat(scale*100, 'f', 1, 64)+"%")
	}
	return append(a, "png:"+out)
}

// capture saves a screenshot in dir; its size is the agent's coordinates from now on.
func (d *Desktop) capture(scale float64, dir string) (Result, error) {
	if dir == "" {
		return Result{}, errors.New("falta la carpeta de la sesión para guardar la captura")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, err
	}
	out := filepath.Join(dir, "escritorio-"+time.Now().Format("20060102-150405.000")+".png")
	d.mu.Lock()
	w, h := d.width, d.height
	d.mu.Unlock()
	if _, err := d.run(d.tools.Import, captureArgs(d.display, w, h, scale, out)...); err != nil {
		return Result{}, fmt.Errorf("no se ha podido hacer la captura: %v", err)
	}
	f, err := os.Open(out)
	if err != nil {
		return Result{}, err
	}
	cfg, err := png.DecodeConfig(f)
	f.Close()
	if err != nil {
		return Result{}, fmt.Errorf("la captura no es una imagen PNG: %v", err)
	}
	d.mu.Lock()
	d.capW, d.capH = cfg.Width, cfg.Height
	d.mu.Unlock()
	return Result{Path: out, Text: fmt.Sprintf("Captura: %s (%dx%d). Las coordenadas de clic, mueve y arrastra son las de esta imagen.", out, cfg.Width, cfg.Height)}, nil
}

// typeText: as Unicode through FreeRDP's pipe (keyboard «unicode») or as keys with xdotool.
func (d *Desktop) typeText(text string) error {
	if d.cfg.Keyboard == KeyboardUnicode {
		if _, err := os.Stat(d.pipe); err == nil {
			return writePipe(d.pipe, text)
		}
	}
	return d.xdotool("type", "--delay", "25", "--", text)
}

// resize changes FreeRDP's window; with dynamic resolution the server adjusts its session.
func (d *Desktop) resize(w, h int) (Result, error) {
	win, err := d.window()
	if err != nil {
		return Result{}, err
	}
	if win == "" {
		return Result{}, errors.New("todavía no está la ventana de FreeRDP (¿sigue iniciando sesión?)")
	}
	if err := d.xdotool("windowsize", "--sync", win, strconv.Itoa(w), strconv.Itoa(h)); err != nil {
		return Result{}, err
	}
	d.mu.Lock()
	d.width, d.height, d.capW, d.capH = w, h, w, h
	d.mu.Unlock()
	return Result{Text: fmt.Sprintf("tamaño: %dx%d (las coordenadas son ya las de la pantalla, hasta la próxima captura)", w, h)}, nil
}

// recordArgs: video of FreeRDP's window (even sizes: H.264 needs them).
func recordArgs(display string, w, h int, out string) []string {
	return []string{"-loglevel", "error", "-f", "x11grab", "-framerate", "10", "-video_size", fmt.Sprintf("%dx%d", w&^1, h&^1),
		"-i", display, "-pix_fmt", "yuv420p", "-y", out}
}

// recordStart: video of the session in the shared folder (grabaciones), for evidence.
func (d *Desktop) recordStart() (Result, error) {
	if d.tools.FFmpeg == "" {
		return Result{}, errors.New("falta ffmpeg para grabar")
	}
	d.mu.Lock()
	recording, w, h := d.recorder.alive(), d.width, d.height
	d.mu.Unlock()
	if recording {
		return Result{}, errors.New("ya se está grabando")
	}
	dir := filepath.Join(d.opts.Exchange, "grabaciones")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, err
	}
	out := filepath.Join(dir, "escritorio-"+time.Now().Format("20060102-150405")+".mp4")
	var stdin io.WriteCloser
	p, err := d.launch(d.tools.FFmpeg, recordArgs(d.display, w, h, out), func(c *exec.Cmd) {
		in, err := c.StdinPipe()
		if err == nil {
			stdin = in
		}
	})
	if err != nil {
		return Result{}, fmt.Errorf("no arranca ffmpeg: %v", err)
	}
	d.mu.Lock()
	d.recorder, d.recStdin, d.video = p, stdin, out
	d.mu.Unlock()
	return Result{Path: out, Text: "grabando en " + out}, nil
}

// recordStop ends the video (ffmpeg closes the file when it reads «q»).
func (d *Desktop) recordStop() (Result, error) {
	d.mu.Lock()
	rec, out := d.recorder, d.video
	d.mu.Unlock()
	if !rec.alive() {
		return Result{}, errors.New("no se está grabando")
	}
	d.stopRecorder(rec)
	d.mu.Lock()
	d.recorder = nil
	d.mu.Unlock()
	return Result{Path: out, Text: "grabación guardada: " + out}, nil
}

func (d *Desktop) stopRecorder(rec *proc) {
	if !rec.alive() {
		return
	}
	d.mu.Lock()
	in := d.recStdin
	d.mu.Unlock()
	if in != nil {
		in.Write([]byte("q"))
		in.Close()
	}
	select {
	case <-rec.done:
	case <-time.After(5 * time.Second):
		stop(rec, stopTimeout)
	}
}

// statusText: the state for the agent.
func statusText(s Status) string {
	state := map[string]string{StateStarting: "iniciando sesión", StateConnected: "conectado", StateEnded: "terminado"}[s.State]
	var b strings.Builder
	fmt.Fprintf(&b, "Escritorio del agente: %s\n", state)
	if s.Error != "" {
		fmt.Fprintf(&b, "Motivo: %s\n", s.Error)
	}
	fmt.Fprintf(&b, "Equipo: %s como %s\nTamaño: %dx%d\nTeclado: %s\n", s.Host, s.User, s.Width, s.Height, s.Keyboard)
	if s.Recording {
		fmt.Fprintf(&b, "Grabando en: %s\n", s.Video)
	}
	fmt.Fprintf(&b, "Carpeta de intercambio: %s (en Windows, \\\\tsclient\\Intercambio)\n", s.Exchange)
	return b.String()
}

// tailBuffer keeps the last bytes of FreeRDP's output (its error codes).
type tailBuffer struct {
	mu  sync.Mutex
	b   []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = append([]byte(nil), t.b[len(t.b)-t.max:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}
