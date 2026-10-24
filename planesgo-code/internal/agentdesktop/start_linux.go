//go:build linux

package agentdesktop

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"planesgo-code/internal/freerdp"
)

// start: the private folder with the X cookie, the virtual screen and FreeRDP with the password on its input.
func (d *Desktop) start(password string) error {
	base := d.opts.RuntimeDir
	if base == "" {
		run := os.Getenv("XDG_RUNTIME_DIR")
		if run == "" {
			return errors.New("falta XDG_RUNTIME_DIR: el escritorio del agente no guarda nada en /tmp")
		}
		base = filepath.Join(run, "rdp-agente")
	}
	if err := privateDir(base); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(base, "escritorio-")
	if err != nil {
		return err
	}
	d.dir, d.xauth, d.pipe = dir, filepath.Join(dir, "Xauthority"), filepath.Join(dir, "teclado.fifo")
	cookie := make([]byte, 16)
	if _, err := rand.Read(cookie); err != nil {
		return err
	}
	if err := os.WriteFile(d.xauth, xauthority(cookie), 0o600); err != nil {
		return err
	}
	if err := d.startScreen(); err != nil {
		return err
	}
	var stdin interface {
		Write([]byte) (int, error)
		Close() error
	}
	client, err := d.launch(d.tools.XFreeRDP, clientArgs(d.cfg, d.pipe, d.opts.Exchange), func(c *exec.Cmd) {
		c.Stdout, c.Stderr = d.log, d.log
		if in, err := c.StdinPipe(); err == nil {
			stdin = in
		}
	})
	if err != nil {
		return fmt.Errorf("no arranca FreeRDP: %v", err)
	}
	d.mu.Lock()
	d.client = client
	d.mu.Unlock()
	if stdin != nil {
		freerdp.SendPassword(stdin, password)
	}
	go d.clientEnded()
	return nil
}

// startScreen launches Xvfb and waits until it says (through -displayfd) which display it serves.
func (d *Desktop) startScreen() error {
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	defer r.Close()
	d.display = "" // Xvfb's environment: no display of its own
	p, err := d.launch(d.tools.Xvfb, xvfbArgs(d.opts.Display, d.xauth), func(c *exec.Cmd) { c.ExtraFiles = []*os.File{w} })
	w.Close()
	if err != nil {
		return fmt.Errorf("no arranca Xvfb: %v", err)
	}
	d.mu.Lock()
	d.xvfb = p
	d.mu.Unlock()
	line := make(chan string, 1)
	go func() {
		s, _ := bufio.NewReader(r).ReadString('\n')
		line <- strings.TrimSpace(s)
	}()
	select {
	case s := <-line:
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			d.display = ":" + strconv.Itoa(n)
			return nil
		}
		return errors.New("Xvfb no ha arrancado (¿está ocupada esa pantalla virtual?)")
	case <-time.After(startTimeout):
		return errors.New("Xvfb no ha arrancado a tiempo")
	}
}

// privateDir creates (if needed) and checks a runtime folder: ours and 700. No /tmp fallback: there another user
// could create it first and plant a socket or a pipe.
func privateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !st.IsDir() || !ok || int(sys.Uid) != os.Getuid() || st.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s: no es una carpeta propia con permisos 700; no se usa", path)
	}
	return nil
}

// terminate asks a program to end (SIGTERM: FreeRDP disconnects cleanly).
func terminate(p *os.Process) { p.Signal(syscall.SIGTERM) }

// writePipe sends text to FreeRDP's keyboard pipe (without blocking if FreeRDP is not reading it).
func writePipe(path, text string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("no se puede escribir en el teclado de FreeRDP: %v", err)
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}
