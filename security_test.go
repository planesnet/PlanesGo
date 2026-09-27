package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"pasigo/config"
	"pasigo/store"
)

func newStateWithStoredUser(t *testing.T) *AppState {
	t.Helper()
	userStore, err := store.NewUserSettingsStore(filepath.Join(t.TempDir(), "user_settings.json"))
	if err != nil {
		t.Fatalf("error creando store: %v", err)
	}
	if err := userStore.SaveSettings(store.UserSettings{
		Email:     "victima@planesnet.com",
		OdooUser:  "victima@planesnet.com",
		OdooToken: "secreto-odoo",
		OdooURL:   DefaultOdooURL,
		OdooDB:    DefaultOdooDB,
	}); err != nil {
		t.Fatalf("error guardando ajustes: %v", err)
	}
	return &AppState{cfg: &config.Config{}, userStore: userStore}
}

// Sin token ni cookie no debe reutilizarse la cuenta de ningún usuario del almacén.
func TestAntigravitySessionNoFallbackToStoredUser(t *testing.T) {
	state := newStateWithStoredUser(t)

	for name, setup := range map[string]func(r *http.Request){
		"sin cabeceras":     func(r *http.Request) {},
		"email en cabecera": func(r *http.Request) { r.Header.Set("X-User-Email", "victima@planesnet.com") },
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/antigravity/status", nil)
			setup(req)
			if sess, err := state.resolveAntigravitySession(req, ""); err == nil || sess != nil {
				t.Fatalf("se esperaba error de autenticación, obtenida sesión %+v", sess)
			}
			rec := httptest.NewRecorder()
			state.handleAntigravityStatus(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("esperado 401, obtenido %d", rec.Code)
			}
		})
	}
}

// Una cookie fabricada con el formato antiguo (JSON en base64) no debe aceptarse.
func TestDecodeSessionRejectsForgedCookie(t *testing.T) {
	forged, _ := json.Marshal(SessionData{UserEmail: "victima@planesnet.com"})
	for _, val := range []string{
		base64.StdEncoding.EncodeToString(forged),
		base64.RawURLEncoding.EncodeToString(forged),
	} {
		if sess, err := decodeSession(val); err == nil {
			t.Errorf("cookie fabricada aceptada: %+v", sess)
		}
	}
}

func TestSessionCookieRoundTripAndTamper(t *testing.T) {
	orig := SessionData{UserEmail: "luis@planesnet.com", Password: "clave", DB: DefaultOdooDB}
	cookie := encodeSession(orig)
	if cookie == "" {
		t.Fatal("encodeSession devolvió cadena vacía")
	}

	got, err := decodeSession(cookie)
	if err != nil || got.UserEmail != orig.UserEmail || got.Password != orig.Password {
		t.Fatalf("roundtrip fallido: %+v, err=%v", got, err)
	}

	raw, _ := base64.RawURLEncoding.DecodeString(cookie)
	raw[len(raw)-1] ^= 0x01
	if _, err := decodeSession(base64.RawURLEncoding.EncodeToString(raw)); err == nil {
		t.Error("cookie manipulada aceptada")
	}
}

// Las credenciales de servicio del entorno no deben usarse para peticiones anónimas.
func TestResolveUserOdooConfigAnonymousGetsNoDefaultCredentials(t *testing.T) {
	state := &AppState{cfg: &config.Config{Odoo: config.OdooConfig{Username: "servicio", Password: "clave-servicio"}}}
	if cfg := state.resolveUserOdooConfig(nil); cfg.Password != "" {
		t.Errorf("sesión anónima obtuvo credenciales por defecto (usuario %q)", cfg.Username)
	}
}
