package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"pasigo/config"
	"pasigo/odoo"
	"pasigo/store"
)

func TestAntigravityStatusWithoutTokenFails(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/antigravity/status", nil)
	rec := httptest.NewRecorder()

	state.handleAntigravityStatus(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Esperado status 401 Unauthorized, obtenido %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("error decodificando json: %v", err)
	}
	if resp["ready"] != false {
		t.Errorf("Esperado ready: false, obtenido %v", resp["ready"])
	}
}

func TestAntigravityStatusWithValidToken(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "user_settings.json")
	userStore, err := store.NewUserSettingsStore(jsonPath)
	if err != nil {
		t.Fatalf("error creando store: %v", err)
	}

	testToken := "plg_sec_test_secret_token_123"
	_ = userStore.SaveSettings(store.UserSettings{
		Email:            "dev@planesnet.com",
		OdooUser:         "dev@planesnet.com",
		OdooToken:        "secret",
		OdooURL:          DefaultOdooURL,
		OdooDB:           "ap113",
		AntigravityToken: testToken,
	})

	state := &AppState{
		cfg:       &config.Config{},
		userStore: userStore,
	}

	req := httptest.NewRequest(http.MethodGet, "/antigravity/status", nil)
	req.Header.Set("X-Antigravity-Token", testToken)
	rec := httptest.NewRecorder()

	state.handleAntigravityStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Esperado status 200 OK, obtenido %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("error decodificando json: %v", err)
	}

	if resp["ready"] != true {
		t.Errorf("Esperado ready: true, obtenido %v", resp["ready"])
	}
	if resp["user_email"] != "dev@planesnet.com" {
		t.Errorf("Esperado user_email 'dev@planesnet.com', obtenido '%v'", resp["user_email"])
	}
}

func TestMultiTimerConcurrencyAndWatchdogInactivity(t *testing.T) {
	state := &AppState{}
	userUID := 42
	nowMs := time.Now().UnixMilli()

	// 1. Iniciar Tarea A (activa y con latido reciente)
	timerA := &odoo.ActiveTimer{
		TaskID:        101,
		TaskName:      "Tarea A Concurrente",
		IsRunning:     true,
		StartedAt:     nowMs - 60000,
		LastHeartbeat: nowMs,
	}
	state.setActiveTimer(userUID, timerA)

	// 2. Iniciar Tarea B (sin latidos desde hace 20 minutos)
	twentyMinutesAgo := nowMs - (20 * 60 * 1000)
	timerB := &odoo.ActiveTimer{
		TaskID:        102,
		TaskName:      "Tarea B Inactiva",
		IsRunning:     true,
		StartedAt:     twentyMinutesAgo - 5000,
		LastHeartbeat: twentyMinutesAgo,
	}
	state.setActiveTimer(userUID, timerB)

	// Comprobar que ambas tareas coexisten concurrentemente
	allTimers := state.getActiveTimers(userUID)
	if len(allTimers) != 2 {
		t.Fatalf("Se esperaban 2 temporizadores concurrentes, obtenidos %d", len(allTimers))
	}

	// 3. Ejecutar el comprobador del Watchdog con umbral de 15 minutos
	state.checkIdleTimers(time.Now(), 15*time.Minute)

	// 4. Verificar estados tras la pasada del watchdog
	savedA := state.getActiveTimerByKey(userUID, "task_101")
	if savedA == nil || !savedA.IsRunning {
		t.Errorf("La Tarea A (reciente) debería seguir corriendo")
	}

	savedB := state.getActiveTimerByKey(userUID, "task_102")
	if savedB == nil {
		t.Fatalf("La Tarea B no se encontró en el mapa")
	}
	if savedB.IsRunning {
		t.Errorf("La Tarea B debería haber sido pausada por inactividad tras 20 minutos sin latidos")
	}
	if savedB.AccumulatedMs <= 0 {
		t.Errorf("La Tarea B debería haber acumulado el tiempo hasta su último latido, obtenido: %d", savedB.AccumulatedMs)
	}
}

