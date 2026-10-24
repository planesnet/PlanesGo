package servidor

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"

	"planesgo-code/internal/sesiones"
)

// What the connections of the work sessions share (SQL Server in sqlconn.go, the agent's desktop in
// agentdesktop.go): one per project and session, defined by the person in the session properties, used by that
// session's agents only, closed when pgocode closes, and an audit log in JSON lines without secrets.

// safeSession: the id of a work session (the interface makes them; it goes into keys and the audit logs).
var safeSession = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,80}$`)

func sessionKey(project int, session string) string { return strconv.Itoa(project) + "|" + session }

// sessionConns: the open connections of one kind, by project and session.
type sessionConns[T interface{ Close() error }] struct {
	mu sync.Mutex
	m  map[string]T
}

func (c *sessionConns[T]) get(project int, session string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	x, ok := c.m[sessionKey(project, session)]
	return x, ok
}

// put stores the connection of a session, closing the one it replaces.
func (c *sessionConns[T]) put(project int, session string, x T) {
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string]T{}
	}
	old, had := c.m[sessionKey(project, session)]
	c.m[sessionKey(project, session)] = x
	c.mu.Unlock()
	if had {
		old.Close()
	}
}

// remove closes and forgets the connection of a session.
func (c *sessionConns[T]) remove(project int, session string) bool {
	c.mu.Lock()
	x, ok := c.m[sessionKey(project, session)]
	delete(c.m, sessionKey(project, session))
	c.mu.Unlock()
	if ok {
		x.Close()
	}
	return ok
}

// all: the open connections.
func (c *sessionConns[T]) all() []T {
	c.mu.Lock()
	defer c.mu.Unlock()
	l := make([]T, 0, len(c.m))
	for _, x := range c.m {
		l = append(l, x)
	}
	return l
}

func (c *sessionConns[T]) closeAll() {
	c.mu.Lock()
	l := c.m
	c.m = nil
	c.mu.Unlock()
	for _, x := range l {
		x.Close()
	}
}

// sessionTarget: the project and work session a request of the interface is about.
type sessionTarget struct {
	Project int    `json:"project"`
	Session string `json:"session"`
}

func (t *sessionTarget) target() *sessionTarget { return t }

// readSessionRequest reads a request about a session's connection (GET: project and session in the URL; else the
// JSON body into p) and checks the project is the user's and the session id is valid.
func (s *Servidor) readSessionRequest(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion, p interface{ target() *sessionTarget }) bool {
	t := p.target()
	if r.Method == http.MethodGet {
		t.Project, _ = strconv.Atoi(r.URL.Query().Get("project"))
		t.Session = r.URL.Query().Get("session")
	} else if err := lee(r, p); err != nil {
		errorLectura(w, err)
		return false
	}
	if _, ok := s.proyecto(w, r, ses, t.Project); !ok {
		return false
	}
	if !safeSession.MatchString(t.Session) {
		jsonError(w, http.StatusBadRequest, "sesión no válida")
		return false
	}
	return true
}

var auditMu sync.Mutex

// appendAudit adds one JSON line to an audit log (600; nothing if path is empty). The caller leaves secrets out.
func appendAudit(path string, e map[string]any) {
	if path == "" {
		return
	}
	b, _ := json.Marshal(e)
	auditMu.Lock()
	defer auditMu.Unlock()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("auditoría %s: %v", path, err)
		return
	}
	f.Write(append(b, '\n'))
	f.Close()
}
