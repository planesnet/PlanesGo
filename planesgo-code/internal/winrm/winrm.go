// Package winrm is pgocode's remote PowerShell: a WinRM connection to a Windows computer (a local user or
// DOMAIN\user, NTLM) that runs PowerShell scripts and returns their output, error stream and exit code, and moves
// files to and from it.
//
// The transport is always protected: HTTP (5985) with NTLM message encryption (every SOAP message sealed with the
// NTLM session key, 128-bit keys only), or HTTPS (5986) validating the server's certificate unless the person
// chooses to accept it without checking.
//
// Read only unless the connection allows changes: PowerShell has no transactions, so nothing can be undone. A
// read-only script has to pass ReadOnlyCheck (only query commands and harmless .NET members: readonly.go) and runs
// in PowerShell's ConstrainedLanguage mode. That stops accidents and the usual ways of changing things, not a
// determined user: for a guaranteed read-only access, the Windows user needs read-only rights on that computer.
package winrm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/unicode"

	wsman "github.com/masterzen/winrm"
)

const (
	MaxScript   = 64 << 10 // characters of one script
	MaxOutput   = 1 << 20  // bytes kept of each stream (stdout, stderr)
	MaxTransfer = 4 << 20  // bytes of a file sent or fetched
	HTTPPort    = 5985
	HTTPSPort   = 5986
	openTimeout = 30 * time.Second
	// operationTimeout: how long the server holds a request for output before answering «nothing yet» (WS-Man
	// OperationTimeout); responseTimeout has to be longer.
	operationTimeout = "PT30S"
	responseTimeout  = 60 * time.Second
	chunk            = 57 * 1024 // bytes per base64 line of a file transfer (a multiple of 3: no padding inside)
)

// Config: where and as whom. The password never leaves the process (it is not in String nor in any JSON).
type Config struct {
	Host         string `json:"host"` // computer or IP, with :port if it is not the default one
	User         string `json:"user"` // local user, or DOMAIN\user
	Password     string `json:"-"`
	HTTPS        bool   `json:"https"`
	AllowChanges bool   `json:"allow_changes"`
	// TrustCertificate (HTTPS): accept the server's certificate without checking it (a self-signed one, the usual
	// in a LAN); the traffic still goes encrypted.
	TrustCertificate bool `json:"trust_certificate"`
}

var (
	safeHost = regexp.MustCompile(`^(?:[A-Za-z0-9][A-Za-z0-9.\-_]{0,252}|\[[0-9A-Fa-f:.]{2,45}\])(?::(\d{1,5}))?$`)
	safeUser = regexp.MustCompile(`^(?:[A-Za-z0-9][A-Za-z0-9.\-_]{0,62}\\)?[^\x00-\x1f"/\\\[\]:;|=,+*?<>@]{1,104}$`)
)

// Validate checks the fields before they reach a connection.
func (c Config) Validate() error {
	m := safeHost.FindStringSubmatch(c.Host)
	switch {
	case m == nil:
		return errors.New("equipo no válido (nombre, IP o equipo:puerto)")
	case m[1] != "" && !validPort(m[1]):
		return errors.New("puerto no válido")
	case !safeUser.MatchString(c.User) || strings.HasPrefix(c.User, "-"):
		return errors.New(`usuario no válido (usuario local o DOMINIO\usuario)`)
	case c.Password == "":
		return errors.New("falta la contraseña")
	}
	return nil
}

func validPort(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0 && n < 65536
}

// String: the connection without its password, for the interface and the logs.
func (c Config) String() string {
	host, port := c.hostPort()
	scheme := "http"
	if c.HTTPS {
		scheme = "https"
	}
	return c.User + "@" + net.JoinHostPort(host, strconv.Itoa(port)) + " (" + scheme + ")"
}

