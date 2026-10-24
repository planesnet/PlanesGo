package servidor

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/freerdp"
	"planesgo-code/internal/imputacion"
	"planesgo-code/internal/proceso"
	"planesgo-code/internal/sesiones"
)

// Remote desktop (pgocode --rdp): the native FreeRDP window (SDL3) on a connection of a project, opened from the
// interface with the user's pgocode session. The time goes to the project with the imputation service, like the
// rest of pgocode: activity while the session is connected and the keyboard or mouse are in use.

// rdpConn: an open remote desktop.
type rdpConn struct {
	ID       string    `json:"id"`
	Project  int       `json:"proyecto"`
	Name     string    `json:"nombre"`
	Host     string    `json:"equipo"`
	User     string    `json:"usuario"`
	Started  time.Time `json:"inicio"`
	State    string    `json:"estado"` // conectando, conectado, terminado
	Error    string    `json:"error,omitempty"`
	closed   bool      // the person disconnected it from pgocode: not an error
	cancel   context.CancelFunc
	finished chan struct{}
}

type rdpSessions struct {
	mu    sync.Mutex
	conns map[string]*rdpConn
}

// closeAll closes every open remote desktop (pgocode is closing: nobody would count its time any more).
func (r *rdpSessions) closeAll() {
	r.mu.Lock()
	var l []*rdpConn
	for _, c := range r.conns {
		if c.State != "terminado" {
			c.closed = true
			l = append(l, c)
		}
	}
	r.mu.Unlock()
	for _, c := range l {
		c.cancel()
		select {
		case <-c.finished: // its period is closed by the goroutine that waits for FreeRDP
		case <-time.After(3 * time.Second):
		}
	}
}

func (r *rdpSessions) open() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.conns {
		if c.State != "terminado" {
			n++
		}
	}
	return n
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

// rdpState: GET /api/rdp. Whether the client is installed and the open remote desktops.
func (s *Servidor) rdpState(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	bin := freerdp.SDLClient()
	_, err := os.Stat(bin)
	s.escritorios.mu.Lock()
	l := []rdpConn{}
	for _, c := range s.escritorios.conns {
		l = append(l, *c)
	}
	s.escritorios.mu.Unlock()
	jsonOK(w, map[string]any{"disponible": bin != "" && err == nil, "conexiones": l})
}

// rdpConnect: POST /api/rdp/conectar. Opens the remote desktop of a connection on a project (mandatory: its time
// goes there). The password goes to FreeRDP's input and is not kept.
func (s *Servidor) rdpConnect(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Project  int    `json:"proyecto"`
		Name     string `json:"nombre"`
		Host     string `json:"equipo"`
		User     string `json:"usuario"` // «DOMINIO\usuario» or «usuario»
		Password string `json:"contrasena"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	projectName, ok := s.proyecto(w, r, ses, p.Project)
	if !ok {
		return
	}
	p.Host, p.User, p.Name = strings.TrimSpace(p.Host), strings.TrimSpace(p.User), strings.TrimSpace(p.Name)
	if err := freerdp.ValidateTarget(p.Host, p.User); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if p.Name == "" {
		p.Name = p.Host
	}
	bin := freerdp.SDLClient()
	if _, err := os.Stat(bin); bin == "" || err != nil {
		jsonError(w, http.StatusServiceUnavailable, "falta FreeRDP: ejecuta connectors/instala.sh")
		return
	}
	b := make([]byte, 6)
	rand.Read(b)
	c := &rdpConn{ID: hex.EncodeToString(b), Project: p.Project, Name: p.Name, Host: p.Host, User: p.User,
		Started: time.Now(), State: "conectando", finished: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	cmd := exec.CommandContext(ctx, bin, rdpArgs(p.Name, p.Host, p.User)...)
	proceso.GrupoPropio(cmd)
	cmd.Env = proceso.CleanEnviron() // nothing of pgocode's (tokens, desktop secret) reaches FreeRDP
	var log bytes.Buffer
	cmd.Stdout, cmd.Stderr = &log, &log
	stdin, err := cmd.StdinPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		cancel()
		jsonError(w, http.StatusInternalServerError, "no arranca FreeRDP: "+err.Error())
		return
	}
	freerdp.SendPassword(stdin, p.Password)
	p.Password = ""
	s.escritorios.mu.Lock()
	if s.escritorios.conns == nil {
		s.escritorios.conns = map[string]*rdpConn{}
	}
	s.escritorios.conns[c.ID] = c
	s.escritorios.mu.Unlock()
	s.Eventos.Emite(map[string]string{"tipo": "rdp"})

	work := imputacion.Trabajo{Sesion: "rdp-" + c.ID, Titulo: "Escritorio remoto: " + p.Name, Proyecto: p.Project,
		NombreProy: projectName, UID: ses.UID, OdooSesion: ses.OdooSesion, Modelo: "Escritorio remoto",
		Desc: imputacion.Descripcion("Escritorio remoto en "+p.Name+" ("+p.Host+") como "+p.User, "trabajo en el escritorio remoto")}
	go s.rdpTime(c, work)
	go func() {
		err := cmd.Wait()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		s.escritorios.mu.Lock()
		c.State = "terminado"
		if !c.closed {
			c.Error = freerdp.Explain(log.String(), code)
		}
		s.escritorios.mu.Unlock()
		close(c.finished)
		cancel()
		if s.Imputacion != nil {
			s.Imputacion.Cierra(work.Sesion, "escritorio remoto cerrado")
		}
		s.Eventos.Emite(map[string]string{"tipo": "rdp"})
	}()
	s.escritorios.mu.Lock()
	snap := *c // the goroutines above may already be changing it
	s.escritorios.mu.Unlock()
	jsonOK(w, snap)
}

// rdpTime reports the remote desktop's activity to the imputation service while it is connected and in use.
func (s *Servidor) rdpTime(c *rdpConn, work imputacion.Trabajo) {
	t := time.NewTicker(remoteBeat)
	defer t.Stop()
	for {
		select {
		case <-c.finished:
			return
		case <-t.C:
		}
		if time.Since(c.Started) < remoteFirstBeat {
			continue
		}
		s.escritorios.mu.Lock()
		if c.State == "conectando" {
			c.State = "conectado"
		}
		s.escritorios.mu.Unlock()
		if !s.remoteActivity(work, keyboardInUse()) {
			continue
		}
	}
}

// rdpDisconnect: POST /api/rdp/desconectar {id}. Closes the remote desktop window.
func (s *Servidor) rdpDisconnect(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		ID string `json:"id"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	s.escritorios.mu.Lock()
	c := s.escritorios.conns[p.ID]
	if c != nil {
		c.closed = true
	}
	s.escritorios.mu.Unlock()
	if c == nil {
		jsonError(w, http.StatusNotFound, "no hay ese escritorio remoto")
		return
	}
	c.cancel()
	jsonOK(w, map[string]bool{"ok": true})
}
