// Package gworkspace: acceso de SOLO LECTURA a Google Workspace (Drive, Docs y Sheets) para pgocode y sus agentes.
//
// Camino principal (oauth.go): la aplicación OAuth «PlanesGo» (cliente de escritorio registrado una vez por la
// empresa) y, por cada usuario de pgocode, «Conectar con Google»: el navegador del sistema enseña la pantalla de
// consentimiento de Google y vuelve a un puerto efímero de 127.0.0.1 (RFC 8252, con PKCE). El token de renovación
// se guarda solo en el equipo (un fichero 0600 por usuario); el de acceso vive en memoria y se renueva solo.
//
// Respaldo silencioso: si el usuario no ha conectado su cuenta y en el equipo hay una gcloud ya autorizada con
// Drive, se usa su token (en memoria, nunca a disco). La interfaz no enseña órdenes de gcloud.
//
// pgocode solo hace peticiones de lectura (GET) a las APIs, aunque el token tenga un ámbito más amplio.
package gworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Ámbitos de Drive que sirven (también para exportar Docs y leer Sheets).
const (
	ScopeDriveReadonly = "https://www.googleapis.com/auth/drive.readonly"
	ScopeDrive         = "https://www.googleapis.com/auth/drive"
)

// EnvQuotaProject: el proyecto de cuota (cabecera x-goog-user-project) si no vale el de por defecto.
const EnvQuotaProject = "PGOCODE_GOOGLE_QUOTA_PROJECT"

// Endpoints de Google (en las pruebas, los de un servidor simulado). Los de OAuth son fijos (no los del fichero del
// cliente): así un fichero manipulado no puede mandar el secreto ni los tokens a otro sitio.
type Endpoints struct {
	TokenInfo, Drive, Sheets string
	Auth, Token, Revoke      string
}

// GoogleEndpoints: los de verdad.
var GoogleEndpoints = Endpoints{
	TokenInfo: "https://oauth2.googleapis.com/tokeninfo",
	Drive:     "https://www.googleapis.com/drive/v3",
	Sheets:    "https://sheets.googleapis.com/v4",
	Auth:      "https://accounts.google.com/o/oauth2/v2/auth",
	Token:     "https://oauth2.googleapis.com/token",
	Revoke:    "https://oauth2.googleapis.com/revoke",
}

// Fuentes del token.
const (
	SourceOAuth  = "oauth"  // la conexión propia del usuario («Conectar con Google»)
	SourceADC    = "adc"    // respaldo: gcloud auth application-default print-access-token
	SourceGcloud = "gcloud" // respaldo: gcloud auth print-access-token
)

// Errores que ve el usuario (o el agente) tal cual.
var (
	ErrNoGcloud = errors.New("no está la herramienta gcloud de Google en este equipo")
	ErrNoAccess = errors.New("no hay acceso de lectura a Google Drive: conecta tu cuenta de Google en pgocode (Propiedades → Google Workspace → Conectar con Google)")
)

// TokenInfo: lo que dice Google de un token (sin el token).
type TokenInfo struct {
	Email  string
	Scopes []string
	Expiry time.Time
}

type cachedToken struct {
	token, source string
	info          TokenInfo
}

// Manager: el acceso a Google de los usuarios de pgocode.
type Manager struct {
	Endpoints Endpoints
	HTTP      *http.Client
	// Dir: la carpeta de datos de pgocode (google-oauth-client.json y google/<usuario>.json). Vacía: sin conexión
	// propia (solo el respaldo de gcloud).
	Dir string
	// ClientSource: otra fuente del cliente OAuth si no está el fichero (más adelante, el servidor de PlanesGo para
	// todo el equipo). nil: ninguna.
	ClientSource func(ctx context.Context) (*OAuthClient, error)
	// Listen abre el puerto de vuelta de Google (por defecto, uno efímero de 127.0.0.1). En las pruebas, otro.
	Listen func() (net.Listener, error)
	// AuthTimeout: cuánto se espera a que el usuario acepte en Google (por defecto, 10 minutos).
	AuthTimeout time.Duration
	// Fetch pide un token a gcloud (source: adc | gcloud) para el respaldo; nil: sin respaldo.
	Fetch func(ctx context.Context, source string) (string, error)
	// QuotaProject: el proyecto de cuota que se manda (x-goog-user-project) si Google lo pide.
	QuotaProject string
	Now          func() time.Time

	mu        sync.Mutex
	cached    *cachedToken // el de gcloud
	useQuota  bool         // Google pidió un proyecto de cuota: se manda desde entonces
	active    string       // el último usuario de pgocode que ha usado la interfaz (para los agentes)
	access    map[string]*cachedToken
	pending   map[string]*authFlow
	refreshMu sync.Mutex // una renovación a la vez
}

// New: un gestor con los endpoints de Google, la carpeta de datos de pgocode, el respaldo de gcloud y el proyecto
// de cuota dado (vacío: sin él).
func New(dir, quotaProject string) *Manager {
	if q := strings.TrimSpace(os.Getenv(EnvQuotaProject)); q != "" {
		quotaProject = q
	}
	return &Manager{Endpoints: GoogleEndpoints, HTTP: &http.Client{Timeout: 60 * time.Second}, Dir: dir, Fetch: GcloudToken, QuotaProject: quotaProject}
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) client() *http.Client {
	if m.HTTP != nil {
		return m.HTTP
	}
	return http.DefaultClient
}

