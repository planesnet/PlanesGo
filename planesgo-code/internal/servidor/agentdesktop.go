package servidor

import (
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"runtime"
	"time"
	"unicode/utf8"

	"planesgo-code/internal/agentdesktop"
	"planesgo-code/internal/freerdp"
	"planesgo-code/internal/procesos"
	"planesgo-code/internal/sesiones"
)

// The agent's desktop of a work session (internal/agentdesktop): the person defines it in the session properties
// (computer, user, password) and starts it; the session's agents drive it with «pgo escritorio» and nobody else
// does. The password only goes to FreeRDP's input; every order goes to the audit log (never the text typed).

// agentDesktopView: what the interface shows.
type agentDesktopView struct {
	Supported bool                 `json:"supported"`
	Missing   string               `json:"missing,omitempty"` // programs that are not installed
	Exchange  string               `json:"exchange"`
	Running   bool                 `json:"running"`
	Status    *agentdesktop.Status `json:"status,omitempty"`
	Actions   int                  `json:"actions,omitempty"`
}

// agentDesktopRequest: the body of the /api/agent-desktop requests.
type agentDesktopRequest struct {
	sessionRef
	Host     string `json:"host"`
	User     string `json:"user"`
	Password string `json:"password"`
	Keyboard string `json:"keyboard"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

// desktopOptions: the programs and folders (the installed ones, except in the tests).
func (s *Servidor) desktopOptions() (agentdesktop.Options, error) {
	if s.agentDesktopOptions != nil {
		return s.agentDesktopOptions()
	}
	t, err := agentdesktop.FindTools()
	return agentdesktop.Options{Tools: t}, err
}

func (s *Servidor) agentDesktopView(project int, session string) agentDesktopView {
	v := agentDesktopView{Supported: runtime.GOOS == "linux", Exchange: freerdp.ExchangeDir()}
	if opts, err := s.desktopOptions(); err != nil {
		v.Missing = err.Error()
	} else if opts.Exchange != "" {
		v.Exchange = opts.Exchange
	}
	if x := s.agentDesktops.get(project, session); x != nil {
		st := x.conn.Status()
		v.Running, v.Status, v.Exchange = x.conn.Running(), &st, st.Exchange
		v.Actions = s.agentDesktops.counted(x)
	}
	return v
}

// GET /api/agent-desktop?project=&session=
func (s *Servidor) agentDesktopState(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p agentDesktopRequest
	if s.readSessionRequest(w, r, ses, &p, &p.sessionRef) {
		jsonOK(w, s.agentDesktopView(p.Project, p.Session))
	}
}

// POST /api/agent-desktop/start: starts (or restarts) the session's desktop. The password goes to FreeRDP's input
// and is not kept.
func (s *Servidor) agentDesktopStart(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p agentDesktopRequest
	if !s.readSessionRequest(w, r, ses, &p, &p.sessionRef) {
		return
	}
	cfg := agentdesktop.Config{Host: p.Host, User: p.User, Password: p.Password, Keyboard: p.Keyboard, Width: p.Width, Height: p.Height}
	p.Password = ""
	opts, err := s.desktopOptions()
	status := http.StatusServiceUnavailable // programs missing, or not Linux
	var d *agentdesktop.Desktop
	if err == nil {
		status = http.StatusBadRequest
		s.agentDesktops.remove(p.Project, p.Session) // one per session: the old one goes first (its screen, its login)
		d, err = agentdesktop.Start(cfg, opts)
	}
	cfg.Password = ""
	conn := cfg.User + "@" + cfg.Host
	if err != nil {
		s.desktopAudit(ses.Login, "interfaz", p.Project, p.Session, conn, map[string]any{"action": "start"}, err)
		if errors.Is(err, agentdesktop.ErrUnsupported) {
			status = http.StatusServiceUnavailable
		}
		jsonError(w, status, err.Error())
		return
	}
	s.agentDesktops.open(d, p.Project, p.Session, ses.Login)
	s.desktopAudit(ses.Login, "interfaz", p.Project, p.Session, conn, map[string]any{"action": "start"}, nil)
	log.Printf("escritorio del agente: %s lo inicia en %s (proyecto %d, sesión %s)", ses.Login, conn, p.Project, p.Session)
	jsonOK(w, s.agentDesktopView(p.Project, p.Session))
}

// POST /api/agent-desktop/stop
func (s *Servidor) agentDesktopStop(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p agentDesktopRequest
	if !s.readSessionRequest(w, r, ses, &p, &p.sessionRef) {
		return
	}
	if x := s.agentDesktops.get(p.Project, p.Session); x != nil {
		st := x.conn.Status()
		s.desktopAudit(ses.Login, "interfaz", p.Project, p.Session, st.User+"@"+st.Host, map[string]any{"action": "stop"}, nil)
	}
	s.agentDesktops.remove(p.Project, p.Session)
	jsonOK(w, s.agentDesktopView(p.Project, p.Session))
}

// agentDesktopsRunning: how many agents' desktops are connected (pgocode does not update meanwhile).
func (s *Servidor) agentDesktopsRunning() int {
	n := 0
	for _, x := range s.agentDesktops.all() {
		if x.conn.Running() {
			n++
		}
	}
	return n
}

// agentDesktopRun: an order of an agent («pgo escritorio»), on its own session's desktop only. Screenshots go to
// .pgocode/salidas of the agent's folder (it has to be one of the project).
func (s *Servidor) agentDesktopRun(w http.ResponseWriter, project int, session, folder string, a *agentdesktop.Action) {
	x := s.agentDesktops.get(project, session)
	if x == nil {
		jsonError(w, http.StatusConflict, "esta sesión no tiene escritorio del agente: la persona lo inicia en las propiedades de la sesión (Escritorio del agente)")
		return
	}
	if a == nil {
		jsonError(w, http.StatusBadRequest, "falta la orden del escritorio")
		return
	}
	dir := ""
	if a.Name == "capture" {
		if !filepath.IsAbs(folder) || !s.dentroDelProyecto(project, folder) {
			jsonError(w, http.StatusForbidden, "la carpeta tiene que ser una del proyecto")
			return
		}
		dir = procesos.DirRegistros(folder)
		excluyeDeGit(folder)
	}
	res, err := x.conn.Do(*a, dir)
	s.agentDesktops.used(x)
	st := x.conn.Status()
	s.desktopAudit("agente", "agente", project, session, st.User+"@"+st.Host, actionAudit(*a, res), err)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, res)
}

// actionAudit: what the audit keeps of an order: never the text typed (it may be a password), only its length.
func actionAudit(a agentdesktop.Action, res agentdesktop.Result) map[string]any {
	e := map[string]any{"action": a.Name}
	switch a.Name {
	case "click", "move":
		e["x"], e["y"], e["double"], e["right"] = a.X, a.Y, a.Double, a.Right
	case "drag":
		e["x"], e["y"], e["x2"], e["y2"] = a.X, a.Y, a.X2, a.Y2
	case "wheel":
		e["up"], e["count"] = a.Up, a.Count
	case "type":
		e["characters"] = utf8.RuneCountInString(a.Text)
	case "key":
		e["keys"] = a.Keys
	case "size":
		e["width"], e["height"] = a.Width, a.Height
	case "capture":
		e["scale"] = a.Scale
	case "record":
		e["record"] = a.Record
	}
	if res.Path != "" {
		e["path"] = res.Path
	}
	return e
}

// desktopAudit writes one line per order to the agent desktop's audit log.
func (s *Servidor) desktopAudit(login, origin string, project int, session, conn string, e map[string]any, err error) {
	e["t"], e["login"], e["origin"], e["project"], e["session"], e["connection"] = time.Now().Format(time.RFC3339Nano), login, origin, project, session, conn
	if err != nil {
		e["error"] = err.Error()
	}
	appendAudit(s.AgentDesktopAudit, "escritorio del agente", e)
}
