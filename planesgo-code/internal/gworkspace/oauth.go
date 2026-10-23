package gworkspace

// Conexión propia con Google: OAuth 2.0 de aplicación de escritorio (RFC 8252) con PKCE (RFC 7636) y vuelta a un
// puerto efímero de 127.0.0.1.
//
//   - El cliente OAuth («PlanesGo», tipo «Aplicación de escritorio») se registra una vez por empresa en Google Cloud.
//     Su JSON (el que descarga la consola) se carga en Propiedades y se guarda en <datos>/google-oauth-client.json
//     (0600); si no está, ClientSource (más adelante, el servidor de PlanesGo). Según Google, el secreto de un cliente
//     de escritorio no es confidencial, pero no sale nunca del backend (ni a la interfaz ni a los registros).
//   - Cada usuario de pgocode conecta su cuenta: StartAuth abre un puerto efímero y devuelve la URL de consentimiento
//     (la interfaz la abre en el navegador del sistema: Google no deja iniciar sesión en navegadores integrados como
//     el de Electron); Google vuelve a http://127.0.0.1:<puerto>/?code=…&state=…, se cambia el código por los tokens
//     (con el verificador PKCE) y se cierra el puerto.
//   - El token de renovación se guarda solo en el equipo: <datos>/google/<usuario>.json (carpeta 0700, fichero 0600).
//     El de acceso vive en memoria y se renueva solo. Disconnect lo revoca en Google y borra el fichero.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ClientFile: el fichero del cliente OAuth en la carpeta de datos de pgocode.
const ClientFile = "google-oauth-client.json"

// connDir: la carpeta (dentro de la de datos) de las conexiones de cada usuario.
const connDir = "google"

// OAuthScopes: lo que se pide a Google: la cuenta, leer Drive y leer Gmail (nada más). En la pantalla de Google se
// puede desmarcar Drive o Gmail: lo que quede marcado funciona y lo otro sale como no autorizado.
const OAuthScopes = "openid email " + ScopeDriveReadonly + " " + ScopeGmailReadonly

// Origen del cliente OAuth.
const (
	ClientFromFile   = "fichero"
	ClientFromServer = "servidor"
)

var (
	// ErrNoClient: no hay cliente OAuth (ni fichero ni servidor).
	ErrNoClient = errors.New("falta registrar la aplicación de Google en pgocode: carga el fichero del cliente (Propiedades → Google Workspace)")
	// ErrRevoked: Google ya no acepta el token de renovación (se quitó el permiso o cambió la aplicación).
	errInvalidClient = errors.New("Google no reconoce la aplicación de pgocode (cliente OAuth): comprueba el fichero del cliente y vuelve a cargarlo")
	ErrRevoked       = fmt.Errorf("%w (Google ha retirado el permiso de esta conexión: vuelve a conectar)", ErrNoAccess)
)

// OAuthClient: el cliente OAuth de escritorio. El secreto no se serializa nunca hacia fuera.
type OAuthClient struct {
	ID        string
	Secret    string
	ProjectID string
	Origin    string // fichero | servidor
}

// clientJSON: el formato de la consola de Google (solo lo que se usa).
type clientJSON struct {
	Installed *struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		ProjectID    string `json:"project_id,omitempty"`
	} `json:"installed"`
	Web json.RawMessage `json:"web,omitempty"`
}

// ParseClientFile lee el JSON de un cliente OAuth de escritorio tal como lo descarga la consola de Google.
func ParseClientFile(b []byte) (*OAuthClient, error) {
	var j clientJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return nil, errors.New("no es el fichero JSON de un cliente OAuth de Google")
	}
	if j.Installed == nil {
		if len(j.Web) > 0 {
			return nil, errors.New("es un cliente de tipo «Aplicación web»: hace falta uno de tipo «Aplicación de escritorio»")
		}
		return nil, errors.New("no es el fichero de un cliente OAuth de escritorio de Google (falta «installed»)")
	}
	c := &OAuthClient{ID: strings.TrimSpace(j.Installed.ClientID), Secret: strings.TrimSpace(j.Installed.ClientSecret), ProjectID: strings.TrimSpace(j.Installed.ProjectID)}
	if !strings.HasSuffix(c.ID, ".apps.googleusercontent.com") || strings.ContainsAny(c.ID, " /?#&") {
		return nil, errors.New("el fichero no tiene un identificador de cliente de Google válido")
	}
	if c.Secret == "" {
		return nil, errors.New("el fichero no tiene el secreto del cliente")
	}
	return c, nil
}