type userKey struct{}

// WithUser: las peticiones con este contexto usan la conexión de ese usuario de pgocode (la interfaz). Sin él (los
// agentes), la del último usuario que ha usado la interfaz o, si solo hay una conexión en el equipo, esa.
func WithUser(ctx context.Context, login string) context.Context {
	return context.WithValue(ctx, userKey{}, login)
}

// SetActive: el usuario de pgocode que está usando la interfaz (sus agentes leen Drive con su cuenta).
func (m *Manager) SetActive(login string) {
	m.mu.Lock()
	m.active = login
	m.mu.Unlock()
}

// userFor: el usuario cuya conexión vale para esta petición ("" si ninguno tiene conexión propia).
func (m *Manager) userFor(ctx context.Context) string {
	if u, ok := ctx.Value(userKey{}).(string); ok {
		if m.hasConnection(u) {
			return u
		}
		return ""
	}
	m.mu.Lock()
	a := m.active
	m.mu.Unlock()
	if a != "" && m.hasConnection(a) {
		return a
	}
	if m.Dir == "" {
		return ""
	}
	l, _ := filepath.Glob(filepath.Join(m.Dir, connDir, "*.json"))
	if len(l) == 1 {
		if c, err := m.readConnection(l[0]); err == nil {
			return c.Login
		}
	}
	return ""
}

// GcloudToken pide un token de acceso a gcloud (no lee ningún fichero de credenciales: lo hace gcloud).
func GcloudToken(ctx context.Context, source string) (string, error) {
	if _, err := exec.LookPath("gcloud"); err != nil {
		return "", ErrNoGcloud
	}
	args := []string{"auth", "print-access-token"}
	if source == SourceADC {
		args = []string{"auth", "application-default", "print-access-token"}
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	cmd.Stdin = nil // nunca se queda esperando a que el usuario conteste
	out, err := cmd.Output()
	if err != nil {
		msg := err.Error()
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			msg = firstLine(string(ee.Stderr))
		}
		return "", fmt.Errorf("gcloud (%s): %s", source, msg)
	}
	return strings.TrimSpace(string(out)), nil
}

// hasDrive: los ámbitos permiten leer Drive.
func hasDrive(scopes []string) bool {
	return contains(scopes, ScopeDriveReadonly) || contains(scopes, ScopeDrive)
}

// token devuelve un token con acceso a Drive: el de la conexión propia del usuario o, si no tiene, el de gcloud.
func (m *Manager) token(ctx context.Context) (string, error) {
	if u := m.userFor(ctx); u != "" {
		c, err := m.oauthToken(ctx, u)
		if err != nil {
			return "", err
		}
		return c.token, nil
	}
	m.mu.Lock()
	if c := m.cached; c != nil && m.now().Before(c.info.Expiry) {
		m.mu.Unlock()
		return c.token, nil
	}
	m.mu.Unlock()
	c, _, err := m.resolve(ctx)
	if err != nil {
		return "", err
	}
	return c.token, nil
}

// resolve pide el token a gcloud (respaldo): el de ADC y, si no lleva Drive, el de la sesión de gcloud. Sin gcloud
// o sin Drive: ErrNoAccess (lo que hay que hacer es conectar la cuenta, no tocar gcloud).
func (m *Manager) resolve(ctx context.Context) (*cachedToken, TokenInfo, error) {
	if m.Fetch == nil {
		return nil, TokenInfo{}, ErrNoAccess
	}
	var netErr error // sin conexión con Google: eso es lo que hay que decir
	for _, src := range []string{SourceADC, SourceGcloud} {
		tok, err := m.Fetch(ctx, src)
		if errors.Is(err, ErrNoGcloud) {
			return nil, TokenInfo{}, ErrNoAccess
		}
		if err != nil || tok == "" {
			continue
		}
		info, err := m.tokenInfo(ctx, tok)
		if err != nil && strings.HasPrefix(err.Error(), "no se pudo hablar") && netErr == nil {
			netErr = err
		}
		if err != nil || !hasDrive(info.Scopes) {
			continue
		}
		c := &cachedToken{token: tok, source: src, info: info}
		// Con un minuto de margen y como mucho media hora: se vuelve a preguntar a gcloud de vez en cuando.
		exp := info.Expiry.Add(-time.Minute)
		if lim := m.now().Add(30 * time.Minute); exp.After(lim) {
			exp = lim
		}
		c.info.Expiry = exp
		m.mu.Lock()
		m.cached = c
		m.mu.Unlock()
		return c, info, nil
	}
	if netErr != nil {
		return nil, TokenInfo{}, netErr
	}
	return nil, TokenInfo{}, ErrNoAccess
}

// forget olvida los tokens de acceso en memoria de esta petición (Google los ha rechazado).
func (m *Manager) forget(ctx context.Context) {
	u := m.userFor(ctx)
	m.mu.Lock()
	m.cached = nil
	if u != "" {
		delete(m.access, u)
	}
	m.mu.Unlock()
}

