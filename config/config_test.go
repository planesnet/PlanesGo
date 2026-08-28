package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")
	envContent := `
# Comentario
GOOGLE_CLIENT_ID="test-client-id-env"
GOOGLE_CLIENT_SECRET='test-secret-env'
`
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("error creando .env temporal: %v", err)
	}

	// Limpiar variables de entorno para el test
	os.Unsetenv("GOOGLE_CLIENT_ID")
	os.Unsetenv("GOOGLE_CLIENT_SECRET")

	LoadDotEnv(envPath)

	if got := os.Getenv("GOOGLE_CLIENT_ID"); got != "test-client-id-env" {
		t.Errorf("GOOGLE_CLIENT_ID esperado 'test-client-id-env', obtenido '%s'", got)
	}
	if got := os.Getenv("GOOGLE_CLIENT_SECRET"); got != "test-secret-env" {
		t.Errorf("GOOGLE_CLIENT_SECRET esperado 'test-secret-env', obtenido '%s'", got)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("ODOO_URL", "https://erp.test.com")
	os.Setenv("ODOO_DB", "test_db")
	os.Setenv("GOOGLE_CLIENT_ID", "g-client-123")
	os.Setenv("GOOGLE_CLIENT_SECRET", "g-secret-456")

	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("ODOO_URL")
		os.Unsetenv("ODOO_DB")
		os.Unsetenv("GOOGLE_CLIENT_ID")
		os.Unsetenv("GOOGLE_CLIENT_SECRET")
	}()

	cfg := LoadConfig()

	if cfg.Server.Port != 9090 {
		t.Errorf("Puerto esperado 9090, obtenido %d", cfg.Server.Port)
	}
	if cfg.Odoo.URL != "https://erp.test.com" {
		t.Errorf("Odoo URL esperada https://erp.test.com, obtenida %s", cfg.Odoo.URL)
	}
	if cfg.Odoo.DB != "test_db" {
		t.Errorf("Odoo DB esperada test_db, obtenida %s", cfg.Odoo.DB)
	}
	if !cfg.IsGoogleConfigured() {
		t.Errorf("IsGoogleConfigured() debería ser true")
	}
}