// SaveClientFile valida y guarda el fichero del cliente (solo lo necesario, 0600). Devuelve el cliente.
func (m *Manager) SaveClientFile(b []byte) (*OAuthClient, error) {
	if m.Dir == "" {
		return nil, errors.New("pgocode no tiene carpeta de datos")
	}
	if len(b) > 64<<10 {
		return nil, errors.New("el fichero es demasiado grande para ser el de un cliente OAuth")
	}
	c, err := ParseClientFile(b)
	if err != nil {
		return nil, err
	}
	var j clientJSON
	j.Installed = &struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		ProjectID    string `json:"project_id,omitempty"`
	}{c.ID, c.Secret, c.ProjectID}
	out, _ := json.MarshalIndent(j, "", "  ")
	if err := writePrivate(filepath.Join(m.Dir, ClientFile), out); err != nil {
		return nil, fmt.Errorf("no se pudo guardar el fichero del cliente: %w", err)
	}
	c.Origin = ClientFromFile
	return c, nil
}

// oauthClient: el cliente OAuth (el fichero; si no está, ClientSource).
func (m *Manager) oauthClient(ctx context.Context) (*OAuthClient, error) {
	if m.Dir != "" {
		b, err := os.ReadFile(filepath.Join(m.Dir, ClientFile))
		if err == nil {
			c, err := ParseClientFile(b)
			if err != nil {
				return nil, fmt.Errorf("el fichero del cliente de Google no vale (%v): vuelve a cargarlo", err)
			}
			c.Origin = ClientFromFile
			return c, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("no se pudo leer el fichero del cliente de Google: %w", err)
		}
	}
	if m.ClientSource != nil {
		c, err := m.ClientSource(ctx)
		if err == nil && c != nil && c.ID != "" {
			c.Origin = ClientFromServer
			return c, nil
		}
	}
	return nil, ErrNoClient
}

// ---- conexiones (una por usuario de pgocode) ----

type connection struct {
	Login        string    `json:"login"`
	Email        string    `json:"email,omitempty"`
	Scopes       []string  `json:"ambitos,omitempty"`
	RefreshToken string    `json:"refresh_token"`
	ClientID     string    `json:"client_id"`
	Created      time.Time `json:"creada"`
}

// connName: el nombre del fichero de un usuario (el login si es seguro; si no, un resumen).
func connName(login string) string {
	l := strings.ToLower(strings.TrimSpace(login))
	ok := l != "" && len(l) <= 100 && l[0] != '.'
	for _, r := range l {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("._@-+", r)) {
			ok = false
			break
		}
	}
	if !ok {
		h := sha256.Sum256([]byte(login))
		l = "u-" + hex.EncodeToString(h[:12])
	}
	return l + ".json"
}

func (m *Manager) connPath(login string) string {
	return filepath.Join(m.Dir, connDir, connName(login))
}

func (m *Manager) hasConnection(login string) bool {
	if login == "" || m.Dir == "" {
		return false
	}
	_, err := os.Stat(m.connPath(login))
	return err == nil
}

func (m *Manager) readConnection(path string) (*connection, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c connection
	if err := json.Unmarshal(b, &c); err != nil || c.RefreshToken == "" {
		return nil, errors.New("conexión con Google dañada")
	}
	return &c, nil
}

func (m *Manager) loadConnection(login string) (*connection, error) {
	if login == "" || m.Dir == "" {
		return nil, fs.ErrNotExist
	}
	return m.readConnection(m.connPath(login))
}