// tokenInfo pregunta a Google por el token (cuenta, ámbitos y caducidad). Va en el cuerpo, no en la URL.
func (m *Manager) tokenInfo(ctx context.Context, tok string) (TokenInfo, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, m.Endpoints.TokenInfo, strings.NewReader(url.Values{"access_token": {tok}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := m.client().Do(req)
	if err != nil {
		return TokenInfo{}, fmt.Errorf("no se pudo hablar con Google: %w", err)
	}
	defer res.Body.Close()
	var t struct {
		Scope     string `json:"scope"`
		Email     string `json:"email"`
		Exp       string `json:"exp"`
		ExpiresIn string `json:"expires_in"`
		Error     string `json:"error_description"`
	}
	json.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(&t)
	if res.StatusCode != http.StatusOK {
		return TokenInfo{}, fmt.Errorf("Google no acepta el token (%s)", strings.TrimSpace(t.Error+" "+res.Status))
	}
	info := TokenInfo{Email: t.Email, Scopes: strings.Fields(t.Scope)}
	sort.Strings(info.Scopes)
	if n, err := strconv.ParseInt(t.Exp, 10, 64); err == nil && n > 0 {
		info.Expiry = time.Unix(n, 0)
	} else if n, err := strconv.Atoi(t.ExpiresIn); err == nil {
		info.Expiry = m.now().Add(time.Duration(n) * time.Second)
	} else {
		info.Expiry = m.now().Add(30 * time.Minute)
	}
	return info, nil
}

// Status: lo que ve la interfaz (nunca un token ni el secreto del cliente).
type Status struct {
	Connected     bool     `json:"conectado"`                  // hay un token que puede leer Drive
	Email         string   `json:"email,omitempty"`            // la cuenta de Google
	Scopes        []string `json:"ambitos,omitempty"`          // los del token
	Source        string   `json:"fuente,omitempty"`           // oauth | adc | gcloud
	ReadOnly      bool     `json:"solo_lectura,omitempty"`     // el ámbito es drive.readonly (no el de Drive completo)
	Since         string   `json:"desde,omitempty"`            // cuándo se conectó (RFC 3339)
	Client        bool     `json:"cliente"`                    // la aplicación de Google está registrada en este equipo
	ClientOrigin  string   `json:"cliente_origen,omitempty"`   // fichero | servidor
	ClientProject string   `json:"cliente_proyecto,omitempty"` // el proyecto de Google Cloud del cliente
	Pending       bool     `json:"en_curso,omitempty"`         // esperando a que el usuario acepte en Google
	Problem       string   `json:"problema,omitempty"`         // por qué no hay acceso (o por qué falló la conexión)
}

// Status: el acceso a Google del usuario de pgocode login (fresh: comprueba con Google, sin el token en memoria).
func (m *Manager) Status(ctx context.Context, login string, fresh bool) Status {
	var st Status
	if c, err := m.oauthClient(ctx); err == nil {
		st.Client, st.ClientOrigin, st.ClientProject = true, c.Origin, c.ProjectID
	} else if !errors.Is(err, ErrNoClient) {
		st.Problem = err.Error()
	}
	m.mu.Lock()
	if f := m.pending[login]; f != nil {
		st.Pending = !f.done
		if f.err != "" {
			st.Problem = f.err
		}
		if f.done { // el resultado se dice una vez
			delete(m.pending, login)
		}
	}
	m.mu.Unlock()
	if conn, err := m.loadConnection(login); err == nil {
		st.Email, st.Scopes, st.Source, st.Since = conn.Email, conn.Scopes, SourceOAuth, conn.Created.Format(time.RFC3339)
		if fresh {
			m.mu.Lock()
			delete(m.access, login)
			m.mu.Unlock()
		}
		c, err := m.oauthToken(ctx, login)
		if err != nil {
			st.Problem = err.Error()
			if errors.Is(err, ErrRevoked) {
				st.Email, st.Scopes, st.Source, st.Since = "", nil, "", ""
			}
			return st
		}
		st.Connected, st.Problem = true, ""
		if len(c.info.Scopes) > 0 {
			st.Scopes = c.info.Scopes
		}
		st.ReadOnly = contains(st.Scopes, ScopeDriveReadonly) && !contains(st.Scopes, ScopeDrive)
		return st
	}
	// Sin conexión propia: el respaldo de gcloud, en silencio (sin enseñar sus errores).
	if fresh {
		m.mu.Lock()
		m.cached = nil
		m.mu.Unlock()
	}
	m.mu.Lock()
	c := m.cached
	m.mu.Unlock()
	var info TokenInfo
	if c == nil || !m.now().Before(c.info.Expiry) {
		var err error
		if c, info, err = m.resolve(ctx); err != nil {
			return st
		}
	} else {
		info = c.info
	}
	st.Connected, st.Source, st.Email, st.Scopes = true, c.source, info.Email, info.Scopes
	st.ReadOnly = contains(info.Scopes, ScopeDriveReadonly) && !contains(info.Scopes, ScopeDrive)
	return st
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
