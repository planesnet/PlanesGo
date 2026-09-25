package main

import (
	"net/http"
	"net/http/httptest"
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

