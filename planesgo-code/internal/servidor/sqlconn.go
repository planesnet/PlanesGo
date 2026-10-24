package servidor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"planesgo-code/internal/depuracion"
	"planesgo-code/internal/sesiones"
	"planesgo-code/internal/sqlserver"
)

// SQL Server connections of the work sessions (sessionconn.go): server, database, SQL Server or Windows user and
// password, defined in the session's properties; its agents use it with «pgo sql», pgocode's services through
// SQLFor. Read only unless the connection allows changes; every batch goes to the audit log (the batch, not its
// data).

const sqlRunTimeout = 2 * time.Minute

type sqlConns = sessionConns[*sqlserver.Conn]

// A session can have several connections, each with its name («pruebas», «contenedor»…), its own read only or
// updatable mode and its audit lines. The one without a name is «principal» (what there was before names).
const sqlDefault = "principal"

var sqlName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// sqlConnName: the name of a connection («» is the main one); ok false if it is not a valid one.
func sqlConnName(name string) (string, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return sqlDefault, true
	}
	return name, sqlName.MatchString(name)
}

// sqlKey: where a named connection of a session is kept (sessionConns keys by project and this).
func sqlKey(session, name string) string { return session + "|" + name }

// sqlNames: the names of a session's open connections, sorted.
func (s *Servidor) sqlNames(project int, session string) []string {
	var l []string
	for _, x := range s.sqls.all() {
		if n, ok := strings.CutPrefix(x.session, session+"|"); ok && x.project == project {
			l = append(l, n)
		}
	}
	sort.Strings(l)
	return l
}

// SQLFor: the main SQL Server connection of a work session, for pgocode's own services (nil if it has none).
func (s *Servidor) SQLFor(project int, session string) *sqlserver.Conn {
	if x := s.sqls.get(project, sqlKey(session, sqlDefault)); x != nil {
		return x.conn
	}
	return nil
}

// sqlView: what the interface shows of a session's connection.
type sqlView struct {
	Name      string            `json:"name"`
	Names     []string          `json:"names"` // the session's open connections
	Connected bool              `json:"connected"`
	Databases []string          `json:"databases,omitempty"` // the ones the login can use
	Config    *sqlserver.Config `json:"config,omitempty"`
	Server    string            `json:"server_version,omitempty"`
	Opened    *time.Time        `json:"opened,omitempty"`
	Batches   int               `json:"batches,omitempty"`
}

func (s *Servidor) sqlView(project int, session, name string) sqlView {
	v := sqlView{Name: name, Names: s.sqlNames(project, session)}
	x := s.sqls.get(project, sqlKey(session, name))
	if x == nil {
		return v
	}
	cfg := x.conn.Config()
	v.Connected, v.Config, v.Server, v.Opened, v.Batches = true, &cfg, x.conn.Server, &x.opened, s.sqls.counted(x)
	return v
}

// sqlRequest: the body of the /api/sql requests of the interface.
type sqlRequest struct {
	sessionRef
	Name     string `json:"name"` // the connection («» the main one)
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
	if !s.readSessionRequest(w, r, ses, &p, &p.sessionRef) {
		return p, false
	}
	if r.Method == http.MethodGet {
		p.Name = r.URL.Query().Get("name")
	}
	name, ok := sqlConnName(p.Name)
	if !ok {
		jsonError(w, http.StatusBadRequest, "nombre de conexión no válido (letras minúsculas, cifras y guiones)")
		return p, false
	}
	p.Name = name
	return p, true
}

