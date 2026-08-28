package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Port int `yaml:"port"`
}

type OdooConfig struct {
	URL      string `yaml:"url"`
	DB       string `yaml:"db"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Limit    int    `yaml:"limit"`
}

type GoogleAuthConfig struct {
	Enabled       bool   `yaml:"enabled"`
	ClientID      string `yaml:"client_id"`
	ClientSecret  string `yaml:"client_secret"`
	RedirectURL   string `yaml:"redirect_url"`
	AllowedDomain string `yaml:"allowed_domain"` // Opcional: e.g. "planesnet.com"
}

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Odoo       OdooConfig       `yaml:"odoo"`
	GoogleAuth GoogleAuthConfig `yaml:"google_auth"`
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

// LoadConfig lee y valida el archivo de configuración YAML especificado.
// Si el archivo no existe, utiliza valores por defecto, config.example.yml y variables de entorno / .env.
func LoadConfig(filename string) (*Config, error) {
	// Cargar automáticamente .env si existe
	LoadDotEnv()

	var cfg Config
	data, err := os.ReadFile(filename)
	if err != nil {
		// Si no existe filename (p. ej. en contenedor con solo env vars), intentar cargar config.example.yml
		if exampleData, exErr := os.ReadFile("config.example.yml"); exErr == nil {
			_ = yaml.Unmarshal(exampleData, &cfg)
		}
	} else {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("error al parsear el archivo YAML %s: %w", filename, err)
		}
	}

	// Valores por defecto para Server y Odoo
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Odoo.Limit <= 0 {
		cfg.Odoo.Limit = 200
	}

	if cfg.Odoo.URL == "" {
		cfg.Odoo.URL = "https://www.planesnet.com"
	}
	if cfg.Odoo.DB == "" {
		cfg.Odoo.DB = "pasi"
	}

	cfg.Odoo.URL = strings.TrimRight(cfg.Odoo.URL, "/")

	// Variables de entorno para Google Auth si están presentes
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

	// Si hay ClientID configurado, activar por defecto
	if cfg.GoogleAuth.ClientID != "" {
		cfg.GoogleAuth.Enabled = true
	}

	if cfg.GoogleAuth.RedirectURL == "" {
		cfg.GoogleAuth.RedirectURL = "http://localhost:8080/auth/google/callback"
	}

	return &cfg, nil
}

// GetGoogleAuthCredentials retorna ClientID y ClientSecret dando prioridad absoluta a las variables de entorno del sistema o fichero .env.
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

// SaveConfig guarda la configuración en el archivo YAML especificado.
func SaveConfig(filename string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("error al serializar configuración a YAML: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("error al escribir el archivo %s: %w", filename, err)
	}

	return nil
}
