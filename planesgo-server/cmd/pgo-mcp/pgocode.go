package main

// Blindaje del Odoo de PlanesNet (planesgo-code/docs/odoo-blindaje.md): dentro de pgocode solo imputa pgocode.
// Un agente lanzado por pgocode no puede usar pgo-mcp para escribir en Odoo (vía PlanesGo), ni como servidor MCP
// ni por la línea de órdenes, aunque se quite PLANESGO_GESTOR del entorno: se mira también el de sus antepasados
// (en Linux, /proc), que el agente no puede cambiar.

import (
	"bytes"
	"os"
	"strconv"
	"strings"
)

// textoBloqueadoPgocode: la respuesta a una orden de imputación desde un agente de pgocode.
const textoBloqueadoPgocode = "pgo-mcp: dentro de pgocode la imputación la hace pgocode; los agentes no pueden escribir en Odoo"

// dentroDePgocode: la comprobación que se usa (las pruebas la cambian: si se ejecutan desde un agente de pgocode,
// todos sus procesos serían «de pgocode»).
var dentroDePgocode = lanzadoPorPgocode

// lanzadoPorPgocode: este proceso o alguno de sus antepasados es un agente (o un programa) lanzado por pgocode.
func lanzadoPorPgocode() bool {
	if os.Getenv("PLANESGO_GESTOR") == "pgocode" || os.Getenv("PGO_EJECUCION") != "" {
		return true
	}
	pid := os.Getppid()
	for i := 0; pid > 1 && i < 64; i++ {
		if entornoDePgocode(pid) {
			return true
		}
		pid = padreDe(pid)
	}
	return false
}

// entornoDePgocode: el entorno con el que arrancó el proceso lleva la marca de pgocode.
func entornoDePgocode(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/environ")
	if err != nil {
		return false
	}
	for _, kv := range bytes.Split(b, []byte{0}) {
		if bytes.HasPrefix(kv, []byte("PGO_EJECUCION=")) || bytes.Equal(kv, []byte("PLANESGO_GESTOR=pgocode")) {
			return true
		}
	}
	return false
}

// padreDe: el padre de un proceso (0 si no se sabe).
func padreDe(pid int) int {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 2 {
		return 0
	}
	p, _ := strconv.Atoi(f[1])
	return p
}