// hostPort: the computer and the port (the default one of the transport if none is given).
func (c Config) hostPort() (string, int) {
	host, port := c.Host, 0
	if h, p, err := net.SplitHostPort(c.Host); err == nil {
		host, port = h, atoi(p)
	}
	host = strings.Trim(host, "[]")
	if port == 0 {
		port = HTTPPort
		if c.HTTPS {
			port = HTTPSPort
		}
	}
	return host, port
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// endpoint and params: how the library reaches the server. HTTP: NTLM with message encryption (128-bit keys only);
// HTTPS: NTLM inside TLS.
func (c Config) endpoint() *wsman.Endpoint {
	host, port := c.hostPort()
	return wsman.NewEndpoint(host, port, c.HTTPS, c.HTTPS && c.TrustCertificate, nil, nil, nil, responseTimeout)
}

func (c Config) params() (*wsman.Parameters, error) {
	p := *wsman.DefaultParameters
	p.Timeout = operationTimeout
	if c.HTTPS {
		p.TransportDecorator = func() wsman.Transporter { return &wsman.ClientNTLM{} }
		return &p, nil
	}
	enc, err := wsman.NewEncryptionWithSettings("ntlm", &wsman.Settings{NTLMKeyExchangeOptions: wsman.NTLMKeyExchangeOptions{MinimumKeyBits: 128}})
	if err != nil {
		return nil, err
	}
	p.TransportDecorator = func() wsman.Transporter { return enc }
	return &p, nil
}

// Conn: an open connection. Read only or with changes can be switched while it is open (SetAllowChanges). One
// command at a time: NTLM keeps its state on a single HTTP connection.
type Conn struct {
	client  *wsman.Client
	cfg     Config // without the password
	changes atomic.Bool
	closed  atomic.Bool
	run     sync.Mutex
	// Server: who the server says we are (computer and user), after opening.
	Server string
}

// Open connects and checks the login with a first command.
func Open(ctx context.Context, c Config) (*Conn, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	params, err := c.params()
	if err != nil {
		return nil, err
	}
	client, err := wsman.NewClientWithParameters(c.endpoint(), c.User, c.Password, params)
	if err != nil {
		return nil, err
	}
	c.Password = ""
	conn := &Conn{client: client, cfg: c}
	conn.changes.Store(c.AllowChanges)
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	res, err := conn.exec(ctx, `"$env:COMPUTERNAME ($env:USERDOMAIN\$env:USERNAME)"`, nil, false, false)
	if err != nil {
		return nil, explain(err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("el servidor no ejecuta PowerShell: %s", firstLine(res.Stderr))
	}
	conn.Server = strings.TrimSpace(res.Stdout)
	return conn, nil
}

// SetAllowChanges switches the connection between read only and with changes.
func (c *Conn) SetAllowChanges(on bool) { c.changes.Store(on) }

// Close: nothing runs on it any more. The library keeps no session on the server (each command opens and closes
// its own shell); its idle HTTP connection is closed by the server.
func (c *Conn) Close() error {
	c.closed.Store(true)
	return nil
}

// Config: the connection's data, without the password.
func (c *Conn) Config() Config {
	x := c.cfg
	x.AllowChanges = c.changes.Load()
	return x
}

// Result: what a script returned.
type Result struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exit_code"`
	Truncated bool   `json:"truncated,omitempty"` // the output was longer than MaxOutput
	ReadOnly  bool   `json:"read_only"`
	Ms        int64  `json:"ms"`
}

// Run executes a PowerShell script. Read only (the connection does not allow changes, or write is false): it has
// to pass ReadOnlyCheck and runs in ConstrainedLanguage mode.
func (c *Conn) Run(ctx context.Context, script string, write bool) (Result, error) {
	start := time.Now()
	script = strings.TrimSpace(script)
	if script == "" {
		return Result{}, errors.New("falta la orden PowerShell")
	}
	if utf8.RuneCountInString(script) > MaxScript {
		return Result{}, fmt.Errorf("la orden es demasiado larga (máximo %d caracteres)", MaxScript)
	}
	write = write && c.changes.Load()
	if !write {
		if why := ReadOnlyCheck(script); why != "" {
			return Result{}, errors.New(why)
		}
	}
	res, err := c.exec(ctx, script, nil, !write, true)
	res.Ms = time.Since(start).Milliseconds()
	return res, explain(err)
}

// Download copies a remote file (at most MaxTransfer bytes) to w. Allowed when read only: the script is pgocode's.
func (c *Conn) Download(ctx context.Context, remote string, w io.Writer) (int64, error) {
	if err := validRemotePath(remote); err != nil {
		return 0, err
	}
	p := quote(remote)
	script := fmt.Sprintf(`$f = Get-Item -LiteralPath %s -Force -ErrorAction Stop
if ($f.PSIsContainer) { throw 'es una carpeta, no un fichero' }
if ($f.Length -gt %d) { throw "el fichero es demasiado grande ($($f.Length) bytes; máximo %d)" }
$s = [IO.File]::OpenRead($f.FullName)
try { $b = New-Object byte[] %d; while (($n = $s.Read($b, 0, $b.Length)) -gt 0) { [Console]::Out.WriteLine([Convert]::ToBase64String($b, 0, $n)) } } finally { $s.Close() }`,
		p, MaxTransfer, MaxTransfer, chunk)
	dec := &base64Lines{w: w, max: MaxTransfer}
	res, err := c.execTo(ctx, script, nil, false, false, dec)
	if err != nil {
		return dec.n, explain(err)
	}
	if res.ExitCode != 0 {
		return dec.n, fmt.Errorf("no se puede bajar %s: %s", remote, firstLine(res.Stderr))
	}
	return dec.n, dec.flush()
}

// Upload writes r (at most MaxTransfer bytes) to a remote file, replacing it. Only when changes are allowed.
func (c *Conn) Upload(ctx context.Context, r io.Reader, remote string) (int64, error) {
	if !c.changes.Load() {
		return 0, errors.New("la conexión es de solo lectura: para subir ficheros, permite los cambios")
	}
	if err := validRemotePath(remote); err != nil {
		return 0, err
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxTransfer+1))
	if err != nil {
		return 0, err
	}
	if len(data) > MaxTransfer {
		return 0, fmt.Errorf("el fichero es demasiado grande (máximo %d bytes)", MaxTransfer)
	}
	script := fmt.Sprintf(`$s = [IO.File]::Create(%s)
try { foreach ($l in $pgoInput) { if ($l) { $b = [Convert]::FromBase64String($l); $s.Write($b, 0, $b.Length) } } } finally { $s.Close() }`, quote(remote))
	var payload bytes.Buffer
	for off := 0; off < len(data); off += chunk {
		payload.WriteString(base64.StdEncoding.EncodeToString(data[off:min(off+chunk, len(data))]) + "\n")
	}
	res, err := c.exec(ctx, script, &payload, false, false)
	if err != nil {
		return 0, explain(err)
	}
	if res.ExitCode != 0 {
		return 0, fmt.Errorf("no se puede subir a %s: %s", remote, firstLine(res.Stderr))
	}
	return int64(len(data)), nil
}