// GET /api/sql?project=&session=
func (s *Servidor) sqlState(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if p, ok := s.sqlRead(w, r, ses); ok {
		jsonOK(w, s.sqlViewWithDatabases(r.Context(), p.Project, p.Session, p.Name))
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
		s.sqlAudit(ses.Login, "interfaz", p.Project, p.Session, p.Name, cfg, "(conectar)", false, nil, err)
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	if !s.sqls.open(conn, p.Project, sqlKey(p.Session, p.Name), ses.Login) {
		jsonError(w, http.StatusServiceUnavailable, "pgocode se está cerrando: vuelve a intentarlo cuando esté abierto")
		return
	}
	s.sqlAudit(ses.Login, "interfaz", p.Project, p.Session, p.Name, cfg, "(conectar)", false, nil, nil)
	log.Printf("SQL Server: %s conectado a %s (proyecto %d, sesión %s, cambios %v)", ses.Login, cfg, p.Project, p.Session, cfg.AllowChanges)
	jsonOK(w, s.sqlViewWithDatabases(r.Context(), p.Project, p.Session, p.Name))
}

// POST /api/sql/disconnect
func (s *Servidor) sqlDisconnect(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, ok := s.sqlRead(w, r, ses)
	if !ok {
		return
	}
	if x := s.sqls.get(p.Project, sqlKey(p.Session, p.Name)); x != nil {
		s.sqlAudit(ses.Login, "interfaz", p.Project, p.Session, p.Name, x.conn.Config(), "(desconectar)", false, nil, nil)
	}
	s.sqls.remove(p.Project, sqlKey(p.Session, p.Name))
	jsonOK(w, s.sqlView(p.Project, p.Session, p.Name))
}

// POST /api/sql/database {database}: the database the session's batches run in, without reconnecting.
func (s *Servidor) sqlDatabase(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, ok := s.sqlRead(w, r, ses)
	if !ok {
		return
	}
	x := s.sqls.get(p.Project, sqlKey(p.Session, p.Name))
	if x == nil {
		jsonError(w, http.StatusConflict, "esta sesión no tiene conexión con SQL Server")
		return
	}
	if err := x.conn.SetDatabase(r.Context(), p.Database); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.sqlAudit(ses.Login, "interfaz", p.Project, p.Session, p.Name, x.conn.Config(), "(base de datos: "+p.Database+")", false, nil, nil)
	jsonOK(w, s.sqlViewWithDatabases(r.Context(), p.Project, p.Session, p.Name))
}

// sqlViewWithDatabases: the view with the databases the login can use (one query to the server).
func (s *Servidor) sqlViewWithDatabases(ctx context.Context, project int, session, name string) sqlView {
	v := s.sqlView(project, session, name)
	if x := s.sqls.get(project, sqlKey(session, name)); x != nil {
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
	x := s.sqls.get(p.Project, sqlKey(p.Session, p.Name))
	if x == nil {
		jsonError(w, http.StatusConflict, "esta sesión no tiene conexión con SQL Server")
		return
	}
	x.conn.SetAllowChanges(p.Changes)
	mode := map[bool]string{true: "(modo: actualizable)", false: "(modo: solo lectura)"}[p.Changes]
	s.sqlAudit(ses.Login, "interfaz", p.Project, p.Session, p.Name, x.conn.Config(), mode, false, nil, nil)
	log.Printf("SQL Server: %s pasa la conexión de la sesión %s a %s", ses.Login, p.Session, mode)
	jsonOK(w, s.sqlViewWithDatabases(r.Context(), p.Project, p.Session, p.Name))
}

// POST /api/sql/run: a batch from the interface.
func (s *Servidor) sqlRunUI(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, ok := s.sqlRead(w, r, ses)
	if !ok {
		return
	}
	s.sqlRun(w, r.Context(), ses.Login, "interfaz", p.Project, p.Session, p.Name, p.SQL, p.Rows, p.Write)
}

// sqlRun runs a batch on a session's connection and answers with its result (shared by the interface and «pgo sql»).
func (s *Servidor) sqlRun(w http.ResponseWriter, ctx context.Context, login, origin string, project int, session, name, batch string, rows int, write bool) {
	x, ok := s.sqlConnFor(w, project, session, name)
	if !ok {
		return
	}
	if write && !x.conn.Config().AllowChanges {
		jsonError(w, http.StatusForbidden, "la conexión es de solo lectura: para escribir, permite los cambios en las propiedades de la sesión")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, sqlRunTimeout)
	defer cancel()
	res, err := x.conn.Run(ctx, batch, rows, write)
	s.sqls.used(x)
	s.sqlAudit(login, origin, project, session, name, x.conn.Config(), batch, write, &res, err)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, res)
}

// sqlAudit writes one line per batch to the audit log (JSON lines): who, from where, on which connection, the batch
// (secrets hidden, at most 4000 characters), rows and time; never the data.
func (s *Servidor) sqlAudit(login, origin string, project int, session, name string, cfg sqlserver.Config, batch string, write bool, res *sqlserver.Result, err error) {
	appendAudit(s.SQLAudit, "SQL Server", s.sqlAuditEntry(login, origin, project, session, name, cfg, batch, write, res, err))
}

// sqlAuditEntry: the audit line of a batch.
func (s *Servidor) sqlAuditEntry(login, origin string, project int, session, name string, cfg sqlserver.Config, batch string, write bool, res *sqlserver.Result, err error) map[string]any {
	e := map[string]any{"t": time.Now().Format(time.RFC3339Nano), "login": login, "origin": origin, "project": project,
		"session": session, "name": name, "connection": cfg.String(), "allow_changes": cfg.AllowChanges, "write": write,
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
	return e
}

// sqlConnFor: a session's named connection, for a batch (409 explained if it has none with that name).
func (s *Servidor) sqlConnFor(w http.ResponseWriter, project int, session, name string) (*sessionConn[*sqlserver.Conn], bool) {
	n, ok := sqlConnName(name)
	if !ok {
		jsonError(w, http.StatusBadRequest, "nombre de conexión no válido (letras minúsculas, cifras y guiones)")
		return nil, false
	}
	x := s.sqls.get(project, sqlKey(session, n))
	if x == nil {
		msg := "esta sesión no tiene conexión con SQL Server (se define en las propiedades de la sesión)"
		if names := s.sqlNames(project, session); len(names) > 0 {
			msg = fmt.Sprintf("esta sesión no tiene la conexión «%s»; tiene: %s", n, strings.Join(names, ", "))
		}
		jsonError(w, http.StatusConflict, msg)
		return nil, false
	}
	return x, true
}

// sqlConnection: one of a session's connections, as a program sees it (nothing secret).
type sqlConnection struct {
	Name         string `json:"name"`
	Server       string `json:"server"`
	Database     string `json:"database,omitempty"`
	User         string `json:"user"`
	AllowChanges bool   `json:"allow_changes"`
}

// sqlConnections: the open connections of a session.
func (s *Servidor) sqlConnections(project int, session string) []sqlConnection {
	l := []sqlConnection{}
	for _, n := range s.sqlNames(project, session) {
		if x := s.sqls.get(project, sqlKey(session, n)); x != nil {
			c := x.conn.Config()
			l = append(l, sqlConnection{Name: n, Server: c.Server, Database: x.conn.Database(), User: c.User, AllowChanges: c.AllowChanges})
		}
	}
	return l
}

// sqlStreamMax: the most rows a streamed batch gives (an application reading a catalog; not a whole database).
const sqlStreamMax = 2_000_000

// sqlStream runs a batch and answers with all its rows as they come, in JSON lines (application/x-ndjson):
//
//	{"set":0,"columns":[…],"types":[…]}
//	{"row":[…]}
//	{"end":{"affected":…,"rolled_back":…,"ms":…}}   or   {"error":"…"} (at the end, if it fails halfway)
//
// For programs that read large catalogs (internal/sqlserver: Stream); same rules and audit as sqlRun.
func (s *Servidor) sqlStream(w http.ResponseWriter, r *http.Request, login, origin string, project int, session, name, batch string, write bool) {
	x, ok := s.sqlConnFor(w, project, session, name)
	if !ok {
		return
	}
	if write && !x.conn.Config().AllowChanges {
		jsonError(w, http.StatusForbidden, "la conexión es de solo lectura: para escribir, permite los cambios en las propiedades de la sesión")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	w.Header().Set("Content-Type", "application/x-ndjson")
	sink := &ndjsonSink{enc: json.NewEncoder(w)}
	sink.flush, _ = w.(http.Flusher)
	res, err := x.conn.Stream(ctx, batch, write, sink)
	s.sqls.used(x)
	n, _ := sqlConnName(name)
	e := s.sqlAuditEntry(login, origin, project, session, n, x.conn.Config(), batch, write, &res, err)
	e["rows"], e["stream"] = sink.n, true
	appendAudit(s.SQLAudit, "SQL Server", e)
	if err != nil {
		sink.enc.Encode(map[string]string{"error": err.Error()})
		return
	}
	sink.enc.Encode(map[string]any{"end": map[string]any{"affected": res.Affected, "rolled_back": res.RolledBack, "ms": res.Ms, "rows": sink.n}})
}

// ndjsonSink writes a batch's result sets and rows as JSON lines (sqlStream).
type ndjsonSink struct {
	enc   *json.Encoder
	flush http.Flusher
	sets  int
	n     int
}

func (k *ndjsonSink) Set(columns, types []string) error {
	err := k.enc.Encode(map[string]any{"set": k.sets, "columns": columns, "types": types})
	k.sets++
	return err
}

func (k *ndjsonSink) Row(values []any) error {
	k.n++
	if k.n > sqlStreamMax {
		return fmt.Errorf("demasiadas filas: como mucho %d por orden", sqlStreamMax)
	}
	if err := k.enc.Encode(map[string][]any{"row": values}); err != nil {
		return err // the program has gone: the batch stops
	}
	if k.flush != nil && k.n%500 == 0 {
		k.flush.Flush()
	}
	return nil
}
