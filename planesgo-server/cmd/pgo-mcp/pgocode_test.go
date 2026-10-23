package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// Las pruebas no dependen de dónde se ejecuten (desde un agente de pgocode, todo sería «de pgocode»): solo cuenta
// PLANESGO_GESTOR, que ponen las propias pruebas.
func TestMain(m *testing.M) {
	if os.Getenv("PGO_MCP_AYUDA_LANZADO") == "1" {
		fmt.Print(lanzadoPorPgocode())
		os.Exit(0)
	}
	dentroDePgocode = func() bool { return os.Getenv("PLANESGO_GESTOR") == "pgocode" }
	os.Exit(m.Run())
}

// Un agente de pgocode no imputa con la línea de órdenes (ni busca ni vincula): código 3 y sin llamar a PlanesGo.
func TestCLIBloqueadaEnPgocode(t *testing.T) {
	antes := dentroDePgocode
	dentroDePgocode = func() bool { return true }
	defer func() { dentroDePgocode = antes }()
	for _, args := range [][]string{{"--time", "30"}, {"--beat", "--task", "x"}, {"--stop"}, {"--project", "x"}, {"--project-id", "5", "--link", "--path", t.TempDir()}, {"--check"}} {
		var sal, err bytes.Buffer
		codigo, servidor := ejecutaCLI(args, &sal, &err)
		if codigo != 3 || servidor || !strings.Contains(err.String(), "pgocode") {
			t.Errorf("%v: código %d, servidor %v, %q %q", args, codigo, servidor, sal.String(), err.String())
		}
	}
	if !gestorPgocode() {
		t.Error("como servidor MCP, sin herramientas dentro de pgocode")
	}
}

// Linux: lo lanzado por un agente de pgocode se reconoce aunque se quite el entorno (env -i): por sus antepasados.
func TestLanzadoPorPgocode(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("solo Linux")
	}
	exe, _ := os.Executable()
	corre := func(marca string) string {
		cmd := exec.Command("bash", "-c", `env -i PGO_MCP_AYUDA_LANZADO=1 "$0" -test.run=XXX; true`, exe)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
		if marca != "" {
			cmd.Env = append(cmd.Env, marca)
		}
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	if lanzadoPorPgocode() {
		t.Log("las pruebas corren dentro de pgocode: no se comprueba el caso «fuera»")
	} else if r := corre(""); r != "false" {
		t.Errorf("sin pgocode: %s", r)
	}
	if r := corre("PGO_EJECUCION=123-claude-code-1"); r != "true" {
		t.Errorf("agente de pgocode con env -i: %s", r)
	}
	if r := corre("PLANESGO_GESTOR=pgocode"); r != "true" {
		t.Errorf("hijo de un proceso con PLANESGO_GESTOR=pgocode: %s", r)
	}
}
