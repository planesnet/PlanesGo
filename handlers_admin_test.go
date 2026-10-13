package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pasigo/config"
)

func TestHandleAdminAccessControl(t *testing.T) {
	cfg := &config.Config{
		GoogleAuth: config.GoogleAuthConfig{
			Enabled: true,
		},
	}
	state := &AppState{
		cfg: cfg,
	}

	// 1. /admin sin sesión -> redirección a /login?next=/admin
	reqNoSession := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rrNoSession := httptest.NewRecorder()
	state.handleAdmin(rrNoSession, reqNoSession)

	if rrNoSession.Code != http.StatusFound {
		t.Errorf("sin sesión: esperado 302 Found, obtenido %d", rrNoSession.Code)
	}
	if loc := rrNoSession.Header().Get("Location"); loc != "/login?next=/admin" {
		t.Errorf("sin sesión: esperado redirect a '/login?next=/admin', obtenido '%s'", loc)
	}

	// 2. Sesión de Google con email que no es de administrador -> 403 Forbidden
	nonAdminGoogle := SessionData{
		Username:   "usuario@planesnet.com",
		UserEmail:  "usuario@planesnet.com",
		AuthMethod: "google",
	}
	reqNonAdmin := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqNonAdmin.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: encodeSession(nonAdminGoogle),
	})
	rrNonAdmin := httptest.NewRecorder()
	state.handleAdmin(rrNonAdmin, reqNonAdmin)

	if rrNonAdmin.Code != http.StatusForbidden {
		t.Errorf("Google no admin: esperado 403 Forbidden, obtenido %d", rrNonAdmin.Code)
	}

	// 3. Sesión 'local' con el email de luis -> no da acceso (redirige a Google Auth si está activo)
	luisLocal := SessionData{
		Username:   "luis@planesnet.com",
		UserEmail:  "luis@planesnet.com",
		AuthMethod: "local",
	}
	reqLuisLocal := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqLuisLocal.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: encodeSession(luisLocal),
	})
	rrLuisLocal := httptest.NewRecorder()
	state.handleAdmin(rrLuisLocal, reqLuisLocal)

	// Con GoogleAuth activo, debe redirigir a Google Auth
	if rrLuisLocal.Code != http.StatusFound {
		t.Errorf("Luis local con GoogleAuth activo: esperado 302 Redirect, obtenido %d", rrLuisLocal.Code)
	}
	if loc := rrLuisLocal.Header().Get("Location"); loc != "/auth/google?next=/admin" {
		t.Errorf("Luis local: esperado redirect a '/auth/google?next=/admin', obtenido '%s'", loc)
	}

	// 3b. Luis local pero con GoogleAuth desactivado -> 403 Forbidden
	stateDisabledOAuth := &AppState{
		cfg: &config.Config{
			GoogleAuth: config.GoogleAuthConfig{
				Enabled: false,
			},
		},
	}
	rrLuisLocalNoOAuth := httptest.NewRecorder()
	stateDisabledOAuth.handleAdmin(rrLuisLocalNoOAuth, reqLuisLocal)
	if rrLuisLocalNoOAuth.Code != http.StatusForbidden {
		t.Errorf("Luis local sin OAuth: esperado 403 Forbidden, obtenido %d", rrLuisLocalNoOAuth.Code)
	}

	// 4. Sesión de Google de luis -> 200 OK
	luisGoogle := SessionData{
		Username:   "luis@planesnet.com",
		UserEmail:  "luis@planesnet.com",
		AuthMethod: "google",
	}
	reqLuisGoogle := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqLuisGoogle.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: encodeSession(luisGoogle),
	})
	rrLuisGoogle := httptest.NewRecorder()
	state.handleAdmin(rrLuisGoogle, reqLuisGoogle)

	if rrLuisGoogle.Code != http.StatusOK {
		t.Errorf("Luis Google: esperado 200 OK, obtenido %d. Body: %s", rrLuisGoogle.Code, rrLuisGoogle.Body.String())
	}
}

func TestHandleAPIAdminGCPInvoicesAccessControl(t *testing.T) {
	state := &AppState{
		cfg: &config.Config{
			GoogleAuth: config.GoogleAuthConfig{
				Enabled: true,
			},
		},
	}

	// 1. Sin sesión -> 401 Unauthorized
	reqNoSession := httptest.NewRequest(http.MethodGet, "/api/admin/gcp-invoices", nil)
	rrNoSession := httptest.NewRecorder()
	state.handleAPIAdminGCPInvoices(rrNoSession, reqNoSession)

	if rrNoSession.Code != http.StatusUnauthorized {
		t.Errorf("API sin sesión: esperado 401 Unauthorized, obtenido %d", rrNoSession.Code)
	}
	var res1 map[string]interface{}
	if err := json.Unmarshal(rrNoSession.Body.Bytes(), &res1); err != nil || res1["success"] != false {
		t.Errorf("API sin sesión: JSON no contiene success: false: %v", res1)
	}

	// 2. Sesión Google no admin -> 403 Forbidden
	nonAdminGoogle := SessionData{
		Username:   "empleado@planesnet.com",
		UserEmail:  "empleado@planesnet.com",
		AuthMethod: "google",
	}
	reqNonAdmin := httptest.NewRequest(http.MethodGet, "/api/admin/gcp-invoices", nil)
	reqNonAdmin.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: encodeSession(nonAdminGoogle),
	})
	rrNonAdmin := httptest.NewRecorder()
	state.handleAPIAdminGCPInvoices(rrNonAdmin, reqNonAdmin)

	if rrNonAdmin.Code != http.StatusForbidden {
		t.Errorf("API Google no admin: esperado 403 Forbidden, obtenido %d", rrNonAdmin.Code)
	}

	// 3. Sesión local con email de luis -> 403 Forbidden
	luisLocal := SessionData{
		Username:   "luis@planesnet.com",
		UserEmail:  "luis@planesnet.com",
		AuthMethod: "local",
	}
	reqLuisLocal := httptest.NewRequest(http.MethodGet, "/api/admin/gcp-invoices", nil)
	reqLuisLocal.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: encodeSession(luisLocal),
	})
	rrLuisLocal := httptest.NewRecorder()
	state.handleAPIAdminGCPInvoices(rrLuisLocal, reqLuisLocal)

	if rrLuisLocal.Code != http.StatusForbidden {
		t.Errorf("API Luis local: esperado 403 Forbidden, obtenido %d", rrLuisLocal.Code)
	}
}
