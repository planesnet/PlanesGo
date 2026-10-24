package servidor

import (
	"context"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

// Pictures of the program itself (the evidence of an automatic PSF session): the project's command (e.g.
// «go run ./cmd/tetris -auto») runs for a few seconds in a virtual terminal (util-linux «script») in the issue
// worktree, and its screen is drawn, with its colours, at a few moments. No tokens. A small VT100 emulator is enough
// for most terminal programs: cursor moves, erasing and colours (8, 256 and 24 bits).

const (
	termRows = 30
	termCols = 90
)

type termCell struct {
	r    rune
	fg   string
	bg   string
	bold bool
}

// vtScreen: the terminal screen, fed with the program output.
type vtScreen struct {
	rows, cols int
	cells      [][]termCell
	x, y       int
	fg, bg     string
	bold       bool
	pending    []byte // an escape sequence or UTF-8 rune split across chunks
}

func newVTScreen(rows, cols int) *vtScreen {
	s := &vtScreen{rows: rows, cols: cols}
	s.clear()
	return s
}

func (s *vtScreen) clear() {
	s.cells = make([][]termCell, s.rows)
	for y := range s.cells {
		s.cells[y] = make([]termCell, s.cols)
	}
}

func (s *vtScreen) newline() {
	s.y++
	if s.y >= s.rows { // scroll
		s.cells = append(s.cells[1:], make([]termCell, s.cols))
		s.y = s.rows - 1
	}
}

// Write feeds output bytes into the screen.
func (s *vtScreen) Write(b []byte) {
	b = append(s.pending, b...)
	s.pending = nil
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == 0x1b:
			n, ok := s.escape(b[i:])
			if !ok {
				s.pending = append([]byte(nil), b[i:]...)
				return
			}
			i += n
			continue
		case c == '\r':
			s.x = 0
		case c == '\n':
			s.newline()
		case c == '\b':
			if s.x > 0 {
				s.x--
			}
		case c == '\t':
			s.x = min(s.cols-1, (s.x/8+1)*8)
		case c < 0x20:
		default:
			r, n := utf8.DecodeRune(b[i:])
			if r == utf8.RuneError && n == 1 && !utf8.FullRune(b[i:]) {
				s.pending = append([]byte(nil), b[i:]...)
				return
			}
			if s.x >= s.cols {
				s.x = 0
				s.newline()
			}
			s.cells[s.y][s.x] = termCell{r, s.fg, s.bg, s.bold}
			s.x++
			i += n
			continue
		}
		i++
	}
}

// escape handles one escape sequence at the start of b: its length, or false if it is not complete yet.
func (s *vtScreen) escape(b []byte) (int, bool) {
	if len(b) < 2 {
		return 0, false
	}
	switch b[1] {
	case '[':
	case ']': // OSC: up to BEL or ST
		for j := 2; j < len(b); j++ {
			if b[j] == 0x07 {
				return j + 1, true
			}
			if b[j] == 0x1b && j+1 < len(b) && b[j+1] == '\\' {
				return j + 2, true
			}
		}
		return 0, false
	default:
		return 2, true
	}
	j := 2
	for j < len(b) && (b[j] < 0x40 || b[j] > 0x7e) {
		j++
	}
	if j >= len(b) {
		return 0, false
	}
	params, final := string(b[2:j]), b[j]
	if strings.HasPrefix(params, "?") { // private modes (cursor visibility…): nothing to draw
		return j + 1, true
	}
	nums := []int{}
	for _, p := range strings.Split(params, ";") {
		n, _ := strconv.Atoi(p)
		nums = append(nums, n)
	}
	arg := func(i, def int) int {
		if i < len(nums) && nums[i] > 0 {
			return nums[i]
		}
		return def
	}
	switch final {
	case 'H', 'f':
		s.y, s.x = min(s.rows-1, arg(0, 1)-1), min(s.cols-1, arg(1, 1)-1)
	case 'A':
		s.y = max(0, s.y-arg(0, 1))
	case 'B':
		s.y = min(s.rows-1, s.y+arg(0, 1))
	case 'C':
		s.x = min(s.cols-1, s.x+arg(0, 1))
	case 'D':
		s.x = max(0, s.x-arg(0, 1))
	case 'G':
		s.x = min(s.cols-1, arg(0, 1)-1)
	case 'J':
		if len(nums) > 0 && nums[0] >= 2 {
			s.clear()
		} else {
			for x := s.x; x < s.cols; x++ {
				s.cells[s.y][x] = termCell{}
			}
			for y := s.y + 1; y < s.rows; y++ {
				s.cells[y] = make([]termCell, s.cols)
			}
		}
	case 'K':
		for x := s.x; x < s.cols; x++ {
			s.cells[s.y][x] = termCell{}
		}
	case 'm':
		s.sgr(nums)
	}
	return j + 1, true
}

var ansi16 = []string{"#1e1e1e", "#cd3131", "#0dbc79", "#e5e510", "#2472c8", "#bc3fbc", "#11a8cd", "#e5e5e5",
	"#666666", "#f14c4c", "#23d18b", "#f5f543", "#3b8eea", "#d670d6", "#29b8db", "#ffffff"}

// color256: the xterm 256-colour palette.
func color256(n int) string {
	switch {
	case n < 16:
		return ansi16[n]
	case n < 232:
		n -= 16
		lv := []int{0, 95, 135, 175, 215, 255}
		return fmt.Sprintf("#%02x%02x%02x", lv[n/36], lv[n/6%6], lv[n%6])
	default:
		g := 8 + (n-232)*10
		return fmt.Sprintf("#%02x%02x%02x", g, g, g)
	}
}

