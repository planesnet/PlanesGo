// Package sqlserver is pgocode's connection to a Microsoft SQL Server (Sage and the like): open it with a SQL Server
// user or a Windows one (DOMAIN\user, NTLM), and run batches that return their result sets as plain values.
//
// Read only unless the connection allows changes: each batch runs inside a transaction that is always rolled back,
// and batches that would end that transaction themselves (COMMIT, ROLLBACK, BEGIN/SAVE TRANSACTION) are refused, as is
// dynamic SQL (EXEC of a string, sp_executesql), where those statements would hide inside a literal.
// That stops accidents, not a determined user: for a guaranteed read-only access, the login needs read-only rights
// in SQL Server too (db_datareader).
package sqlserver

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	mssql "github.com/microsoft/go-mssqldb"
)

const (
	DefaultRows = 200
	MaxRows     = 5000
	MaxBatch    = 64 << 10 // characters of one batch
	openTimeout = 20 * time.Second
)

// Config: where and as whom. The password never leaves the process (it is not in String nor in any JSON).
type Config struct {
	Server       string `json:"server"`   // host, host:port or host\instance
	Database     string `json:"database"` // empty: the login's default database
	User         string `json:"user"`     // SQL Server user, or DOMAIN\user for Windows authentication
	Password     string `json:"-"`
	AllowChanges bool   `json:"allow_changes"`
	// TrustCertificate: accept the server's certificate without checking it (servers with a self-signed one,
	// the usual in a LAN); the login and the data still go encrypted.
	TrustCertificate bool `json:"trust_certificate"`
}

