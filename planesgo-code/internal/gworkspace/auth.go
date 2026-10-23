// Package gworkspace: acceso de SOLO LECTURA a Google Workspace (Drive, Docs y Sheets) para pgocode y sus agentes.
//
// Sin cliente OAuth propio: pgocode usa la autorización que el usuario ya ha dado a la herramienta gcloud de Google
// con su cuenta. El token de acceso se pide a gcloud cuando hace falta (primero el de las credenciales por defecto,
// ADC; si no lleva el ámbito de Drive, el de la sesión de gcloud) y solo vive en memoria, hasta que caduca o Google
// lo rechaza; nunca se copia a disco. Con sus ámbitos se sabe la cuenta y si puede leer Drive (tokeninfo).
//
// pgocode solo hace peticiones de lectura (GET), aunque el token tenga un ámbito más amplio.
package gworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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

// LoginCommand: lo que ejecuta el usuario (una vez) para dar acceso de lectura a Drive a sus credenciales por defecto
// (ADC). Lleva también cloud-platform: las mismas credenciales sirven a Vertex AI y a las demás herramientas.
const LoginCommand = "gcloud auth application-default login --scopes=openid,https://www.googleapis.com/auth/userinfo.email," +
	ScopeDriveReadonly + ",https://www.googleapis.com/auth/cloud-platform"

// AltLoginCommand: la alternativa si Google bloquea la anterior («Esta aplicación está bloqueada»): la sesión de la
// propia gcloud con acceso a Drive (ámbito de Drive completo; pgocode solo lee).
const AltLoginCommand = "gcloud auth login --enable-gdrive-access"

// EnvQuotaProject: el proyecto de cuota (cabecera x-goog-user-project) si no vale el de por defecto.
const EnvQuotaProject = "PGOCODE_GOOGLE_QUOTA_PROJECT"

// Endpoints de Google (en las pruebas, los de un servidor simulado).
type Endpoints struct {
	TokenInfo, Drive, Sheets string
}

// GoogleEndpoints: los de verdad.
var GoogleEndpoints = Endpoints{
	TokenInfo: "https://oauth2.googleapis.com/tokeninfo",
	Drive:     "https://www.googleapis.com/drive/v3",
	Sheets:    "https://sheets.googleapis.com/v4",
}

// Fuentes del token.
const (
	SourceADC    = "adc"    // gcloud auth application-default print-access-token
	SourceGcloud = "gcloud" // gcloud auth print-access-token
)