// writePrivate escribe un fichero 0600 (carpeta 0700) de una vez (temporal y renombrado).
func writePrivate(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ---- endpoint de tokens ----

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

// postToken: una petición al endpoint de tokens (los datos en el cuerpo, nunca en la URL).
func (m *Manager) postToken(ctx context.Context, v url.Values) (int, tokenResponse, error) {
	var t tokenResponse
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, m.Endpoints.Token, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := m.client().Do(req)
	if err != nil {
		return 0, t, fmt.Errorf("no se pudo hablar con Google: %w", err)
	}
	defer res.Body.Close()
	json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&t)
	return res.StatusCode, t, nil
}

func (t tokenResponse) problem() string {
	return firstLine(strings.TrimSpace(t.Error + ": " + t.Description))
}

// oauthToken: el token de acceso del usuario (de la memoria o renovado con el de renovación).
func (m *Manager) oauthToken(ctx context.Context, login string) (*cachedToken, error) {
	fresh := func() *cachedToken {
		m.mu.Lock()
		defer m.mu.Unlock()
		if c := m.access[login]; c != nil && m.now().Before(c.info.Expiry) {
			return c
		}
		return nil
	}
	if c := fresh(); c != nil {
		return c, nil
	}
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	if c := fresh(); c != nil {
		return c, nil
	}
	conn, err := m.loadConnection(login)
	if err != nil {
		return nil, ErrNoAccess
	}
	client, err := m.oauthClient(ctx)
	if err != nil {
		return nil, err
	}
	code, t, err := m.postToken(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {conn.RefreshToken},
		"client_id": {client.ID}, "client_secret": {client.Secret}})
	if err != nil {
		return nil, err
	}
	switch {
	case code == http.StatusOK && t.AccessToken != "":
	case t.Error == "invalid_grant" || t.Error == "unauthorized_client":
		// El permiso ya no vale (revocado en la cuenta, caducado o de otro cliente): la conexión sobra.
		m.removeConnection(login)
		return nil, ErrRevoked
	case t.Error == "invalid_client":
		return nil, errInvalidClient
	default:
		return nil, fmt.Errorf("Google no renueva el acceso (%s)", strings.TrimSpace(t.problem()+" "+http.StatusText(code)))
	}
	scopes := strings.Fields(t.Scope)
	if len(scopes) == 0 {
		scopes = conn.Scopes
	}
	sort.Strings(scopes)
	exp := t.ExpiresIn
	if exp <= 0 {
		exp = 3600
	}
	c := &cachedToken{token: t.AccessToken, source: SourceOAuth, info: TokenInfo{Email: conn.Email, Scopes: scopes,
		Expiry: m.now().Add(time.Duration(exp)*time.Second - time.Minute)}}
	m.mu.Lock()
	if m.access == nil {
		m.access = map[string]*cachedToken{}
	}
	m.access[login] = c
	m.mu.Unlock()
	return c, nil
}

func (m *Manager) removeConnection(login string) {
	if m.Dir != "" && login != "" {
		os.Remove(m.connPath(login))
	}
	m.mu.Lock()
	delete(m.access, login)
	m.mu.Unlock()
}

// ---- autorización ----

type authFlow struct {
	state, verifier, redirect string
	done                      bool   // terminado (bien, mal o sustituido)
	err                       string // por qué falló (lo ve la interfaz)
	once                      sync.Once
	cancel                    context.CancelFunc
}

func randomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// StartAuth empieza la conexión del usuario login: abre el puerto de vuelta y devuelve la URL de consentimiento de
// Google (la abre la interfaz). Lo que pase después se ve en Status (en_curso, problema, conectado).
func (m *Manager) StartAuth(ctx context.Context, login string) (string, error) {
	if login == "" {
		return "", errors.New("sin usuario de pgocode")
	}
	if m.Dir == "" {
		return "", errors.New("pgocode no tiene carpeta de datos")
	}
	client, err := m.oauthClient(ctx)
	if err != nil {
		return "", err
	}
	listen := m.Listen
	if listen == nil {
		listen = func() (net.Listener, error) { return net.Listen("tcp", "127.0.0.1:0") }
	}
	ln, err := listen()
	if err != nil {
		return "", fmt.Errorf("no se pudo abrir el puerto de vuelta de Google: %w", err)
	}
	f := &authFlow{state: randomString(24), verifier: randomString(48),
		redirect: fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)}
	sum := sha256.Sum256([]byte(f.verifier))
	q := url.Values{
		"client_id":             {client.ID},
		"redirect_uri":          {f.redirect},
		"response_type":         {"code"},
		"scope":                 {OAuthScopes},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"state":                 {f.state},
		"access_type":           {"offline"}, // para tener token de renovación
		"prompt":                {"consent"}, // siempre lo da (también si ya se aceptó antes)
		// Al volver a conectar (p. ej. para añadir Gmail), el token nuevo lleva también lo concedido antes.
		"include_granted_scopes": {"true"},
	}
	if strings.Contains(login, "@") {
		q.Set("login_hint", login)
	}
	timeout := m.AuthTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	wait, cancel := context.WithTimeout(context.Background(), timeout)
	f.cancel = cancel
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.callback(w, r, login, client, f)
	})}
	m.mu.Lock()
	if old := m.pending[login]; old != nil {
		old.done = true
		old.cancel()
	}
	if m.pending == nil {
		m.pending = map[string]*authFlow{}
	}
	m.pending[login] = f
	m.mu.Unlock()
	go srv.Serve(ln)
	go func() {
		<-wait.Done()
		m.mu.Lock()
		if !f.done {
			f.done = true
			f.err = "Se acabó el tiempo para aceptar en Google: vuelve a pulsar «Conectar con Google»"
		}
		m.mu.Unlock()
		sctx, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		srv.Shutdown(sctx)
	}()
	return m.Endpoints.Auth + "?" + q.Encode(), nil
}

// callback: Google vuelve al puerto efímero con el código (o con el error).
func (m *Manager) callback(w http.ResponseWriter, r *http.Request, login string, client *OAuthClient, f *authFlow) {
	q := r.URL.Query()
	if r.URL.Path != "/" || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if q.Get("state") != f.state || f.state == "" {
		page(w, http.StatusBadRequest, "Esta petición no es la que esperaba pgocode. Vuelve a pulsar «Conectar con Google».")
		return
	}
	handled := false
	f.once.Do(func() {
		handled = true
		var msg string
		switch e := q.Get("error"); {
		case e == "access_denied":
			msg = "Has cancelado el permiso en Google: pgocode no ha cambiado su acceso a tu cuenta."
		case e != "":
			msg = "Google no ha dado el permiso (" + firstLine(e+" "+q.Get("error_description")) + ")."
		case q.Get("code") == "":
			msg = "Google no ha devuelto el código de autorización."
		default:
			ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
			if err := m.exchange(ctx, login, client, f, q.Get("code")); err != nil {
				msg = err.Error()
			}
			c()
		}
		m.mu.Lock()
		f.done, f.err = true, msg
		m.mu.Unlock()
		page(w, http.StatusOK, msg)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		f.cancel()
	})
	if !handled {
		page(w, http.StatusOK, "Esta autorización ya se ha usado. Vuelve a pgocode.")
	}
}

// exchange cambia el código por los tokens (con el verificador PKCE) y guarda la conexión.
func (m *Manager) exchange(ctx context.Context, login string, client *OAuthClient, f *authFlow, code string) error {
	st, t, err := m.postToken(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {f.redirect},
		"client_id": {client.ID}, "client_secret": {client.Secret}, "code_verifier": {f.verifier}})
	if err != nil {
		return err
	}
	if t.Error == "invalid_client" {
		return errInvalidClient
	}
	if st != http.StatusOK || t.AccessToken == "" {
		return fmt.Errorf("Google no ha aceptado la autorización (%s)", strings.TrimSpace(t.problem()+" "+http.StatusText(st)))
	}
	scopes := strings.Fields(t.Scope)
	sort.Strings(scopes)
	if !hasDrive(scopes) && !hasGmail(scopes) {
		// En la pantalla de Google se pueden desmarcar permisos: sin Drive ni Gmail no sirve; se devuelve lo concedido.
		m.revoke(ctx, firstNonEmpty(t.RefreshToken, t.AccessToken))
		return errors.New("No has marcado ningún permiso (ver tus ficheros de Google Drive o leer tu correo de Gmail): vuelve a conectar y márcalos.")
	}
	if t.RefreshToken == "" {
		return errors.New("Google no ha dado un token de renovación: vuelve a conectar")
	}
	conn := &connection{Login: login, Email: emailFromIDToken(t.IDToken), Scopes: scopes, RefreshToken: t.RefreshToken,
		ClientID: client.ID, Created: m.now().UTC().Truncate(time.Second)}
	b, _ := json.MarshalIndent(conn, "", "  ")
	if err := writePrivate(m.connPath(login), b); err != nil {
		return fmt.Errorf("no se pudo guardar la conexión con Google: %w", err)
	}
	exp := t.ExpiresIn
	if exp <= 0 {
		exp = 3600
	}
	m.mu.Lock()
	if m.access == nil {
		m.access = map[string]*cachedToken{}
	}
	m.access[login] = &cachedToken{token: t.AccessToken, source: SourceOAuth, info: TokenInfo{Email: conn.Email, Scopes: scopes,
		Expiry: m.now().Add(time.Duration(exp)*time.Second - time.Minute)}}
	m.mu.Unlock()
	return nil
}

