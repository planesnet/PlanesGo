package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
	esperados := []string{".claude/hooks/planesgo_claude_hook.py", ".claude/commands/pgo.md", ".opencode/plugins/planesgo.ts", "go.mod", "go.sum", "scripts/install-claude-hook.sh", "scripts/install-opencode-plugin.sh"}
	// Todo el código de pgo-mcp (menos las pruebas), para que el instalador pueda compilar el paquete
	fuentes, err := filepath.Glob("cmd/pgo-mcp/*.go")
	if err != nil || len(fuentes) == 0 {
		t.Fatalf("no se encuentran los fuentes de cmd/pgo-mcp: %v", err)
	}
	for _, f := range fuentes {
		if !strings.HasSuffix(f, "_test.go") {
			esperados = append(esperados, filepath.ToSlash(f))
		}
	}
	for _, f := range esperados {
		if _, ok := found[f]; !ok {
			t.Errorf("falta %s en el paquete (añádelo a //go:embed en handlers_install.go)", f)
		}
	}
	for f := range found {
		if strings.HasSuffix(f, "_test.go") {
			t.Errorf("las pruebas no deben ir en el paquete: %s", f)
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
	for _, route := range []string{"/install/claude-hook.sh", "/install/opencode-plugin.sh"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", route, nil))
		if rec.Code != http.StatusOK || rec.Body.Len() < 100 {
			t.Fatalf("instalador %s no servido: %d", route, rec.Code)
		}
	}
}