// validRemotePath: a Windows path for a transfer (it goes quoted into a script).
func validRemotePath(p string) error {
	if strings.TrimSpace(p) == "" || len(p) > 1024 || strings.ContainsFunc(p, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return errors.New("ruta remota no válida")
	}
	return nil
}

// quote: a PowerShell literal string ('…', with its quotes doubled, also the typographic ones PowerShell accepts).
func quote(s string) string {
	return "'" + strings.NewReplacer("'", "''", "‘", "‘‘", "’", "’’", "‚", "‚‚", "‛", "‛‛").Replace(s) + "'"
}

// exec runs a script (with optional extra input lines for it, in $pgoInput) and keeps at most MaxOutput bytes of each
// stream.
func (c *Conn) exec(ctx context.Context, script string, input io.Reader, constrained, format bool) (Result, error) {
	out := &capped{max: MaxOutput}
	return c.execTo(ctx, script, input, constrained, format, out)
}

func (c *Conn) execTo(ctx context.Context, script string, input io.Reader, constrained, format bool, stdout io.Writer) (Result, error) {
	if c.closed.Load() {
		return Result{}, errors.New("la conexión está cerrada")
	}
	cmd, err := command(constrained, format)
	if err != nil {
		return Result{}, err
	}
	stdin := io.MultiReader(strings.NewReader(base64.StdEncoding.EncodeToString([]byte(script))+"\n"), orEmpty(input))
	stderr := &capped{max: MaxOutput}
	c.run.Lock()
	defer c.run.Unlock()
	code, err := c.client.RunWithContextWithInput(ctx, cmd, stdout, stderr, stdin)
	res := Result{Stderr: cleanErrors(stderr.String()), ExitCode: code, ReadOnly: constrained, Truncated: stderr.truncated}
	if o, ok := stdout.(*capped); ok {
		res.Stdout, res.Truncated = o.String(), res.Truncated || o.truncated
	}
	if err != nil && ctx.Err() != nil {
		return res, errors.New("la orden ha tardado demasiado: se ha cancelado")
	}
	return res, err
}

func orEmpty(r io.Reader) io.Reader {
	if r == nil {
		return strings.NewReader("")
	}
	return r
}