// emailFromIDToken: la cuenta del id_token. Llega directamente del endpoint de tokens de Google por TLS, así que
// (como permite OpenID Connect) no se comprueba su firma: solo se usa para enseñar la cuenta.
func emailFromIDToken(tok string) string {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return ""
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return ""
	}
	var c struct {
		Email string `json:"email"`
	}
	json.Unmarshal(b, &c)
	return c.Email
}

// revoke revoca un token en Google (el de renovación revoca todo el permiso).
func (m *Manager) revoke(ctx context.Context, tok string) error {
	if tok == "" {
		return nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, m.Endpoints.Revoke, strings.NewReader(url.Values{"token": {tok}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := m.client().Do(req)
	if err != nil {
		return fmt.Errorf("no se pudo hablar con Google: %w", err)
	}
	defer res.Body.Close()
	var t tokenResponse
	json.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(&t)
	if res.StatusCode == http.StatusOK || t.Error == "invalid_token" { // ya no valía: da igual
		return nil
	}
	return fmt.Errorf("Google responde %s", strings.TrimSpace(t.problem()+" "+res.Status))
}

// Disconnect revoca el permiso en Google y borra la conexión del usuario en este equipo. Si Google no confirma la
// revocación, la conexión se borra igual y el error lo dice.
func (m *Manager) Disconnect(ctx context.Context, login string) error {
	m.mu.Lock()
	if f := m.pending[login]; f != nil {
		f.done = true
		f.cancel()
		delete(m.pending, login)
	}
	m.mu.Unlock()
	conn, err := m.loadConnection(login)
	if err != nil {
		m.removeConnection(login) // dañada o inexistente: fuera
		return nil
	}
	rerr := m.revoke(ctx, conn.RefreshToken)
	m.removeConnection(login)
	if rerr != nil {
		return fmt.Errorf("se ha borrado la conexión de este equipo, pero Google no ha confirmado la revocación (%v): puedes quitar el acceso en https://myaccount.google.com/permissions", rerr)
	}
	return nil
}

// page: la página que ve el usuario en el navegador al volver de Google (msg vacío: todo bien).
func page(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(code)
	title, text := "pgocode ya puede leer tu cuenta de Google", "Conectado en solo lectura. Puedes cerrar esta pestaña y volver a pgocode."
	if msg != "" {
		title, text = "No se ha podido conectar con Google", msg+" Puedes cerrar esta pestaña."
	}
	fmt.Fprintf(w, `<!doctype html><html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>pgocode · Google</title>`+
		`<style>body{font:16px system-ui,sans-serif;max-width:34rem;margin:4rem auto;padding:0 1rem;color:#222;background:#fff}@media(prefers-color-scheme:dark){body{color:#eee;background:#1e1e1e}}</style></head>`+
		`<body><h1 style="font-size:1.3rem">%s</h1><p>%s</p></body></html>`, html.EscapeString(title), html.EscapeString(text))
}

func firstNonEmpty(l ...string) string {
	for _, s := range l {
		if s != "" {
			return s
		}
	}
	return ""
}
