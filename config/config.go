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

// LoadConfig lee y valida el archivo de configuración YAML especificado.
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error al leer el archivo %s: %w", filename, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("error al parsear el archivo YAML %s: %w", filename, err)
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