var (
	safeServer = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-_]*(?:\\[A-Za-z0-9$_\-]+)?(?::\d{1,5})?$`)
	safeName   = regexp.MustCompile(`^[^\x00-\x1f;{}\[\]]{1,128}$`)
)

// Validate checks the fields before they reach a connection string.
func (c Config) Validate() error {
	switch {
	case !safeServer.MatchString(c.Server):
		return errors.New("servidor no válido (equipo, equipo:puerto o equipo\\instancia)")
	case c.Database != "" && !safeName.MatchString(c.Database):
		return errors.New("base de datos no válida")
	case !safeName.MatchString(c.User):
		return errors.New("usuario no válido")
	case c.Password == "":
		return errors.New("falta la contraseña")
	}
	return nil
}

// Windows: whether the user is a Windows one (DOMAIN\user: NTLM).
func (c Config) Windows() bool { return strings.Contains(c.User, `\`) }

// String: the connection without its password, for the interface and the logs.
func (c Config) String() string {
	s := c.User + "@" + c.Server
	if c.Database != "" {
		s += "/" + c.Database
	}
	return s
}

// dsn builds the connection URL (every value escaped by net/url).
func (c Config) dsn() string {
	host, instance, _ := strings.Cut(c.Server, `\`)
	port := ""
	if h, p, err := net.SplitHostPort(host); err == nil {
		host, port = h, p
	} else if i := strings.LastIndex(instance, ":"); i >= 0 {
		instance, port = instance[:i], instance[i+1:]
	}
	u := url.URL{Scheme: "sqlserver", User: url.UserPassword(c.User, c.Password), Host: host}
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	}
	if instance != "" {
		u.Path = instance
	}
	q := url.Values{}
	q.Set("app name", "pgocode")
	q.Set("dial timeout", strconv.Itoa(int(openTimeout.Seconds())))
	q.Set("encrypt", "true")
	if c.TrustCertificate {
		q.Set("TrustServerCertificate", "true")
	}
	if c.Database != "" {
		q.Set("database", c.Database)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// Conn: an open connection. Read only or with changes can be switched while it is open (SetAllowChanges).
type Conn struct {
	db       *sql.DB
	cfg      Config // without the password
	changes  atomic.Bool
	database atomic.Value // string: the database the batches run in ("" = the login's default)
	// Server: what SQL Server says it is (version and edition), after opening.
	Server string
}

// Open connects and checks the login.
func Open(ctx context.Context, c Config) (*Conn, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlserver", c.dsn())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	db.SetConnMaxIdleTime(10 * time.Minute)
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	var version string
	if err := db.QueryRowContext(ctx, "SELECT CAST(SERVERPROPERTY('ProductVersion') AS nvarchar(64)) + ' ' + CAST(SERVERPROPERTY('Edition') AS nvarchar(128))").Scan(&version); err != nil {
		db.Close()
		return nil, explain(err)
	}
	c.Password = ""
	conn := &Conn{db: db, cfg: c, Server: version}
	conn.changes.Store(c.AllowChanges)
	conn.database.Store(c.Database)
	return conn, nil
}

// Databases: the databases the login can use.
func (c *Conn) Databases(ctx context.Context) ([]string, error) {
	rows, err := c.db.QueryContext(ctx, "SELECT name FROM sys.databases WHERE HAS_DBACCESS(name) = 1 AND state_desc = 'ONLINE' ORDER BY name")
	if err != nil {
		return nil, explain(err)
	}
	defer rows.Close()
	var l []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			l = append(l, n)
		}
	}
	return l, rows.Err()
}

// SetDatabase changes the database the batches run in (it has to be one the login can use).
func (c *Conn) SetDatabase(ctx context.Context, name string) error {
	if name != "" && !safeName.MatchString(name) {
		return errors.New("base de datos no válida")
	}
	if name != "" {
		l, err := c.Databases(ctx)
		if err != nil {
			return err
		}
		if !slices.Contains(l, name) {
			return fmt.Errorf("el usuario no tiene acceso a la base de datos «%s»", name)
		}
	}
	c.database.Store(name)
	return nil
}

// quoteName: a SQL Server name between brackets (QUOTENAME).
func quoteName(s string) string { return "[" + strings.ReplaceAll(s, "]", "]]") + "]" }

// SetAllowChanges switches the connection between read only and with changes.
func (c *Conn) SetAllowChanges(on bool) { c.changes.Store(on) }

// Close closes the connection.
func (c *Conn) Close() error { return c.db.Close() }

// Config: the connection's data, without the password.
func (c *Conn) Config() Config {
	x := c.cfg
	x.AllowChanges = c.changes.Load()
	x.Database, _ = c.database.Load().(string)
	return x
}

// ResultSet: one result of a batch.
type ResultSet struct {
	Columns   []string `json:"columns"`
	Types     []string `json:"types"`
	Rows      [][]any  `json:"rows"`
	Truncated bool     `json:"truncated,omitempty"` // there were more rows than the limit
}

// Result: what a batch returned.
type Result struct {
	Sets       []ResultSet `json:"sets"`
	Affected   int64       `json:"affected"` // rows of the batch's last statement (@@ROWCOUNT; undone when read only)
	RolledBack bool        `json:"rolled_back"`
	Ms         int64       `json:"ms"`
}

// transactionControl: statements that would end the read-only transaction themselves.
var transactionControl = regexp.MustCompile(`(?i)\b(COMMIT|ROLLBACK|SAVE\s+TRAN(SACTION)?|BEGIN\s+(DISTRIBUTED\s+)?TRAN(SACTION)?)\b`)

// dynamicSQL: SQL built from a string (EXEC('…'), EXEC(@sql), sp_executesql), where a COMMIT would not be seen,
// and procedures that act outside the database (xp_cmdshell, the sp_OA* of OLE objects), which no rollback undoes:
// the literals are removed before looking (the names between brackets are kept).
var dynamicSQL = regexp.MustCompile(`(?i)\bEXEC(UTE)?\s*\(|\bsp_executesql\b|\bsp_prepexec\b|\bsp_prepare\b|\bsp_execute\b|\bsp_sqlexec\b|\bxp_cmdshell\b|\bsp_OA\w+`)

// execVariable: EXEC of a procedure whose name is in a variable, with or without a return value (EXEC @p …,
// EXEC @r = @p …): the variable can hold sp_executesql. EXEC @r = dbo.proc (a name) is not one.
var execVariable = regexp.MustCompile(`(?i)\bEXEC(?:UTE)?\s+@[\w@$#]+`)

// execOfVariable: whether code runs a procedure named by a variable (see execVariable).
func execOfVariable(code string) bool {
	for _, m := range execVariable.FindAllStringIndex(code, -1) {
		rest := strings.TrimLeft(code[m[1]:], " \t\r\n")
		after, isReturn := strings.CutPrefix(rest, "=")
		if !isReturn || strings.HasPrefix(strings.TrimLeft(after, " \t\r\n"), "@") {
			return true
		}
	}
	return false
}

// ReadOnlyCheck: why a batch cannot run in a read-only connection ("" if it can).
func ReadOnlyCheck(batch string) string {
	if m := transactionControl.FindString(stripLiterals(batch, false)); m != "" {
		return fmt.Sprintf("«%s» no se puede usar en una conexión de solo lectura", strings.ToUpper(m))
	}
	code := stripLiterals(batch, true)
	if dynamicSQL.MatchString(code) {
		return "el SQL dinámico (EXEC de un texto, sp_executesql, sp_prepexec…) y xp_cmdshell o sp_OA* no se pueden usar en una conexión de solo lectura"
	}
	if execOfVariable(code) {
		return "EXEC de un procedimiento guardado en una variable no se puede usar en una conexión de solo lectura"
	}
	return ""
}

// stripLiterals removes strings, quoted names and comments, so that keywords are only found in the code. With
// keepNames, the content of quoted names ([sp_executesql], "x") stays, without its quotes: a procedure is still found
// by its name when it is written between brackets.
func stripLiterals(s string, keepNames bool) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "--"):
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				return b.String()
			}
			i += j
		case strings.HasPrefix(s[i:], "/*"):
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				return b.String()
			}
			i += j + 4
			b.WriteByte(' ')
		case s[i] == '\'' || s[i] == '"' || s[i] == '[':
			end := map[byte]byte{'\'': '\'', '"': '"', '[': ']'}[s[i]]
			j := i + 1
			for j < len(s) {
				if s[j] == end {
					if j+1 < len(s) && s[j+1] == end { // doubled quote: still inside
						j += 2
						continue
					}
					break
				}
				j++
			}
			if keepNames && end != '\'' && j <= len(s) {
				b.WriteByte(' ')
				b.WriteString(strings.ReplaceAll(s[i+1:min(j, len(s))], string([]byte{end, end}), string(end)))
			}
			i = j + 1
			b.WriteByte(' ')
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// Run executes a batch and returns up to maxRows rows of each result set. Read only (the connection does not allow
// changes, or write is false): inside a transaction that is always rolled back.
func (c *Conn) Run(ctx context.Context, batch string, maxRows int, write bool) (Result, error) {
	start := time.Now()
	batch = strings.TrimSpace(batch)
	if batch == "" {
		return Result{}, errors.New("falta la orden SQL")
	}
	if utf8.RuneCountInString(batch) > MaxBatch {
		return Result{}, fmt.Errorf("la orden es demasiado larga (máximo %d caracteres)", MaxBatch)
	}
	if maxRows <= 0 {
		maxRows = DefaultRows
	}
	maxRows = min(maxRows, MaxRows)
	write = write && c.changes.Load()
	if !write {
		if why := ReadOnlyCheck(batch); why != "" {
			return Result{}, errors.New(why)
		}
	}
	// One connection of the pool for the whole batch (the chosen database is set on it). Read only: inside a
	// transaction that is always rolled back. With changes: straight on it, as sqlcmd does, so statements that
	// cannot run inside a transaction (CREATE DATABASE, BACKUP…) work too.
	conn, err := c.db.Conn(ctx)
	if err != nil {
		return Result{}, explain(err)
	}
	defer conn.Close()
	var q querier = conn
	var tx *sql.Tx
	if !write {
		if tx, err = conn.BeginTx(ctx, nil); err != nil {
			return Result{}, explain(err)
		}
		defer tx.Rollback()
		q = tx
	}
	if db, _ := c.database.Load().(string); db != "" {
		if _, err := q.ExecContext(ctx, "USE "+quoteName(db)); err != nil {
			return Result{}, explain(err)
		}
	} // without one, the login's default: the driver resets a pooled connection before reusing it
	// The read-only transaction, by its id (SQL Server 2016 or later; NULL before): a batch that commits it and opens
	// another one is found below, which the count of open transactions alone would not show.
	var txID sql.NullInt64
	if tx != nil {
		q.QueryRowContext(ctx, "SELECT CURRENT_TRANSACTION_ID()").Scan(&txID)
	}
	var res Result
	rows, err := q.QueryContext(ctx, batch)
	if err != nil {
		return Result{}, explain(err)
	}
	for {
		set, err := readSet(rows, maxRows)
		if err != nil {
			rows.Close()
			return Result{}, explain(err)
		}
		if len(set.Columns) > 0 {
			res.Sets = append(res.Sets, set)
		}
		if !rows.NextResultSet() {
			break
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Result{}, explain(err)
	}
	rows.Close()
	open := -1 // unknown if the query fails: not taken as ended
	var nowID sql.NullInt64
	if txID.Valid {
		q.QueryRowContext(ctx, "SELECT CAST(@@ROWCOUNT AS bigint), @@TRANCOUNT, CURRENT_TRANSACTION_ID()").Scan(&res.Affected, &open, &nowID)
	} else {
		q.QueryRowContext(ctx, "SELECT CAST(@@ROWCOUNT AS bigint), @@TRANCOUNT").Scan(&res.Affected, &open)
	}
	// Whatever got past ReadOnlyCheck (a procedure with its own COMMIT…): the transaction that undoes the batch is
	// gone (or it is another one), so its changes may have stayed. Said, not hidden.
	if tx != nil && (open == 0 || (txID.Valid && nowID.Valid && nowID.Int64 != txID.Int64)) {
		return Result{}, errors.New("la orden ha terminado por su cuenta la transacción de solo lectura: lo que haya cambiado puede haberse guardado")
	}
	res.RolledBack = tx != nil // the deferred Rollback undoes it
	res.Ms = time.Since(start).Milliseconds()
	return res, nil
}

// querier: a connection or a transaction.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func readSet(rows *sql.Rows, maxRows int) (ResultSet, error) {
	cols, err := rows.Columns()
	if err != nil || len(cols) == 0 {
		return ResultSet{}, err
	}
	types, _ := rows.ColumnTypes()
	set := ResultSet{Columns: cols, Types: make([]string, len(cols)), Rows: [][]any{}}
	for i, t := range types {
		set.Types[i] = t.DatabaseTypeName()
	}
	for rows.Next() {
		if len(set.Rows) >= maxRows {
			set.Truncated = true
			continue // read the rest, so the next result set comes
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return set, err
		}
		for i, v := range vals {
			vals[i] = plain(v, set.Types[i])
		}
		set.Rows = append(set.Rows, vals)
	}
	return set, rows.Err()
}

