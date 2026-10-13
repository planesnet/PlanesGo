package gcpbilling

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultBigQueryScope = "https://www.googleapis.com/auth/bigquery"
	defaultTokenURI      = "https://oauth2.googleapis.com/token"
	metadataTokenURL     = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"
)

// ServiceAccountKey contiene la estructura de un archivo de credenciales de cuenta de servicio de GCP
type ServiceAccountKey struct {
	Type                    string `json:"type"`
	ProjectID               string `json:"project_id"`
	PrivateKeyID            string `json:"private_key_id"`
	PrivateKey              string `json:"private_key"`
	ClientEmail             string `json:"client_email"`
	ClientID                string `json:"client_id"`
	AuthURI                 string `json:"auth_uri"`
	TokenURI                string `json:"token_uri"`
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"`
	ClientX509CertURL       string `json:"client_x509_cert_url"`
}

// TokenProvider es la interfaz para obtener tokens de acceso OAuth2 Bearer
type TokenProvider interface {
	GetToken(ctx context.Context) (string, error)
}

// CachedTokenProvider almacena en memoria el token de acceso hasta 1 minuto antes de su expiración
type CachedTokenProvider struct {
	mu          sync.RWMutex
	underlying  TokenProvider
	cachedToken string
	expiresAt   time.Time
}

func NewCachedTokenProvider(underlying TokenProvider) *CachedTokenProvider {
	return &CachedTokenProvider{
		underlying: underlying,
	}
}

func (c *CachedTokenProvider) GetToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	if c.cachedToken != "" && time.Now().Before(c.expiresAt) {
		token := c.cachedToken
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Doble comprobación tras adquirir el Lock exclusivo
	if c.cachedToken != "" && time.Now().Before(c.expiresAt) {
		return c.cachedToken, nil
	}

	token, expiresAt, err := c.fetchNewToken(ctx)
	if err != nil {
		return "", err
	}

	c.cachedToken = token
	c.expiresAt = expiresAt
	return token, nil
}

func (c *CachedTokenProvider) fetchNewToken(ctx context.Context) (string, time.Time, error) {
	if satp, ok := c.underlying.(*ServiceAccountTokenProvider); ok {
		return satp.fetchToken(ctx)
	}
	if mtp, ok := c.underlying.(*MetadataTokenProvider); ok {
		return mtp.fetchToken(ctx)
	}
	tok, err := c.underlying.GetToken(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	// Fallback por defecto: 10 minutos
	return tok, time.Now().Add(10 * time.Minute), nil
}

// ServiceAccountTokenProvider genera JWTs firmados con RS256 e intercambia por access tokens
type ServiceAccountTokenProvider struct {
	ClientEmail string
	PrivateKey  *rsa.PrivateKey
	TokenURI    string
	Scope       string
	HTTPClient  *http.Client
}

func NewServiceAccountTokenProvider(sa *ServiceAccountKey, httpClient *http.Client) (*ServiceAccountTokenProvider, error) {
	if sa == nil {
		return nil, fmt.Errorf("datos de cuenta de servicio vacíos")
	}
	rsaKey, err := ParseRSAPrivateKey(sa.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("error analizando clave privada RSA: %w", err)
	}

	tokenURI := sa.TokenURI
	if tokenURI == "" {
		tokenURI = defaultTokenURI
	}

	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &ServiceAccountTokenProvider{
		ClientEmail: sa.ClientEmail,
		PrivateKey:  rsaKey,
		TokenURI:    tokenURI,
		Scope:       defaultBigQueryScope,
		HTTPClient:  client,
	}, nil
}

func (p *ServiceAccountTokenProvider) GetToken(ctx context.Context) (string, error) {
	tok, _, err := p.fetchToken(ctx)
	return tok, err
}

func (p *ServiceAccountTokenProvider) fetchToken(ctx context.Context) (string, time.Time, error) {
	now := time.Now()
	jwtStr, err := GenerateSignedJWT(p.ClientEmail, p.TokenURI, p.Scope, p.PrivateKey, now)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("error generando JWT firmado: %w", err)
	}

	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {jwtStr},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("error solicitando token a %s: %w", p.TokenURI, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("error leyendo respuesta de autenticación: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("error al obtener token OAuth2 (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var oauthResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(bodyBytes, &oauthResp); err != nil {
		return "", time.Time{}, fmt.Errorf("error decodificando respuesta OAuth2: %w", err)
	}
	if oauthResp.Error != "" {
		return "", time.Time{}, fmt.Errorf("error OAuth2: %s (%s)", oauthResp.Error, oauthResp.ErrorDesc)
	}
	if oauthResp.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("respuesta OAuth2 sin access_token")
	}

	expiresIn := oauthResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	// Guardar en caché hasta un minuto antes de que caduque
	expiresAt := now.Add(time.Duration(expiresIn) * time.Second).Add(-1 * time.Minute)
	if expiresAt.Before(now) {
		expiresAt = now.Add(time.Duration(expiresIn/2) * time.Second)
	}

	return oauthResp.AccessToken, expiresAt, nil
}

// GenerateSignedJWT genera y firma un JWT RS256 con los reclamos estándar para cuentas de servicio de Google
func GenerateSignedJWT(clientEmail, tokenURI, scope string, key *rsa.PrivateKey, now time.Time) (string, error) {
	if clientEmail == "" {
		return "", fmt.Errorf("client_email es obligatorio para el JWT")
	}
	if tokenURI == "" {
		tokenURI = defaultTokenURI
	}
	if scope == "" {
		scope = defaultBigQueryScope
	}
	if key == nil {
		return "", fmt.Errorf("clave privada RSA no provista")
	}

	headerJSON := `{"alg":"RS256","typ":"JWT"}`

	claims := struct {
		Iss   string `json:"iss"`
		Scope string `json:"scope"`
		Aud   string `json:"aud"`
		Exp   int64  `json:"exp"`
		Iat   int64  `json:"iat"`
	}{
		Iss:   clientEmail,
		Scope: scope,
		Aud:   tokenURI,
		Exp:   now.Add(time.Hour).Unix(),
		Iat:   now.Unix(),
	}

	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("error serializando reclamos JWT: %w", err)
	}

	headerPart := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
	claimsPart := base64.RawURLEncoding.EncodeToString(claimsBytes)
	signingInput := headerPart + "." + claimsPart

	hasher := sha256.New()
	hasher.Write([]byte(signingInput))
	digest := hasher.Sum(nil)

	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest)
	if err != nil {
		return "", fmt.Errorf("fallo al firmar con RSA PKCS1v15: %w", err)
	}

	sigPart := base64.RawURLEncoding.EncodeToString(signature)
	return signingInput + "." + sigPart, nil
}

