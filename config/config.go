package config

import (
	"os"
	"strconv"
	"strings"
)

type ServerConfig struct {
	Port int `json:"port"`
}

type OdooConfig struct {
	URL      string `json:"url"`
	DB       string `json:"db"`
	Username string `json:"username"`
	Password string `json:"password"`
	Limit    int    `json:"limit"`
}

type GoogleAuthConfig struct {
	Enabled       bool   `json:"enabled"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	RedirectURL   string `json:"redirect_url"`
	AllowedDomain string `json:"allowed_domain"` // e.g. "planesnet.com"
}

type Config struct {
	Server     ServerConfig     `json:"server"`
	Odoo       OdooConfig       `json:"odoo"`
	GoogleAuth GoogleAuthConfig `json:"google_auth"`
}

// LoadDotEnv lee un archivo .env si existe en la ruta actual y carga las variables en el entorno
// sin sobrescribir las variables que ya hayan sido exportadas explícitamente en el sistema operativo.
func LoadDotEnv(filename ...string) {
	filePath := ".env"
	if len(filename) > 0 && filename[0] != "" {
		filePath = filename[0]
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return // Si no existe el archivo .env, continuar silenciosamente
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Quitar comillas simples o dobles envolventes
		if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
			(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
			if len(val) >= 2 {
				val = val[1 : len(val)-1]
			}
		}

		// Solo asignar si la variable de entorno no está ya definida en el sistema operativo
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

// LoadConfig carga la configuración basándose estrictamente en variables de entorno del sistema y el archivo .env.
func LoadConfig(envFile ...string) *Config {
	// Cargar automáticamente .env si existe
	LoadDotEnv(envFile...)

	cfg := &Config{
		Server: ServerConfig{
			Port: 8080,
		},
		Odoo: OdooConfig{
			URL:      "https://www.planesnet.com",
			DB:       "pasi",
			Username: "",
			Password: "",
			Limit:    200,
		},
		GoogleAuth: GoogleAuthConfig{
			Enabled:       false,
			ClientID:      "",
			ClientSecret:  "",
			RedirectURL:   "",
			AllowedDomain: "planesnet.com",
		},
	}

	// 1. Puerto del servidor
	if portStr := os.Getenv("PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			cfg.Server.Port = p
		}
	} else if portStr := os.Getenv("SERVER_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			cfg.Server.Port = p
		}
	}

	// 2. Parámetros de Odoo
	if envOdooURL := os.Getenv("ODOO_URL"); envOdooURL != "" {
		cfg.Odoo.URL = strings.TrimRight(envOdooURL, "/")
	}
	if envOdooDB := os.Getenv("ODOO_DB"); envOdooDB != "" {
		cfg.Odoo.DB = envOdooDB
	}
	if envOdooUser := os.Getenv("ODOO_USER"); envOdooUser != "" {
		cfg.Odoo.Username = envOdooUser
	} else if envOdooUsername := os.Getenv("ODOO_USERNAME"); envOdooUsername != "" {
		cfg.Odoo.Username = envOdooUsername
	}
	if envOdooPass := os.Getenv("ODOO_PASSWORD"); envOdooPass != "" {
		cfg.Odoo.Password = envOdooPass
	} else if envOdooPass2 := os.Getenv("ODOO_PASS"); envOdooPass2 != "" {
		cfg.Odoo.Password = envOdooPass2
	}
	if limitStr := os.Getenv("ODOO_LIMIT"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			cfg.Odoo.Limit = l
		}
	}

	// 3. Google OAuth 2.0
	if envClientID := os.Getenv("GOOGLE_CLIENT_ID"); envClientID != "" {
		cfg.GoogleAuth.ClientID = envClientID
	}
	if envClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET"); envClientSecret != "" {
		cfg.GoogleAuth.ClientSecret = envClientSecret
	}
	if envRedirectURL := os.Getenv("GOOGLE_REDIRECT_URL"); envRedirectURL != "" {
		cfg.GoogleAuth.RedirectURL = envRedirectURL
	}
	if envAllowedDomain := os.Getenv("GOOGLE_ALLOWED_DOMAIN"); envAllowedDomain != "" {
		cfg.GoogleAuth.AllowedDomain = envAllowedDomain
	}

	if cfg.GoogleAuth.ClientID != "" && cfg.GoogleAuth.ClientSecret != "" {
		cfg.GoogleAuth.Enabled = true
	}

	return cfg
}

// GetGoogleAuthCredentials retorna ClientID y ClientSecret dando prioridad a variables de entorno / .env.
func (c *Config) GetGoogleAuthCredentials() (string, string) {
	LoadDotEnv()
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	if clientID == "" {
		clientID = c.GoogleAuth.ClientID
	}
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientSecret == "" {
		clientSecret = c.GoogleAuth.ClientSecret
	}
	return strings.TrimSpace(clientID), strings.TrimSpace(clientSecret)
}

// IsGoogleConfigured comprueba si las credenciales de Google OAuth están presentes y no vacías.
func (c *Config) IsGoogleConfigured() bool {
	clientID, clientSecret := c.GetGoogleAuthCredentials()
	return clientID != "" && clientSecret != ""
}
