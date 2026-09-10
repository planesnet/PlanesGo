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

func TestResolveUserOdooConfigAutoRestoresEmptyStore(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "user_settings.json")
	// Almacén recién creado (vacío, simulando regeneración de contenedor)
	emptyStore, err := store.NewUserSettingsStore(jsonPath)
	if err != nil {
		t.Fatalf("error creando store: %v", err)
	}

	cfg := config.Config{
		Odoo: config.OdooConfig{
			URL: DefaultOdooURL,
			DB:  DefaultOdooDB,
		},
	}

	state := &AppState{
		cfg:       &cfg,
		userStore: emptyStore,
	}

	// Usuario que llega con su cookie de sesión válida
	sess := &SessionData{
		URL:        "https://planesnet.autopyme.com",
		DB:         "ap113",
		Username:   "luis_odoo",
		Password:   "mi_token_secreto_123",
		UserEmail:  "luis@planesnet.com",
		AuthMethod: "google",
	}

	// Al resolver la configuración, debe reconocer el token y auto-restaurar en store
	resolved := state.resolveUserOdooConfig(sess)

	if resolved.Password != "mi_token_secreto_123" {
		t.Errorf("Password esperada 'mi_token_secreto_123', obtenida '%s'", resolved.Password)
	}
	if resolved.Username != "luis_odoo" {
		t.Errorf("Username esperado 'luis_odoo', obtenido '%s'", resolved.Username)
	}
	if resolved.DB != "ap113" {
		t.Errorf("DB esperada 'ap113', obtenida '%s'", resolved.DB)
	}

	// Verificar que el store ahora tiene los datos guardados en disco/memoria
	saved, ok := emptyStore.GetSettings("luis@planesnet.com")
	if !ok {
		t.Fatalf("El store debería haber auto-restaurado la configuración de luis@planesnet.com")
	}
	if saved.OdooToken != "mi_token_secreto_123" {
		t.Errorf("Token guardado en store incorrecto: '%s'", saved.OdooToken)
	}
	if saved.OdooUser != "luis_odoo" {
		t.Errorf("Usuario Odoo guardado en store incorrecto: '%s'", saved.OdooUser)
	}
	if saved.OdooDB != "ap113" {
		t.Errorf("OdooDB guardada en store incorrecta: '%s'", saved.OdooDB)
	}
}
