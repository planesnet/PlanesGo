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
		OdooURL:   "https://www.planesnet.com",
		OdooDB:    "pasi",
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
}
