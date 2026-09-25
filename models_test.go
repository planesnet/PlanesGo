package main

import (
	"path/filepath"
	"testing"
	"time"

	"pasigo/config"
	"pasigo/odoo"
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

func TestFormatHoursToHHMM(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{0.0, "0:00"},
		{-1.5, "0:00"},
		{0.5, "0:30"},
		{1.0, "1:00"},
		{2.62, "2:37"},
		{3.00, "3:00"},
		{4.32, "4:19"},
		{11.50, "11:30"},
		{0.13, "0:08"},
	}

	for _, tc := range tests {
		actual := FormatHoursToHHMM(tc.input)
		if actual != tc.expected {
			t.Errorf("FormatHoursToHHMM(%f) = '%s', esperado '%s'", tc.input, actual, tc.expected)
		}
	}
}

func TestCheckIdleTimersAntigravityNotAccumulatingEpoch(t *testing.T) {
	state := &AppState{
		activeTimers: make(map[int]map[string]*odoo.ActiveTimer),
	}

	// Temporizador Antigravity con StartedAt en 0 (típico de latidos externos)
	userUID := 1
	timerKey := "dev:123"
	curTimer := &odoo.ActiveTimer{
		Source:        "antigravity",
		StartedAt:     0,
		LastHeartbeat: 1000,
		AccumulatedMs: 18 * 60 * 1000, // 18 minutos ya acumulados
		IsRunning:     true,
		UnitAmount:    0.3,
	}
	state.activeTimers[userUID] = map[string]*odoo.ActiveTimer{
		timerKey: curTimer,
	}

	// Ejecutar checkIdleTimers con timeout de 5 minutos
	state.checkIdleTimers(time.UnixMilli(2000000), 5*time.Minute)

	// Verificar que no se sumó epoch Unix y que UnitAmount no se disparó
	if curTimer.UnitAmount > 24.0 {
		t.Fatalf("UnitAmount superó las 24 horas: %f", curTimer.UnitAmount)
	}
	if curTimer.AccumulatedMs > 24*3600*1000 {
		t.Fatalf("AccumulatedMs superó las 24 horas: %d", curTimer.AccumulatedMs)
	}
	if curTimer.IsRunning {
		t.Errorf("El temporizador debería estar pausado")
	}
}

func TestCalculateWallClockHours(t *testing.T) {
	// Caso 1: Concurrencia exacta - dos tareas de 2h que corren en el mismo intervalo [10:00, 12:00]
	entriesExactConcurrency := []odoo.TimesheetEntry{
		{
			ID:         1,
			Date:       "2026-09-25",
			UnitAmount: 2.0,
			CreateDate: "2026-09-25 10:00:00",
			WriteDate:  "2026-09-25 12:00:00",
		},
		{
			ID:         2,
			Date:       "2026-09-25",
			UnitAmount: 2.0,
			CreateDate: "2026-09-25 10:00:00",
			WriteDate:  "2026-09-25 12:00:00",
		},
	}
	wc1 := CalculateWallClockHours(entriesExactConcurrency)
	if wc1 != 2.0 {
		t.Errorf("Esperado 2.0h para concurrencia exacta, obtenido: %f", wc1)
	}

	// Caso 2: Tareas secuenciales sin solapamiento - [09:00, 11:00] (2h) y [14:00, 16:00] (2h)
	entriesDisjoint := []odoo.TimesheetEntry{
		{
			ID:         3,
			Date:       "2026-09-25",
			UnitAmount: 2.0,
			CreateDate: "2026-09-25 09:00:00",
			WriteDate:  "2026-09-25 11:00:00",
		},
		{
			ID:         4,
			Date:       "2026-09-25",
			UnitAmount: 2.0,
			CreateDate: "2026-09-25 14:00:00",
			WriteDate:  "2026-09-25 16:00:00",
		},
	}
	wc2 := CalculateWallClockHours(entriesDisjoint)
	if wc2 != 4.0 {
		t.Errorf("Esperado 4.0h para tareas disjuntas, obtenido: %f", wc2)
	}

	// Caso 3: Solapamiento parcial - [09:00, 11:00] (2h) y [10:00, 12:00] (2h) -> [09:00, 12:00] = 3h
	entriesPartial := []odoo.TimesheetEntry{
		{
			ID:         5,
			Date:       "2026-09-25",
			UnitAmount: 2.0,
			CreateDate: "2026-09-25 09:00:00",
			WriteDate:  "2026-09-25 11:00:00",
		},
		{
			ID:         6,
			Date:       "2026-09-25",
			UnitAmount: 2.0,
			CreateDate: "2026-09-25 10:00:00",
			WriteDate:  "2026-09-25 12:00:00",
		},
	}
	wc3 := CalculateWallClockHours(entriesPartial)
	if wc3 != 3.0 {
		t.Errorf("Esperado 3.0h para solapamiento parcial, obtenido: %f", wc3)
	}

	// Caso 4: Tareas en días distintos - no deben fusionarse entre sí
	entriesMultiDay := []odoo.TimesheetEntry{
		{
			ID:         7,
			Date:       "2026-09-24",
			UnitAmount: 3.0,
			CreateDate: "2026-09-24 10:00:00",
			WriteDate:  "2026-09-24 13:00:00",
		},
		{
			ID:         8,
			Date:       "2026-09-25",
			UnitAmount: 3.0,
			CreateDate: "2026-09-25 10:00:00",
			WriteDate:  "2026-09-25 13:00:00",
		},
	}
	wc4 := CalculateWallClockHours(entriesMultiDay)
	if wc4 != 6.0 {
		t.Errorf("Esperado 6.0h para dos días distintos, obtenido: %f", wc4)
	}
}

