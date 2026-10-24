package servidor

import (
	"bytes"
	"context"
	"encoding/base64"
	"log"
	"net/http"
	"time"

	"planesgo-code/internal/depuracion"
	"planesgo-code/internal/sesiones"
	"planesgo-code/internal/winrm"
)

// WinRM (remote PowerShell) connections of the work sessions (sessionconn.go): computer, Windows user and password,
// defined in the session's properties; its agents use it with «pgo ps», pgocode's services through WinRMFor. Read
// only unless the connection allows changes (internal/winrm/readonly.go); every script and transfer goes to the
// audit log (the order, not its output).

const (
	winrmRunTimeout      = 5 * time.Minute
	winrmTransferTimeout = 5 * time.Minute
)

type winrmConns = sessionConns[*winrm.Conn]

// WinRMFor: the WinRM connection of a work session, for pgocode's own services (nil if it has none).
func (s *Servidor) WinRMFor(project int, session string) *winrm.Conn {
	if x := s.winrms.get(project, session); x != nil {
		return x.conn
	}
	return nil
}

// winrmView: what the interface shows of a session's connection.
type winrmView struct {
	Connected bool          `json:"connected"`
	Config    *winrm.Config `json:"config,omitempty"`
	Server    string        `json:"server,omitempty"` // computer and user, as the server says
	Opened    *time.Time    `json:"opened,omitempty"`
	Orders    int           `json:"orders,omitempty"`
}

func (s *Servidor) winrmView(project int, session string) winrmView {
	x := s.winrms.get(project, session)
	if x == nil {
		return winrmView{}
	}
	cfg := x.conn.Config()
	return winrmView{Connected: true, Config: &cfg, Server: x.conn.Server, Opened: &x.opened, Orders: s.winrms.counted(x)}
}

// winrmRequest: the body of the /api/winrm requests of the interface.
type winrmRequest struct {
	sessionRef
	Host     string `json:"host"`
	User     string `json:"user"`
	Password string `json:"password"`
	HTTPS    bool   `json:"https"`
	Trust    bool   `json:"trust_certificate"`
	Changes  bool   `json:"allow_changes"`
	Script   string `json:"script"`
	Write    bool   `json:"write"`
}

func (s *Servidor) winrmRead(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) (winrmRequest, bool) {
	var p winrmRequest
	ok := s.readSessionRequest(w, r, ses, &p, &p.sessionRef)
	return p, ok
}

// GET /api/winrm?project=&session=
func (s *Servidor) winrmState(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if p, ok := s.winrmRead(w, r, ses); ok {
		jsonOK(w, s.winrmView(p.Project, p.Session))
	}
}

// POST /api/winrm/connect: opens (or replaces) the connection of a session.
func (s *Servidor) winrmConnect(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, ok := s.winrmRead(w, r, ses)
	if !ok {
		return
	}
	cfg := winrm.Config{Host: p.Host, User: p.User, Password: p.Password, HTTPS: p.HTTPS, TrustCertificate: p.Trust, AllowChanges: p.Changes}
	p.Password = ""
	conn, err := winrm.Open(r.Context(), cfg)
	cfg.Password = ""
	if err != nil {
		s.winrmAudit(ses.Login, "interfaz", p.sessionRef, cfg, "(conectar)", false, nil, err)
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.winrms.open(conn, p.Project, p.Session, ses.Login)
	s.winrmAudit(ses.Login, "interfaz", p.sessionRef, cfg, "(conectar)", false, nil, nil)
	log.Printf("WinRM: %s conectado a %s (proyecto %d, sesión %s, cambios %v)", ses.Login, cfg, p.Project, p.Session, cfg.AllowChanges)
	jsonOK(w, s.winrmView(p.Project, p.Session))
}

// POST /api/winrm/disconnect
func (s *Servidor) winrmDisconnect(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, ok := s.winrmRead(w, r, ses)
	if !ok {
		return
	}
	if x := s.winrms.get(p.Project, p.Session); x != nil {
		s.winrmAudit(ses.Login, "interfaz", p.sessionRef, x.conn.Config(), "(desconectar)", false, nil, nil)
	}
	s.winrms.remove(p.Project, p.Session)
	jsonOK(w, s.winrmView(p.Project, p.Session))
}

// POST /api/winrm/mode {allow_changes}: switches the open connection between read only and with changes.
func (s *Servidor) winrmMode(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, ok := s.winrmRead(w, r, ses)
	if !ok {
		return
	}
	x := s.winrms.get(p.Project, p.Session)
	if x == nil {
		jsonError(w, http.StatusConflict, "esta sesión no tiene conexión WinRM")
		return
	}
	x.conn.SetAllowChanges(p.Changes)
	mode := map[bool]string{true: "(modo: actualizable)", false: "(modo: solo lectura)"}[p.Changes]
	s.winrmAudit(ses.Login, "interfaz", p.sessionRef, x.conn.Config(), mode, false, nil, nil)
	log.Printf("WinRM: %s pasa la conexión de la sesión %s a %s", ses.Login, p.Session, mode)
	jsonOK(w, s.winrmView(p.Project, p.Session))
}

// POST /api/winrm/run: a script from the interface.
func (s *Servidor) winrmRunUI(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if p, ok := s.winrmRead(w, r, ses); ok {
		s.winrmRun(w, r.Context(), ses.Login, "interfaz", p.sessionRef, p.Script, p.Write)
	}
}

// winrmConnOf: the session's connection, or the answer saying it has none (or that it cannot write).
func (s *Servidor) winrmConnOf(w http.ResponseWriter, ref sessionRef, write bool) *sessionConn[*winrm.Conn] {
	x := s.winrms.get(ref.Project, ref.Session)
	if x == nil {
		jsonError(w, http.StatusConflict, "esta sesión no tiene conexión WinRM (se define en las propiedades de la sesión)")
		return nil
	}
	if write && !x.conn.Config().AllowChanges {
		jsonError(w, http.StatusForbidden, "la conexión es de solo lectura: para cambiar algo, permite los cambios en las propiedades de la sesión")
		return nil
	}
	return x
}

// winrmRun runs a script on a session's connection and answers with its result (the interface and «pgo ps»).
func (s *Servidor) winrmRun(w http.ResponseWriter, ctx context.Context, login, origin string, ref sessionRef, script string, write bool) {
	x := s.winrmConnOf(w, ref, write)
	if x == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, winrmRunTimeout)
	defer cancel()
	res, err := x.conn.Run(ctx, script, write)
	s.winrms.used(x)
	s.winrmAudit(login, origin, ref, x.conn.Config(), script, write, &res, err)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, res)
}

