package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pasigo/config"
)

type GoogleUserInfo struct {
	ID            string `json:"sub"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"email_verified"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	HostedDomain  string `json:"hd"`
}

type GoogleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

type GoogleOAuthService struct {
	cfg config.GoogleAuthConfig
}

func NewGoogleOAuthService(cfg config.GoogleAuthConfig) *GoogleOAuthService {
	return &GoogleOAuthService{cfg: cfg}
}

// GenerateStateToken genera un token CSRF seguro y aleatorio.
func GenerateStateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// CalculateRedirectURI calcula de forma 100% dinámica la URL de redirección
// basándose exclusivamente en la información de la petición HTTP recibida del navegador.
func CalculateRedirectURI(r *http.Request) string {
	if r == nil {
		return "http://localhost:8080/auth/google/callback"
	}

	// 1. Determinar el host original (cabecera Host o proxy inverso)
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = strings.TrimSpace(strings.Split(fwdHost, ",")[0])
	}
	if host == "" {
		host = "localhost:8080"
	}

	// 2. Determinar el esquema/protocolo (http o https)
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.ToLower(strings.TrimSpace(strings.Split(proto, ",")[0]))
	} else if r.Header.Get("X-Forwarded-Ssl") == "on" {
		scheme = "https"
	} else if strings.HasPrefix(r.Header.Get("Referer"), "https://") {
		scheme = "https"
	}

	// Si se accede directamente vía localhost sin TLS explícito, asegurar http
	if (strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") || strings.HasPrefix(host, "[::1]")) && r.TLS == nil && r.Header.Get("X-Forwarded-Proto") == "" {
		scheme = "http"
	}

	return fmt.Sprintf("%s://%s/auth/google/callback", scheme, host)
}

// ResolveRedirectURI delega el cálculo de la URL de callback a la petición HTTP.
func (s *GoogleOAuthService) ResolveRedirectURI(r *http.Request) string {
	if r != nil {
		return CalculateRedirectURI(r)
	}
	if s.cfg.RedirectURL != "" {
		return s.cfg.RedirectURL
	}
	return "http://localhost:8080/auth/google/callback"
}

// GetAuthURL genera la URL de redirección a la pantalla de consentimiento de Google.
func (s *GoogleOAuthService) GetAuthURL(state, redirectURI string) string {
	if redirectURI == "" {
		redirectURI = s.cfg.RedirectURL
	}

	baseURL := "https://accounts.google.com/o/oauth2/v2/auth"
	params := url.Values{}
	params.Set("client_id", s.cfg.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "openid email profile")
	params.Set("state", state)
	params.Set("access_type", "offline")
	params.Set("prompt", "select_account")

	if s.cfg.AllowedDomain != "" {
		params.Set("hd", s.cfg.AllowedDomain)
	}

	return fmt.Sprintf("%s?%s", baseURL, params.Encode())
}

// ExchangeCode intercambia el código de autorización por un token de acceso.
func (s *GoogleOAuthService) ExchangeCode(ctx context.Context, code, redirectURI string) (*GoogleTokenResponse, error) {
	if redirectURI == "" {
		redirectURI = s.cfg.RedirectURL
	}

	tokenURL := "https://oauth2.googleapis.com/token"

	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", s.cfg.ClientID)
	data.Set("client_secret", s.cfg.ClientSecret)
	data.Set("redirect_uri", redirectURI)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("error creando solicitud de token: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error comunicando con Google OAuth: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta de Google: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google OAuth devolvió estado %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp GoogleTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("error parseando token de Google: %w", err)
	}

	return &tokenResp, nil
}

// GetUserInfo obtiene los datos del perfil de Google con el token de acceso.
func (s *GoogleOAuthService) GetUserInfo(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
	userInfoURL := "https://www.googleapis.com/oauth2/v3/userinfo"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creando solicitud de perfil: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error consultando perfil en Google: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error leyendo perfil de Google: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google UserInfo devolvió estado %d: %s", resp.StatusCode, string(body))
	}

	var userInfo GoogleUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("error parseando datos de usuario: %w", err)
	}

	// Validar dominio permitido si está restringido
	if s.cfg.AllowedDomain != "" {
		if !strings.HasSuffix(userInfo.Email, "@"+s.cfg.AllowedDomain) {
			return nil, fmt.Errorf("el correo %s no pertenece al dominio autorizado @%s", userInfo.Email, s.cfg.AllowedDomain)
		}
	}

	return &userInfo, nil
}
