package servidor

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/imputacion"
	"planesgo-code/internal/proceso"
	"planesgo-code/internal/sesiones"
)

// Remote desktop (pgocode --rdp): the native FreeRDP window (SDL3) on a connection of a project, opened from the
// interface with the user's pgocode session. The time goes to the project with the imputation service, like the
// rest of pgocode: activity while the session is connected and the keyboard or mouse are in use.

const (
	rdpBeat      = 20 * time.Second // activity reported to the imputation service
	rdpFirstBeat = 10 * time.Second // connected this long before counting (a failed login logs nothing)
	rdpIdleLimit = 10 * time.Minute // without keyboard or mouse: no more time
)

// rdpConn: an open remote desktop.
type rdpConn struct {
	ID       string         `json:"id"`
	Project  int            `json:"proyecto"`
	Name     string         `json:"nombre"`
	Host     string         `json:"equipo"`
	User     string         `json:"usuario"`
	Started  time.Time      `json:"inicio"`
	State    string         `json:"estado"` // conectando, conectado, terminado
	Error    string         `json:"error,omitempty"`
	closed   bool           // the person disconnected it from pgocode: not an error
	title    io.WriteCloser // descriptor 3 of FreeRDP: one line per window title (connectors/freerdp); nil on Windows
	cancel   context.CancelFunc
	finished chan struct{}
}

type rdpSessions struct {
	mu    sync.Mutex
	conns map[string]*rdpConn
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

// rdpClient: the FreeRDP client (PGO_FREERDP, or the one connectors/instala.sh builds).
func rdpClient() string {
	if p := os.Getenv("PGO_FREERDP"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		p, _ := exec.LookPath("sdl-freerdp.exe")
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "opt", "rdp-agente", "freerdp", "bin", "sdl-freerdp")
}

// rdpExchange: the shared folder (in Windows, «Intercambio»).
func rdpExchange() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "pgocode-connectors", "intercambio")
}

