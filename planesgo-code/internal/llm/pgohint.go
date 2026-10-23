package llm

import (
	"regexp"
	"strings"
)

// El agente no sabe que existe la orden «pgo» (programas del proyecto en pgocode: cmd/pgocode/pgo.go): si el
// mensaje habla de ella, se le añade una línea con su uso. Solo en ese mensaje: nada fijo en cada turno.

// pgoHint: la línea que se añade (el resumen de «pgo --help»).
const pgoHint = "(pgocode: «pgo» es una orden de terminal (ejecútala con tu herramienta de órdenes, no es una herramienta MCP): " +
	"`pgo ejecutar <nombre> <orden…>` arranca un programa del proyecto en la carpeta actual y sigue al acabar el turno; " +
	"`pgo parar <nombre>`; `pgo procesos`; `pgo salida <nombre> [n]` (últimas n líneas); `pgo abrir <url>` (navegador de pgocode).)"

var (
	pgoWord = regexp.MustCompile(`(?i)\bpgo\b`)
	pgoVerb = regexp.MustCompile(`(?i)\bpgo\s+(?:ejecutar|parar|procesos|salida|abrir)\b|\b(?:ejecut|arranc|lanz|levant|parar|det[eé]n|salida|abr[eií]|procesos?|programas?|servidor)`)
)

// pgoHintFor: la línea de uso de «pgo» si el mensaje la menciona (ejecutar, parar, salida, abrir…); "" si no. Las
// órdenes («/pgo …», la skill de imputación) no cuentan.
func pgoHintFor(text string) string {
	t := strings.TrimSpace(text)
	if strings.HasPrefix(t, "/") || !pgoWord.MatchString(t) || !pgoVerb.MatchString(t) {
		return ""
	}
	return pgoHint
}

// hasPgo: el agente tiene la orden «pgo» (su entorno lleva el token de pgocode).
func hasPgo(entorno []string) bool {
	for _, kv := range entorno {
		if strings.HasPrefix(kv, "PGO_TOKEN=") {
			return true
		}
	}
	return false
}
