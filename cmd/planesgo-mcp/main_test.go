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
	if len(tools) != 5 {
		t.Fatalf("expected 5 tools defined, got %d", len(tools))
	}

	expectedNames := map[string]bool{
		"planesgo_check":      false,
		"planesgo_beat":       false,
		"planesgo_stop":       false,
		"planesgo_list_tasks": false,
		"planesgo_status":     false,
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