// Errores que ve el usuario (o el agente) tal cual.
var (
	ErrNoGcloud = errors.New("no está la herramienta gcloud de Google en este equipo (https://cloud.google.com/sdk/docs/install)")
	ErrNoAccess = errors.New("tu cuenta de Google no ha dado acceso de lectura a Drive: ejecuta en un terminal «" + LoginCommand + "» (Propiedades de pgocode → Google Workspace)")
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

// Manager: el acceso a Google con las credenciales de gcloud del usuario del equipo.
type Manager struct {
	Endpoints Endpoints
	HTTP      *http.Client
	// Fetch pide un token a gcloud (source: adc | gcloud). En las pruebas, uno simulado.
	Fetch func(ctx context.Context, source string) (string, error)
	// QuotaProject: el proyecto de cuota que se manda (x-goog-user-project) si Google lo pide.
	QuotaProject string
	Now          func() time.Time

	mu       sync.Mutex
	cached   *cachedToken
	useQuota bool // Google pidió un proyecto de cuota: se manda desde entonces
}

// New: un gestor con los endpoints de Google, gcloud y el proyecto de cuota dado (vacío: sin él).
func New(quotaProject string) *Manager {
	if q := strings.TrimSpace(os.Getenv(EnvQuotaProject)); q != "" {
		quotaProject = q
	}
	return &Manager{Endpoints: GoogleEndpoints, HTTP: &http.Client{Timeout: 60 * time.Second}, Fetch: GcloudToken, QuotaProject: quotaProject}
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

// token devuelve un token con acceso a Drive (de la memoria o pedido a gcloud).
func (m *Manager) token(ctx context.Context) (string, error) {
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

// resolve pide el token a gcloud: el de ADC y, si no lleva Drive, el de la sesión de gcloud. Devuelve también lo
// que se sabe de la cuenta aunque no tenga acceso (para la interfaz).
func (m *Manager) resolve(ctx context.Context) (*cachedToken, TokenInfo, error) {
	var first TokenInfo
	var firstErr error
	for _, src := range []string{SourceADC, SourceGcloud} {
		tok, err := m.Fetch(ctx, src)
		if errors.Is(err, ErrNoGcloud) {
			return nil, first, err
		}
		if err != nil || tok == "" {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		info, err := m.tokenInfo(ctx, tok)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if first.Email == "" && len(first.Scopes) == 0 {
			first = info
		}
		if hasDrive(info.Scopes) {
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
	}
	if first.Email != "" || len(first.Scopes) > 0 {
		return nil, first, ErrNoAccess
	}
	if firstErr != nil {
		return nil, first, fmt.Errorf("%w (%v)", ErrNoAccess, firstErr)
	}
	return nil, first, ErrNoAccess
}

// forget olvida el token en memoria (Google lo ha rechazado).
func (m *Manager) forget() {
	m.mu.Lock()
	m.cached = nil
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

// Status: lo que ve la interfaz (nunca el token).
type Status struct {
	Connected   bool     `json:"conectado"`              // hay un token que puede leer Drive
	Email       string   `json:"email,omitempty"`        // la cuenta de Google
	Scopes      []string `json:"ambitos,omitempty"`      // los del token
	Source      string   `json:"fuente,omitempty"`       // adc | gcloud
	ReadOnly    bool     `json:"solo_lectura,omitempty"` // el ámbito es drive.readonly (no el de Drive completo)
	Gcloud      bool     `json:"gcloud"`                 // gcloud está instalado
	Problem     string   `json:"problema,omitempty"`     // por qué no hay acceso
	Command     string   `json:"orden"`                  // lo que hay que ejecutar para dar acceso
	AltCommand  string   `json:"orden_alternativa"`      // si Google bloquea la anterior
	QuotaProj   string   `json:"proyecto_cuota,omitempty"`
	QuotaInUse  bool     `json:"cuota_en_uso,omitempty"`    // se manda x-goog-user-project
	EnableQuota string   `json:"orden_servicios,omitempty"` // para habilitar las APIs en el proyecto de cuota
}

// Status comprueba el acceso (pregunta a gcloud y a Google; fresh: sin usar el token en memoria).
func (m *Manager) Status(ctx context.Context, fresh bool) Status {
	st := Status{Command: LoginCommand, AltCommand: AltLoginCommand, QuotaProj: m.QuotaProject}
	if m.QuotaProject != "" {
		st.EnableQuota = EnableServicesCommand(m.QuotaProject)
	}
	if fresh {
		m.forget()
	}
	m.mu.Lock()
	c := m.cached
	st.QuotaInUse = m.useQuota
	m.mu.Unlock()
	var info TokenInfo
	var err error
	if c == nil || !m.now().Before(c.info.Expiry) {
		c, info, err = m.resolve(ctx)
	} else {
		info = c.info
	}
	st.Gcloud = !errors.Is(err, ErrNoGcloud)
	st.Email, st.Scopes = info.Email, info.Scopes
	if err != nil {
		st.Problem = err.Error()
		if errors.Is(err, ErrNoAccess) { // la orden ya la enseña la interfaz: solo el porqué
			st.Problem = "Las credenciales de gcloud no tienen permiso de lectura de Drive" + strings.TrimPrefix(err.Error(), ErrNoAccess.Error())
		}
		return st
	}
	st.Connected, st.Source = true, c.source
	st.ReadOnly = contains(info.Scopes, ScopeDriveReadonly) && !contains(info.Scopes, ScopeDrive)
	return st
}

// Connected: hay acceso a Drive ahora (sin preguntar a gcloud: lo que hay en memoria). Para decidir si el aviso
// de «pgo drive» va al agente.
func (m *Manager) Connected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cached != nil
}

// EnableServicesCommand: la orden para habilitar las APIs en el proyecto de cuota.
func EnableServicesCommand(project string) string {
	return "gcloud services enable drive.googleapis.com sheets.googleapis.com docs.googleapis.com --project=" + project
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
