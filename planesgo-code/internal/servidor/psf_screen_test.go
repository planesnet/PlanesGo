package servidor

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVTScreen(t *testing.T) {
	s := newVTScreen(5, 20)
	s.Write([]byte("\x1b[H\x1b[2Jhola\r\n\x1b[31mro\x1b[38;5;208mjo\x1b[0m\x1b[3;5H\x1b[38;2;0;255;255mX"))
	if string([]rune{s.cells[0][0].r, s.cells[0][3].r}) != "ha" || s.cells[1][0].fg != ansi16[1] || s.cells[1][2].fg != color256(208) {
		t.Fatalf("screen: %+v", s.cells[:2])
	}
	if s.cells[2][4].r != 'X' || s.cells[2][4].fg != "#00ffff" {
		t.Errorf("cursor move or truecolor: %+v", s.cells[2][4])
	}
	// A sequence split between two writes.
	s.Write([]byte("\x1b[3"))
	s.Write([]byte("2mY"))
	if s.cells[2][5].r != 'Y' || s.cells[2][5].fg != ansi16[2] {
		t.Errorf("split sequence: %+v", s.cells[2][5])
	}
	if err := xml.Unmarshal(s.SVG("prueba <1>"), new(struct{})); err != nil {
		t.Errorf("not valid XML: %v", err)
	}
}

func TestCaptureProgram(t *testing.T) {
	shots, err := captureProgram(context.Background(), t.TempDir(), `printf '\033[2J\033[H\033[32mverde\033[0m'; sleep 5`, []float64{1, 2})
	if err != nil {
		t.Skip("no virtual terminal here:", err)
	}
	if len(shots) != 2 || !strings.Contains(string(shots[0]), "verde") || !strings.Contains(string(shots[0]), ansi16[2]) {
		t.Fatalf("shots: %d %s", len(shots), shots)
	}
}

// With a real program: PGO_PRUEBA_PROGRAMA=<carpeta> go test -run TestCaptureReal ./internal/servidor
func TestCaptureReal(t *testing.T) {
	dir := os.Getenv("PGO_PRUEBA_PROGRAMA")
	if dir == "" {
		t.Skip("sin PGO_PRUEBA_PROGRAMA")
	}
	shots, err := captureProgram(context.Background(), dir, "go run ./cmd/tetris -auto", []float64{3, 6})
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range shots {
		os.WriteFile(filepath.Join(os.TempDir(), "pgo-captura-"+string(rune('1'+i))+".svg"), b, 0o644)
	}
}