func (s *vtScreen) sgr(n []int) {
	if len(n) == 0 {
		n = []int{0}
	}
	for i := 0; i < len(n); i++ {
		switch c := n[i]; {
		case c == 0:
			s.fg, s.bg, s.bold = "", "", false
		case c == 1:
			s.bold = true
		case c == 22:
			s.bold = false
		case c >= 30 && c <= 37:
			s.fg = ansi16[c-30]
		case c >= 90 && c <= 97:
			s.fg = ansi16[c-90+8]
		case c == 39:
			s.fg = ""
		case c >= 40 && c <= 47:
			s.bg = ansi16[c-40]
		case c >= 100 && c <= 107:
			s.bg = ansi16[c-100+8]
		case c == 49:
			s.bg = ""
		case c == 38 || c == 48:
			col := ""
			if i+2 < len(n) && n[i+1] == 5 {
				col, i = color256(n[i+2]), i+2
			} else if i+4 < len(n) && n[i+1] == 2 {
				col, i = fmt.Sprintf("#%02x%02x%02x", n[i+2]&255, n[i+3]&255, n[i+4]&255), i+4
			}
			if c == 38 {
				s.fg = col
			} else {
				s.bg = col
			}
		}
	}
}

// SVG draws the screen as a terminal window, with a caption.
func (s *vtScreen) SVG(caption string) []byte {
	const cw, ch, pad, top = 8.4, 17.0, 14.0, 34.0
	w, h := pad*2+cw*float64(s.cols), top+pad+ch*float64(s.rows)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family="ui-monospace, Menlo, DejaVu Sans Mono, monospace" font-size="14">`, w, h, w, h)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" rx="8" fill="#1e1e1e"/><circle cx="16" cy="16" r="5" fill="#ff5f57"/><circle cx="32" cy="16" r="5" fill="#febc2e"/><circle cx="48" cy="16" r="5" fill="#28c840"/>`)
	fmt.Fprintf(&b, `<text x="64" y="20" font-family="system-ui, sans-serif" font-size="12" fill="#a0a0a0">%s</text>`, html.EscapeString(caption))
	for y, row := range s.cells {
		by := top + ch*float64(y)
		for x := 0; x < len(row); {
			c := row[x]
			if c.bg != "" { // background runs
				e := x
				for e < len(row) && row[e].bg == c.bg {
					e++
				}
				fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`, pad+cw*float64(x), by, cw*float64(e-x), ch, c.bg)
				x = e
				continue
			}
			x++
		}
		for x := 0; x < len(row); { // text runs of one colour
			c := row[x]
			if c.r == 0 || c.r == ' ' {
				x++
				continue
			}
			e, txt := x, strings.Builder{}
			for e < len(row) && row[e].fg == c.fg && row[e].bold == c.bold && row[e].r != 0 {
				txt.WriteRune(row[e].r)
				e++
			}
			fg := c.fg
			if fg == "" {
				fg = "#e5e5e5"
			}
			weight := ""
			if c.bold {
				weight = ` font-weight="700"`
			}
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s"%s xml:space="preserve" textLength="%.1f" lengthAdjust="spacingAndGlyphs">%s</text>`,
				pad+cw*float64(x), by+ch-4, fg, weight, cw*float64(e-x), html.EscapeString(txt.String()))
			x = e
		}
	}
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

// captureProgram runs the command in dir inside a virtual terminal for the given time and returns the screen at
// each moment (seconds from the start). The program is stopped at the end (its whole process group).
func captureProgram(ctx context.Context, dir, command string, moments []float64) ([][]byte, error) {
	if strings.TrimSpace(command) == "" || len(moments) == 0 {
		return nil, fmt.Errorf("falta la orden del programa")
	}
	if _, err := exec.LookPath("script"); err != nil {
		return nil, fmt.Errorf("falta «script» (util-linux) para el terminal virtual")
	}
	total := moments[len(moments)-1]
	ctx, cancel := context.WithTimeout(ctx, time.Duration(total*float64(time.Second))+20*time.Second)
	defer cancel()
	inner := fmt.Sprintf("stty rows %d cols %d 2>/dev/null; exec sh -c %s", termRows, termCols, shellQuote(command))
	cmd := exec.CommandContext(ctx, "script", "-qfec", inner, "/dev/null")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor", fmt.Sprintf("COLUMNS=%d", termCols), fmt.Sprintf("LINES=%d", termRows))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, _ := cmd.StdinPipe() // kept open: the program waits for keys that never come
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	stop := func() {
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		stdin.Close()
	}
	defer stop()
	var mu sync.Mutex
	var buf []byte
	done := make(chan struct{})
	go func() {
		defer close(done)
		chunk := make([]byte, 32<<10)
		for {
			n, err := out.Read(chunk)
			mu.Lock()
			buf = append(buf, chunk[:n]...)
			mu.Unlock()
			if err != nil {
				if err != io.EOF {
					return
				}
				return
			}
		}
	}()
	screen := newVTScreen(termRows, termCols)
	fed := 0
	start := time.Now()
	var shots [][]byte
	for _, m := range moments {
		wait := time.Until(start.Add(time.Duration(m * float64(time.Second))))
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return shots, ctx.Err()
		}
		mu.Lock()
		screen.Write(buf[fed:])
		fed = len(buf)
		mu.Unlock()
		shots = append(shots, screen.SVG(fmt.Sprintf("%s · %.0f s", command, m)))
	}
	stop()
	<-done
	cmd.Wait()
	return shots, nil
}

// shellQuote: a word for sh, in single quotes.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