// plain turns a driver value into one that JSON shows as it is.
func plain(v any, typ string) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		switch typ {
		case "UNIQUEIDENTIFIER":
			var u mssql.UniqueIdentifier
			if u.Scan(x) == nil {
				return u.String()
			}
		case "DECIMAL", "NUMERIC", "MONEY", "SMALLMONEY":
			return string(x)
		}
		if utf8.Valid(x) && typ != "BINARY" && typ != "VARBINARY" && typ != "IMAGE" && typ != "TIMESTAMP" {
			return string(x)
		}
		return "0x" + strings.ToUpper(hex.EncodeToString(x))
	case time.Time:
		return x.Format("2006-01-02T15:04:05.999999999")
	}
	return v
}

// explain: SQL Server's errors in a sentence (the server's own message, with its number and line).
func explain(err error) error {
	var e mssql.Error
	if errors.As(err, &e) {
		switch e.Number {
		case 18456:
			return errors.New("usuario o contraseña incorrectos (o el usuario no puede entrar en esa base de datos)")
		case 4060:
			return fmt.Errorf("no se puede abrir la base de datos: %s", e.Message)
		}
		return fmt.Errorf("SQL Server %d (línea %d): %s", e.Number, e.LineNo, e.Message)
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return errors.New("no responde el servidor: revisa el nombre o la IP, el puerto (1433) y el cortafuegos")
	}
	return err
}