// winrmTransfer: a file to or from the session's computer («pgo ps baja|sube»), base64 in the JSON.
type winrmTransfer struct {
	Path  string `json:"path"`
	Data  string `json:"data,omitempty"`
	Bytes int64  `json:"bytes"`
}

// winrmDownload: a remote file for the agent (allowed when read only: it only reads).
func (s *Servidor) winrmDownload(w http.ResponseWriter, ctx context.Context, ref sessionRef, path string) {
	x := s.winrmConnOf(w, ref, false)
	if x == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, winrmTransferTimeout)
	defer cancel()
	var buf bytes.Buffer
	n, err := x.conn.Download(ctx, path, &buf)
	s.winrms.used(x)
	s.winrmAudit("agente", "agente", ref, x.conn.Config(), "(bajar "+path+")", false, nil, err)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, winrmTransfer{Path: path, Data: base64.StdEncoding.EncodeToString(buf.Bytes()), Bytes: n})
}

// winrmUpload: a file from the agent to the remote computer (only when changes are allowed and asked for).
func (s *Servidor) winrmUpload(w http.ResponseWriter, ctx context.Context, ref sessionRef, path, data string, write bool) {
	if !write {
		jsonError(w, http.StatusBadRequest, "subir un fichero cambia el equipo: hace falta --escribir (y una conexión actualizable)")
		return
	}
	x := s.winrmConnOf(w, ref, true)
	if x == nil {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "datos del fichero no válidos")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, winrmTransferTimeout)
	defer cancel()
	n, err := x.conn.Upload(ctx, bytes.NewReader(raw), path)
	s.winrms.used(x)
	s.winrmAudit("agente", "agente", ref, x.conn.Config(), "(subir "+path+")", true, nil, err)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, winrmTransfer{Path: path, Bytes: n})
}

// winrmAudit writes one line per order to the audit log (JSON lines): who, from where, on which connection, the
// script (secrets hidden, at most 4000 characters), exit code and time; never the output nor the password.
func (s *Servidor) winrmAudit(login, origin string, ref sessionRef, cfg winrm.Config, script string, write bool, res *winrm.Result, err error) {
	e := map[string]any{"t": time.Now().Format(time.RFC3339Nano), "login": login, "origin": origin, "project": ref.Project,
		"session": ref.Session, "connection": cfg.String(), "allow_changes": cfg.AllowChanges, "write": write,
		"script": recortaTexto(depuracion.Tapa(script), 4000)}
	if res != nil && err == nil {
		e["exit_code"], e["read_only"], e["ms"] = res.ExitCode, res.ReadOnly, res.Ms
	}
	if err != nil {
		e["error"] = err.Error()
	}
	appendAudit(s.WinRMAudit, "WinRM", e)
}
