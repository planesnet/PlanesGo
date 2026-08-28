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

func TestGetGoogleAuthCredentialsPriority(t *testing.T) {
	cfg := &Config{
		GoogleAuth: GoogleAuthConfig{
			ClientID:     "cfg-client-id",
			ClientSecret: "cfg-secret",
		},
	}

	// 1. Con variables de entorno activas
	os.Setenv("GOOGLE_CLIENT_ID", "env-client-id")
	os.Setenv("GOOGLE_CLIENT_SECRET", "env-secret")

	clientID, clientSecret := cfg.GetGoogleAuthCredentials()
	if clientID != "env-client-id" || clientSecret != "env-secret" {
		t.Errorf("Variables de entorno no tuvieron prioridad: id=%s, secret=%s", clientID, clientSecret)
	}

	// 2. Sin variables de entorno, usa config
	os.Unsetenv("GOOGLE_CLIENT_ID")
	os.Unsetenv("GOOGLE_CLIENT_SECRET")

	clientID, clientSecret = cfg.GetGoogleAuthCredentials()
	if clientID != "cfg-client-id" || clientSecret != "cfg-secret" {
		t.Errorf("No cargó de config fallback: id=%s, secret=%s", clientID, clientSecret)
	}

	// 3. Variables vacías
	cfgEmpty := &Config{}
	if cfgEmpty.IsGoogleConfigured() {
		t.Errorf("IsGoogleConfigured debería ser false para configuración vacía")
	}
}
