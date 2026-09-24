package main

import (
	"testing"
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
	if len(tools) != 6 {
		t.Fatalf("expected 6 tools defined, got %d", len(tools))
	}

	expectedNames := map[string]bool{
		"planesgo_check":       false,
		"planesgo_beat":        false,
		"planesgo_stop":        false,
		"planesgo_list_tasks":  false,
		"planesgo_status":      false,
		"planesgo_set_project": false,
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

