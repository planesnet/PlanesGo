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