// rdpUser splits «DOMINIO\usuario» (or just «usuario»).
func rdpUser(s string) (user, domain string) {
	if d, u, ok := strings.Cut(strings.TrimSpace(s), `\`); ok {
		return strings.TrimSpace(u), strings.TrimSpace(d)
	}
	return strings.TrimSpace(s), ""
}

// rdpArgs: modern settings only, no fallbacks for old servers (a development tool, not a product). Everything is
// allowed (clipboard, the shared folder «Intercambio», sound, microphone and this computer's printers) and the
// person decides what to use; full screen whenever wanted (Ctrl+Alt+Intro in the window).
func rdpArgs(name, host, login string) []string {
	user, domain := rdpUser(login)
	a := []string{"/v:" + host, "/u:" + user, "/from-stdin:force", "/title:" + name,
		"/sec:nla",        // network level authentication only
		"/tls:seclevel:2", // no old TLS versions or weak keys
		"/cert:tofu",      // trust on first use; warn if the certificate changes
		"/gfx:AVC444",     // RDP 8+ graphics with H.264 4:4:4
		"/network:auto", "+auto-reconnect", "/auto-reconnect-max-retries:20", "+dynamic-resolution", "/log-level:ERROR",
		// Without grabbing the keyboard: GNOME asks every time for permission to inhibit its shortcuts. Alt+Tab and
		// the Windows key stay on this desktop.
		"-grab-keyboard"}
	if domain != "" {
		a = append(a, "/d:"+domain)
	}
	a = append(a, "/size:85%", "+clipboard", "/sound:sys:pulse", "/microphone:sys:pulse", "/printer")
	if os.MkdirAll(rdpExchange(), 0o755) == nil {
		a = append(a, "/drive:Intercambio,"+rdpExchange())
	}
	return a
}

// rdpMessages: FreeRDP's ERRCONNECT_* codes (in its log) and what to tell the person.
var rdpMessages = []struct {
	codes []string
	text  string
}{
	{[]string{"LOGON_FAILURE", "WRONG_PASSWORD", "AUTHENTICATION_FAILED", "NO_OR_MISSING_CREDENTIALS"}, "Usuario o contraseña incorrectos."},
	{[]string{"PASSWORD_EXPIRED", "PASSWORD_CERTAINLY_EXPIRED", "PASSWORD_MUST_CHANGE"}, "La contraseña ha caducado o hay que cambiarla: cámbiala desde Windows."},
	{[]string{"ACCOUNT_LOCKED_OUT"}, "La cuenta está bloqueada (demasiados intentos fallidos)."},
	{[]string{"ACCOUNT_DISABLED", "ACCOUNT_EXPIRED", "ACCOUNT_RESTRICTION"}, "La cuenta está deshabilitada, caducada o restringida."},
	{[]string{"ACCESS_DENIED", "INSUFFICIENT_PRIVILEGES", "LOGON_TYPE_NOT_GRANTED"}, "Este usuario no tiene permiso para entrar por escritorio remoto en ese equipo."},
	{[]string{"DNS_ERROR", "DNS_NAME_NOT_FOUND"}, "No se encuentra el equipo: revisa el nombre."},
	{[]string{"CONNECT_FAILED", "CONNECT_TRANSPORT_FAILED", "TARGET_BOOTING"}, "No se puede llegar al equipo: revisa el nombre o la IP y que el escritorio remoto esté activo."},
	{[]string{"TLS_CONNECT_FAILED", "SECURITY_NEGO_CONNECT_FAILED", "HYBRID_REQUIRED_BY_SERVER"}, "No se ha podido negociar la seguridad de la conexión con el servidor."},
	{[]string{"KDC_UNREACHABLE"}, "No se llega al controlador de dominio (Kerberos)."},
}

// rdpEnded: why Windows ended the session (FreeRDP's ERRINFO_*): "" when the person closed or signed out.
var rdpEnded = []struct {
	code, text string
}{
	{"RPC_INITIATED_DISCONNECT", ""}, {"RPC_INITIATED_LOGOFF", ""}, {"LOGOFF_BY_USER", ""}, // also *_BY_USER
	{"DISCONNECTED_BY_OTHER_CONNECTION", "Se ha abierto esta misma sesión de Windows desde otra conexión."},
	{"IDLE_TIMEOUT", "Windows ha cerrado la sesión por inactividad."},
	{"LOGON_TIMEOUT", "Windows ha cerrado la sesión porque no se completó el inicio de sesión a tiempo."},
	{"SERVER_DENIED_CONNECTION", "El servidor ha rechazado la conexión."},
	{"SERVER_INSUFFICIENT_PRIVILEGES", "Este usuario no tiene permiso para entrar por escritorio remoto en ese equipo."},
}

// rdpExplain: why the session ended ("" if it ended normally: closed by the person or by Windows).
func rdpExplain(log string, code int) string {
	t := strings.ToUpper(log)
	if strings.Contains(t, "ERRCONNECT_CONNECT_CANCELLED") {
		return ""
	}
	for _, e := range rdpEnded {
		if strings.Contains(t, "ERRINFO_"+e.code) {
			return e.text
		}
	}
	if strings.Contains(t, "CERTIFICATE") && (strings.Contains(t, "CHANGED") || strings.Contains(t, "MISMATCH")) {
		return "El certificado del servidor ha cambiado desde la última conexión. Puede ser normal (se renovó) o un intento de suplantación: compruébalo con quien administra el servidor."
	}
	for _, m := range rdpMessages {
		for _, c := range m.codes {
			if strings.Contains(t, "ERRCONNECT_"+c) {
				return m.text
			}
		}
	}
	switch code {
	case 0, 1, 2, 12, 13: // closed normally, logoff or disconnect
		return ""
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(log), "\n") {
		// Kerberos is tried first and, without a reachable KDC for the domain, NTLM follows: not the error
		// fusermount3: copying files through the clipboard needs FUSE (text works without it)
		if l != "" && !strings.Contains(l, "sspi.Kerberos") && !strings.Contains(l, "default realm") && !strings.Contains(l, "fusermount") {
			lines = append(lines, l)
		}
	}
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	return fmt.Sprintf("La conexión terminó con un error (código %d).\n%s", code, strings.Join(lines, "\n"))
}

var idleRe = regexp.MustCompile(`(\d+)`)

// rdpIdle: time without keyboard or mouse on this desktop (GNOME); ok false if unknown.
func rdpIdle() (time.Duration, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gdbus", "call", "--session", "--dest", "org.gnome.Mutter.IdleMonitor",
		"--object-path", "/org/gnome/Mutter/IdleMonitor/Core", "--method", "org.gnome.Mutter.IdleMonitor.GetIdletime").Output()
	m := idleRe.FindSubmatch(out)
	if err != nil || m == nil {
		return 0, false
	}
	ms, _ := strconv.ParseInt(string(m[1]), 10, 64)
	return time.Duration(ms) * time.Millisecond, true
}

// rdpState: GET /api/rdp. Whether the client is installed and the open remote desktops.
func (s *Servidor) rdpState(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	bin := rdpClient()
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
	if u, _ := rdpUser(p.User); p.Host == "" || u == "" || strings.ContainsAny(p.Host+p.User, " \n\r") {
		jsonError(w, http.StatusBadRequest, "hacen falta el equipo y el usuario (DOMINIO\\usuario o usuario, sin espacios)")
		return
	}
	if p.Name == "" {
		p.Name = p.Host
	}
	bin := rdpClient()
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
	// Nothing of pgocode's (tokens, desktop secret) reaches FreeRDP. Its title (the logged time) goes through its
	// descriptor 3 (PGO_TITULO_FD, the patch in connectors/freerdp), not its input: a new credentials prompt would
	// read a title as the password. ExtraFiles does not exist on Windows.
	cmd.Env = proceso.CleanEnviron()
	if runtime.GOOS != "windows" {
		pr, pw, err := os.Pipe()
		if err != nil {
			cancel()
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer pr.Close() // the child has its own copy once started
		cmd.ExtraFiles = []*os.File{pr}
		cmd.Env = append(cmd.Env, "PGO_TITULO_FD=3")
		c.title = pw
	}
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
	stdin.Write([]byte(p.Password + "\n"))
	stdin.Close()
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
			c.Error = rdpExplain(log.String(), code)
		}
		s.escritorios.mu.Unlock()
		close(c.finished)
		if c.title != nil {
			c.title.Close()
		}
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

// rdpTitle: the window title with the logged time, as pgocode's bar shows it: this session, the project today and
// everything today.
func (s *Servidor) rdpTitle(c *rdpConn, work imputacion.Trabajo) string {
	title := c.Name
	if work.NombreProy != "" {
		title += " · " + work.NombreProy
	}
	if s.Imputacion == nil {
		return title
	}
	hoy, abiertos := s.Imputacion.Estado()
	var session, project, total float64
	for _, a := range abiertos {
		if a.Sesion == work.Sesion {
			session = a.Horas
		}
	}
	for _, l := range hoy {
		total += l.Horas
		if l.Proyecto == c.Project {
			project += l.Horas
		}
	}
	hm := func(h float64) string { m := int(h*60 + 0.5); return fmt.Sprintf("%d:%02d", m/60, m%60) }
	return fmt.Sprintf("%s · sesión %s · proyecto hoy %s · total hoy %s", title, hm(session), hm(project), hm(total))
}

// rdpTime reports the remote desktop's activity to the imputation service while it is connected and in use, and
// shows the logged time in the window title.
func (s *Servidor) rdpTime(c *rdpConn, work imputacion.Trabajo) {
	t := time.NewTicker(rdpBeat)
	defer t.Stop()
	show := func() {
		if c.title != nil {
			fmt.Fprintln(c.title, s.rdpTitle(c, work))
		}
	}
	show()
	for {
		select {
		case <-c.finished:
			return
		case <-t.C:
		}
		if time.Since(c.Started) < rdpFirstBeat {
			continue
		}
		s.escritorios.mu.Lock()
		if c.State == "conectando" {
			c.State = "conectado"
		}
		s.escritorios.mu.Unlock()
		if idle, known := rdpIdle(); known && idle >= rdpIdleLimit {
			continue // nobody at the keyboard: no time (the service closes the period after a while)
		}
		if s.Imputacion != nil {
			s.Imputacion.Actividad(work)
		}
		show()
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
