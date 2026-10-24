package servidor

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"planesgo-code/internal/depuracion"
	"planesgo-code/internal/sesiones"
	"planesgo-code/internal/sqlserver"
)

// SQL Server connections of the work sessions: a service pgocode offers to its own parts. The person defines the
// connection of a session (server, database, SQL Server or Windows user, password) in its properties; then the
// session's agents use it with «pgo sql», and pgocode's services through SQLFor. The password only lives in the
// backend's memory (it is lost when pgocode closes): agents and programs never see it. Read only unless the
// connection allows changes; every batch goes to the audit log (the batch, not its data).

const sqlRunTimeout = 2 * time.Minute

// sqlConn: the open connection of a session.
type sqlConn struct {
	conn    *sqlserver.Conn
	project int
	session string
	login   string // who opened it
	opened  time.Time
	used    time.Time
	batches int
}

type sqlConns struct {
	mu sync.Mutex
	m  map[string]*sqlConn // project|session
}

func sqlKey(project int, session string) string { return strconv.Itoa(project) + "|" + session }

// safeSession: the id of a work session (the interface makes them; it goes into keys and the audit log).
var safeSession = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,80}$`)

func (c *sqlConns) get(project int, session string) *sqlConn {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[sqlKey(project, session)]
}

// put stores the connection of a session, closing the one it replaces.
func (c *sqlConns) put(x *sqlConn) {
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string]*sqlConn{}
	}
	old := c.m[sqlKey(x.project, x.session)]
	c.m[sqlKey(x.project, x.session)] = x
	c.mu.Unlock()
	if old != nil {
		old.conn.Close()
	}
}

func (c *sqlConns) remove(project int, session string) bool {
	c.mu.Lock()
	x := c.m[sqlKey(project, session)]
	delete(c.m, sqlKey(project, session))
	c.mu.Unlock()
	if x != nil {
		x.conn.Close()
	}
	return x != nil
}

// CloseConnections closes, on shutdown, what pgocode keeps open towards other servers: the remote desktops (their
// time would not be counted any more) and the SQL Server connections of the sessions.
func (s *Servidor) CloseConnections() {
	s.escritorios.closeAll()
	s.sqls.closeAll()
}

func (c *sqlConns) closeAll() {
	c.mu.Lock()
	l := c.m
	c.m = nil
	c.mu.Unlock()
	for _, x := range l {
		x.conn.Close()
	}
}

// SQLFor: the SQL Server connection of a work session, for pgocode's own services (nil if it has none).
func (s *Servidor) SQLFor(project int, session string) *sqlserver.Conn {
	if x := s.sqls.get(project, session); x != nil {
		return x.conn
	}
	return nil
}

// sqlView: what the interface shows of a session's connection.
type sqlView struct {
	Connected bool              `json:"connected"`
	Databases []string          `json:"databases,omitempty"` // the ones the login can use
	Config    *sqlserver.Config `json:"config,omitempty"`
	Server    string            `json:"server_version,omitempty"`
	Opened    *time.Time        `json:"opened,omitempty"`
	Batches   int               `json:"batches,omitempty"`
}

func (s *Servidor) sqlView(project int, session string) sqlView {
	x := s.sqls.get(project, session)
	if x == nil {
		return sqlView{}
	}
	cfg := x.conn.Config()
	return sqlView{Connected: true, Config: &cfg, Server: x.conn.Server, Opened: &x.opened, Batches: x.batches}
}

// sqlRequest: the body of the /api/sql requests of the interface.
type sqlRequest struct {
	Project  int    `json:"project"`
	Session  string `json:"session"`
	Server   string `json:"server"`
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"password"`
	Changes  bool   `json:"allow_changes"`
	Trust    bool   `json:"trust_certificate"`
	SQL      string `json:"sql"`
	Rows     int    `json:"rows"`
	Write    bool   `json:"write"`
}

// sqlRead: the request with a project of the user and a valid session.
func (s *Servidor) sqlRead(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) (sqlRequest, bool) {
	var p sqlRequest
	if r.Method == http.MethodGet {
		p.Project, _ = strconv.Atoi(r.URL.Query().Get("project"))
		p.Session = r.URL.Query().Get("session")
	} else if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return p, false
	}
	if _, ok := s.proyecto(w, r, ses, p.Project); !ok {
		return p, false
	}
	if !safeSession.MatchString(p.Session) {
		jsonError(w, http.StatusBadRequest, "sesión no válida")
		return p, false
	}
	return p, true
}

// GET /api/sql?project=&session=
func (s *Servidor) sqlState(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if p, ok := s.sqlRead(w, r, ses); ok {
		jsonOK(w, s.sqlViewWithDatabases(r.Context(), p.Project, p.Session))
	}
}

