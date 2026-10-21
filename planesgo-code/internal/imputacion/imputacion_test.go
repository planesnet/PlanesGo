package imputacion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Con un planesgo-mcp de mentira: primer latido al empezar, cierre tras el plazo sin turnos.
func TestLatidosYCierre(t *testing.T) {
	dir := t.TempDir()
	reg := filepath.Join(dir, "llamadas")
	bin := filepath.Join(dir, "planesgo-mcp")
	os.WriteFile(bin, []byte("#!/bin/sh\necho \"$1\" >> "+reg+"\n"), 0o755)
	if err := Vincula(dir, 447, "MADERAS"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".planesgo.json")); !strings.Contains(string(b), `"odoo_project_id": 447`) {
		t.Fatalf(".planesgo.json: %s", b)
	}
	g := &Gestor{Bin: bin}
	fin := g.Empieza(Trabajo{Sesion: "s1", Proyecto: 447, Carpeta: dir, Modelo: "Claude Code", Desc: "prueba"})
	fin()
	g.CierraTodo()
	time.Sleep(50 * time.Millisecond)
	b, _ := os.ReadFile(reg)
	if string(b) != "--beat\n--stop\n" {
		t.Errorf("llamadas: %q", b)
	}
}
