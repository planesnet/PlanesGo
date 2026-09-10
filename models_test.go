package main

import (
	"path/filepath"
	"testing"

	"pasigo/config"
	"pasigo/store"
)

func TestDecodeSessionSanitizesPasi(t *testing.T) {
	// Codificar una sesión antigua que contenía DB: "pasi"
	legacySession := SessionData{
		URL:        DefaultOdooURL,
		DB:         "pasi",
		Username:   "test@planesnet.com",
		Password:   "secret",
		AuthMethod: "google",
	}
	cookieVal := encodeSession(legacySession)

	decoded, err := decodeSession(cookieVal)
	if err != nil {
		t.Fatalf("error decodificando sesión: %v", err)
	}

	if decoded.DB != "" {
		t.Errorf("DB en sesión decodificada debería ser sanitizada a vacía, obtenida: '%s'", decoded.DB)
	}
}

func TestResolveUserOdooConfigNeverReturnsPasi(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "user_settings.json")
	userStore, err := store.NewUserSettingsStore(jsonPath)
	if err != nil {
		t.Fatalf("error creando store: %v", err)
	}

	cfg := config.Config{
		Odoo: config.OdooConfig{
			URL: DefaultOdooURL,
			DB:  "pasi", // Simulación de config legacy
		},
	}

	state := &AppState{
		cfg:       &cfg,
		userStore: userStore,
	}

	// Caso 1: Sesión nula
	resolved := state.resolveUserOdooConfig(nil)
	if resolved.DB != "" {
		t.Errorf("DB resuelta sin sesión debería ser vacía, pero fue '%s'", resolved.DB)
	}

	// Caso 2: Sesión con "pasi"
	sess := &SessionData{
		Username:  "user1@planesnet.com",
		UserEmail: "user1@planesnet.com",
		DB:        "pasi",
	}
	resolved2 := state.resolveUserOdooConfig(sess)
	if resolved2.DB != "" {
		t.Errorf("DB resuelta con sesión 'pasi' debería ser vacía, pero fue '%s'", resolved2.DB)
	}

	// Caso 3: Con base de datos real ("ap113")
	_ = userStore.SaveSettings(store.UserSettings{
		Email:  "user1@planesnet.com",
		OdooDB: "ap113",
	})
	resolved3 := state.resolveUserOdooConfig(sess)
	if resolved3.DB != "ap113" {
		t.Errorf("DB resuelta debería ser 'ap113', pero fue '%s'", resolved3.DB)
	}
}
