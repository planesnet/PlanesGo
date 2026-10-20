package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"pasigo/config"
	"pasigo/store"
)

func TestGenerateAntigravityTokenUnauthenticated(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "user_settings.json")
	userStore, err := store.NewUserSettingsStore(jsonPath)
	if err != nil {
		t.Fatalf("error creando store: %v", err)
	}

	state := &AppState{
		cfg:       &config.Config{},
		userStore: userStore,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/settings/generate-antigravity-token", nil)
	rec := httptest.NewRecorder()

	state.handleGenerateAntigravityToken(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Esperado 401 Unauthorized sin sesión, obtenido %d", rec.Code)
	}
}

func TestGenerateAntigravityTokenAuthenticated(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "user_settings.json")
	userStore, err := store.NewUserSettingsStore(jsonPath)
	if err != nil {
		t.Fatalf("error creando store: %v", err)
	}

	userEmail := "programador@planesnet.com"
	validSession := SessionData{
		Username:   userEmail,
		UserEmail:  userEmail,
		URL:        DefaultOdooURL,
		DB:         "ap113",
		AuthMethod: "google",
	}
	cookieValue := encodeSession(validSession)

	state := &AppState{
		cfg:       &config.Config{},
		userStore: userStore,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/settings/generate-antigravity-token", nil)
	req.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: cookieValue,
	})
	rec := httptest.NewRecorder()

	state.handleGenerateAntigravityToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Esperado 200 OK, obtenido %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("error decodificando json: %v", err)
	}

	if resp["success"] != true {
		t.Errorf("Esperado success: true, obtenido %v", resp["success"])
	}

	token, ok := resp["token"].(string)
	if !ok || len(token) != 40 || token[:8] != "plg_sec_" {
		t.Errorf("Token generado no tiene formato esperado: %v", resp["token"])
	}

	// Comprobar que el usuario ahora tiene el token en el store
	saved, found := userStore.GetSettings(userEmail)
	if !found {
		t.Fatalf("no se encontraron ajustes guardados para %s", userEmail)
	}
	if saved.AntigravityToken != token {
		t.Errorf("token en store '%s' no coincide con respuesta '%s'", saved.AntigravityToken, token)
	}
}