// bootstrap: the fixed command line. The script comes by the standard input (first line, base64 of UTF-8), so it
// has no length limit and never goes in a command line; the rest of the input is left for it in $pgoInput.
// Constrained: PowerShell's ConstrainedLanguage mode before running it (no arbitrary .NET, COM or Add-Type).
// Format: the output as text lines wide enough not to cut tables; the exit code, 1 if there were errors.
const bootstrap = `[Console]::OutputEncoding = [Text.Encoding]::UTF8
$pgoInput = $input
$null = $pgoInput.MoveNext()
$pgoScript = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String([string]$pgoInput.Current))
%s
%s`

func command(constrained, format bool) (string, error) {
	mode := ""
	if constrained {
		mode = `$ExecutionContext.SessionState.LanguageMode = 'ConstrainedLanguage'`
	}
	run := `Invoke-Expression $pgoScript`
	if format {
		run = `Invoke-Expression $pgoScript | Out-String -Stream -Width 250
if ($LASTEXITCODE) { exit $LASTEXITCODE }
if ($Error.Count) { exit 1 }`
	}
	return encodedCommand(fmt.Sprintf(bootstrap, mode, run))
}

// encodedCommand: powershell.exe with the script as -EncodedCommand (base64 of UTF-16LE).
func encodedCommand(script string) (string, error) {
	u, err := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder().String("$ProgressPreference = 'SilentlyContinue'\n" + script)
	if err != nil {
		return "", err
	}
	return "powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand " + base64.StdEncoding.EncodeToString([]byte(u)), nil
}

// capped keeps the first max bytes written (the rest is read and dropped, so the command does not stall).
type capped struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room < len(p) {
		c.truncated = true
		c.buf.Write(p[:max(room, 0)])
		return len(p), nil
	}
	return c.buf.Write(p)
}

func (c *capped) String() string { return strings.ToValidUTF8(c.buf.String(), "�") }

// base64Lines decodes the lines of a download into w, up to max bytes.
type base64Lines struct {
	w    io.Writer
	max  int64
	n    int64
	part []byte
	err  error
}

func (b *base64Lines) Write(p []byte) (int, error) {
	if b.err != nil {
		return len(p), nil // keep reading, so the command ends
	}
	b.part = append(b.part, p...)
	for {
		i := bytes.IndexByte(b.part, '\n')
		if i < 0 {
			break
		}
		b.line(b.part[:i])
		b.part = b.part[i+1:]
	}
	if len(b.part) > 2*chunk {
		b.err = errors.New("respuesta del servidor no válida")
	}
	return len(p), nil
}

func (b *base64Lines) line(l []byte) {
	l = bytes.TrimSpace(l)
	if len(l) == 0 || b.err != nil {
		return
	}
	d := make([]byte, base64.StdEncoding.DecodedLen(len(l)))
	n, err := base64.StdEncoding.Decode(d, l)
	if err != nil {
		b.err = errors.New("respuesta del servidor no válida")
		return
	}
	if b.n+int64(n) > b.max {
		b.err = fmt.Errorf("el fichero es demasiado grande (máximo %d bytes)", b.max)
		return
	}
	if _, err := b.w.Write(d[:n]); err != nil {
		b.err = err
		return
	}
	b.n += int64(n)
}

func (b *base64Lines) flush() error {
	b.line(b.part)
	b.part = nil
	return b.err
}

func firstLine(s string) string {
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if t := strings.TrimSpace(sc.Text()); t != "" {
			return t
		}
	}
	return "error sin mensaje"
}

// explain: the library's errors in a sentence for the person.
func explain(err error) error {
	if err == nil {
		return nil
	}
	m := err.Error()
	var ne net.Error
	switch {
	case strings.Contains(m, "http error 401") || strings.Contains(m, "returned HTTP 401") || strings.Contains(m, "HTTP 401"):
		return errors.New("usuario o contraseña incorrectos (o el usuario no puede usar WinRM en ese equipo)")
	case strings.Contains(m, "x509") || strings.Contains(m, "certificate"):
		return fmt.Errorf("el certificado del servidor no es válido (%s): si es uno propio de la red local, acepta el certificado sin comprobarlo", m)
	case strings.Contains(m, "below the required minimum"):
		return errors.New("el servidor solo ofrece cifrado NTLM débil (menos de 128 bits): usa HTTPS")
	case errors.As(err, &ne) && ne.Timeout(), strings.Contains(m, "connection refused"), strings.Contains(m, "no such host"):
		return fmt.Errorf("no responde el servidor: revisa el nombre o la IP, el puerto (5985 HTTP, 5986 HTTPS), el cortafuegos y que WinRM esté activado (%s)", m)
	}
	return err
}