// POST /api/sql/connect: opens (or replaces) the connection of a session.
func (s *Servidor) sqlConnect(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, ok := s.sqlRead(w, r, ses)
	if !ok {
		return
	}
	cfg := sqlserver.Config{Server: p.Server, Database: p.Database, User: p.User, Password: p.Password,
		AllowChanges: p.Changes, TrustCertificate: p.Trust}
	p.Password = ""
	conn, err := sqlserver.Open(r.Context(), cfg)
	cfg.Password = ""
	if err != nil {
		s.sqlAudit(ses.Login, "interfaz", p.Project, p.Session, cfg, "(conectar)", false, nil, err)
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	now := time.Now()
	s.sqls.put(&sqlConn{conn: conn, project: p.Project, session: p.Session, login: ses.Login, opened: now, used: now})
	s.sqlAudit(ses.Login, "interfaz", p.Project, p.Session, cfg, "(conectar)", false, nil, nil)
	log.Printf("SQL Server: %s conectado a %s (proyecto %d, sesión %s, cambios %v)", ses.Login, cfg, p.Project, p.Session, cfg.AllowChanges)
	jsonOK(w, s.sqlViewWithDatabases(r.Context(), p.Project, p.Session))
}

// POST /api/sql/disconnect
func (s *Servidor) sqlDisconnect(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, ok := s.sqlRead(w, r, ses)
	if !ok {
		return
	}
	if x := s.sqls.get(p.Project, p.Session); x != nil {
		s.sqlAudit(ses.Login, "interfaz", p.Project, p.Session, x.conn.Config(), "(desconectar)", false, nil, nil)
	}
	s.sqls.remove(p.Project, p.Session)
	jsonOK(w, s.sqlView(p.Project, p.Session))
}

// POST /api/sql/database {database}: the database the session's batches run in, without reconnecting.
func (s *Servidor) sqlDatabase(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, ok := s.sqlRead(w, r, ses)
	if !ok {
		return
	}
	x := s.sqls.get(p.Project, p.Session)
	if x == nil {
		jsonError(w, http.StatusConflict, "esta sesión no tiene conexión con SQL Server")
		return
	}
	if err := x.conn.SetDatabase(r.Context(), p.Database); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.sqlAudit(ses.Login, "interfaz", p.Project, p.Session, x.conn.Config(), "(base de datos: "+p.Database+")", false, nil, nil)
	jsonOK(w, s.sqlViewWithDatabases(r.Context(), p.Project, p.Session))
}

// sqlViewWithDatabases: the view with the databases the login can use (one query to the server).
func (s *Servidor) sqlViewWithDatabases(ctx context.Context, project int, session string) sqlView {
	v := s.sqlView(project, session)
	if x := s.sqls.get(project, session); x != nil {
		v.Databases, _ = x.conn.Databases(ctx)
	}
	return v
}

// POST /api/sql/mode {allow_changes}: switches the open connection between read only and with changes.
func (s *Servidor) sqlMode(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, ok := s.sqlRead(w, r, ses)
	if !ok {
		return
	}
	x := s.sqls.get(p.Project, p.Session)
	if x == nil {
		jsonError(w, http.StatusConflict, "esta sesión no tiene conexión con SQL Server")
		return
	}
	x.conn.SetAllowChanges(p.Changes)
	mode := map[bool]string{true: "(modo: actualizable)", false: "(modo: solo lectura)"}[p.Changes]
	s.sqlAudit(ses.Login, "interfaz", p.Project, p.Session, x.conn.Config(), mode, false, nil, nil)
	log.Printf("SQL Server: %s pasa la conexión de la sesión %s a %s", ses.Login, p.Session, mode)
	jsonOK(w, s.sqlViewWithDatabases(r.Context(), p.Project, p.Session))
}

// POST /api/sql/run: a batch from the interface.
func (s *Servidor) sqlRunUI(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, ok := s.sqlRead(w, r, ses)
	if !ok {
		return
	}
	s.sqlRun(w, r.Context(), ses.Login, "interfaz", p.Project, p.Session, p.SQL, p.Rows, p.Write)
}

// sqlRun runs a batch on a session's connection and answers with its result (shared by the interface and «pgo sql»).
func (s *Servidor) sqlRun(w http.ResponseWriter, ctx context.Context, login, origin string, project int, session, batch string, rows int, write bool) {
	x := s.sqls.get(project, session)
	if x == nil {
		jsonError(w, http.StatusConflict, "esta sesión no tiene conexión con SQL Server (se define en las propiedades de la sesión)")
		return
	}
	if write && !x.conn.Config().AllowChanges {
		jsonError(w, http.StatusForbidden, "la conexión es de solo lectura: para escribir, permite los cambios en las propiedades de la sesión")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, sqlRunTimeout)
	defer cancel()
	res, err := x.conn.Run(ctx, batch, rows, write)
	s.sqls.mu.Lock()
	x.used = time.Now()
	x.batches++
	s.sqls.mu.Unlock()
	s.sqlAudit(login, origin, project, session, x.conn.Config(), batch, write, &res, err)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, res)
}

var sqlAuditMu sync.Mutex

// sqlAudit writes one line per batch to the audit log (JSON lines): who, from where, on which connection, the batch
// (secrets hidden, at most 4000 characters), rows and time; never the data.
func (s *Servidor) sqlAudit(login, origin string, project int, session string, cfg sqlserver.Config, batch string, write bool, res *sqlserver.Result, err error) {
	if s.SQLAudit == "" {
		return
	}
	e := map[string]any{"t": time.Now().Format(time.RFC3339Nano), "login": login, "origin": origin, "project": project,
		"session": session, "connection": cfg.String(), "allow_changes": cfg.AllowChanges, "write": write,
		"sql": recortaTexto(depuracion.Tapa(batch), 4000)}
	if res != nil && err == nil {
		n := 0
		for _, x := range res.Sets {
			n += len(x.Rows)
		}
		e["rows"], e["affected"], e["rolled_back"], e["ms"] = n, res.Affected, res.RolledBack, res.Ms
	}
	if err != nil {
		e["error"] = err.Error()
	}
	b, _ := json.Marshal(e)
	sqlAuditMu.Lock()
	defer sqlAuditMu.Unlock()
	f, ferr := os.OpenFile(s.SQLAudit, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if ferr != nil {
		log.Printf("SQL Server: auditoría: %v", ferr)
		return
	}
	f.Write(append(b, '\n'))
	f.Close()
}
