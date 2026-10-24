package servidor

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"planesgo-code/internal/sesiones"
)

// Connections of the work sessions to other servers (SQL Server: sqlconn.go; WinRM: winrmconn.go), what they share:
// the person defines the connection of a session in its properties; the password only lives in the backend's memory
// (it is lost when pgocode closes); the session's agents use it with «pgo …» (only the one of their own session) and
// pgocode's services through SQLFor/WinRMFor; every order goes to an audit log, without secrets nor data.

// sessionConn: the open connection of a session.
type sessionConn[C interface{ Close() error }] struct {
	conn    C
	project int
	session string
	login   string // who opened it
	opened  time.Time
	used    time.Time
	count   int // orders run on it
}

// sessionConns: the open connections of one kind, by project and session.
type sessionConns[C interface{ Close() error }] struct {
	mu     sync.Mutex
	m      map[string]*sessionConn[C] // project|session
	closed bool                       // pgocode is closing (closeAll): nothing new is kept
}

func sessionKey(project int, session string) string { return strconv.Itoa(project) + "|" + session }

// safeSession: the id of a work session (the interface makes them; it goes into keys and the audit logs).
var safeSession = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,80}$`)

func (c *sessionConns[C]) get(project int, session string) *sessionConn[C] {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[sessionKey(project, session)]
}

// open stores a new connection of a session, closing the one it replaces. If pgocode started closing while it was
// being opened (closeAll already ran), it is closed at once and open answers false: nobody would close it later.
func (c *sessionConns[C]) open(conn C, project int, session, login string) bool {
	now := time.Now()
	x := &sessionConn[C]{conn: conn, project: project, session: session, login: login, opened: now, used: now}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		conn.Close()
		return false
	}
	if c.m == nil {
		c.m = map[string]*sessionConn[C]{}
	}
	old := c.m[sessionKey(project, session)]
	c.m[sessionKey(project, session)] = x
	c.mu.Unlock()
	if old != nil {
		old.conn.Close()
	}
	return true
}

func (c *sessionConns[C]) remove(project int, session string) bool {
	c.mu.Lock()
	x := c.m[sessionKey(project, session)]
	delete(c.m, sessionKey(project, session))
	c.mu.Unlock()
	if x != nil {
		x.conn.Close()
	}
	return x != nil
}

// used counts one more order on the connection.
func (c *sessionConns[C]) used(x *sessionConn[C]) {
	c.mu.Lock()
	x.used = time.Now()
	x.count++
	c.mu.Unlock()
}

// counted: the orders run on the connection (under the lock: used changes it).
func (c *sessionConns[C]) counted(x *sessionConn[C]) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return x.count
}

func (c *sessionConns[C]) closeAll() {
	c.mu.Lock()
	l := c.m
	c.m, c.closed = nil, true
	c.mu.Unlock()
	for _, x := range l {
		x.conn.Close()
	}
}

// CloseConnections closes, on shutdown, what pgocode keeps open towards other servers: the agents' desktops and the
// SQL Server and WinRM connections of the sessions. (The person's remote desktops run in the clients: their
// periods close with Imputacion.CierraTodo.)
func (s *Servidor) CloseConnections() {
	s.agentDesktops.closeAll()
	s.sqls.closeAll()
	s.winrms.closeAll()
}

// sessionRef: the project and session every request about a session's connection carries.
type sessionRef struct {
	Project int    `json:"project"`
	Session string `json:"session"`
}

// readSessionRequest reads the body (or, with GET, the query) into body, whose project and session are ref, and
// checks them: a project of the user and a valid session id.
func (s *Servidor) readSessionRequest(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion, body any, ref *sessionRef) bool {
	if r.Method == http.MethodGet {
		ref.Project, _ = strconv.Atoi(r.URL.Query().Get("project"))
		ref.Session = r.URL.Query().Get("session")
	} else if err := lee(r, body); err != nil {
		errorLectura(w, err)
		return false
	}
	if _, ok := s.proyecto(w, r, ses, ref.Project); !ok {
		return false
	}
	if !safeSession.MatchString(ref.Session) {
		jsonError(w, http.StatusBadRequest, "sesión no válida")
		return false
	}
	return true
}

var auditMu sync.Mutex

// appendAudit writes one JSON line to an audit log (600: only its owner reads it). The caller leaves secrets out.
func appendAudit(path, what string, e map[string]any) {
	if path == "" {
		return
	}
	b, _ := json.Marshal(e)
	auditMu.Lock()
	defer auditMu.Unlock()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("%s: auditoría: %v", what, err)
		return
	}
	f.Write(append(b, '\n'))
	f.Close()
}

// all: the open connections.
func (c *sessionConns[C]) all() []*sessionConn[C] {
	c.mu.Lock()
	defer c.mu.Unlock()
	l := make([]*sessionConn[C], 0, len(c.m))
	for _, x := range c.m {
		l = append(l, x)
	}
	return l
}
