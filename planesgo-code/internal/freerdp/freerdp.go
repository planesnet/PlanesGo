// Package freerdp holds what pgocode's two FreeRDP clients share: the person's remote desktop (the SDL3 window,
// internal/servidor/rdp.go) and the agent's one (the X11 client on a virtual screen, internal/agentdesktop). Where
// connectors/instala.sh leaves FreeRDP and its tools, the modern-only security settings, the shared folder, the
// password through the standard input and FreeRDP's error codes in a sentence.
package freerdp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// InstallDir: where connectors/instala.sh installs FreeRDP (freerdp/) and the Ubuntu tools it unpacks (root/).
func InstallDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "opt", "rdp-agente")
}

// SDLClient: the person's client (PGO_FREERDP, or the one connectors/instala.sh builds).
func SDLClient() string {
	if p := os.Getenv("PGO_FREERDP"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		p, _ := exec.LookPath("sdl-freerdp.exe")
		return p
	}
	return filepath.Join(InstallDir(), "freerdp", "bin", "sdl-freerdp")
}

// X11Client: the agent's client (an X11 window on the virtual screen).
func X11Client() string { return filepath.Join(InstallDir(), "freerdp", "bin", "xfreerdp") }

// ExchangeDir: the shared folder (in Windows, \\tsclient\Intercambio).
func ExchangeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "pgocode-connectors", "intercambio")
}

// DriveArgs: the shared folder for FreeRDP (nothing if it cannot be created).
func DriveArgs() []string {
	if os.MkdirAll(ExchangeDir(), 0o755) != nil {
		return nil
	}
	return []string{"/drive:Intercambio," + ExchangeDir()}
}

// SplitUser splits «DOMINIO\usuario» (or just «usuario»).
func SplitUser(s string) (user, domain string) {
	if d, u, ok := strings.Cut(strings.TrimSpace(s), `\`); ok {
		return strings.TrimSpace(u), strings.TrimSpace(d)
	}
	return strings.TrimSpace(s), ""
}

var (
	safeHost = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-_]*(?::\d{1,5})?$`)
	safeUser = regexp.MustCompile(`^[^\s\\/:,"'<>|*?\x00-\x1f-][^\s\\/:,"'<>|*?\x00-\x1f]{0,103}$`)
)

// ValidateTarget checks the computer (name or IP, optionally :port) and the user («DOMINIO\usuario» or
// «usuario») before they become FreeRDP arguments: nothing that FreeRDP could read as another option.
func ValidateTarget(host, login string) error {
	if !safeHost.MatchString(strings.TrimSpace(host)) {
		return errors.New("equipo no válido (nombre o IP, y si hace falta :puerto)")
	}
	user, domain := SplitUser(login)
	if !safeUser.MatchString(user) || (domain != "" && !safeUser.MatchString(domain)) || strings.Count(login, `\`) > 1 {
		return errors.New("usuario no válido (DOMINIO\\usuario o usuario, sin espacios)")
	}
	return nil
}

// SecureArgs: the connection with modern settings only, no fallbacks for old servers (a development tool, not a
// product). The password is not here: it goes through the standard input (SendPassword).
func SecureArgs(host, login string) []string {
	user, domain := SplitUser(login)
	a := []string{"/v:" + strings.TrimSpace(host), "/u:" + user, "/from-stdin:force",
		"/sec:nla",        // network level authentication only
		"/tls:seclevel:2", // no old TLS versions or weak keys
		"/cert:tofu",      // trust on first use; warn if the certificate changes
		"/gfx:AVC444",     // RDP 8+ graphics with H.264 4:4:4
		"/network:auto"}
	if domain != "" {
		a = append(a, "/d:"+domain)
	}
	return a
}

// SendPassword writes the password to FreeRDP's standard input and closes it: it reads nothing else.
func SendPassword(stdin io.WriteCloser, password string) error {
	_, err := io.WriteString(stdin, password+"\n")
	if cerr := stdin.Close(); err == nil {
		err = cerr
	}
	return err
}

// messages: FreeRDP's ERRCONNECT_* codes (in its log) and what to tell the person. The first credentialMessages are
// about the user and password.
const credentialMessages = 5

var messages = []struct {
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

// ended: why Windows ended the session (FreeRDP's ERRINFO_*): "" when the person closed or signed out.
var ended = []struct {
	code, text string
}{
	{"RPC_INITIATED_DISCONNECT", ""}, {"RPC_INITIATED_LOGOFF", ""}, {"LOGOFF_BY_USER", ""}, // also *_BY_USER
	{"DISCONNECTED_BY_OTHER_CONNECTION", "Se ha abierto esta misma sesión de Windows desde otra conexión."},
	{"IDLE_TIMEOUT", "Windows ha cerrado la sesión por inactividad."},
	{"LOGON_TIMEOUT", "Windows ha cerrado la sesión porque no se completó el inicio de sesión a tiempo."},
	{"SERVER_DENIED_CONNECTION", "El servidor ha rechazado la conexión."},
	{"SERVER_INSUFFICIENT_PRIVILEGES", "Este usuario no tiene permiso para entrar por escritorio remoto en ese equipo."},
}

// Explain: why the session ended, from FreeRDP's log and exit code ("" if it ended normally: closed by the person
// or by Windows).
func Explain(log string, code int) string {
	t := strings.ToUpper(log)
	if strings.Contains(t, "ERRCONNECT_CONNECT_CANCELLED") {
		return ""
	}
	for _, e := range ended {
		if strings.Contains(t, "ERRINFO_"+e.code) {
			return e.text
		}
	}
	var found string
	for i, m := range messages {
		for _, c := range m.codes {
			if found == "" && strings.Contains(t, "ERRCONNECT_"+c) {
				if i < credentialMessages { // the server got as far as the login: a certificate warning did not stop it
					return m.text
				}
				found = m.text
			}
		}
	}
	if strings.Contains(t, "CERTIFICATE") && (strings.Contains(t, "CHANGED") || strings.Contains(t, "MISMATCH")) {
		return "El certificado del servidor ha cambiado desde la última conexión. Puede ser normal (se renovó) o un intento de suplantación: compruébalo con quien administra el servidor."
	}
	if found != "" {
		return found
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
