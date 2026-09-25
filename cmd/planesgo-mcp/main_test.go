package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestCleanURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", DefaultServer},
		{"http://localhost:8089", DefaultServer},
		{"http://127.0.0.1:8089", DefaultServer},
		{"https://planesgo.autopyme.com/", "https://planesgo.autopyme.com"},
		{"https://custom.planesgo.com", "https://custom.planesgo.com"},
	}

	for _, tc := range tests {
		got := cleanURL(tc.input)
		if got != tc.expected {
			t.Errorf("cleanURL(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestToolsDefinition(t *testing.T) {
	tools := getToolsDefinition()
	if len(tools) != 7 {
		t.Fatalf("expected 7 tools defined, got %d", len(tools))
	}

	expectedNames := map[string]bool{
		"planesgo_check":        false,
		"planesgo_beat":         false,
		"planesgo_stop":         false,
		"planesgo_list_tasks":   false,
		"planesgo_status":       false,
		"planesgo_set_project":  false,
		"planesgo_close_ticket": false,
	}

	for _, tool := range tools {
		name, ok := tool["name"].(string)
		if !ok {
			t.Errorf("tool missing name")
			continue
		}
		if _, exists := expectedNames[name]; exists {
			expectedNames[name] = true
		} else {
			t.Errorf("unexpected tool name: %s", name)
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("tool %s was not found in definition", name)
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
			wantType:  "Desarrollo",
			wantClean: "Desarrollo",
		},
		{
			rawTitle:  "Desarrollo de nuevo endpoint REST",
			realWork:  "Creando handler y pruebas unitarias",
			wantType:  "Desarrollo",
			wantClean: "Desarrollo",
		},
		{
			rawTitle:  "[AGY] Implementación de vistas",
			realWork:  "",
			wantType:  "Desarrollo",
			wantClean: "Desarrollo",
		},
		{
			rawTitle:  "Análisis y diseño de la arquitectura de pasigo",
			realWork:  "Investigando patrones y diseño de endpoints",
			wantType:  "Análisis y diseño",
			wantClean: "Análisis y diseño",
		},
		{
			rawTitle:  "Batería de pruebas unitarias y de integración",
			realWork:  "Ejecutando tests de concurrencia y validación",
			wantType:  "Pruebas",
			wantClean: "Pruebas",
		},
		{
			rawTitle:  "[Pruebas] Verificación de suite Odoo",
			realWork:  "",
			wantType:  "Pruebas",
			wantClean: "Pruebas",
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

func TestDoWithRetry(t *testing.T) {
	var attempts int32

	// Servidor que falla con 502 en los 2 primeros intentos y responde 200 en el tercero
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := atomic.AddInt32(&attempts, 1)
		if att < 3 {
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`{"error":"bad gateway temporario"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	client := &PlanesGoClient{
		BaseURL:    ts.URL,
		Token:      "test-token",
		HTTPClient: ts.Client(),
		RetryDelay: 10 * time.Millisecond, // Delay mínimo para el test unitario
	}

	body, status, err := client.doWithRetry(http.MethodGet, ts.URL+"/test", nil, nil)
	if err != nil {
		t.Fatalf("doWithRetry falló inesperadamente: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status esperado 200, obtenido %d", status)
	}
	if string(body) != `{"status":"ok"}` {
		t.Fatalf("cuerpo inesperado: %s", string(body))
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("se esperaban 3 intentos, pero hubo %d", attempts)
	}
}

func TestFindConfigStrictLocation(t *testing.T) {
	// 1. Probar que no busca en directorios padres
	parentDir := t.TempDir()
	childDir := filepath.Join(parentDir, "subproject")
	if err := os.MkdirAll(childDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Poner .planesgo.json en el padre
	parentConfig := []byte(`{"odoo_project_id": 999, "odoo_project_name": "PROYECTO PADRE"}`)
	if err := os.WriteFile(filepath.Join(parentDir, ".planesgo.json"), parentConfig, 0644); err != nil {
		t.Fatal(err)
	}

	// Buscar desde el subdirectorio NO debe encontrar el del padre
	_, _, err := findConfig(childDir)
	if err == nil {
		t.Errorf("findConfig(childDir) debería fallar y no heredar el .planesgo.json del padre")
	}

	// 2. Probar que busca estrictamente en el directorio raíz del proyecto
	projectDir := t.TempDir()
	projConfig := []byte(`{"odoo_project_id": 123, "odoo_project_name": "MI PROYECTO"}`)
	if err := os.WriteFile(filepath.Join(projectDir, ".planesgo.json"), projConfig, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, foundPath, err := findConfig(projectDir)
	if err != nil {
		t.Fatalf("findConfig(projectDir) falló: %v", err)
	}
	if cfg.OdooProjectID != 123 || cfg.OdooProjectName != "MI PROYECTO" {
		t.Errorf("configuración leída incorrecta: %+v", cfg)
	}
	if foundPath != filepath.Join(projectDir, ".planesgo.json") {
		t.Errorf("ruta encontrada inesperada: %s", foundPath)
	}

	// 3. Probar que no busca en directorios hijos (ej. custom/)
	noRootProj := t.TempDir()
	customSubdir := filepath.Join(noRootProj, "custom")
	if err := os.MkdirAll(customSubdir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(customSubdir, ".planesgo.json"), projConfig, 0644); err != nil {
		t.Fatal(err)
	}

	// Si no está en la raíz, debe fallar
	_, _, err = findConfig(noRootProj)
	if err == nil {
		t.Errorf("findConfig(noRootProj) debería fallar cuando solo existe en el subdirectorio hijo 'custom'")
	}
}

func TestFormatTokens(t *testing.T) {
	cases := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{50, "50"},
		{999, "999"},
		{1000, "1.000"},
		{15420, "15.420"},
		{1234567, "1.234.567"},
	}

	for _, c := range cases {
		result := formatTokens(c.input)
		if result != c.expected {
			t.Errorf("formatTokens(%d) = %s, esperado %s", c.input, result, c.expected)
		}
	}
}

func TestSendTaskActionWithTokens(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/antigravity/update_tasks" {
			_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "ok"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := &PlanesGoClient{
		BaseURL:    server.URL,
		Token:      "test-token",
		HTTPClient: server.Client(),
	}

	_, err := client.SendTaskActionWithTokens(
		"stop",
		"Desarrollo de feature",
		10,
		567,
		"PLANESGO",
		"Completado con éxito",
		"Desarrollo",
		"T123",
		15420,
		10000,
		5420,
		"Gemini 3.8 Flash",
		0.05,
		"session-abc",
	)
	if err != nil {
		t.Fatalf("SendTaskActionWithTokens falló: %v", err)
	}

	if receivedPayload["tokens_total"] != float64(15420) {
		t.Errorf("tokens_total esperado 15420, obtenido %v", receivedPayload["tokens_total"])
	}
	if receivedPayload["ai_model"] != "Gemini 3.8 Flash" {
		t.Errorf("ai_model esperado 'Gemini 3.8 Flash', obtenido %v", receivedPayload["ai_model"])
	}
	if receivedPayload["ai_session_id"] != "session-abc" {
		t.Errorf("ai_session_id esperado 'session-abc', obtenido %v", receivedPayload["ai_session_id"])
	}
}