// ParseRSAPrivateKey analiza una clave privada PEM tanto en formato PKCS#1 como PKCS#8
func ParseRSAPrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("no se encontró ningún bloque PEM válido")
	}

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("la clave privada PKCS#8 no es de tipo RSA")
		}
		return rsaKey, nil
	}

	if rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return rsaKey, nil
	}

	return nil, fmt.Errorf("formato de clave privada no soportado (se requiere RSA PKCS#1 o PKCS#8)")
}

// MetadataTokenProvider obtiene tokens del servidor de metadatos de GCP en Compute Engine / Cloud Run
type MetadataTokenProvider struct {
	URL        string
	HTTPClient *http.Client
}

func NewMetadataTokenProvider(httpClient *http.Client) *MetadataTokenProvider {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &MetadataTokenProvider{
		URL:        metadataTokenURL,
		HTTPClient: client,
	}
}

func (m *MetadataTokenProvider) GetToken(ctx context.Context) (string, error) {
	tok, _, err := m.fetchToken(ctx)
	return tok, err
}

func (m *MetadataTokenProvider) fetchToken(ctx context.Context) (string, time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := m.HTTPClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("no se pudo contactar con el servidor de metadatos: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("el servidor de metadatos devolvió código HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("error leyendo token de metadatos: %w", err)
	}

	var metaResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &metaResp); err != nil {
		return "", time.Time{}, fmt.Errorf("error analizando respuesta de metadatos: %w", err)
	}
	if metaResp.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("servidor de metadatos no devolvió access_token")
	}

	expiresIn := metaResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second).Add(-1 * time.Minute)
	return metaResp.AccessToken, expiresAt, nil
}

// ResolveTokenProvider resuelve el proveedor de tokens según la prelación establecida:
// 1. GCP_BILLING_SA_JSON (JSON en variable)
// 2. GOOGLE_APPLICATION_CREDENTIALS (ruta a archivo JSON)
// 3. Servidor de metadatos de GCP
func ResolveTokenProvider(httpClient *http.Client) (TokenProvider, error) {
	// 1. GCP_BILLING_SA_JSON
	if saJSON := strings.TrimSpace(os.Getenv("GCP_BILLING_SA_JSON")); saJSON != "" {
		var sa ServiceAccountKey
		if err := json.Unmarshal([]byte(saJSON), &sa); err != nil {
			return nil, fmt.Errorf("la variable GCP_BILLING_SA_JSON contiene un JSON inválido: %w", err)
		}
		provider, err := NewServiceAccountTokenProvider(&sa, httpClient)
		if err != nil {
			return nil, err
		}
		return NewCachedTokenProvider(provider), nil
	}

	// 2. GOOGLE_APPLICATION_CREDENTIALS
	if credPath := strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")); credPath != "" {
		data, err := os.ReadFile(credPath)
		if err != nil {
			return nil, fmt.Errorf("no se pudo leer el archivo GOOGLE_APPLICATION_CREDENTIALS (%s): %w", credPath, err)
		}
		var sa ServiceAccountKey
		if err := json.Unmarshal(data, &sa); err != nil {
			return nil, fmt.Errorf("el archivo de credenciales (%s) contiene un JSON inválido: %w", credPath, err)
		}
		provider, err := NewServiceAccountTokenProvider(&sa, httpClient)
		if err != nil {
			return nil, err
		}
		return NewCachedTokenProvider(provider), nil
	}

	// 3. Servidor de metadatos (Compute Engine / Cloud Run)
	metaProvider := NewMetadataTokenProvider(httpClient)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := metaProvider.GetToken(ctx); err == nil {
		return NewCachedTokenProvider(metaProvider), nil
	}

	return nil, fmt.Errorf("faltan credenciales de Google Cloud: configure la variable GCP_BILLING_SA_JSON o GOOGLE_APPLICATION_CREDENTIALS, o ejecute en un entorno GCP con cuenta de servicio asignada")
}
