package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInstallBundleContainsHookAndSource(t *testing.T) {
	mux := http.NewServeMux()
	setupInstallRoutes(mux, &AppState{})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/install/planesgo.tar.gz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código %d", rec.Code)
	}
	gz, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	found := map[string]int64{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		found[h.Name] = h.Mode
	}
	for _, f := range []string{".claude/hooks/planesgo_claude_hook.py", ".claude/commands/planesgo.md", ".claude/commands/pgo.md", "cmd/planesgo-mcp/main.go", "go.mod", "scripts/install-claude-hook.sh"} {
		if _, ok := found[f]; !ok {
			t.Errorf("falta %s en el paquete", f)
		}
	}
	if found[".claude/hooks/planesgo_claude_hook.py"] != 0755 {
		t.Errorf("el hook debe ser ejecutable")
	}
}

func TestInstallPSFRequiresToken(t *testing.T) {
	mux := http.NewServeMux()
	setupInstallRoutes(mux, &AppState{})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/install/psf.tar.gz", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin token debe dar 401, da %d", rec.Code)
	}
}

func TestInstallScriptIsServed(t *testing.T) {
	mux := http.NewServeMux()
	setupInstallRoutes(mux, &AppState{})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/install/claude-hook.sh", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() < 100 {
		t.Fatalf("instalador no servido: %d", rec.Code)
	}
}