func TestProjectNamesMatchAndTaskIsolation(t *testing.T) {
	// 1. Pruebas de projectNamesMatch (insensible a mayúsculas/minúsculas y tolerancia de espacios)
	testCases := []struct {
		a, b     string
		expected bool
	}{
		{"PLANES SECURITY ALERT", "PLANES SECURITY ALERT", true},
		{"PlanesSecurityAlert", "PLANES SECURITY ALERT", true},
		{"planes security alert", "PLANES SECURITY ALERT", true},
		{"PLANES SECURITY ALERT", "planessecurityalert", true},
		{"PLANESGO", "PlanesGo", true},
		{"PlanesGo", "planesgo", true},
		{"PlanesSecurityAlert", "AutoPyme Logistics", false},
		{"", "PLANESGO", false},
		{"PLANESGO", "", false},
	}

	for _, tc := range testCases {
		res := projectNamesMatch(tc.a, tc.b)
		if res != tc.expected {
			t.Errorf("projectNamesMatch(%q, %q) = %v; esperado %v", tc.a, tc.b, res, tc.expected)
		}
	}

	// 2. Pruebas de cleanAntigravityTaskName
	taskCases := []struct {
		input    string
		expected string
	}{
		{"[AGY] PlanesSecurityAlert", "PlanesSecurityAlert"},
		{"[ANTIGRAVITY] Auditoría de Seguridad", "Auditoría de Seguridad"},
		{"[agy] Tarea en minúsculas", "Tarea en minúsculas"},
		{"Tarea Manual Usuario", "Tarea Manual Usuario"},
	}

	for _, tc := range taskCases {
		res := cleanAntigravityTaskName(tc.input)
		if res != tc.expected {
			t.Errorf("cleanAntigravityTaskName(%q) = %q; esperado %q", tc.input, res, tc.expected)
		}
	}
}

func TestNormalizeTaskType(t *testing.T) {
	testCases := []struct {
		rawTitle    string
		realWork    string
		wantType    string
		wantClean   string
	}{
		{
			rawTitle:  "Implementación de tag_ids y vistas en PlanesGo",
			realWork:  "Añadiendo soporte de tags y columnas en la interfaz",
			wantType:  "Implementación",
			wantClean: "Implementación",
		},
		{
			rawTitle:  "Desarrollo de nuevo endpoint REST",
			realWork:  "Creando handler y pruebas unitarias",
			wantType:  "Desarrollo",
			wantClean: "Desarrollo",
		},
		{
			rawTitle:  "Auditoría y análisis de base de datos",
			realWork:  "Inspeccionando índices en PostgreSQL",
			wantType:  "Análisis",
			wantClean: "Análisis",
		},
		{
			rawTitle:  "Ajustes menores de diseño",
			realWork:  "Corrigiendo espaciado y márgenes",
			wantType:  "Ajustes",
			wantClean: "Ajustes",
		},
		{
			rawTitle:  "Mantenimiento de servidor y Docker",
			realWork:  "Reiniciando contenedores y Nginx",
			wantType:  "Servidor",
			wantClean: "Servidor",
		},
		{
			rawTitle:  "Soporte cliente y resolución de dudas",
			realWork:  "Atendiendo consulta funcional",
			wantType:  "Cliente",
			wantClean: "Cliente",
		},
		{
			rawTitle:  "[AGY] Implementación de vistas",
			realWork:  "",
			wantType:  "Implementación",
			wantClean: "Implementación",
		},
	}

	for _, tc := range testCases {
		gotType, gotClean := NormalizeTaskType(tc.rawTitle, tc.realWork, "")
		if gotType != tc.wantType {
			t.Errorf("NormalizeTaskType(%q, %q) type = %q; want %q", tc.rawTitle, tc.realWork, gotType, tc.wantType)
		}
		if gotClean != tc.wantClean {
			t.Errorf("NormalizeTaskType(%q, %q) cleanName = %q; want %q", tc.rawTitle, tc.realWork, gotClean, tc.wantClean)
		}
	}
}

