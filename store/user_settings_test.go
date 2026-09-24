package store

import (
	"path/filepath"
	"testing"
)

func TestUserSettingsStore(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "user_settings.json")

	store, err := NewUserSettingsStore(jsonPath)
	if err != nil {
		t.Fatalf("error creando almacén: %v", err)
	}

	userEmail := "luis@planesnet.com"
	initialSettings := UserSettings{
		Email:     userEmail,
		OdooUser:  "luis@planesnet.com",
		OdooToken: "secret_api_key_12345",
		OdooURL:   "https://planesnet.autopyme.com",
		OdooDB:    "ap113",
		PageLimit: 200,
	}

	if err := store.SaveSettings(initialSettings); err != nil {
		t.Fatalf("error guardando ajustes: %v", err)
	}

	// Recuperar en memoria
	got, ok := store.GetSettings(userEmail)
	if !ok {
		t.Fatalf("no se encontró el usuario guardado")
	}
	if got.OdooToken != "secret_api_key_12345" {
		t.Errorf("OdooToken esperado 'secret_api_key_12345', obtenido '%s'", got.OdooToken)
	}
	if got.OdooDB != "ap113" {
		t.Errorf("OdooDB esperado 'ap113', obtenido '%s'", got.OdooDB)
	}

	if sharedDB := store.GetSharedOdooDB(); sharedDB != "ap113" {
		t.Errorf("GetSharedOdooDB esperado 'ap113', obtenido '%s'", sharedDB)
	}
	if sharedURL := store.GetSharedOdooURL(); sharedURL != "https://planesnet.autopyme.com" {
		t.Errorf("GetSharedOdooURL esperado 'https://planesnet.autopyme.com', obtenido '%s'", sharedURL)
	}

	// Reinicializar almacén para verificar persistencia en disco
	store2, err := NewUserSettingsStore(jsonPath)
	if err != nil {
		t.Fatalf("error recargando almacén desde disco: %v", err)
	}

	got2, ok2 := store2.GetSettings(userEmail)
	if !ok2 {
		t.Fatalf("no se encontró el usuario tras recargar desde disco")
	}
	if got2.OdooToken != "secret_api_key_12345" {
		t.Errorf("OdooToken persistido esperado 'secret_api_key_12345', obtenido '%s'", got2.OdooToken)
	}
	if got2.OdooDB != "ap113" {
		t.Errorf("OdooDB persistido esperado 'ap113', obtenido '%s'", got2.OdooDB)
	}
	if sharedDB2 := store2.GetSharedOdooDB(); sharedDB2 != "ap113" {
		t.Errorf("GetSharedOdooDB persistido esperado 'ap113', obtenido '%s'", sharedDB2)
	}
}

func TestUserSettingsStoreSanitizePasi(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "user_settings.json")

	store, err := NewUserSettingsStore(jsonPath)
	if err != nil {
		t.Fatalf("error creando almacén: %v", err)
	}

	userEmail := "test@planesnet.com"
	settings := UserSettings{
		Email:     userEmail,
		OdooUser:  "test@planesnet.com",
		OdooToken: "tok123",
		OdooDB:    "pasi", // Valor legacy que debe ser sanitizado
	}

	if err := store.SaveSettings(settings); err != nil {
		t.Fatalf("error guardando ajustes: %v", err)
	}

	got, ok := store.GetSettings(userEmail)
	if !ok {
		t.Fatalf("no se encontró usuario")
	}
	if got.OdooDB != "" {
		t.Errorf("OdooDB debería haber sido sanitizado a '', pero es '%s'", got.OdooDB)
	}
	if sharedDB := store.GetSharedOdooDB(); sharedDB != "" {
		t.Errorf("GetSharedOdooDB debería ser '', pero es '%s'", sharedDB)
	}
}

func TestGenerateAntigravityToken(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "user_settings.json")

	store, err := NewUserSettingsStore(jsonPath)
	if err != nil {
		t.Fatalf("error creando almacén: %v", err)
	}

	userEmail := "dev@planesnet.com"
	token, err := store.GenerateAntigravityToken(userEmail)
	if err != nil {
		t.Fatalf("error generando token: %v", err)
	}

	if len(token) != 40 || token[:8] != "plg_sec_" {
		t.Fatalf("formato de token inválido: %s", token)
	}

	// Comprobar que se guardó en el store
	settings, ok := store.GetSettings(userEmail)
	if !ok {
		t.Fatalf("no se encontró configuración guardada para el usuario")
	}
	if settings.AntigravityToken != token {
		t.Fatalf("token en store no coincide: esperado %s, obtenido %s", token, settings.AntigravityToken)
	}

	// Comprobar búsqueda inversa por token
	foundUser, found := store.GetUserByAntigravityToken(token)
	if !found {
		t.Fatalf("no se encontró usuario por token")
	}
	if foundUser.Email != userEmail {
		t.Fatalf("email de usuario por token incorrecto: %s", foundUser.Email)
	}
}

func TestSaveSettingsPreservesAntigravityToken(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "user_settings.json")

	store, err := NewUserSettingsStore(jsonPath)
	if err != nil {
		t.Fatalf("error creando almacén: %v", err)
	}

	userEmail := "dev@planesnet.com"
	token, err := store.GenerateAntigravityToken(userEmail)
	if err != nil {
		t.Fatalf("error generando token: %v", err)
	}

	// Guardar nuevos ajustes de Odoo omitiendo el token de Antigravity
	newOdooSettings := UserSettings{
		Email:     userEmail,
		OdooUser:  "dev@planesnet.com",
		OdooToken: "new_api_key",
		OdooDB:    "ap113",
		OdooURL:   "https://planesnet.autopyme.com",
	}

	if err := store.SaveSettings(newOdooSettings); err != nil {
		t.Fatalf("error guardando nuevos ajustes: %v", err)
	}

	// Verificar que el token de Antigravity se preservó
	updated, ok := store.GetSettings(userEmail)
	if !ok {
		t.Fatalf("no se encontraron ajustes")
	}
	if updated.AntigravityToken != token {
		t.Fatalf("el token de Antigravity no fue preservado: esperado %s, obtenido %s", token, updated.AntigravityToken)
	}
	if updated.OdooToken != "new_api_key" {
		t.Fatalf("el nuevo OdooToken no se actualizó: %s", updated.OdooToken)
	}
}

